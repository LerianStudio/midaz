// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

// recordingSpanContext wires an in-memory SpanRecorder into the lib-observability
// tracking context so worker spans are recorded onto inspectable SDK spans.
func recordingSpanContext() (context.Context, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	ctx := libObservability.ContextWithTracer(context.Background(), tp.Tracer("balance-sync-worker-test"))

	return ctx, recorder
}

// spanAttributes returns the string attributes of the named ended span.
func spanAttributes(t *testing.T, recorder *tracetest.SpanRecorder, name string) map[string]string {
	t.Helper()

	for _, s := range recorder.Ended() {
		if s.Name() != name {
			continue
		}

		attrs := make(map[string]string, len(s.Attributes()))
		for _, kv := range s.Attributes() {
			attrs[string(kv.Key)] = kv.Value.AsString()
		}

		return attrs
	}

	t.Fatalf("span %q was not recorded", name)

	return nil
}

// TestProcessSyncBatch_SpanCarriesScope verifies the worker span carries the scope
// IDs, so a stuck tenant is locatable from the trace without correlating timestamps.
func TestProcessSyncBatch_SpanCarriesScope(t *testing.T) {
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

			ctx, recorder := recordingSpanContext()
			if tt.tenantID != "" {
				ctx = tmcore.ContextWithTenantID(ctx, tt.tenantID)
			}

			require.False(t, w.processSyncBatch(ctx, orgID, ledgerID, []redisTransaction.SyncKey{
				{Key: "balance:{transactions}:org:ledger:alias#key"},
			}), "a failed batch must not report progress")

			attrs := spanAttributes(t, recorder, "balance.worker.process_batch")

			assert.Equal(t, orgID.String(), attrs["app.request.organization_id"])
			assert.Equal(t, ledgerID.String(), attrs["app.request.ledger_id"])
			require.Contains(t, attrs, "app.tenant_id",
				"the attribute must be set on both modes, not conditionally")
			assert.Equal(t, tt.wantTenant, attrs["app.tenant_id"])
		})
	}
}
