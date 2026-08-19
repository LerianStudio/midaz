// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-streaming/v3/billing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSerializer is a hand-rolled double for the unexported billingSerializer
// seam. GoMock cannot generate a mock for a package-private interface without
// exporting it, and the codebase already hand-rolls its streaming double
// (pkgStreaming.MockEmitter) for the same reason, so this mirrors that
// convention: canned, deterministic bytes or a configured error.
// fixedBillingTime is a deterministic timestamp for the emit-path fixture so the
// EmitRequest.Timestamp assertion is stable and never reads time.Now().
var fixedBillingTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

type fakeSerializer struct {
	raw []byte
	err error

	// got records every payload the emit path handed to Serialize so tests can
	// assert the resolved SubscriptionId. Populated only on the success path; a
	// serialize error captures nothing.
	got []*billing.BillablePayload
}

func (f *fakeSerializer) Serialize(p *billing.BillablePayload) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}

	f.got = append(f.got, p)

	return f.raw, nil
}

func newBillingTransaction(id, status string, ops ...*operation.Operation) *transaction.Transaction {
	return &transaction.Transaction{
		ID:         id,
		Status:     transaction.Status{Code: status},
		Operations: ops,
	}
}

func TestBuildActiveAccountBillingPayloads(t *testing.T) {
	t.Parallel()

	const (
		txID  = "11111111-1111-1111-1111-111111111111"
		accA  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		accB  = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		accC  = "cccccccc-cccc-cccc-cccc-cccccccccccc"
		extID = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
		subID = "99999999-9999-9999-9999-999999999999"
	)

	tests := []struct {
		name           string
		tran           *transaction.Transaction
		wantAccountIDs []string
	}{
		{
			name: "mixed internal and external ops drops external",
			tran: newBillingTransaction(txID, constant.APPROVED,
				&operation.Operation{AccountID: accA, AccountAlias: "@person1"},
				&operation.Operation{AccountID: extID, AccountAlias: constant.DefaultExternalAccountAliasPrefix + "USD"},
			),
			wantAccountIDs: []string{accA},
		},
		{
			name: "same internal account across two ops yields one payload",
			tran: newBillingTransaction(txID, constant.APPROVED,
				&operation.Operation{AccountID: accA, AccountAlias: "@person1"},
				&operation.Operation{AccountID: accA, AccountAlias: "@person1"},
			),
			wantAccountIDs: []string{accA},
		},
		{
			name: "all external yields nothing",
			tran: newBillingTransaction(txID, constant.APPROVED,
				&operation.Operation{AccountID: extID, AccountAlias: constant.DefaultExternalAccountAliasPrefix + "USD"},
				&operation.Operation{AccountID: extID, AccountAlias: constant.DefaultExternalAccountAliasPrefix + "BRL"},
			),
			wantAccountIDs: nil,
		},
		{
			name: "non-approved transaction yields nothing",
			tran: newBillingTransaction(txID, constant.PENDING,
				&operation.Operation{AccountID: accA, AccountAlias: "@person1"},
			),
			wantAccountIDs: nil,
		},
		{
			name:           "nil transaction yields nothing",
			tran:           nil,
			wantAccountIDs: nil,
		},
		{
			name: "three distinct internal accounts yield three payloads preserving order",
			tran: newBillingTransaction(txID, constant.APPROVED,
				&operation.Operation{AccountID: accA, AccountAlias: "@person1"},
				&operation.Operation{AccountID: accB, AccountAlias: "@person2"},
				&operation.Operation{AccountID: accC, AccountAlias: "@person3"},
			),
			wantAccountIDs: []string{accA, accB, accC},
		},
		{
			name: "operation with empty account id is skipped",
			tran: newBillingTransaction(txID, constant.APPROVED,
				&operation.Operation{AccountID: "", AccountAlias: "@person1"},
				&operation.Operation{AccountID: accA, AccountAlias: "@person1"},
			),
			wantAccountIDs: []string{accA},
		},
		{
			name: "nil operation entry is skipped",
			tran: newBillingTransaction(txID, constant.APPROVED,
				nil,
				&operation.Operation{AccountID: accA, AccountAlias: "@person1"},
			),
			wantAccountIDs: []string{accA},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buildActiveAccountBillingPayloads(tt.tran, subID)

			require.Len(t, got, len(tt.wantAccountIDs))

			for i, wantID := range tt.wantAccountIDs {
				// &got[i].Payload — never a copy: BillablePayload embeds a
				// sync.Mutex, so copying it by value trips govet copylocks.
				assert.Equal(t, wantID, got[i].AccountID)

				payload := &got[i].Payload

				assert.Equal(t, "active_account", payload.GetMetric())
				assert.Equal(t, subID, payload.GetSubscriptionId())

				props := payload.GetProperties()
				require.Contains(t, props, "account_id")
				require.Contains(t, props, "transaction_id")
				assert.Equal(t, wantID, props["account_id"].GetStringValue())
				assert.Equal(t, txID, props["transaction_id"].GetStringValue())
			}
		})
	}
}

func TestSendActiveAccountBillingEvents(t *testing.T) {
	t.Parallel()

	const (
		txID     = "11111111-1111-1111-1111-111111111111"
		accA     = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		accB     = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		orgID    = "22222222-2222-2222-2222-222222222222"
		tenantID = "33333333-3333-3333-3333-333333333333"
	)

	fakeBytes := []byte{0x00, 0x00, 0x00, 0x00, 0x01}

	approvedTwo := newBillingTransaction(txID, constant.APPROVED,
		&operation.Operation{AccountID: accA, AccountAlias: "@person1"},
		&operation.Operation{AccountID: accB, AccountAlias: "@person2"},
	)
	approvedTwo.CreatedAt = fixedBillingTime

	// approvedTwoWithOrg carries an OrganizationID so the subscription-identity
	// cases can assert the resolved SubscriptionId. Never mutated during a run,
	// so parallel subtests may share it read-only.
	approvedTwoWithOrg := newBillingTransaction(txID, constant.APPROVED,
		&operation.Operation{AccountID: accA, AccountAlias: "@person1"},
		&operation.Operation{AccountID: accB, AccountAlias: "@person2"},
	)
	approvedTwoWithOrg.OrganizationID = orgID
	approvedTwoWithOrg.CreatedAt = fixedBillingTime

	tests := []struct {
		name               string
		ctx                context.Context
		cancelCtx          bool
		tran               *transaction.Transaction
		phase              string
		multiTenant        bool
		withStreaming      bool
		withSerializer     bool
		emitErr            error
		serializeErr       error
		wantSubjects       []string
		wantSubscriptionID string
		wantTenantID       string
	}{
		{
			name:           "two unique internal accounts emit two events",
			ctx:            context.Background(),
			tran:           approvedTwo,
			phase:          TransactionLifecyclePhaseCreated,
			withStreaming:  true,
			withSerializer: true,
			wantSubjects:   []string{accA, accB},
		},
		{
			name:               "single-tenant uses transaction organization id as subscription",
			ctx:                context.Background(),
			tran:               approvedTwoWithOrg,
			phase:              TransactionLifecyclePhaseCreated,
			multiTenant:        false,
			withStreaming:      true,
			withSerializer:     true,
			wantSubjects:       []string{accA, accB},
			wantSubscriptionID: orgID,
			wantTenantID:       pkgStreaming.DefaultTenantID,
		},
		{
			name:               "multi-tenant uses resolved tenant id as subscription",
			ctx:                tmcore.ContextWithTenantID(context.Background(), tenantID),
			tran:               approvedTwoWithOrg,
			phase:              TransactionLifecyclePhaseCreated,
			multiTenant:        true,
			withStreaming:      true,
			withSerializer:     true,
			wantSubjects:       []string{accA, accB},
			wantSubscriptionID: tenantID,
			wantTenantID:       tenantID,
		},
		{
			name:           "noop phase emits nothing even when approved",
			ctx:            context.Background(),
			tran:           approvedTwo,
			phase:          TransactionLifecyclePhaseNoop,
			withStreaming:  true,
			withSerializer: true,
			wantSubjects:   nil,
		},
		{
			name:           "emitter error is swallowed and nothing captured",
			ctx:            context.Background(),
			tran:           approvedTwo,
			phase:          TransactionLifecyclePhaseCreated,
			withStreaming:  true,
			withSerializer: true,
			emitErr:        errors.New("broker down"),
			wantSubjects:   nil,
		},
		{
			name:           "serializer error skips emit",
			ctx:            context.Background(),
			tran:           approvedTwo,
			phase:          TransactionLifecyclePhaseCreated,
			withStreaming:  true,
			withSerializer: true,
			serializeErr:   errors.New("registry down"),
			wantSubjects:   nil,
		},
		{
			name:           "nil serializer is a clean no-op",
			ctx:            context.Background(),
			tran:           approvedTwo,
			phase:          TransactionLifecyclePhaseCreated,
			withStreaming:  true,
			withSerializer: false,
			wantSubjects:   nil,
		},
		{
			name:           "nil streaming is a clean no-op",
			ctx:            context.Background(),
			tran:           approvedTwo,
			phase:          TransactionLifecyclePhaseCreated,
			withStreaming:  false,
			withSerializer: true,
			wantSubjects:   nil,
		},
		{
			name:           "non-approved transaction emits nothing",
			ctx:            context.Background(),
			tran:           newBillingTransaction(txID, constant.PENDING, &operation.Operation{AccountID: accA, AccountAlias: "@person1"}),
			phase:          TransactionLifecyclePhaseCreated,
			withStreaming:  true,
			withSerializer: true,
			wantSubjects:   nil,
		},
		{
			name:           "nil transaction is a clean no-op",
			ctx:            context.Background(),
			tran:           nil,
			phase:          TransactionLifecyclePhaseCreated,
			withStreaming:  true,
			withSerializer: true,
			wantSubjects:   nil,
		},
		{
			name:           "canceled context emits nothing before build or emit",
			ctx:            context.Background(),
			cancelCtx:      true,
			tran:           approvedTwo,
			phase:          TransactionLifecyclePhaseCreated,
			withStreaming:  true,
			withSerializer: true,
			wantSubjects:   nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var mockEmitter *pkgStreaming.MockEmitter

			uc := &UseCase{MultiTenantEnabled: tt.multiTenant}

			if tt.withStreaming {
				mockEmitter = pkgStreaming.NewMockEmitter()
				mockEmitter.SetError(tt.emitErr)
				uc.Streaming = mockEmitter
			}

			var fs *fakeSerializer
			if tt.withSerializer {
				fs = &fakeSerializer{raw: fakeBytes, err: tt.serializeErr}
				uc.BillingSerializer = fs
			}

			// Build the canceled context per-run so parallel subtests never
			// share one; a canceled context is deterministic (no time.Now()).
			callCtx := tt.ctx
			if tt.cancelCtx {
				var cancel context.CancelFunc
				callCtx, cancel = context.WithCancel(tt.ctx)
				cancel()
			}

			require.NotPanics(t, func() {
				uc.SendActiveAccountBillingEvents(callCtx, tt.tran, tt.phase)
			})

			if mockEmitter == nil {
				return
			}

			events := mockEmitter.Events()
			require.Len(t, events, len(tt.wantSubjects))

			wantTenant := tt.wantTenantID
			if wantTenant == "" {
				wantTenant = pkgStreaming.DefaultTenantID
			}

			for i, wantSubject := range tt.wantSubjects {
				assert.Equal(t, "billing_recorded", events[i].DefinitionKey)
				assert.Equal(t, wantSubject, events[i].Subject)
				assert.Equal(t, wantTenant, events[i].TenantID)
				assert.Equal(t, fakeBytes, []byte(events[i].Payload))
				assert.Equal(t, fixedBillingTime, events[i].Timestamp)

				// ce-subject stays the internal account ID; the billing customer
				// identity lives on the payload's SubscriptionId.
				if tt.wantSubscriptionID != "" {
					require.NotNil(t, fs)
					require.Greater(t, len(fs.got), i)
					assert.Equal(t, tt.wantSubscriptionID, fs.got[i].GetSubscriptionId())
				}
			}
		})
	}
}
