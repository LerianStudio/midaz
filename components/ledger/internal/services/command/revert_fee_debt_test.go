// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	constant "github.com/LerianStudio/lib-commons/v7/commons/constants"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/constant"
)

type feeDebtOriginReader struct {
	TransactionReader
	reverted map[uuid.UUID]bool
	reads    *[]uuid.UUID
}

func (reader feeDebtOriginReader) GetParentByTransactionID(_ context.Context, _, _, parentID uuid.UUID) (*transaction.Transaction, error) {
	*reader.reads = append(*reader.reads, parentID)
	if reader.reverted[parentID] {
		return &transaction.Transaction{ID: uuid.NewString()}, nil
	}

	return nil, nil
}

func TestReverseTransactionKeepsSettlementsOfRevertedOrigins(t *testing.T) {
	t.Parallel()

	settle := func(origin string, amount int64) FeeDebtSettlement {
		return FeeDebtSettlement{
			DebtID: origin + ":from:1:debit", DebtorRef: "@debtor#default", CreditRef: "@fees#default",
			Amount: decimal.NewFromInt(amount), Opened: decimal.NewFromInt(20), Seq: 1,
			DebitRoute: &accounting.FeeDebtRoute{ID: "from-" + origin}, CreditRoute: &accounting.FeeDebtRoute{ID: "to-" + origin},
		}
	}
	row := func(kind, alias, direction, route string, value int64) *operation.Operation {
		amount := decimal.NewFromInt(value)

		return &operation.Operation{
			Type: kind, Direction: direction, AccountAlias: alias, BalanceKey: pkgConstant.DefaultBalanceKey,
			AssetCode: "USD", Amount: operation.Amount{Value: &amount}, RouteID: &route,
		}
	}

	amount := decimal.NewFromInt(20)
	credited := &transaction.Transaction{
		AssetCode: "USD", Amount: &amount,
		Metadata: feeDebtRevertMetadata(t, nil, []FeeDebtSettlement{settle(feeDebtOriginX, 5), settle(feeDebtOriginX, 7), settle(feeDebtOriginO, 3)}),
		Operations: []*operation.Operation{
			row(constant.DEBIT, "@source", pkgConstant.DirectionDebit, "", 20), row(constant.CREDIT, "@debtor", pkgConstant.DirectionCredit, "", 20),
			row(pkgConstant.FEE_SETTLEMENT, "@debtor", pkgConstant.DirectionDebit, "from-"+feeDebtOriginX, 12),
			row(pkgConstant.FEE_SETTLEMENT, "@fees", pkgConstant.DirectionCredit, "to-"+feeDebtOriginX, 12),
			row(pkgConstant.FEE_SETTLEMENT, "@debtor", pkgConstant.DirectionDebit, "from-"+feeDebtOriginO, 3),
			row(pkgConstant.FEE_SETTLEMENT, "@fees", pkgConstant.DirectionCredit, "to-"+feeDebtOriginO, 3),
		},
	}

	var reads []uuid.UUID

	uc := &UseCase{TransactionReader: feeDebtOriginReader{reverted: map[uuid.UUID]bool{uuid.MustParse(feeDebtOriginX): true}, reads: &reads}}

	reversal, err := uc.reverseTransaction(context.Background(), RevertTransactionInput{}, credited)
	require.NoError(t, err)

	assert.Equal(t, []uuid.UUID{uuid.MustParse(feeDebtOriginX), uuid.MustParse(feeDebtOriginO)}, reads, "one read per origin")
	assert.Equal(t, []string{feeDebtOriginX}, reversal.FeeDebtRevertedOrigins)

	sources := make(map[string]int64)
	for _, from := range reversal.Send.Source.From {
		sources[from.AccountAlias+"|"+*from.RouteID] = from.Amount.Value.IntPart()
	}

	assert.Equal(t, map[string]int64{"@debtor|": 17, "@fees|to-" + feeDebtOriginO: 3}, sources, "the reverted origin's 12 stays with the creditor")

	credited.Metadata = map[string]any{pkgConstant.MetadataKeyFeeDebtSettlements: `[{"debtId":"not-a-transaction:from:1:debit","debtorRef":"@debtor#default","creditRef":"@fees#default","amount":"1","opened":"1","seq":1}]`}

	_, err = uc.reverseTransaction(context.Background(), RevertTransactionInput{}, credited)
	require.ErrorIs(t, err, ErrInvalidEngineTranslation)
}
