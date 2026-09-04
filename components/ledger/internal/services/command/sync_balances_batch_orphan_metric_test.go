// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// orphanDropByReason returns the orphan-drop counter values keyed by the `reason`
// label, plus the full label set recorded for each reason.
func orphanDropByReason(t *testing.T, reader *sdkmetric.ManualReader) (map[string]int64, map[string]map[string]string) {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	values := make(map[string]int64)
	labels := make(map[string]map[string]string)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != utils.BalanceSyncOrphanDropped.Name {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "orphan drop counter data type must be Sum[int64], got %T", m.Data)

			for _, dp := range sum.DataPoints {
				set := make(map[string]string, dp.Attributes.Len())
				for _, kv := range dp.Attributes.ToSlice() {
					set[string(kv.Key)] = kv.Value.AsString()
				}

				values[set["reason"]] = dp.Value
				labels[set["reason"]] = set
			}
		}
	}

	return values, labels
}

// TestSyncBalancesBatch_OrphanDropCounter locks the orphan-drop counter onto the three
// exits of the batch: the all-orphans early return, the DB-error path, and the happy
// path with a mixed batch. A dropped key means the pending delta is gone for good, so
// every exit has to be attributable.
func TestSyncBalancesBatch_OrphanDropCounter(t *testing.T) {
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	prefix := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":"

	t.Run("all_orphans_early_return", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		expiredKey := prefix + "@acc1#default"
		unparseableKey := "invalid:key:format"
		keys := []string{expiredKey, unparseableKey}

		mockRedis := redis.NewMockRedisRepository(ctrl)
		mockRedis.EXPECT().
			GetBalancesByKeys(gomock.Any(), keys).
			Return(map[string]*mmodel.BalanceRedis{
				expiredKey:     nil,
				unparseableKey: {ID: uuid.Must(libCommons.GenerateUUIDv7()).String(), AssetCode: "USD", Version: 1},
			}, nil).
			Times(1)
		mockRedis.EXPECT().
			RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
			Return(int64(2), nil).
			Times(1)

		reader, factory := newReaderFactory(t)
		ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)
		ctx = tmcore.ContextWithTenantID(ctx, "acme")

		uc := UseCase{TransactionRedisRepo: mockRedis}

		_, err := uc.SyncBalancesBatch(ctx, organizationID, ledgerID, toSyncKeys(keys))
		require.NoError(t, err)

		values, labels := orphanDropByReason(t, reader)
		assert.Equal(t, int64(1), values[orphanReasonExpired], "the expired key must be counted")
		assert.Equal(t, int64(1), values[orphanReasonUnparseable], "the unparseable key must be counted")
		assert.Equal(t, organizationID.String(), labels[orphanReasonExpired]["organization_id"])
		assert.Equal(t, ledgerID.String(), labels[orphanReasonExpired]["ledger_id"])
		assert.Equal(t, "acme", labels[orphanReasonExpired]["tenant_id"])
	})

	t.Run("db_error_path", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		validKey := prefix + "@acc1#default"
		expiredKey := prefix + "@acc2#default"
		keys := []string{validKey, expiredKey}

		mockRedis := redis.NewMockRedisRepository(ctrl)
		mockRedis.EXPECT().
			GetBalancesByKeys(gomock.Any(), keys).
			Return(map[string]*mmodel.BalanceRedis{
				validKey: {
					ID:        uuid.Must(libCommons.GenerateUUIDv7()).String(),
					Alias:     "@acc1",
					AssetCode: "USD",
					Version:   4,
					Available: decimal.NewFromInt(500),
				},
				expiredKey: nil,
			}, nil).
			Times(1)
		mockRedis.EXPECT().
			RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
			Return(int64(1), nil).
			Times(1)

		mockBalance := balance.NewMockRepository(ctrl)
		mockBalance.EXPECT().
			UpdateMany(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(int64(0), errors.New("sql: database is closed")).
			Times(1)

		reader, factory := newReaderFactory(t)
		ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)

		uc := UseCase{TransactionRedisRepo: mockRedis, BalanceRepo: mockBalance}

		_, err := uc.SyncBalancesBatch(ctx, organizationID, ledgerID, toSyncKeys(keys))
		require.Error(t, err, "the DB failure must still propagate")

		values, labels := orphanDropByReason(t, reader)
		assert.Equal(t, int64(1), values[orphanReasonExpired],
			"a drop on the DB-error exit must still be counted")
		assert.Equal(t, "", labels[orphanReasonExpired]["tenant_id"],
			"single-tenant carries an empty tenant_id so existing series keep their identity")
	})

	t.Run("mixed_batch_happy_path", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		validKey1 := prefix + "@acc1#default"
		validKey2 := prefix + "@acc2#default"
		expiredKey := prefix + "@acc3#default"
		unparseableKey := "invalid:key:format"
		keys := []string{validKey1, validKey2, expiredKey, unparseableKey}

		mockRedis := redis.NewMockRedisRepository(ctrl)
		mockRedis.EXPECT().
			GetBalancesByKeys(gomock.Any(), keys).
			Return(map[string]*mmodel.BalanceRedis{
				validKey1: {
					ID:        uuid.Must(libCommons.GenerateUUIDv7()).String(),
					Alias:     "@acc1",
					AssetCode: "USD",
					Version:   2,
					Available: decimal.NewFromInt(100),
				},
				validKey2: {
					ID:        uuid.Must(libCommons.GenerateUUIDv7()).String(),
					Alias:     "@acc2",
					AssetCode: "USD",
					Version:   3,
					Available: decimal.NewFromInt(200),
				},
				expiredKey:     nil,
				unparseableKey: {ID: uuid.Must(libCommons.GenerateUUIDv7()).String(), AssetCode: "USD", Version: 1},
			}, nil).
			Times(1)
		mockRedis.EXPECT().
			RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
			Return(int64(4), nil).
			Times(1)

		mockBalance := balance.NewMockRepository(ctrl)
		mockBalance.EXPECT().
			UpdateMany(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []mmodel.BalanceRedis) (int64, error) {
				assert.Len(t, balances, 2, "the two valid balances must still be persisted")

				return int64(len(balances)), nil
			}).
			Times(1)

		reader, factory := newReaderFactory(t)
		ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)

		uc := UseCase{TransactionRedisRepo: mockRedis, BalanceRepo: mockBalance}

		result, err := uc.SyncBalancesBatch(ctx, organizationID, ledgerID, toSyncKeys(keys))
		require.NoError(t, err)
		assert.Equal(t, int64(2), result.BalancesSynced)

		values, _ := orphanDropByReason(t, reader)
		assert.Equal(t, int64(1), values[orphanReasonExpired])
		assert.Equal(t, int64(1), values[orphanReasonUnparseable])
	})

	t.Run("no_orphans_emits_nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		validKey := prefix + "@acc1#default"
		keys := []string{validKey}

		mockRedis := redis.NewMockRedisRepository(ctrl)
		mockRedis.EXPECT().
			GetBalancesByKeys(gomock.Any(), keys).
			Return(map[string]*mmodel.BalanceRedis{
				validKey: {
					ID:        uuid.Must(libCommons.GenerateUUIDv7()).String(),
					Alias:     "@acc1",
					AssetCode: "USD",
					Version:   7,
					Available: decimal.NewFromInt(10),
				},
			}, nil).
			Times(1)
		mockRedis.EXPECT().
			RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
			Return(int64(1), nil).
			Times(1)

		mockBalance := balance.NewMockRepository(ctrl)
		mockBalance.EXPECT().
			UpdateMany(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(int64(1), nil).
			Times(1)

		reader, factory := newReaderFactory(t)
		ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)

		uc := UseCase{TransactionRedisRepo: mockRedis, BalanceRepo: mockBalance}

		_, err := uc.SyncBalancesBatch(ctx, organizationID, ledgerID, toSyncKeys(keys))
		require.NoError(t, err)

		values, _ := orphanDropByReason(t, reader)
		assert.Empty(t, values, "a batch with no orphans must not emit a zero-valued series")
	})
}

// TestSyncBalancesBatch_CleanupFailureCounterCarriesTenantID verifies the cleanup
// counter is attributable per tenant like every other balance-sync counter — a
// schedule that will not drain has to point at the tenant it belongs to.
func TestSyncBalancesBatch_CleanupFailureCounterCarriesTenantID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	expiredKey := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":@acc1#default"

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().
		GetBalancesByKeys(gomock.Any(), []string{expiredKey}).
		Return(map[string]*mmodel.BalanceRedis{expiredKey: nil}, nil).
		Times(1)
	mockRedis.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		Return(int64(0), errors.New("redis: connection refused")).
		Times(1)

	reader, factory := newReaderFactory(t)
	ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)
	ctx = tmcore.ContextWithTenantID(ctx, "acme")

	uc := UseCase{TransactionRedisRepo: mockRedis}

	_, err := uc.SyncBalancesBatch(ctx, organizationID, ledgerID, toSyncKeys([]string{expiredKey}))
	require.NoError(t, err, "a cleanup failure must not fail the batch")

	labels := cleanupFailureLabels(t, reader)
	require.Len(t, labels, 1)
	assert.Equal(t, organizationID.String(), labels[0]["organization_id"])
	assert.Equal(t, ledgerID.String(), labels[0]["ledger_id"])
	require.Contains(t, labels[0], "tenant_id",
		"the cleanup counter must carry the tenant like the failure and orphan counters")
	assert.Equal(t, "acme", labels[0]["tenant_id"])
}

// cleanupFailureLabels returns the label sets recorded on the cleanup-failure counter.
func cleanupFailureLabels(t *testing.T, reader *sdkmetric.ManualReader) []map[string]string {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	var out []map[string]string

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != utils.BalanceSyncCleanupFailures.Name {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "cleanup counter data type must be Sum[int64], got %T", m.Data)

			for _, dp := range sum.DataPoints {
				labels := make(map[string]string, dp.Attributes.Len())
				for _, kv := range dp.Attributes.ToSlice() {
					labels[string(kv.Key)] = kv.Value.AsString()
				}

				out = append(out, labels)
			}
		}
	}

	return out
}
