// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"encoding/json"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const getBalancesSpanName = "query.get_balances"

// A fully cached read is one query.get_balances span, with no cache_read child.
func TestGetBalancesSpans_CacheHitOpensNoCacheReadSpan(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{TransactionRedisRepo: mockRedisRepo}

	organizationID := uuid.New()
	ledgerID := uuid.New()

	cached, err := json.Marshal(mmodel.BalanceRedis{
		ID:             uuid.NewString(),
		AccountID:      uuid.NewString(),
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   1,
		AllowReceiving: 1,
		AssetCode:      "USD",
	})
	require.NoError(t, err)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), utils.BalanceInternalKey(organizationID, ledgerID, "@alice#default")).
		Return(string(cached), nil)

	ctx, recorder := spantest.RecordingContext()

	balances, err := uc.GetBalances(ctx, organizationID, ledgerID, []string{"@alice#default"})
	require.NoError(t, err)
	require.Len(t, balances, 1)

	spantest.AssertNoSpan(t, recorder, "query.get_balances.cache_read")
	assert.Equal(t, []string{getBalancesSpanName}, spantest.SpanNames(recorder))
	assert.Equal(t, codes.Unset, spantest.FindSpan(t, recorder, getBalancesSpanName).Status().Code)
}

// A corrupted cache entry falls back to the database and is recorded as a technical
// error on the caller's query.get_balances span.
func TestGetBalancesSpans_DeserializationFailureIsRecordedOnTheCallerSpan(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{TransactionRedisRepo: mockRedisRepo}

	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), utils.BalanceInternalKey(organizationID, ledgerID, "@alice#default")).
		Return("{not-a-balance", nil)

	ctx, recorder := spantest.RecordingContext()
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, parent := tracer.Start(ctx, getBalancesSpanName)
	balances, misses := uc.getBalancesFromCache(ctx, organizationID, ledgerID, []string{"@alice#default"})
	parent.End()

	assert.Empty(t, balances)
	assert.Equal(t, []string{"@alice#default"}, misses, "the corrupted entry must fall back to the database")

	spantest.AssertNoSpan(t, recorder, "query.get_balances.cache_read")

	span := spantest.FindSpan(t, recorder, getBalancesSpanName)
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Contains(t, span.Status().Description, "Failed to deserialize cached balance")
}
