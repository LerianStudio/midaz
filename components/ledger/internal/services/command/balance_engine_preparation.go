// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type balanceEnginePreparationInput struct {
	organizationID       uuid.UUID
	ledgerID             uuid.UUID
	translation          BalanceEngineTranslationInput
	validateBalanceRules bool
}

type balanceEnginePreparedTransaction struct {
	pool        BalanceEngineSnapshotPool
	transaction engine.Transaction
	projection  []FrozenProjectionContext
}

// prepareBalanceEngineTransaction performs only snapshot-dependent reads and
// validation. Route validation receives ordered explicit legs, never speculative
// overdraft movements. The complete pool is retained for authoritative execution.
func (uc *UseCase) prepareBalanceEngineTransaction(ctx context.Context, input balanceEnginePreparationInput) (balanceEnginePreparedTransaction, error) {
	if err := ctx.Err(); err != nil {
		return balanceEnginePreparedTransaction{}, err
	}

	if uc == nil || uc.TransactionReader == nil || input.translation.Validate == nil {
		return balanceEnginePreparedTransaction{}, invalidBalanceEngineTranslation("preparation requires a reader and validated intent")
	}

	ctx = readrouting.WithPrimaryRead(ctx)
	aliases := input.translation.Validate.Aliases

	// Cancellation restores sources only; destination state must not introduce
	// an eligibility requirement after the original hold was accepted.
	if input.translation.Action == constant.ActionCancel {
		aliases = make([]string, 0, len(input.translation.TransactionInput.Send.Source.From))
		for _, leg := range input.translation.TransactionInput.Send.Source.From {
			aliases = append(aliases, mtransaction.SplitAliasWithKey(leg.AccountAlias))
		}
	}

	pool, err := LoadBalanceEngineSnapshotPool(ctx, input.organizationID, input.ledgerID, aliases, uc.TransactionReader.GetBalances)
	if err != nil {
		return balanceEnginePreparedTransaction{}, err
	}

	if err := rejectInternalScopeBalances(ctx, pool.ExplicitBalances); err != nil {
		return balanceEnginePreparedTransaction{}, err
	}

	operations, err := orderedBalanceEngineValidationOperations(input.translation, pool.ExplicitBalances)
	if err != nil {
		return balanceEnginePreparedTransaction{}, err
	}

	if err := ctx.Err(); err != nil {
		return balanceEnginePreparedTransaction{}, err
	}

	routeCache, err := uc.TransactionReader.ValidateAccountingRules(ctx, input.organizationID, input.ledgerID, operations, input.translation.Validate, input.translation.Action)
	if err != nil {
		return balanceEnginePreparedTransaction{}, err
	}

	if err := ctx.Err(); err != nil {
		return balanceEnginePreparedTransaction{}, err
	}

	if input.validateBalanceRules {
		balances, err := deduplicateBalances(operations)
		if err != nil {
			return balanceEnginePreparedTransaction{}, err
		}

		if err := mtransaction.ValidateBalancesRules(ctx, input.translation.TransactionInput, *input.translation.Validate, balances, nil); err != nil {
			return balanceEnginePreparedTransaction{}, err
		}
	}

	input.translation.Balances = pool.Balances
	input.translation.RouteCache = routeCache

	translated, projection, err := TranslateBalanceEngineTransaction(input.translation)
	if err != nil {
		return balanceEnginePreparedTransaction{}, err
	}

	return balanceEnginePreparedTransaction{pool: pool, transaction: translated, projection: projection}, nil
}

// orderedBalanceEngineValidationOperations preserves the existing route DTO's
// double-entry shape without calculating balances or adding overdraft companions.
// Repeated aliases remain separate legs; deduplicateBalances subsequently collapses
// only the two static entries belonging to the same source leg.
func orderedBalanceEngineValidationOperations(input BalanceEngineTranslationInput, explicit []*mmodel.Balance) ([]mmodel.BalanceOperation, error) {
	balances, err := indexTranslationBalances(explicit)
	if err != nil {
		return nil, err
	}

	operations := make([]mmodel.BalanceOperation, 0, len(input.Validate.From)+len(input.Validate.To))
	for _, side := range []struct {
		legs    []mtransaction.FromTo
		amounts map[string]mtransaction.Amount
		source  bool
	}{
		{input.TransactionInput.Send.Source.From, input.Validate.From, true},
		{input.TransactionInput.Send.Distribute.To, input.Validate.To, false},
	} {
		if input.Action == constant.ActionCancel && !side.source {
			continue
		}

		for _, leg := range side.legs {
			amount, ok := side.amounts[leg.AccountAlias]
			if !ok {
				return nil, invalidBalanceEngineTranslation("missing validated leg during preparation")
			}

			balance, ok := balances[mtransaction.SplitAliasWithKey(leg.AccountAlias)]
			if !ok {
				return nil, pkg.ValidateBusinessError(constant.ErrAccountIneligibility, "ValidateAccounts")
			}

			if side.source && mtransaction.IsDoubleEntrySource(amount) {
				first, second := mtransaction.SplitDoubleEntryOps(amount)
				operations = append(
					operations,
					mmodel.BalanceOperation{Balance: balance, Alias: leg.AccountAlias, Amount: first},
					mmodel.BalanceOperation{Balance: balance, Alias: leg.AccountAlias, Amount: second},
				)
			} else {
				operations = append(operations, mmodel.BalanceOperation{Balance: balance, Alias: leg.AccountAlias, Amount: amount})
			}
		}
	}

	return operations, nil
}
