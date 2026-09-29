// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// reverseTransaction builds the non-empty reversal of tran. A settlement of a debt
// whose origin is already reverted stays with its creditor, whose refund paid it
// back, and none of that origin's debts reopens.
func (uc *UseCase) reverseTransaction(ctx context.Context, in RevertTransactionInput, tran *transaction.Transaction) (mtransaction.Transaction, error) {
	_, settlements, err := feeDebtRevertFacts(tran.Metadata)
	if err != nil {
		return mtransaction.Transaction{}, err
	}

	reverted := make(map[string]bool)

	var (
		origins []string
		kept    map[transaction.FeeSettlementGroup]decimal.Decimal
	)

	for _, settlement := range settlements {
		origin := feeDebtOrigin(settlement.DebtID)

		isReverted, checked := reverted[origin]
		if !checked {
			isReverted, err = uc.feeDebtOriginReverted(ctx, in, origin)
			if err != nil {
				return mtransaction.Transaction{}, err
			}

			reverted[origin] = isReverted
			if isReverted {
				origins = append(origins, origin)
			}
		}

		if !isReverted {
			continue
		}

		if kept == nil {
			kept = make(map[transaction.FeeSettlementGroup]decimal.Decimal)
		}

		credit := transaction.FeeSettlementGroup{Ref: settlement.CreditRef, RouteID: feeDebtRouteID(settlement.CreditRoute)}
		debit := transaction.FeeSettlementGroup{Ref: settlement.DebtorRef, RouteID: feeDebtRouteID(settlement.DebitRoute)}
		kept[credit], kept[debit] = kept[credit].Add(settlement.Amount), kept[debit].Sub(settlement.Amount)
	}

	reversal := tran.TransactionRevert(kept)
	if reversal.IsEmpty() {
		return mtransaction.Transaction{}, pkg.ValidateBusinessError(constant.ErrTransactionCantRevert, "RevertTransaction")
	}

	reversal.FeeDebtRevertedOrigins = origins

	return reversal, nil
}

// feeDebtOriginReverted reports whether the debt origin has a reversal in the
// revert's scope, with the read the revert gate uses.
func (uc *UseCase) feeDebtOriginReverted(ctx context.Context, in RevertTransactionInput, origin string) (bool, error) {
	originID, err := uuid.Parse(origin)
	if err != nil {
		return false, fmt.Errorf("%w: fee debt id has no origin transaction: %w", ErrInvalidEngineTranslation, err)
	}

	reversal, err := uc.TransactionReader.GetParentByTransactionID(ctx, in.OrganizationID, in.LedgerID, originID)
	if err != nil {
		return false, fmt.Errorf("read reversal of fee debt origin: %w", err)
	}

	return reversal != nil, nil
}

// feeDebtRouteID is the id of a stored fee-debt route, empty without one.
func feeDebtRouteID(route *accounting.FeeDebtRoute) string {
	if route == nil {
		return ""
	}

	return route.ID
}
