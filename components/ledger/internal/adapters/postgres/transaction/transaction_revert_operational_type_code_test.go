// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"testing"

	constant "github.com/LerianStudio/lib-commons/v6/commons/constants"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
)

// TestTransactionRevert_CopiesOperationalTypeCode proves the reverse inherits the
// operationalTypeCode (economic-content parity, like ChartOfAccounts/Metadata). The
// TRANSPASSE grant is NOT inherited here — phase 2.3 re-evaluates exceptions at revert.
func TestTransactionRevert_CopiesOperationalTypeCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
	}{
		{name: "absent stays empty", code: ""},
		{name: "present is inherited by the reverse", code: "PIX_IN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			amount := decimal.NewFromInt(40)
			txn := Transaction{
				Description:         "transfer",
				AssetCode:           "BRL",
				Amount:              &amount,
				OperationalTypeCode: tt.code,
				Operations: []*operation.Operation{
					{
						Type:         constant.CREDIT,
						AccountAlias: "@receiver",
						AssetCode:    "BRL",
						Amount:       operation.Amount{Value: &amount},
					},
					{
						Type:         constant.DEBIT,
						AccountAlias: "@sender",
						AssetCode:    "BRL",
						Amount:       operation.Amount{Value: &amount},
					},
				},
			}

			reverted := txn.TransactionRevert()

			assert.Equal(t, tt.code, reverted.OperationalTypeCode,
				"TransactionRevert must copy OperationalTypeCode onto the reverse carrier")
		})
	}
}
