// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// gaugePoints returns the label sets and values recorded on the named gauge.
func gaugePoints(t *testing.T, reader *sdkmetric.ManualReader, name string) []struct {
	labels map[string]string
	value  int64
} {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	var out []struct {
		labels map[string]string
		value  int64
	}

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}

			g, ok := m.Data.(metricdata.Gauge[int64])
			require.True(t, ok, "gauge data type must be Gauge[int64], got %T", m.Data)

			for _, dp := range g.DataPoints {
				labels := make(map[string]string, dp.Attributes.Len())
				for _, kv := range dp.Attributes.ToSlice() {
					labels[string(kv.Key)] = kv.Value.AsString()
				}

				out = append(out, struct {
					labels map[string]string
					value  int64
				}{labels: labels, value: dp.Value})
			}
		}
	}

	return out
}

// TestProcessSyncBatch_LastSuccessGauge locks the staleness signal: a batch that ran
// to completion stamps the gauge, a failed one does not. A failure counter cannot see
// a stall in which nothing fails because nothing runs, so this gauge is what the
// staleness alert reads.
func TestProcessSyncBatch_LastSuccessGauge(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	t.Run("successful batch stamps the gauge with the full scope", func(t *testing.T) {
		t.Parallel()

		key := syncKeyFor(orgID, ledgerID, "acc1")

		ctrl := gomock.NewController(t)
		repo := redisTransaction.NewMockRedisRepository(ctrl)
		repo.EXPECT().
			GetBalancesByKeys(gomock.Any(), gomock.Any()).
			Return(map[string]*mmodel.BalanceRedis{key: nil}, nil).
			Times(1)
		repo.EXPECT().
			RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
			Return(int64(1), nil).
			Times(1)

		reader, factory := newBalanceSyncReaderFactory(t)

		w := NewBalanceSyncWorker(
			newTestLogger(),
			&command.UseCase{TransactionRedisRepo: repo},
			BalanceSyncConfig{},
		)

		ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)
		ctx = tmcore.ContextWithTenantID(ctx, "acme")

		w.processSyncBatch(ctx, orgID, ledgerID, []redisTransaction.SyncKey{{Key: key}})

		require.Equal(t, "balance_sync_last_success_timestamp", utils.BalanceSyncLastSuccessTimestamp.Name,
			"the declared name must carry no unit suffix: OTLP appends one from Unit, and "+
				"spelling _seconds here lands the series as _seconds_seconds in Mimir")

		points := gaugePoints(t, reader, utils.BalanceSyncLastSuccessTimestamp.Name)
		require.Len(t, points, 1, "a completed batch must stamp exactly one point")
		assert.Equal(t, orgID.String(), points[0].labels["organization_id"])
		assert.Equal(t, ledgerID.String(), points[0].labels["ledger_id"])
		assert.Equal(t, "acme", points[0].labels["tenant_id"])
		assert.Positive(t, points[0].value, "the gauge must carry a unix timestamp")
	})

	t.Run("cycle that syncs nothing but completes still stamps the gauge", func(t *testing.T) {
		t.Parallel()

		key := syncKeyFor(orgID, ledgerID, "acc1")

		ctrl := gomock.NewController(t)
		repo := redisTransaction.NewMockRedisRepository(ctrl)
		repo.EXPECT().
			GetBalancesByKeys(gomock.Any(), gomock.Any()).
			Return(map[string]*mmodel.BalanceRedis{}, nil).
			Times(1)
		repo.EXPECT().
			RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
			Return(int64(1), nil).
			Times(1)

		reader, factory := newBalanceSyncReaderFactory(t)

		w := NewBalanceSyncWorker(
			newTestLogger(),
			&command.UseCase{TransactionRedisRepo: repo},
			BalanceSyncConfig{},
		)

		ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)

		w.processSyncBatch(ctx, orgID, ledgerID, []redisTransaction.SyncKey{{Key: key}})

		points := gaugePoints(t, reader, utils.BalanceSyncLastSuccessTimestamp.Name)
		require.Len(t, points, 1,
			"a cycle that persisted nothing but ran to completion is still a sign of life")
		assert.Equal(t, "", points[0].labels["tenant_id"],
			"single-tenant carries an empty tenant_id so existing series keep their identity")
	})

	t.Run("failed batch leaves the gauge unstamped", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		repo := redisTransaction.NewMockRedisRepository(ctrl)
		repo.EXPECT().
			GetBalancesByKeys(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("sql: database is closed")).
			Times(1)

		reader, factory := newBalanceSyncReaderFactory(t)

		w := NewBalanceSyncWorker(
			newTestLogger(),
			&command.UseCase{TransactionRedisRepo: repo},
			BalanceSyncConfig{},
		)

		ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)

		require.False(t, w.processSyncBatch(ctx, orgID, ledgerID, []redisTransaction.SyncKey{
			{Key: syncKeyFor(orgID, ledgerID, "acc1")},
		}))

		assert.Empty(t, gaugePoints(t, reader, utils.BalanceSyncLastSuccessTimestamp.Name),
			"a failed batch must not look like a successful sync to the staleness alert")
	})
}
