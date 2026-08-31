// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// counterLabels returns the label sets recorded on the named counter.
func counterLabels(t *testing.T, reader *sdkmetric.ManualReader, name string) []map[string]string {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	var out []map[string]string

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "counter data type must be Sum[int64], got %T", m.Data)

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

// syncKeyFor builds a Redis balance key the worker can parse back into org and ledger.
func syncKeyFor(orgID, ledgerID uuid.UUID, alias string) string {
	return "balance:{transactions}:" + orgID.String() + ":" + ledgerID.String() + ":@" + alias + "#default"
}

// TestProcessSyncBatch_SyncedCounterCarriesTenantID verifies the synced counter is
// attributable per tenant, matching every other balance-sync counter.
func TestProcessSyncBatch_SyncedCounterCarriesTenantID(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	key := syncKeyFor(orgID, ledgerID, "acc1")

	ctrl := gomock.NewController(t)
	repo := redisTransaction.NewMockRedisRepository(ctrl)
	repo.EXPECT().
		GetBalancesByKeys(gomock.Any(), gomock.Any()).
		Return(map[string]*mmodel.BalanceRedis{
			key: {ID: uuid.MustParse("33333333-3333-3333-3333-333333333333").String(), Alias: "@acc1", AssetCode: "USD", Version: 2},
		}, nil).
		Times(1)
	repo.EXPECT().
		RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
		Return(int64(1), nil).
		Times(1)

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().
		UpdateMany(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(int64(1), nil).
		Times(1)

	reader, factory := newBalanceSyncReaderFactory(t)

	w := NewBalanceSyncWorker(
		newTestLogger(),
		&command.UseCase{TransactionRedisRepo: repo, BalanceRepo: balanceRepo},
		BalanceSyncConfig{},
	)

	ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)
	ctx = tmcore.ContextWithTenantID(ctx, "acme")

	require.True(t, w.processSyncBatch(ctx, orgID, ledgerID, []redisTransaction.SyncKey{{Key: key}}))

	labels := counterLabels(t, reader, utils.BalanceSynced.Name)
	require.Len(t, labels, 1)
	require.Contains(t, labels[0], "tenant_id",
		"the synced counter must be attributable per tenant like the failure counter")
	assert.Equal(t, "acme", labels[0]["tenant_id"])
	assert.Equal(t, "batch", labels[0]["mode"])
}
