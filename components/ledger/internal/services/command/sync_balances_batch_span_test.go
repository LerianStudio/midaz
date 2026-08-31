// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// TestSyncBalancesBatch_SpanCarriesScope locks the scope attributes onto the use-case
// span: org and ledger are method arguments (app.request.*), the tenant is a system
// observation, and the orphan count is what makes a silent drop visible on the trace.
func TestSyncBalancesBatch_SpanCarriesScope(t *testing.T) {
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	prefix := "balance:{transactions}:" + organizationID.String() + ":" + ledgerID.String() + ":"

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
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			expiredKey := prefix + "@acc1#default"
			keys := []string{expiredKey}

			mockRedis := redis.NewMockRedisRepository(ctrl)
			mockRedis.EXPECT().
				GetBalancesByKeys(gomock.Any(), keys).
				Return(map[string]*mmodel.BalanceRedis{expiredKey: nil}, nil).
				Times(1)
			mockRedis.EXPECT().
				RemoveBalanceSyncKeysBatch(gomock.Any(), gomock.Any()).
				Return(int64(1), nil).
				Times(1)

			ctx, recorder := recordingContext()
			if tt.tenantID != "" {
				ctx = tmcore.ContextWithTenantID(ctx, tt.tenantID)
			}

			uc := UseCase{TransactionRedisRepo: mockRedis}

			_, err := uc.SyncBalancesBatch(ctx, organizationID, ledgerID, toSyncKeys(keys))
			require.NoError(t, err)

			span := findSpan(t, recorder, "command.sync_balances_batch")

			attrs := make(map[string]string, len(span.Attributes()))
			ints := make(map[string]int64, len(span.Attributes()))

			for _, kv := range span.Attributes() {
				attrs[string(kv.Key)] = kv.Value.AsString()
				ints[string(kv.Key)] = kv.Value.AsInt64()
			}

			assert.Equal(t, organizationID.String(), attrs["app.request.organization_id"])
			assert.Equal(t, ledgerID.String(), attrs["app.request.ledger_id"])
			require.Contains(t, attrs, "app.tenant_id",
				"the attribute must be set on both modes, not conditionally")
			assert.Equal(t, tt.wantTenant, attrs["app.tenant_id"])
			assert.Equal(t, int64(1), ints["app.balance_sync.orphaned_keys"])
		})
	}
}
