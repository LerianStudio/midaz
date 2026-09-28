// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// ValidateGroupPartAccountingRules validates one ledger part of a cross-ledger
// group leg by leg. It applies every check of ValidateAccountingRules except the
// route count and the bidirectional counterparts: a part carries only its share
// of the group's legs, so those two checks run once over the whole group phase
// (ValidateGroupAccountingRoutes).
//
// The bridge legs, the legs whose operation route is the transaction route's
// crossLedger route, are not client legs: they are left out of the action's
// template and checked by their own rule, which requires the crossLedger rubric
// for the direction posted (credit when value leaves the ledger, debit when it
// arrives).
//
// When the part's ledger does not validate routes this is a no-op and the
// returned cache is nil.
func (uc *UseCase) ValidateGroupPartAccountingRules(ctx context.Context, organizationID, ledgerID uuid.UUID, operations []mmodel.BalanceOperation, validate *mtransaction.Responses, action string) (*mmodel.TransactionRouteCache, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.validate_group_part_accounting_rules")
	defer span.End()

	transactionRouteCache, enabled, err := uc.loadValidatedTransactionRoute(ctx, span, organizationID, ledgerID, validate)
	if err != nil || !enabled {
		return nil, err
	}

	actionRoutes, err := resolveActionRoutes(transactionRouteCache, action)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to resolve action routes", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to resolve action routes", libLog.String("action", action), libLog.Err(err))

		return nil, err
	}

	if err := validateOperationRouteIDs(validate, actionRoutes.isSourceOnly); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing operation route ID", err)
		logger.Log(ctx, libLog.LevelWarn, "Missing operation route ID", libLog.Err(err))

		return nil, err
	}

	clientOperations, bridgeOperations := splitCrossLedgerBridgeOperations(transactionRouteCache, validate, operations)

	validateFrom := validate
	if actionRoutes.isSourceOnly {
		validateFrom = &mtransaction.Responses{
			From:                validate.From,
			OperationRoutesFrom: validate.OperationRoutesFrom,
		}
	}

	if err := validateAccountRules(ctx, actionRoutes.source, actionRoutes.destination, actionRoutes.bidirectional, validateFrom, clientOperations); err != nil {
		return nil, err
	}

	if err := validateOverdraftRoutes(ctx, transactionRouteCache, validate, operations); err != nil {
		return nil, err
	}

	if err := validateCrossLedgerBridgeRoutes(ctx, transactionRouteCache, validate, bridgeOperations); err != nil {
		return nil, err
	}

	return &transactionRouteCache, nil
}

// ValidateGroupAccountingRoutes checks the client legs of every route-validating
// part of one cross-ledger group phase together against that phase's template,
// with the rules ValidateAccountingRules applies to a single transaction:
//
//   - every leg names a route of the template, on a side that route accepts
//     (legs that were not executed, such as the destination parts of a hold, are
//     checked here only);
//   - the distinct routes of all parts together match the template count;
//   - a bidirectional route used on both sides has a debit and a credit
//     somewhere in the group.
//
// Bridge legs are ignored: they are never part of a template. Cancel is
// source-only and, as for a single transaction, has no group-wide rule.
func (uc *UseCase) ValidateGroupAccountingRoutes(ctx context.Context, organizationID uuid.UUID, transactionRoute string, uses []mmodel.AccountingRouteUse, action string) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.validate_group_accounting_routes")
	defer span.End()

	transactionRouteCache, err := uc.loadTransactionRouteCache(ctx, span, organizationID, &mtransaction.Responses{TransactionRouteID: &transactionRoute})
	if err != nil {
		return err
	}

	actionRoutes, err := resolveActionRoutes(transactionRouteCache, action)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to resolve action routes", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to resolve action routes", libLog.String("action", action), libLog.Err(err))

		return err
	}

	if actionRoutes.isSourceOnly {
		return nil
	}

	clientUses := make([]mmodel.AccountingRouteUse, 0, len(uses))

	for _, use := range uses {
		if !isCrossLedgerBridgeRoute(transactionRouteCache, use.RouteID) {
			clientUses = append(clientUses, use)
		}
	}

	if err := validateGroupRouteMembership(actionRoutes, clientUses); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Accounting route not found", err)
		logger.Log(ctx, libLog.LevelWarn, "Group leg route not found in the action's routes", libLog.String("action", action))

		return err
	}

	if err := validateGroupRouteCountAndCounterparts(actionRoutes, clientUses); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Group route count or counterpart validation failed", err)
		logger.Log(ctx, libLog.LevelWarn, "Group route count or counterpart validation failed", libLog.String("action", action), libLog.Err(err))

		return err
	}

	return nil
}

// splitCrossLedgerBridgeOperations separates the bridge operations, those whose
// operation route is under the crossLedger action, from the client operations.
func splitCrossLedgerBridgeOperations(cache mmodel.TransactionRouteCache, validate *mtransaction.Responses, operations []mmodel.BalanceOperation) ([]mmodel.BalanceOperation, []mmodel.BalanceOperation) {
	client := make([]mmodel.BalanceOperation, 0, len(operations))
	bridge := make([]mmodel.BalanceOperation, 0)

	for _, operation := range operations {
		if isCrossLedgerBridgeRoute(cache, operationRouteID(validate, operation)) {
			bridge = append(bridge, operation)
			continue
		}

		client = append(client, operation)
	}

	return client, bridge
}

// validateCrossLedgerBridgeRoutes is the sibling of validateOverdraftRoutes for
// bridge legs: the crossLedger entry of the bridge route must carry the rubric
// for the direction posted, or the leg would post with no classification.
func validateCrossLedgerBridgeRoutes(ctx context.Context, cache mmodel.TransactionRouteCache, validate *mtransaction.Responses, bridgeOperations []mmodel.BalanceOperation) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "usecase.validate_cross_ledger_bridge_routes")
	defer span.End()

	for _, operation := range bridgeOperations {
		routeID := operationRouteID(validate, operation)
		if crossLedgerRubricConfigured(cache, routeID, operation.Amount.Direction) {
			continue
		}

		err := pkg.ValidateBusinessError(constant.ErrCrossLedgerRouteNotConfigured, constant.EntityTransactionRoute)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Cross-ledger route not configured", err)

		logger.Log(ctx, libLog.LevelWarn, "Bridge route lacks the crossLedger rubric for the posted direction",
			libLog.String("route_id", routeID),
			libLog.String("direction", operation.Amount.Direction))

		return err
	}

	return nil
}

// crossLedgerRubricConfigured reports whether the bridge route carries a
// crossLedger rubric with a code for the given posted direction.
func crossLedgerRubricConfigured(cache mmodel.TransactionRouteCache, routeID, direction string) bool {
	route, ok := cache.Actions[constant.ActionCrossLedger].FindRoute(routeID)
	if !ok || route.AccountingEntries == nil || route.AccountingEntries.CrossLedger == nil {
		return false
	}

	var rubric *mmodel.AccountingRubric

	switch strings.ToLower(direction) {
	case constant.DirectionDebit:
		rubric = route.AccountingEntries.CrossLedger.Debit
	case constant.DirectionCredit:
		rubric = route.AccountingEntries.CrossLedger.Credit
	default:
		return false
	}

	return rubric != nil && rubric.Code != ""
}

func isCrossLedgerBridgeRoute(cache mmodel.TransactionRouteCache, routeID string) bool {
	if routeID == "" {
		return false
	}

	_, ok := cache.Actions[constant.ActionCrossLedger].FindRoute(routeID)

	return ok
}

// operationRouteID returns the operation route of the leg an operation belongs
// to, resolving the side as validateAccountRules does.
func operationRouteID(validate *mtransaction.Responses, operation mmodel.BalanceOperation) string {
	if _, ok := validate.From[operation.Alias]; ok {
		return validate.OperationRoutesFrom[operation.Alias]
	}

	return validate.OperationRoutesTo[operation.Alias]
}

func validateGroupRouteMembership(routes actionRoutesResult, uses []mmodel.AccountingRouteUse) error {
	for _, use := range uses {
		sideRoutes := routes.destination
		if use.Source {
			sideRoutes = routes.source
		}

		_, found := sideRoutes[use.RouteID]
		if !found {
			_, found = routes.bidirectional[use.RouteID]
		}

		if use.RouteID == "" || !found {
			return pkg.ValidateBusinessError(constant.ErrAccountingRouteNotFound, constant.EntityOperationRoute, use.RouteID, use.Alias)
		}
	}

	return nil
}

// validateGroupRouteCountAndCounterparts is validateRouteCountAndCounterparts
// over the legs of several transactions: routes are counted once per side
// however many parts use them, and counterparts are looked for across parts.
func validateGroupRouteCountAndCounterparts(routes actionRoutesResult, uses []mmodel.AccountingRouteUse) error {
	fromRoutes := make(map[string]bool)
	toRoutes := make(map[string]bool)

	for _, use := range uses {
		if use.Source {
			fromRoutes[use.RouteID] = true
		} else {
			toRoutes[use.RouteID] = true
		}
	}

	sharedBidirectionalRoutes := make(map[string]bool)

	for routeID := range fromRoutes {
		if _, isBidirectional := routes.bidirectional[routeID]; isBidirectional && toRoutes[routeID] {
			sharedBidirectionalRoutes[routeID] = true
		}
	}

	sourceCount := len(routes.source)
	destinationCount := len(routes.destination)
	bidirectionalCount := len(routes.bidirectional)

	totalCacheRoutes := sourceCount + destinationCount + bidirectionalCount
	totalUsedRoutes := len(fromRoutes) + len(toRoutes) - len(sharedBidirectionalRoutes)

	if totalUsedRoutes != totalCacheRoutes || len(fromRoutes) < sourceCount || len(toRoutes) < destinationCount {
		return pkg.ValidateBusinessError(constant.ErrAccountingRouteCountMismatch, constant.EntityTransactionRoute, len(fromRoutes), len(toRoutes), sourceCount, destinationCount, bidirectionalCount)
	}

	debited := make(map[string]bool)
	credited := make(map[string]bool)

	for _, use := range uses {
		if !sharedBidirectionalRoutes[use.RouteID] {
			continue
		}

		switch strings.ToLower(use.Direction) {
		case constant.DirectionDebit:
			debited[use.RouteID] = true
		case constant.DirectionCredit:
			credited[use.RouteID] = true
		}
	}

	for routeID := range sharedBidirectionalRoutes {
		if !debited[routeID] || !credited[routeID] {
			return pkg.ValidateBusinessError(constant.ErrMissingCounterpart, constant.EntityOperationRoute, routeID)
		}
	}

	return nil
}
