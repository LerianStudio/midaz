// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/LerianStudio/lib-streaming/v2/billing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buildActiveAccountBillingPayloads(tt.tran)

			require.Len(t, got, len(tt.wantAccountIDs))

			for i, wantID := range tt.wantAccountIDs {
				payload := got[i]
				require.NotNil(t, payload)

				assert.Equal(t, activeAccountMetric, payload.GetMetric())
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

// ensure the billing import is exercised even if the table changes.
var _ = billing.BillablePayload{}
