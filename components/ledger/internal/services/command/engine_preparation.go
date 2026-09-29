// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"slices"

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

	debtRefs, err := feeDebtPreparationRefs(input)
	if err != nil {
		return enginePreparedTransaction{}, err
	}

	pool, err := loadPreparedEngineSnapshots(ctx, uc.TransactionReader, input.organizationID, input.ledgerID, enginePreparationAliases(input), debtRefs)
	if err != nil {
		return enginePreparedTransaction{}, err
	}

	return uc.prepareEngineTransactionWithPool(ctx, input, pool)
}

// prepareEngineTransactionWithPool applies one transaction's isolated route
// validation and translation against an already validated shared snapshot
// inventory. Batch callers can therefore reuse one primary-routed read without
// exposing unrelated balances to the item's explicit-target checks.
func (uc *UseCase) prepareEngineTransactionWithPool(
	ctx context.Context,
	input enginePreparationInput,
	pool EngineSnapshotPool,
) (enginePreparedTransaction, error) {
	ctx, itemPool, operations, err := uc.engineValidationOperations(ctx, input, pool)
	if err != nil {
		return enginePreparedTransaction{}, err
	}

	validate, err := feeDebtRouteView(&input.translation)
	if err != nil {
		return enginePreparedTransaction{}, err
	}

	routeCache, err := uc.TransactionReader.ValidateAccountingRules(ctx, input.organizationID, input.ledgerID, operations, validate, input.translation.Action)
	if err != nil {
		return enginePreparedTransaction{}, err
	}

	return translatePreparedEngineTransaction(ctx, input, itemPool, routeCache)
}

// engineValidationOperations selects the item's slice of the shared snapshot
// inventory and builds the balance operations route validation reads.
func (uc *UseCase) engineValidationOperations(
	ctx context.Context,
	input enginePreparationInput,
	pool EngineSnapshotPool,
) (context.Context, EngineSnapshotPool, []mmodel.BalanceOperation, error) {
	if err := ctx.Err(); err != nil {
		return ctx, EngineSnapshotPool{}, nil, err
	}

	if uc == nil || uc.TransactionReader == nil || input.translation.Validate == nil {
		return ctx, EngineSnapshotPool{}, nil, invalidEngineTranslation("preparation requires a reader and validated intent")
	}

	ctx = readrouting.WithPrimaryRead(ctx)

	itemPool, err := selectEnginePreparationPool(input, pool)
	if err != nil {
		return ctx, EngineSnapshotPool{}, nil, err
	}

	if err := rejectInternalScopeBalances(ctx, itemPool.ExplicitBalances); err != nil {
		return ctx, EngineSnapshotPool{}, nil, err
	}

	operations, err := orderedEngineValidationOperations(input.translation, itemPool.ExplicitBalances)
	if err != nil {
		return ctx, EngineSnapshotPool{}, nil, err
	}

	if err := ctx.Err(); err != nil {
		return ctx, EngineSnapshotPool{}, nil, err
	}

	return ctx, itemPool, operations, nil
}

// translatePreparedEngineTransaction translates a route-validated item into
// engine postings and its operation projection.
func translatePreparedEngineTransaction(
	ctx context.Context,
	input enginePreparationInput,
	itemPool EngineSnapshotPool,
	routeCache *mmodel.TransactionRouteCache,
) (enginePreparedTransaction, error) {
	if err := ctx.Err(); err != nil {
		return enginePreparedTransaction{}, err
	}

	input.translation.Balances = itemPool.Balances
	input.translation.RouteCache = routeCache
	input.translation.FeeDebtSeeds = itemPool.FeeDebtSeeds

	translated, projection, err := TranslateEngineTransaction(input.translation)
	if err != nil {
		return enginePreparedTransaction{}, err
	}

	translated.OrganizationID = input.organizationID
	translated.LedgerID = input.ledgerID

	return enginePreparedTransaction{pool: itemPool, transaction: translated, projection: projection}, nil
}

func enginePreparationAliases(input enginePreparationInput) []string {
	// Cancellation restores sources only; destination state must not introduce
	// an eligibility requirement after the original hold was accepted.
	if input.translation.Action == constant.ActionCancel {
		aliases := make([]string, 0, len(input.translation.TransactionInput.Send.Source.From))
		for _, leg := range input.translation.TransactionInput.Send.Source.From {
			aliases = append(aliases, mtransaction.SplitAliasWithKey(leg.AccountAlias))
		}

		return aliases
	}

	return input.translation.Validate.Aliases
}

func selectEnginePreparationPool(input enginePreparationInput, shared EngineSnapshotPool) (EngineSnapshotPool, error) {
	aliases := enginePreparationAliases(input)

	requested := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		requested[alias] = struct{}{}
	}

	explicit := make([]*mmodel.Balance, 0, len(requested))

	for _, balance := range shared.ExplicitBalances {
		snapshot, err := balanceToEngineSnapshot(input.organizationID, input.ledgerID, balance)
		if err != nil {
			return EngineSnapshotPool{}, err
		}

		if _, ok := requested[snapshot.BalanceRef]; ok {
			explicit = append(explicit, balance)
		}
	}

	return EngineSnapshotPool{
		ExplicitBalances: explicit,
		Balances:         shared.Balances,
		Snapshots:        shared.Snapshots,
		FeeDebtSeeds:     shared.FeeDebtSeeds,
	}, nil
}

// loadPreparedEngineSnapshots loads the pool of aliases plus what fee-debt
// composition needs: the seeds of debt.debtors in one read, the balances debt
// names and every creditor a seed names. Without debt refs it loads aliases only.
func loadPreparedEngineSnapshots(ctx context.Context, reader TransactionReader, organizationID, ledgerID uuid.UUID, aliases []string, debt feeDebtPoolRefs) (EngineSnapshotPool, error) {
	var seeds map[string][]accounting.FeeDebtItem

	if len(debt.debtors) > 0 {
		var err error

		seeds, err = reader.GetFeeDebtSeeds(ctx, organizationID, ledgerID, debt.debtors)
		if err != nil {
			return EngineSnapshotPool{}, fmt.Errorf("load fee debt seeds: %w", err)
		}
	}

	aliases = appendMissingRefs(slices.Clip(aliases), debt.balances)
	for _, debtor := range debt.debtors {
		for _, item := range seeds[debtor] {
			aliases = appendMissingRefs(aliases, []string{item.CreditRef})
		}
	}

	explicitBalances, executionBalances, err := reader.GetEngineBalances(ctx, organizationID, ledgerID, aliases)
	if err != nil {
		return EngineSnapshotPool{}, fmt.Errorf("load engine balances: %w", err)
	}

	pool, err := BuildEngineSnapshotPool(ctx, organizationID, ledgerID, aliases, explicitBalances, executionBalances)
	if err != nil {
		return EngineSnapshotPool{}, err
	}

	pool.FeeDebtSeeds = seeds

	return pool, nil
}

// feeDebtPoolRefs names what fee-debt composition reads beyond a transaction's
// legs: the debtors whose seeds a collect needs, and the balances a revert's
// refunds and reopens touch.
type feeDebtPoolRefs struct {
	debtors  []string
	balances []string
}

func feeDebtPreparationRefs(input enginePreparationInput) (feeDebtPoolRefs, error) {
	var refs feeDebtPoolRefs

	translation := input.translation

	switch translation.Action {
	case constant.ActionDirect, constant.ActionCommit, constant.ActionRevert:
		if translation.FeeDebtEligible {
			for _, leg := range translation.TransactionInput.Send.Distribute.To {
				refs.debtors = appendMissingRefs(refs.debtors, []string{mtransaction.SplitAliasWithKey(leg.AccountAlias)})
			}
		}
	}

	if translation.Action != constant.ActionRevert {
		return refs, nil
	}

	openings, settlements, err := feeDebtRevertFacts(translation.TransactionInput.Metadata)
	if err != nil {
		return feeDebtPoolRefs{}, err
	}

	for _, opening := range openings {
		refs.balances = appendMissingRefs(refs.balances, []string{opening.DebtorRef, opening.CreditRef})
	}

	for _, settlement := range settlements {
		refs.balances = appendMissingRefs(refs.balances, []string{settlement.DebtorRef})
	}

	return refs, nil
}

// appendMissingRefs appends each ref not already in refs, keeping order.
func appendMissingRefs(refs, more []string) []string {
	for _, ref := range more {
		if !slices.Contains(refs, ref) {
			refs = append(refs, ref)
		}
	}

	return refs
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
