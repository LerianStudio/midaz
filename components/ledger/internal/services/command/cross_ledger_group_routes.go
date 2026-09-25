// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// GroupAccountingRouteValidator is the cross-ledger extension implemented by the
// production query use case. It stays separate from TransactionReader so the
// singular transaction readers and their test doubles do not gain it.
//
// A cross-ledger group spreads one request's legs over one transaction per
// ledger, so accounting routes are validated in two steps: each part leg by leg,
// then the client legs of every part of the phase together against the phase's
// template.
type GroupAccountingRouteValidator interface {
	// ValidateGroupPartAccountingRules validates one part leg by leg, without
	// the route count and counterparts, and checks its bridge legs against the
	// crossLedger route. It returns nil when the part's ledger does not validate
	// routes.
	ValidateGroupPartAccountingRules(ctx context.Context, organizationID, ledgerID uuid.UUID, operations []mmodel.BalanceOperation, validate *mtransaction.Responses, action string) (*mmodel.TransactionRouteCache, error)

	// ValidateGroupAccountingRoutes checks the client legs of the validating
	// parts of one phase together against the phase's template: routes, count
	// and bidirectional counterparts.
	ValidateGroupAccountingRoutes(ctx context.Context, organizationID uuid.UUID, transactionRoute string, uses []mmodel.AccountingRouteUse, action string) error
}

// crossLedgerGroupRoutePart is one part's contribution to the route check of its
// group phase. Routes belong to the organization, so a part whose ledger does
// not validate routes still contributes the client legs that name one; only a
// validated part carries the organization and transaction route the phase is
// checked against.
type crossLedgerGroupRoutePart struct {
	validated        bool
	organizationID   uuid.UUID
	transactionRoute string
	uses             []mmodel.AccountingRouteUse
}

// prepareCrossLedgerGroupPart loads one executed part's snapshots and prepares
// it as prepareCrossLedgerGroupPartWithPool does.
func (uc *UseCase) prepareCrossLedgerGroupPart(
	ctx context.Context,
	input enginePreparationInput,
	validatesRoutes bool,
) (enginePreparedTransaction, crossLedgerGroupRoutePart, error) {
	if err := ctx.Err(); err != nil {
		return enginePreparedTransaction{}, crossLedgerGroupRoutePart{}, err
	}

	if uc == nil || uc.TransactionReader == nil || input.translation.Validate == nil {
		return enginePreparedTransaction{}, crossLedgerGroupRoutePart{}, invalidEngineTranslation("preparation requires a reader and validated intent")
	}

	readCtx := readrouting.WithPrimaryRead(ctx)

	pool, err := loadPreparedEngineSnapshots(readCtx, uc.TransactionReader, input.organizationID, input.ledgerID, enginePreparationAliases(input))
	if err != nil {
		return enginePreparedTransaction{}, crossLedgerGroupRoutePart{}, err
	}

	return uc.prepareCrossLedgerGroupPartWithPool(readCtx, input, validatesRoutes, pool)
}

// prepareCrossLedgerGroupPartWithPool is prepareEngineTransactionWithPool for one
// executed part of a cross-ledger group: the part is validated leg by leg
// against the template of its route action, and its client legs are returned for
// the check that spans the whole phase.
func (uc *UseCase) prepareCrossLedgerGroupPartWithPool(
	ctx context.Context,
	input enginePreparationInput,
	validatesRoutes bool,
	pool EngineSnapshotPool,
) (enginePreparedTransaction, crossLedgerGroupRoutePart, error) {
	ctx, itemPool, operations, err := uc.engineValidationOperations(ctx, input, pool)
	if err != nil {
		return enginePreparedTransaction{}, crossLedgerGroupRoutePart{}, err
	}

	var routeCache *mmodel.TransactionRouteCache

	routes := crossLedgerGroupRoutePart{uses: namedAccountingRouteUses(executedAccountingRouteUses(operations, input.translation.Validate))}

	if validatesRoutes {
		validator, err := uc.groupAccountingRouteValidator()
		if err != nil {
			return enginePreparedTransaction{}, crossLedgerGroupRoutePart{}, err
		}

		routeCache, err = validator.ValidateGroupPartAccountingRules(ctx, input.organizationID, input.ledgerID, operations, input.translation.Validate, input.translation.routeAction())
		if err != nil {
			return enginePreparedTransaction{}, crossLedgerGroupRoutePart{}, err
		}

		if routeCache != nil {
			routes = crossLedgerGroupRoutePart{
				validated:        true,
				organizationID:   input.organizationID,
				transactionRoute: crossLedgerTransactionRoute(input.translation.TransactionInput),
				uses:             executedAccountingRouteUses(operations, input.translation.Validate),
			}
		}
	}

	prepared, err := translatePreparedEngineTransaction(ctx, input, itemPool, routeCache)
	if err != nil {
		return enginePreparedTransaction{}, crossLedgerGroupRoutePart{}, err
	}

	return prepared, routes, nil
}

// heldDestinationRouteParts returns the route contribution of the destination
// parts a cross-ledger hold defers to its commit. They are not executed at hold,
// so their client legs count by the route IDs persisted in the intent only;
// account rules, which need balances, are checked when the commit creates them.
// A part in a ledger that does not validate routes contributes its named routes.
func (uc *UseCase) heldDestinationRouteParts(ctx context.Context, parts []CrossLedgerGroupIntentPart) ([]crossLedgerGroupRoutePart, error) {
	routes := make([]crossLedgerGroupRoutePart, 0, len(parts))
	validatesByRef := make(map[atomicTransactionBatchLedgerRef]bool, len(parts))

	for index := range parts {
		part := parts[index]
		ref := atomicTransactionBatchLedgerRef{organizationID: part.OrganizationID, ledgerID: part.LedgerID}

		validates, known := validatesByRef[ref]
		if !known {
			settings, err := uc.TransactionReader.GetParsedLedgerSettings(ctx, part.OrganizationID, part.LedgerID)
			if err != nil {
				return nil, fmt.Errorf("get cross-ledger held destination settings: %w", err)
			}

			validates = settings.Accounting.ValidateRoutes
			validatesByRef[ref] = validates
		}

		if !validates {
			routes = append(routes, crossLedgerGroupRoutePart{uses: namedAccountingRouteUses(intentAccountingRouteUses(part.Transaction))})

			continue
		}

		routes = append(routes, crossLedgerGroupRoutePart{
			validated:        true,
			organizationID:   part.OrganizationID,
			transactionRoute: crossLedgerTransactionRoute(part.Transaction),
			uses:             intentAccountingRouteUses(part.Transaction),
		})
	}

	return routes, nil
}

// validateCrossLedgerGroupRoutes runs the group-wide route check of one phase
// over the client legs every part contributes. A group with no part in a ledger
// that validates routes has nothing to check. The parts share the request's
// transaction route, which belongs to the group's one organization.
func (uc *UseCase) validateCrossLedgerGroupRoutes(ctx context.Context, phase string, parts []crossLedgerGroupRoutePart) error {
	var (
		first *crossLedgerGroupRoutePart
		uses  []mmodel.AccountingRouteUse
	)

	for index := range parts {
		if parts[index].validated && first == nil {
			first = &parts[index]
		}

		uses = append(uses, parts[index].uses...)
	}

	if first == nil {
		return nil
	}

	validator, err := uc.groupAccountingRouteValidator()
	if err != nil {
		return err
	}

	return validator.ValidateGroupAccountingRoutes(ctx, first.organizationID, first.transactionRoute, uses, phase)
}

func (uc *UseCase) groupAccountingRouteValidator() (GroupAccountingRouteValidator, error) {
	validator, ok := uc.TransactionReader.(GroupAccountingRouteValidator)
	if !ok {
		return nil, errors.New("cross-ledger group accounting route validator is not configured")
	}

	return validator, nil
}

// executedAccountingRouteUses lists the route of every validated operation of
// an executed part, attributing each to its leg's side as route validation does.
func executedAccountingRouteUses(operations []mmodel.BalanceOperation, validate *mtransaction.Responses) []mmodel.AccountingRouteUse {
	uses := make([]mmodel.AccountingRouteUse, 0, len(operations))

	for _, operation := range operations {
		if _, source := validate.From[operation.Alias]; source {
			uses = append(uses, mmodel.AccountingRouteUse{
				Alias: operation.Alias, RouteID: validate.OperationRoutesFrom[operation.Alias], Source: true, Direction: operation.Amount.Direction,
			})

			continue
		}

		if _, destination := validate.To[operation.Alias]; destination {
			uses = append(uses, mmodel.AccountingRouteUse{
				Alias: operation.Alias, RouteID: validate.OperationRoutesTo[operation.Alias], Source: false, Direction: operation.Amount.Direction,
			})
		}
	}

	return uses
}

// intentAccountingRouteUses lists the route of every leg of a part that has not
// been executed: sources are debits and destinations credits.
func intentAccountingRouteUses(transaction mtransaction.Transaction) []mmodel.AccountingRouteUse {
	uses := make([]mmodel.AccountingRouteUse, 0, len(transaction.Send.Source.From)+len(transaction.Send.Distribute.To))

	for _, leg := range transaction.Send.Source.From {
		uses = append(uses, mmodel.AccountingRouteUse{
			Alias: leg.AccountAlias, RouteID: legRouteID(leg), Source: true, Direction: constant.DirectionDebit,
		})
	}

	for _, leg := range transaction.Send.Distribute.To {
		uses = append(uses, mmodel.AccountingRouteUse{
			Alias: leg.AccountAlias, RouteID: legRouteID(leg), Source: false, Direction: constant.DirectionCredit,
		})
	}

	return uses
}

// namedAccountingRouteUses keeps the legs that name a route: in a ledger that
// does not validate routes, a leg without one is not checked and counts for
// nothing.
func namedAccountingRouteUses(uses []mmodel.AccountingRouteUse) []mmodel.AccountingRouteUse {
	named := make([]mmodel.AccountingRouteUse, 0, len(uses))

	for _, use := range uses {
		if use.RouteID != "" {
			named = append(named, use)
		}
	}

	return named
}

func legRouteID(leg mtransaction.FromTo) string {
	if leg.RouteID != nil {
		return *leg.RouteID
	}

	return ""
}
