// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// TestBuildStandardOp_OperationTypeOverride verifies the BLOCK/UNBLOCK
// operation-type marker threaded through Transaction.OperationTypeOverride:
//   - absent marker reproduces current behavior (Type == DEBIT/CREDIT);
//   - present marker overrides only Type, leaving Direction untouched;
//   - the overdraft balance key still wins, yielding Type == OVERDRAFT.
func TestBuildStandardOp_OperationTypeOverride(t *testing.T) {
	t.Parallel()

	handler := &TransactionHandler{}
	txDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// baseTransaction returns a minimal parsed transaction with the given override.
	baseTransaction := func(override string) mtransaction.Transaction {
		return mtransaction.Transaction{
			Description:           "test",
			OperationTypeOverride: override,
			Send: mtransaction.Send{
				Asset: "BRL",
			},
		}
	}

	// leg builds the per-leg validation result. amt.Operation is the default
	// type label (DEBIT for source, CREDIT for destination); amt.Direction is
	// the accounting direction, which must never be touched by the override.
	leg := func(opTypeLabel, direction string) (mtransaction.FromTo, mtransaction.Amount) {
		return mtransaction.FromTo{Description: "leg"},
			mtransaction.Amount{
				Asset:     "BRL",
				Value:     decimal.NewFromInt(100),
				Operation: opTypeLabel,
				Direction: direction,
			}
	}

	balance := func(key string) *mmodel.Balance {
		return &mmodel.Balance{
			ID:        "bal-1",
			AccountID: "acc-1",
			Alias:     "@person1",
			Key:       key,
		}
	}

	bat := mtransaction.Balance{}
	tran := transaction.Transaction{ID: "tx-1"}

	tests := []struct {
		name              string
		override          string
		balanceKey        string
		opTypeLabel       string
		direction         string
		expectedType      string
		expectedDirection string
	}{
		{
			name:              "no override source leg keeps DEBIT",
			override:          "",
			balanceKey:        "default",
			opTypeLabel:       constant.DEBIT,
			direction:         constant.DirectionDebit,
			expectedType:      constant.DEBIT,
			expectedDirection: constant.DirectionDebit,
		},
		{
			name:              "no override destination leg keeps CREDIT",
			override:          "",
			balanceKey:        "default",
			opTypeLabel:       constant.CREDIT,
			direction:         constant.DirectionCredit,
			expectedType:      constant.CREDIT,
			expectedDirection: constant.DirectionCredit,
		},
		{
			name:              "BLOCK override on source leg overrides Type but keeps debit Direction",
			override:          constant.BLOCK,
			balanceKey:        "default",
			opTypeLabel:       constant.DEBIT,
			direction:         constant.DirectionDebit,
			expectedType:      constant.BLOCK,
			expectedDirection: constant.DirectionDebit,
		},
		{
			name:              "BLOCK override on destination leg overrides Type but keeps credit Direction",
			override:          constant.BLOCK,
			balanceKey:        "default",
			opTypeLabel:       constant.CREDIT,
			direction:         constant.DirectionCredit,
			expectedType:      constant.BLOCK,
			expectedDirection: constant.DirectionCredit,
		},
		{
			name:              "UNBLOCK override on source leg overrides Type but keeps debit Direction",
			override:          constant.UNBLOCK,
			balanceKey:        "default",
			opTypeLabel:       constant.DEBIT,
			direction:         constant.DirectionDebit,
			expectedType:      constant.UNBLOCK,
			expectedDirection: constant.DirectionDebit,
		},
		{
			name:              "overdraft balance key wins over BLOCK override",
			override:          constant.BLOCK,
			balanceKey:        constant.OverdraftBalanceKey,
			opTypeLabel:       constant.DEBIT,
			direction:         constant.DirectionDebit,
			expectedType:      constant.OVERDRAFT,
			expectedDirection: constant.DirectionDebit,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ft, amt := leg(tt.opTypeLabel, tt.direction)
			blc := balance(tt.balanceKey)
			txInput := baseTransaction(tt.override)

			op, err := handler.buildStandardOp(blc, ft, amt, bat, tran, txInput, txDate, false)
			require.NoError(t, err)
			require.NotNil(t, op)

			require.Equal(t, tt.expectedType, op.Type, "Type label")
			require.Equal(t, tt.expectedDirection, op.Direction, "Direction must be preserved")
		})
	}
}
