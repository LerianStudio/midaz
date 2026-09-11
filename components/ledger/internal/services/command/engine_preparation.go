// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type enginePreparationInput struct {
	organizationID uuid.UUID
	ledgerID       uuid.UUID
	translation    EngineTranslationInput
}

type enginePreparedTransaction struct {
	pool        EngineSnapshotPool
	transaction accounting.Transaction
	projection  []OperationRecordSpec
}

// prepareEngineTransaction performs cache-aside seed loading and route
// validation. Balance state and account eligibility are evaluated later against
// live cached values by the atomic engine execution.
func (uc *UseCase) prepareEngineTransaction(ctx context.Context, input enginePreparationInput) (enginePreparedTransaction, error) {
	if err := ctx.Err(); err != nil {
		return enginePreparedTransaction{}, err
	}

	if uc == nil || uc.TransactionReader == nil || input.translation.Validate == nil {
		return enginePreparedTransaction{}, invalidEngineTranslation("preparation requires a reader and validated intent")
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

	pool, err := loadPreparedEngineSnapshots(ctx, uc.TransactionReader, input.organizationID, input.ledgerID, aliases)
	if err != nil {
		return enginePreparedTransaction{}, err
	}

	if err := rejectInternalScopeBalances(ctx, pool.ExplicitBalances); err != nil {
		return enginePreparedTransaction{}, err
	}

	operations, err := orderedEngineValidationOperations(input.translation, pool.ExplicitBalances)
	if err != nil {
		return enginePreparedTransaction{}, err
	}

	if err := ctx.Err(); err != nil {
		return enginePreparedTransaction{}, err
	}

	routeCache, err := uc.TransactionReader.ValidateAccountingRules(ctx, input.organizationID, input.ledgerID, operations, input.translation.Validate, input.translation.Action)
	if err != nil {
		return enginePreparedTransaction{}, err
	}

	if err := ctx.Err(); err != nil {
		return enginePreparedTransaction{}, err
	}

	input.translation.Balances = pool.Balances
	input.translation.RouteCache = routeCache

	translated, projection, err := TranslateEngineTransaction(input.translation)
	if err != nil {
		return enginePreparedTransaction{}, err
	}

	return enginePreparedTransaction{pool: pool, transaction: translated, projection: projection}, nil
}

func loadPreparedEngineSnapshots(ctx context.Context, reader TransactionReader, organizationID, ledgerID uuid.UUID, aliases []string) (EngineSnapshotPool, error) {
	explicitBalances, executionBalances, err := reader.GetEngineBalances(ctx, organizationID, ledgerID, aliases)
	if err != nil {
		return EngineSnapshotPool{}, fmt.Errorf("load engine balances: %w", err)
	}

	return BuildEngineSnapshotPool(ctx, organizationID, ledgerID, aliases, explicitBalances, executionBalances)
}

// orderedEngineValidationOperations preserves the existing route DTO's
// double-entry shape without calculating balances or adding overdraft companions.
// Repeated aliases remain separate legs; deduplicateBalances subsequently collapses
// only the two static entries belonging to the same source leg.
func orderedEngineValidationOperations(input EngineTranslationInput, explicit []*mmodel.Balance) ([]mmodel.BalanceOperation, error) {
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
				return nil, invalidEngineTranslation("missing validated leg during preparation")
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
