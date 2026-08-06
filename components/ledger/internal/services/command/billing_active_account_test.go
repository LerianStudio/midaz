// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/lib-streaming/v2/billing"
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
type fakeSerializer struct {
	raw []byte
	err error
}

func (f fakeSerializer) Serialize(*billing.BillablePayload) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}

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

			got := buildActiveAccountBillingPayloads(tt.tran)

			require.Len(t, got, len(tt.wantAccountIDs))

			for i, wantID := range tt.wantAccountIDs {
				payload := &got[i]

				assert.Equal(t, "active_account", payload.GetMetric())
				assert.Equal(t, wantID, payload.GetSubscriptionId())

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
		txID = "11111111-1111-1111-1111-111111111111"
		accA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		accB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	)

	fakeBytes := []byte{0x00, 0x00, 0x00, 0x00, 0x01}

	approvedTwo := newBillingTransaction(txID, constant.APPROVED,
		&operation.Operation{AccountID: accA, AccountAlias: "@person1"},
		&operation.Operation{AccountID: accB, AccountAlias: "@person2"},
	)

	tests := []struct {
		name           string
		tran           *transaction.Transaction
		phase          string
		withStreaming  bool
		withSerializer bool
		emitErr        error
		serializeErr   error
		wantSubjects   []string
	}{
		{
			name:           "two unique internal accounts emit two events",
			tran:           approvedTwo,
			phase:          TransactionLifecyclePhaseCreated,
			withStreaming:  true,
			withSerializer: true,
			wantSubjects:   []string{accA, accB},
		},
		{
			name:           "noop phase emits nothing even when approved",
			tran:           approvedTwo,
			phase:          TransactionLifecyclePhaseNoop,
			withStreaming:  true,
			withSerializer: true,
			wantSubjects:   nil,
		},
		{
			name:           "emitter error is swallowed and nothing captured",
			tran:           approvedTwo,
			phase:          TransactionLifecyclePhaseCreated,
			withStreaming:  true,
			withSerializer: true,
			emitErr:        errors.New("broker down"),
			wantSubjects:   nil,
		},
		{
			name:           "serializer error skips emit",
			tran:           approvedTwo,
			phase:          TransactionLifecyclePhaseCreated,
			withStreaming:  true,
			withSerializer: true,
			serializeErr:   errors.New("registry down"),
			wantSubjects:   nil,
		},
		{
			name:           "nil serializer is a clean no-op",
			tran:           approvedTwo,
			phase:          TransactionLifecyclePhaseCreated,
			withStreaming:  true,
			withSerializer: false,
			wantSubjects:   nil,
		},
		{
			name:           "nil streaming is a clean no-op",
			tran:           approvedTwo,
			phase:          TransactionLifecyclePhaseCreated,
			withStreaming:  false,
			withSerializer: true,
			wantSubjects:   nil,
		},
		{
			name:           "non-approved transaction emits nothing",
			tran:           newBillingTransaction(txID, constant.PENDING, &operation.Operation{AccountID: accA, AccountAlias: "@person1"}),
			phase:          TransactionLifecyclePhaseCreated,
			withStreaming:  true,
			withSerializer: true,
			wantSubjects:   nil,
		},
		{
			name:           "nil transaction is a clean no-op",
			tran:           nil,
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

			uc := &UseCase{}

			if tt.withStreaming {
				mockEmitter = pkgStreaming.NewMockEmitter()
				mockEmitter.SetError(tt.emitErr)
				uc.Streaming = mockEmitter
			}

			if tt.withSerializer {
				uc.BillingSerializer = fakeSerializer{raw: fakeBytes, err: tt.serializeErr}
			}

			require.NotPanics(t, func() {
				uc.SendActiveAccountBillingEvents(context.Background(), tt.tran, tt.phase)
			})

			if mockEmitter == nil {
				return
			}

			events := mockEmitter.Events()
			require.Len(t, events, len(tt.wantSubjects))

			for i, wantSubject := range tt.wantSubjects {
				assert.Equal(t, "billing_recorded", events[i].DefinitionKey)
				assert.Equal(t, wantSubject, events[i].Subject)
				assert.Equal(t, pkgStreaming.DefaultTenantID, events[i].TenantID)
				assert.Equal(t, fakeBytes, []byte(events[i].Payload))
			}
		})
	}
}
