// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// Scenario: Agregação do BalanceSync registra contagem no pai.
func TestSyncBalancesBatchSpans_AggregationRecordsCountOnParent(t *testing.T) {
	ctrl := gomock.NewController(t)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	prefix := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":"

	// Three valid keys on two composite keys: the duplicate scheduling of @acc1 is
	// collapsed by the aggregation, so 3 keys go in and 2 balances come out.
	acc1 := prefix + "@acc1#default"
	acc2 := prefix + "@acc2#default"
	keys := []string{acc1, acc1, acc2}

	balanceData := map[string]*mmodel.BalanceRedis{
		acc1: {ID: uuid.NewString(), Alias: "@acc1", AssetCode: "USD", Version: 3, Available: decimal.NewFromInt(10)},
		acc2: {ID: uuid.NewString(), Alias: "@acc2", AssetCode: "USD", Version: 7, Available: decimal.NewFromInt(20)},
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().GetBalancesByKeys(gomock.Any(), keys).Return(balanceData, nil).Times(1)
	mockRedis.EXPECT().RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).Return(int64(3), nil).Times(1)

	mockBalance := balance.NewMockRepository(ctrl)
	mockBalance.EXPECT().
		UpdateMany(gomock.Any(), organizationID, ledgerID, gomock.Len(2)).
		Return(int64(2), nil).
		Times(1)

	uc := UseCase{TransactionRedisRepo: mockRedis, BalanceRepo: mockBalance}

	ctx, recorder := spantest.RecordingContext()

	result, err := uc.SyncBalancesBatch(ctx, organizationID, ledgerID, toSyncKeys(keys))
	require.NoError(t, err)
	require.Equal(t, 2, result.BalancesAggregated)

	spantest.AssertNoSpan(t, recorder, "balance_sync.aggregate")

	span := spantest.FindSpan(t, recorder, "command.sync_balances_batch")
	assert.Contains(t, span.Attributes(), attribute.Int("app.balance_sync.aggregated_keys", 3))
	assert.Contains(t, span.Attributes(), attribute.Int("app.balance_sync.balances_aggregated", 2))
}
