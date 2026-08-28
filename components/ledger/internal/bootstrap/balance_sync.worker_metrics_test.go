// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	"github.com/LerianStudio/lib-observability/v2/metrics"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// TestBalanceSyncWorker_WithMetricsFactory verifies the fluent setter wires the
// factory onto the worker.
func TestBalanceSyncWorker_WithMetricsFactory(t *testing.T) {
	t.Parallel()

	meter := sdkmetric.NewMeterProvider().Meter("test")
	factory, err := metrics.NewMetricsFactory(meter, nil)
	require.NoError(t, err)

	worker := NewBalanceSyncWorker(newTestLogger(), &command.UseCase{}, BalanceSyncConfig{}).
		WithMetricsFactory(factory)

	assert.Same(t, factory, worker.metricsFactory, "WithMetricsFactory must set the factory")
}

// TestBalanceSyncWorker_EmitTenantSkip verifies the tenant-skip counter is
// nil-safe and emits without error (with a bounded tenant_id label) when a
// factory is wired.
func TestBalanceSyncWorker_EmitTenantSkip(t *testing.T) {
	t.Parallel()

	t.Run("nil factory is a no-op", func(t *testing.T) {
		t.Parallel()

		worker := NewBalanceSyncWorker(newTestLogger(), &command.UseCase{}, BalanceSyncConfig{})

		require.NotPanics(t, func() {
			worker.emitTenantSkip(context.Background(), "tenant-123")
		})
	})

	t.Run("with factory emits", func(t *testing.T) {
		t.Parallel()

		meter := sdkmetric.NewMeterProvider().Meter("test")
		factory, err := metrics.NewMetricsFactory(meter, nil)
		require.NoError(t, err)

		worker := NewBalanceSyncWorker(newTestLogger(), &command.UseCase{}, BalanceSyncConfig{}).
			WithMetricsFactory(factory)

		require.NotPanics(t, func() {
			worker.emitTenantSkip(context.Background(), "tenant-123")
		})
	})
}

// TestProcessSyncBatch_FailureCounterCarriesTenantID verifies a batch failure is
// attributable to a tenant: the failure counter carries tenant_id, empty in
// single-tenant so existing series keep their identity.
func TestProcessSyncBatch_FailureCounterCarriesTenantID(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	tests := []struct {
		name       string
		tenantID   string
		wantTenant string
	}{
		{name: "multi_tenant_carries_tenant_id", tenantID: "acme", wantTenant: "acme"},
		{name: "single_tenant_carries_empty_tenant_id", tenantID: "", wantTenant: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reader, factory := newBalanceSyncReaderFactory(t)

			ctrl := gomock.NewController(t)
			repo := redisTransaction.NewMockRedisRepository(ctrl)
			repo.EXPECT().
				GetBalancesByKeys(gomock.Any(), gomock.Any()).
				Return(nil, errors.New("sql: database is closed")).
				Times(1)

			w := NewBalanceSyncWorker(
				newTestLogger(),
				&command.UseCase{TransactionRedisRepo: repo},
				BalanceSyncConfig{},
			)

			ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)
			if tt.tenantID != "" {
				ctx = tmcore.ContextWithTenantID(ctx, tt.tenantID)
			}

			ok := w.processSyncBatch(ctx, orgID, ledgerID, []redisTransaction.SyncKey{
				{Key: "balance:{transactions}:org:ledger:alias#key"},
			})

			require.False(t, ok, "a failed batch must not report progress")

			labels := failureCounterLabels(t, reader)
			require.Len(t, labels, 1, "exactly one failure point must be recorded")
			assert.Equal(t, orgID.String(), labels[0]["organization_id"])
			assert.Equal(t, ledgerID.String(), labels[0]["ledger_id"])
			require.Contains(t, labels[0], "tenant_id",
				"the label must be emitted on both modes, not conditionally")
			assert.Equal(t, tt.wantTenant, labels[0]["tenant_id"],
				"tenant_id must carry the tenant, empty in single-tenant")
		})
	}
}

// failureCounterLabels returns the label sets recorded on the batch-failure counter.
func failureCounterLabels(t *testing.T, reader *sdkmetric.ManualReader) []map[string]string {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	var out []map[string]string

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != utils.BalanceSyncBatchFailures.Name {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "failure counter data type must be Sum[int64], got %T", m.Data)

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
