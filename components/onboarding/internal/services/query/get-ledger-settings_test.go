// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetLedgerSettings_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	expectedSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(expectedSettings, nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.NotNil(t, settings)
	assert.Equal(t, expectedSettings, settings)
}

func TestGetLedgerSettings_EmptySettings(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(map[string]any{}, nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.NotNil(t, settings)
	assert.Empty(t, settings)
}

func TestGetLedgerSettings_NilSettingsReturnsEmptyMap(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(nil, nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.NotNil(t, settings)
	assert.Empty(t, settings)
}

func TestGetLedgerSettings_LedgerNotFound(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(nil, errors.New("ledger not found"))

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.Error(t, err)
	assert.Nil(t, settings)
}

func TestGetLedgerSettings_DatabaseError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(nil, errors.New("database error"))

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.Error(t, err)
	assert.Nil(t, settings)
	assert.Contains(t, err.Error(), "database error")
}
