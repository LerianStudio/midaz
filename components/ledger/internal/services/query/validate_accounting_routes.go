// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"slices"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
)

// ValidateAccountingRules validates the accounting rules for the given operations.
// It is the central gate that enforces route correctness for every transaction type.
//
// When validateRoutes is disabled in ledger settings, this method is a no-op.
// When enabled, every transaction must specify a transaction route, and each
// operation must reference a valid operation route within that transaction route.
//
// # Validation matrix by transaction type
//
// The "action" parameter determines which route entries are used and what gets validated:
//
//	Type        | Status   | Action | Source validates against    | Destination validates against | Notes
//	------------|----------|--------|----------------------------|-------------------------------|-------------------------------
//	Direct      | CREATED  | direct | direct source routes       | direct destination routes     | Full validation both sides
//	Pending     | PENDING  | hold   | hold source routes         | commit destination routes     | Dest validated at creation time using commit routes
//	Commit      | APPROVED | commit | commit source routes       | commit destination routes     | Confirms the pending transaction
//	Cancel      | CANCELED | cancel | cancel source routes       | (skipped)                     | Source-only: releases held funds
//	Revert      | CREATED  | revert | revert source routes       | revert destination routes     | Pre-validated: handler checks routes are bidirectional
//
// # What is validated (when routes are active)
//
//   - Transaction route exists and is a valid UUID
//   - Each operation has a non-empty operation route ID
//   - Operation route count matches the transaction route configuration
//   - Bidirectional routes have both debit and credit counterparts
//   - Operation direction matches the route type (source=debit, destination=credit)
//   - Account rules (alias or account_type) match the operation route definition
//
// # Cancel special handling
//
// For cancel actions, only source-side operations are validated. The destination
// never participates in a cancel (it only releases held funds on the source).
// Operation route IDs for the To side are not checked.
//
// # Revert special handling
//
// Reverts pass through this method with action "revert", which looks up revert-specific
// route entries. Before reaching this point, the handler (RevertTransaction) pre-validates
// that all operation routes are bidirectional — a requirement for reversals.
func (uc *UseCase) ValidateAccountingRules(ctx context.Context, organizationID, ledgerID uuid.UUID, operations []mmodel.BalanceOperation, validate *pkgTransaction.Responses, action string) (*mmodel.TransactionRouteCache, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.validate_accounting_rules")
	defer span.End()

	ledgerSettings, err := uc.GetParsedLedgerSettings(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger settings", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get ledger settings", libLog.Err(err))

		return nil, err
	}

	if !ledgerSettings.Accounting.ValidateRoutes {
		logger.Log(ctx, libLog.LevelDebug, "Route validation disabled, skipping accounting rules validation", libLog.String("ledger_id", ledgerID.String()))

		return nil, nil
	}

	logger.Log(ctx, libLog.LevelDebug, "Route validation enabled, validating accounting rules", libLog.String("ledger_id", ledgerID.String()))

	transactionRouteID, err := resolveTransactionRouteID(validate)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to resolve transaction route ID", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to resolve transaction route ID", libLog.Err(err))

		return nil, err
	}

	transactionRouteCache, err := uc.GetOrCreateTransactionRouteCache(ctx, organizationID, ledgerID, transactionRouteID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to load transaction route cache", err)

		logger.Log(ctx, libLog.LevelError, "Failed to load transaction route cache", libLog.Err(err))

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

	// Validate route count and bidirectional counterparts (skipped for source-only actions).
	if !actionRoutes.isSourceOnly {
		if err := validateRouteCountAndCounterparts(validate, actionRoutes, operations); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Route count or counterpart validation failed", err)
			logger.Log(ctx, libLog.LevelWarn, "Route count or counterpart validation failed", libLog.Err(err))

			return nil, err
		}
	}

	// For source-only actions (cancel), only validate source-side operations.
	// The destination doesn't participate in cancel (release is source-only).
	validateFrom := validate
	if actionRoutes.isSourceOnly {
		validateFrom = &pkgTransaction.Responses{
			From:                validate.From,
			OperationRoutesFrom: validate.OperationRoutesFrom,
		}
	}

	err = validateAccountRules(ctx, actionRoutes.source, actionRoutes.destination, actionRoutes.bidirectional, validateFrom, operations)
	if err != nil {
		return nil, err
	}

	return &transactionRouteCache, nil
}

// validateRouteCountAndCounterparts verifies that the number of unique operation
// routes used matches the expected count from the route cache, and that every
// bidirectional route shared between from and to sides has both a debit and a
// credit counterpart.
func validateRouteCountAndCounterparts(validate *pkgTransaction.Responses, routes actionRoutesResult, operations []mmodel.BalanceOperation) error {
	uniqueFromCount := uniqueValues(validate.OperationRoutesFrom)
	uniqueToCount := uniqueValues(validate.OperationRoutesTo)

	sourceCount := len(routes.source)
	destinationCount := len(routes.destination)
	bidirectionalCount := len(routes.bidirectional)

	// Identify bidirectional routes that appear on both from and to sides.
	// These are counted once in uniqueFrom and once in uniqueTo, so we must
	// subtract the shared count to avoid double-counting.
	bidirectionalFromRoutes := make(map[string]bool)

	for _, routeID := range validate.OperationRoutesFrom {
		if _, isBidirectional := routes.bidirectional[routeID]; isBidirectional {
			bidirectionalFromRoutes[routeID] = true
		}
	}

	sharedBidirectionalRoutes := make(map[string]bool)

	for _, routeID := range validate.OperationRoutesTo {
		if bidirectionalFromRoutes[routeID] {
			sharedBidirectionalRoutes[routeID] = true
		}
	}

	totalCacheRoutes := sourceCount + destinationCount + bidirectionalCount
	totalUsedRoutes := uniqueFromCount + uniqueToCount - len(sharedBidirectionalRoutes)

	if totalUsedRoutes != totalCacheRoutes || uniqueFromCount < sourceCount || uniqueToCount < destinationCount {
		return pkg.ValidateBusinessError(constant.ErrAccountingRouteCountMismatch, constant.EntityTransactionRoute, uniqueFromCount, uniqueToCount, sourceCount, destinationCount, bidirectionalCount)
	}

	// Validate that shared bidirectional routes have both debit and credit counterparts.
	mergedRouteMap := make(map[string]string)

	for alias, routeID := range validate.OperationRoutesFrom {
		if sharedBidirectionalRoutes[routeID] {
			mergedRouteMap[alias] = routeID
		}
	}

	for alias, routeID := range validate.OperationRoutesTo {
		if sharedBidirectionalRoutes[routeID] {
			mergedRouteMap[alias] = routeID
		}
	}

	if len(mergedRouteMap) > 0 {
		return validateCounterparts(operations, mergedRouteMap)
	}

	return nil
}

// validateOperationRouteIDs checks that every operation entry has a non-empty
// route ID. For source-only actions (e.g. cancel), only From routes are checked
// since the destination side does not participate.
func validateOperationRouteIDs(validate *pkgTransaction.Responses, isSourceOnly bool) error {
	for alias, routeID := range validate.OperationRoutesFrom {
		if routeID == "" {
			return pkg.ValidateBusinessError(constant.ErrAccountingRouteNotFound, constant.EntityOperationRoute, routeID, alias)
		}
	}

	if isSourceOnly {
		return nil
	}

	for alias, routeID := range validate.OperationRoutesTo {
		if routeID == "" {
			return pkg.ValidateBusinessError(constant.ErrAccountingRouteNotFound, constant.EntityOperationRoute, routeID, alias)
		}
	}

	return nil
}

// actionRoutesResult holds the resolved route maps for a given action,
// including whether the action only involves the source side (e.g. cancel).
type actionRoutesResult struct {
	source        map[string]mmodel.OperationRouteCache
	destination   map[string]mmodel.OperationRouteCache
	bidirectional map[string]mmodel.OperationRouteCache
	isSourceOnly  bool
}

// resolveActionRoutes looks up the route maps for the given action in the
// transaction route cache. For "hold" actions, destination routes come from
// the "commit" action. For "cancel", only source routes apply.
func resolveActionRoutes(cache mmodel.TransactionRouteCache, action string) (actionRoutesResult, error) {
	actionCache, found := cache.Actions[action]
	if !found {
		return actionRoutesResult{}, pkg.ValidateBusinessError(constant.ErrNoRoutesForAction, constant.EntityTransactionRoute, action)
	}

	result := actionRoutesResult{
		source:        actionCache.Source,
		destination:   actionCache.Destination,
		bidirectional: actionCache.Bidirectional,
		isSourceOnly:  action == constant.ActionCancel,
	}

	// For "hold" (pending) transactions, the destination only participates at
	// confirmation time — look up destination routes from "commit" instead.
	if action == constant.ActionHold {
		if commitCache, ok := cache.Actions[constant.ActionCommit]; ok {
			result.destination = commitCache.Destination
		}
	}

	return result, nil
}

// resolveTransactionRouteID extracts the transaction route UUID from the
// validated response. It prefers the new routeId field and falls back to the
// deprecated route string. Returns an error if neither is set or the value
// is not a valid UUID.
func resolveTransactionRouteID(validate *pkgTransaction.Responses) (uuid.UUID, error) {
	if !libCommons.IsNilOrEmpty(validate.TransactionRouteID) {
		id, err := uuid.Parse(*validate.TransactionRouteID)
		if err != nil {
			return uuid.Nil, pkg.ValidateBusinessError(constant.ErrInvalidTransactionRouteID, "")
		}

		return id, nil
	}

	if !libCommons.IsNilOrEmpty(&validate.TransactionRoute) {
		id, err := uuid.Parse(validate.TransactionRoute)
		if err != nil {
			return uuid.Nil, pkg.ValidateBusinessError(constant.ErrInvalidTransactionRouteID, "")
		}

		return id, nil
	}

	return uuid.Nil, pkg.ValidateBusinessError(constant.ErrTransactionRouteNotInformed, "")
}

// validateAccountRules validates each operation against its corresponding route rule.
// Route existence and direction matching are always enforced when validateRoutes is active.
// Account type checks (alias/account_type rules) are only enforced when validateAccountType is also active.
func validateAccountRules(ctx context.Context, sourceRoutes, destinationRoutes, bidirectionalRoutes map[string]mmodel.OperationRouteCache, validate *pkgTransaction.Responses, operations []mmodel.BalanceOperation) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "usecase.validate_account_rules")
	defer span.End()

	for _, operation := range operations {
		// Get route ID and determine if operation is source or destination
		var routeID string

		var isSource bool

		if _, exists := validate.From[operation.Alias]; exists {
			routeID = validate.OperationRoutesFrom[operation.Alias]
			isSource = true
		} else if _, existsTo := validate.To[operation.Alias]; existsTo {
			routeID = validate.OperationRoutesTo[operation.Alias]
			isSource = false
		} else {
			continue
		}

		var cacheRule mmodel.OperationRouteCache

		var found bool

		if isSource {
			cacheRule, found = sourceRoutes[routeID]
		} else {
			cacheRule, found = destinationRoutes[routeID]
		}

		if !found {
			cacheRule, found = bidirectionalRoutes[routeID]
		}

		if !found {
			err := pkg.ValidateBusinessError(constant.ErrAccountingRouteNotFound, constant.EntityOperationRoute, routeID, operation.Alias)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Accounting route not found", err)

			logger.Log(ctx, libLog.LevelWarn, "Route ID not found in cache for operation",
				libLog.String("route_id", routeID), libLog.String("alias", operation.Alias))

			return err
		}

		if err := validateDirectionRouteMatch(operation, cacheRule); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Direction does not match route operation type", err)

			logger.Log(ctx, libLog.LevelWarn, "Operation direction does not match route operation type",
				libLog.String("alias", operation.Alias),
				libLog.String("direction", operation.Amount.Direction),
				libLog.String("route_type", cacheRule.OperationType))

			return err
		}

		// Account rules (alias, account_type) are always enforced when route
		// validation is active. The validateAccountType flag controls account
		// creation/update validation only, not the transactional route rules.
		if cacheRule.Account != nil {
			if err := validateSingleOperationRule(operation, cacheRule.Account); err != nil {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Operation failed validation against route rules", err)

				logger.Log(ctx, libLog.LevelWarn, "Operation failed validation against route rules",
					libLog.String("alias", operation.Alias), libLog.Err(err))

				return err
			}
		}
	}

	return nil
}

// validateSingleOperationRule validates if an operation matches the account rule defined in the transaction route
func validateSingleOperationRule(op mmodel.BalanceOperation, account *mmodel.AccountCache) error {
	switch account.RuleType {
	case constant.AccountRuleTypeAlias:
		expected, ok := account.ValidIf.(string)
		if !ok {
			return pkg.ValidateBusinessError(constant.ErrCorruptedAccountRule, constant.EntityAccountRule)
		}

		alias := pkgTransaction.SplitAlias(op.Alias)

		if alias != expected {
			return pkg.ValidateBusinessError(
				constant.ErrAccountingAliasValidationFailed,
				constant.EntityAccountRule,
				alias,
				expected,
			)
		}

	case constant.AccountRuleTypeAccountType:
		allowedTypes := extractStringSlice(account.ValidIf)
		if allowedTypes == nil {
			return pkg.ValidateBusinessError(constant.ErrCorruptedAccountRule, constant.EntityAccountRule)
		}

		if slices.Contains(allowedTypes, op.Balance.AccountType) {
			return nil
		}

		return pkg.ValidateBusinessError(
			constant.ErrAccountingAccountTypeValidationFailed,
			constant.EntityAccountRule,
			op.Balance.AccountType,
			allowedTypes,
		)

	default:
		return pkg.ValidateBusinessError(constant.ErrCorruptedAccountRule, constant.EntityAccountRule)
	}

	return nil
}

// uniqueValues counts the number of unique values in a map
func uniqueValues(m map[string]string) int {
	if len(m) == 0 {
		return 0
	}

	if len(m) == 1 {
		return 1
	}

	seen := make(map[string]struct{}, len(m))
	for _, value := range m {
		seen[value] = struct{}{}
	}

	return len(seen)
}

// validateDirectionRouteMatch validates that an operation's direction is compatible with the route's operation type.
// Source routes only accept debit, destination routes only accept credit, bidirectional routes accept both.
func validateDirectionRouteMatch(operation mmodel.BalanceOperation, routeCache mmodel.OperationRouteCache) error {
	// Double-entry split operations use ON_HOLD, RELEASE, and reversal CREDIT/DEBIT
	// that intentionally cross the normal direction-route mapping. Skip validation
	// for these:
	//   - ON_HOLD: pending hold on source (credit direction on source)
	//   - RELEASE: cancel release on source (debit direction, already OK)
	//   - CREDIT during CANCELED: restores available balance on source
	//   - DEBIT during APPROVED commit: decrements onHold on source
	opAmount := strings.ToUpper(operation.Amount.Operation)
	txType := strings.ToUpper(operation.Amount.TransactionType)

	if opAmount == constant.ONHOLD || opAmount == constant.RELEASE {
		return nil
	}

	// Cancel produces RELEASE + CREDIT on the source. The CREDIT restores
	// the available balance and is a reversal, not a regular credit.
	if txType == constant.CANCELED && operation.Amount.RouteValidationEnabled {
		return nil
	}

	direction := strings.ToLower(operation.Amount.Direction)
	opType := strings.ToLower(routeCache.OperationType)

	switch opType {
	case "source":
		if direction != constant.DirectionDebit {
			return pkg.ValidateBusinessError(
				constant.ErrDirectionRouteMismatch,
				constant.EntityOperationRoute,
				operation.Amount.Direction,
				routeCache.OperationType,
				operation.Alias,
			)
		}
	case "destination":
		if direction != constant.DirectionCredit {
			return pkg.ValidateBusinessError(
				constant.ErrDirectionRouteMismatch,
				constant.EntityOperationRoute,
				operation.Amount.Direction,
				routeCache.OperationType,
				operation.Alias,
			)
		}
	case "bidirectional":
		// Accepts both debit and credit
	default:
		return pkg.ValidateBusinessError(
			constant.ErrInvalidOperationRouteType,
			constant.EntityOperationRoute,
		)
	}

	return nil
}

// validateCounterparts validates that each route has at least one debit and one credit operation.
// The routeMap maps operation alias to route ID.
func validateCounterparts(operations []mmodel.BalanceOperation, routeMap map[string]string) error {
	type directionFlags struct {
		hasDebit  bool
		hasCredit bool
	}

	routeDirections := make(map[string]*directionFlags)

	for _, op := range operations {
		routeID, exists := routeMap[op.Alias]
		if !exists {
			continue
		}

		if _, ok := routeDirections[routeID]; !ok {
			routeDirections[routeID] = &directionFlags{}
		}

		direction := strings.ToLower(op.Amount.Direction)

		switch direction {
		case constant.DirectionDebit:
			routeDirections[routeID].hasDebit = true
		case constant.DirectionCredit:
			routeDirections[routeID].hasCredit = true
		}
	}

	for routeID, flags := range routeDirections {
		if !flags.hasDebit || !flags.hasCredit {
			return pkg.ValidateBusinessError(
				constant.ErrMissingCounterpart,
				constant.EntityOperationRoute,
				routeID,
			)
		}
	}

	return nil
}

// extractStringSlice helper function to handle []string and []any conversion
func extractStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, len(v))

		for i, item := range v {
			if str, ok := item.(string); ok {
				result[i] = str
			} else {
				return nil
			}
		}

		return result
	}

	return nil
}
