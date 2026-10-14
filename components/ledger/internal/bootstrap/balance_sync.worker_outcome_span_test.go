// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// spanInt64Attributes returns the integer attributes of the named ended span.
func spanInt64Attributes(t *testing.T, recorder *tracetest.SpanRecorder, name string) map[string]int64 {
	t.Helper()

	for _, s := range recorder.Ended() {
		if s.Name() != name {
			continue
		}

		attrs := make(map[string]int64, len(s.Attributes()))

		for _, kv := range s.Attributes() {
			if kv.Value.Type() == attribute.INT64 {
				attrs[string(kv.Key)] = kv.Value.AsInt64()
			}
		}

		return attrs
	}

	t.Fatalf("span %q was not recorded", name)

	return nil
}

// TestProcessSyncBatch_SpanCarriesBatchOutcome verifies the batch outcome rides the
// worker span instead of a per-batch INFO log: every completed batch carries the four
// counters, including one that synced nothing, and the worker logs nothing on success.
func TestProcessSyncBatch_SpanCarriesBatchOutcome(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	aliases := []string{"acc1", "acc2", "acc3"}

	keys := make([]redisTransaction.SyncKey, 0, len(aliases))
	for _, alias := range aliases {
		keys = append(keys, redisTransaction.SyncKey{Key: syncKeyFor(orgID, ledgerID, alias)})
	}

	tests := []struct {
		name         string
		balances     map[string]*mmodel.BalanceRedis
		wantProgress bool
		want         map[string]int64
	}{
		{
			name: "batch_of_three_keys_synced",
			balances: map[string]*mmodel.BalanceRedis{
				keys[0].Key: {ID: uuid.MustParse("33333333-3333-3333-3333-333333333331").String(), Alias: "@acc1", AssetCode: "USD", Version: 2},
				keys[1].Key: {ID: uuid.MustParse("33333333-3333-3333-3333-333333333332").String(), Alias: "@acc2", AssetCode: "USD", Version: 2},
				keys[2].Key: {ID: uuid.MustParse("33333333-3333-3333-3333-333333333333").String(), Alias: "@acc3", AssetCode: "USD", Version: 2},
			},
			wantProgress: true,
			want: map[string]int64{
				"app.balance_sync.keys_processed":      3,
				"app.balance_sync.balances_aggregated": 3,
				"app.balance_sync.balances_synced":     3,
				"app.balance_sync.keys_removed":        3,
			},
		},
		{
			name:         "batch_that_synced_nothing_still_carries_outcome",
			balances:     map[string]*mmodel.BalanceRedis{},
			wantProgress: true,
			want: map[string]int64{
				"app.balance_sync.keys_processed":      3,
				"app.balance_sync.balances_aggregated": 0,
				"app.balance_sync.balances_synced":     0,
				"app.balance_sync.keys_removed":        3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			repo := redisTransaction.NewMockRedisRepository(ctrl)
			repo.EXPECT().
				GetBalancesByKeys(gomock.Any(), gomock.Any()).
				Return(tt.balances, nil).
				Times(1)
			repo.EXPECT().
				RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
				Return(int64(len(keys)), nil).
				Times(1)

			balanceRepo := balance.NewMockRepository(ctrl)
			if len(tt.balances) > 0 {
				balanceRepo.EXPECT().
					UpdateMany(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(int64(len(tt.balances)), nil).
					Times(1)
			}

			// No Log expectation is registered: any worker log call on a completed
			// batch, at any level, fails the test through gomock.
			logger := libLog.NewMockLogger(ctrl)

			w := NewBalanceSyncWorker(
				logger,
				&command.UseCase{TransactionRedisRepo: repo, BalanceRepo: balanceRepo},
				BalanceSyncConfig{},
			)

			_, factory := newBalanceSyncReaderFactory(t)

			ctx, recorder := recordingSpanContext()
			ctx = libObservability.ContextWithMetricFactory(ctx, factory)

			require.Equal(t, tt.wantProgress, w.processSyncBatch(ctx, orgID, ledgerID, keys))

			attrs := spanInt64Attributes(t, recorder, "balance.worker.process_batch")

			for key, want := range tt.want {
				require.Contains(t, attrs, key, "the outcome attribute must be set on every completed batch")
				assert.Equal(t, want, attrs[key], key)
			}
		})
	}
}

// TestProcessSyncBatch_FailureLogsErrorOnce verifies removing the per-batch INFO log
// left the failure signal intact: a failed batch logs "batch sync failed" once at
// ERROR and emits exactly one failure counter point.
func TestProcessSyncBatch_FailureLogsErrorOnce(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	ctrl := gomock.NewController(t)

	repo := redisTransaction.NewMockRedisRepository(ctrl)
	repo.EXPECT().
		GetBalancesByKeys(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("sql: database is closed")).
		Times(1)

	// Only the ERROR line is expected (tenant_id, organization_id, ledger_id, err);
	// any other worker log call fails the test through gomock.
	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().
		Log(gomock.Any(), libLog.LevelError, "BalanceSyncWorker: batch sync failed",
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(1)

	w := NewBalanceSyncWorker(
		logger,
		&command.UseCase{TransactionRedisRepo: repo},
		BalanceSyncConfig{},
	)

	reader, factory := newBalanceSyncReaderFactory(t)
	ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)

	require.False(t, w.processSyncBatch(ctx, orgID, ledgerID, []redisTransaction.SyncKey{
		{Key: syncKeyFor(orgID, ledgerID, "acc1")},
	}), "a failed batch must not report progress")

	require.Len(t, failureCounterLabels(t, reader), 1, "a failed batch must emit exactly one failure point")
}
