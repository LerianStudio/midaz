// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const ledgerSettingsSpanName = "query.get_ledger_settings"

// Scenario: Cache hit gera um único span.
func TestLedgerSettingsSpans_CacheHitEmitsASingleSpan(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		LedgerRepo:          ledger.NewMockRepository(ctrl),
		OnboardingRedisRepo: mockRedisRepo,
	}

	orgID := uuid.New()
	ledgerID := uuid.New()

	cachedJSON, err := json.Marshal(map[string]any{"accounting": map[string]any{"validateRoutes": true}})
	require.NoError(t, err)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), utils.LedgerSettingsInternalKey(orgID, ledgerID)).
		Return(string(cachedJSON), nil)

	ctx, recorder := spantest.RecordingContext()

	settings, err := uc.GetParsedLedgerSettings(ctx, orgID, ledgerID)
	require.NoError(t, err)
	assert.True(t, settings.Accounting.ValidateRoutes)

	assert.Equal(t, 1, spantest.CountSpans(recorder, ledgerSettingsSpanName),
		"one read must emit exactly one %s span, never nested in itself; recorded: %v",
		ledgerSettingsSpanName, spantest.SpanNames(recorder))
	spantest.AssertNoSpan(t, recorder,
		"query.get_ledger_settings.cache_read",
		"query.get_ledger_settings.cache_write")

	span := spantest.FindSpan(t, recorder, ledgerSettingsSpanName)
	assert.Equal(t, codes.Unset, span.Status().Code)
}

// Scenario: Falha de leitura do cache cai para o banco e fica registrada.
func TestLedgerSettingsSpans_CacheReadFailureFallsBackAndIsRecordedOnTheSingleSpan(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		LedgerRepo:          mockLedgerRepo,
		OnboardingRedisRepo: mockRedisRepo,
	}

	orgID := uuid.New()
	ledgerID := uuid.New()
	cacheKey := utils.LedgerSettingsInternalKey(orgID, ledgerID)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), cacheKey).
		Return("", errors.New("redis connection error"))
	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(map[string]any{"accounting": map[string]any{"validateRoutes": true}}, nil)
	mockRedisRepo.EXPECT().
		Set(gomock.Any(), cacheKey, gomock.Any(), SettingsCacheTTL).
		Return(nil)

	ctx, recorder := spantest.RecordingContext()

	settings, err := uc.GetParsedLedgerSettings(ctx, orgID, ledgerID)
	require.NoError(t, err, "a cache failure must not stop the fallback to the database")
	assert.True(t, settings.Accounting.ValidateRoutes, "settings must come from the database")

	assert.Equal(t, 1, spantest.CountSpans(recorder, ledgerSettingsSpanName), "recorded: %v", spantest.SpanNames(recorder))
	spantest.AssertNoSpan(t, recorder,
		"query.get_ledger_settings.cache_read",
		"query.get_ledger_settings.cache_write")

	span := spantest.FindSpan(t, recorder, ledgerSettingsSpanName)
	assert.Equal(t, codes.Error, span.Status().Code, "a cache read failure is a technical error on the single span")
	assert.Contains(t, span.Status().Description, "Cache read error")
}

// A cache write failure keeps the operation green for the caller and is recorded on the
// same single span, never on a .cache_write child.
func TestLedgerSettingsSpans_CacheWriteFailureIsRecordedOnTheSingleSpan(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		LedgerRepo:          mockLedgerRepo,
		OnboardingRedisRepo: mockRedisRepo,
	}

	orgID := uuid.New()
	ledgerID := uuid.New()
	cacheKey := utils.LedgerSettingsInternalKey(orgID, ledgerID)

	mockRedisRepo.EXPECT().Get(gomock.Any(), cacheKey).Return("", nil)
	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(map[string]any{}, nil)
	mockRedisRepo.EXPECT().
		Set(gomock.Any(), cacheKey, gomock.Any(), SettingsCacheTTL).
		Return(errors.New("redis write error"))

	ctx, recorder := spantest.RecordingContext()

	_, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)
	require.NoError(t, err)

	assert.Equal(t, 1, spantest.CountSpans(recorder, ledgerSettingsSpanName), "recorded: %v", spantest.SpanNames(recorder))
	spantest.AssertNoSpan(t, recorder, "query.get_ledger_settings.cache_write")

	span := spantest.FindSpan(t, recorder, ledgerSettingsSpanName)
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Contains(t, span.Status().Description, "Failed to cache ledger settings")
}

// A database failure read through the parse wrapper is recorded once, on the single span
// GetLedgerSettings owns, and the error still reaches the caller unchanged.
func TestLedgerSettingsSpans_DatabaseFailureThroughParseWrapperIsRecordedOnce(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{LedgerRepo: mockLedgerRepo}

	orgID := uuid.New()
	ledgerID := uuid.New()
	dbErr := errors.New("connection error")

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(nil, dbErr)

	ctx, recorder := spantest.RecordingContext()

	_, err := uc.GetParsedLedgerSettings(ctx, orgID, ledgerID)
	require.ErrorIs(t, err, dbErr)

	assert.Equal(t, 1, spantest.CountSpans(recorder, ledgerSettingsSpanName), "recorded: %v", spantest.SpanNames(recorder))

	span := spantest.FindSpan(t, recorder, ledgerSettingsSpanName)
	assert.Equal(t, codes.Error, span.Status().Code)
}
