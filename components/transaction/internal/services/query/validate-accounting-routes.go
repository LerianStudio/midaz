// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"

	// ValidateAccountingRules validates the accounting rules for the given operations.
	// Validation is controlled by ledger settings:
	//   - validateRoutes: enables route validation (transaction route must be specified and valid)
	//   - validateAccountType: enables account type validation (accounts must match route rules)
	//
	// Returns nil if validation is disabled or passes.
	// Returns an error if validation fails.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) ValidateAccountingRules(ctx context.Context, organizationID, ledgerID uuid.UUID, operations []mmodel.BalanceOperation, validate *pkgTransaction.Responses, action string) (*mmodel.TransactionRouteCache, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.validate_accounting_rules")
	defer span.End()

	// Get ledger settings for this ledger
	ledgerSettings := uc.GetLedgerSettings(ctx, organizationID, ledgerID)

	// If route validation is disabled, skip all route-related validation
	if !ledgerSettings.Accounting.ValidateRoutes {
		logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("Route validation disabled for ledger %s, skipping accounting rules validation", ledgerID.String()))

		return nil, nil
	}

	logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("Route validation enabled for ledger %s, validating accounting rules", ledgerID.String()))

	if libCommons.IsNilOrEmpty(&validate.TransactionRoute) {
		err := pkg.ValidateBusinessError(constant.ErrTransactionRouteNotInformed, "")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction route is empty", err)

		logger.Log(ctx, libLog.LevelWarn, "Transaction route is empty")

		return nil, err
	}

	transactionRouteID, err := uuid.Parse(validate.TransactionRoute)
	if err != nil {
		validationErr := pkg.ValidateBusinessError(constant.ErrInvalidTransactionRouteID, "")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid transaction route ID format", validationErr)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Invalid transaction route ID format: %v", err))

		return nil, validationErr
	}

	transactionRouteCache, err := uc.GetOrCreateTransactionRouteCache(ctx, organizationID, ledgerID, transactionRouteID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to load transaction route cache", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to load transaction route cache: %v", err))

		return nil, err
	}

	actionCache, found := transactionRouteCache.Actions[action]
	if !found {
		err := pkg.ValidateBusinessError(constant.ErrNoRoutesForAction, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), action)
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "No routes found for action", err)

		logger.Warnf("No routes found for action '%s' in transaction route cache", action)

		return nil, err
	}

	sourceRoutes := actionCache.Source
	destinationRoutes := actionCache.Destination
	bidirectionalRoutes := actionCache.Bidirectional

	// Reject operations missing a per-operation route ID.
	// When validateRoutes is active, each operation must explicitly specify
	// which operation route it belongs to.
	for alias, routeID := range validate.OperationRoutesFrom {
		if routeID == "" {
			err := pkg.ValidateBusinessError(constant.ErrAccountingRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name(), routeID, alias)
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Missing operation route ID", err)

			logger.Warnf("Operation '%s' (source) has no route ID — each operation must specify its operation route when route validation is enabled", alias)

			return nil, err
		}
	}

	for alias, routeID := range validate.OperationRoutesTo {
		if routeID == "" {
			err := pkg.ValidateBusinessError(constant.ErrAccountingRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name(), routeID, alias)
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Missing operation route ID", err)

			logger.Warnf("Operation '%s' (destination) has no route ID — each operation must specify its operation route when route validation is enabled", alias)

			return nil, err
		}
	}

	uniqueFromCount := uniqueValues(validate.OperationRoutesFrom)
	uniqueToCount := uniqueValues(validate.OperationRoutesTo)
	sourceRoutesCount := len(sourceRoutes)
	destinationRoutesCount := len(destinationRoutes)
	bidirectionalRoutesCount := len(bidirectionalRoutes)

	// Build shared bidirectional route set first — needed both for count
	// validation and for counterpart validation below.
	// A bidirectional route appearing on BOTH from and to sides is counted
	// once in uniqueFrom and once in uniqueTo, so we must subtract the shared
	// count to avoid double-counting.
	bidirectionalFromRoutes := make(map[string]bool)

	for _, routeID := range validate.OperationRoutesFrom {
		if _, isBidirectional := bidirectionalRoutes[routeID]; isBidirectional {
			bidirectionalFromRoutes[routeID] = true
		}
	}

	sharedBidirectionalRoutes := make(map[string]bool)

	for _, routeID := range validate.OperationRoutesTo {
		if bidirectionalFromRoutes[routeID] {
			sharedBidirectionalRoutes[routeID] = true
		}
	}

	totalCacheRoutes := sourceRoutesCount + destinationRoutesCount + bidirectionalRoutesCount
	totalUsedRoutes := uniqueFromCount + uniqueToCount - len(sharedBidirectionalRoutes)

	if totalUsedRoutes != totalCacheRoutes || uniqueFromCount < sourceRoutesCount || uniqueToCount < destinationRoutesCount {
		err := pkg.ValidateBusinessError(constant.ErrAccountingRouteCountMismatch, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), uniqueFromCount, uniqueToCount, sourceRoutesCount, destinationRoutesCount, bidirectionalRoutesCount)
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Accounting route count mismatch", err)

		logger.Warnf("Route count mismatch: from=%d to=%d, cache has source=%d destination=%d bidirectional=%d shared=%d", uniqueFromCount, uniqueToCount, sourceRoutesCount, destinationRoutesCount, bidirectionalRoutesCount, len(sharedBidirectionalRoutes))

		return nil, err
	}

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
		if err := validateCounterparts(operations, mergedRouteMap); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Route missing counterpart", err)

			logger.Warnf("Route counterpart validation failed: %v", err)

			return nil, err
		}
	}

	// Pass ledgerSettings and action-filtered routes to validateAccountRules for account type validation control
	err = validateAccountRules(ctx, sourceRoutes, destinationRoutes, bidirectionalRoutes, validate, operations, ledgerSettings)
	if err != nil {
		return nil, err
	}

	return &transactionRouteCache, nil
}

// validateAccountRules validates each operation against its corresponding route rule.
// Route existence and direction matching are always enforced when validateRoutes is active.
// Account type checks (alias/account_type rules) are only enforced when validateAccountType is also active.
func validateAccountRules(ctx context.Context, sourceRoutes, destinationRoutes, bidirectionalRoutes map[string]mmodel.OperationRouteCache, validate *pkgTransaction.Responses, operations []mmodel.BalanceOperation, ledgerSettings mmodel.LedgerSettings) error {
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
			err := pkg.ValidateBusinessError(constant.ErrAccountingRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name(), routeID, operation.Alias)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Accounting route not found", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Route ID '%s' not found in cache for operation '%s'", routeID, operation.Alias))

			return err
		}

		if err := validateDirectionRouteMatch(operation, cacheRule); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Direction does not match route operation type", err)

			logger.Warnf("Operation '%s' direction '%s' does not match route operation type '%s'", operation.Alias, operation.Amount.Direction, cacheRule.OperationType)

			return err
		}

		// Account type rules only apply when validateAccountType is enabled
		if ledgerSettings.Accounting.ValidateAccountType && cacheRule.Account != nil {
			if err := validateSingleOperationRule(operation, cacheRule.Account); err != nil {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Operation failed validation against route rules", err)

				logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Operation '%s' failed validation against route rules: %v", operation.Alias, err))

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
			return pkg.ValidateBusinessError(constant.ErrInvalidAccountingRoute, reflect.TypeOf(mmodel.AccountRule{}).Name())
		}

		alias := pkgTransaction.SplitAlias(op.Alias)

		if alias != expected {
			return pkg.ValidateBusinessError(
				constant.ErrAccountingAliasValidationFailed,
				reflect.TypeOf(mmodel.AccountRule{}).Name(),
				alias,
				expected,
			)
		}

	case constant.AccountRuleTypeAccountType:
		allowedTypes := extractStringSlice(account.ValidIf)
		if allowedTypes == nil {
			return pkg.ValidateBusinessError(constant.ErrInvalidAccountingRoute, reflect.TypeOf(mmodel.AccountRule{}).Name())
		}

		if slices.Contains(allowedTypes, op.Balance.AccountType) {
			return nil
		}

		return pkg.ValidateBusinessError(
			constant.ErrAccountingAccountTypeValidationFailed,
			reflect.TypeOf(mmodel.AccountRule{}).Name(),
			op.Balance.AccountType,
			allowedTypes,
		)

	default:
		return pkg.ValidateBusinessError(constant.ErrInvalidAccountingRoute, reflect.TypeOf(mmodel.AccountRule{}).Name())
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
	// PENDING transactions use ON_HOLD and RELEASE which invert the normal
	// direction-route mapping, so skip validation for these operation types.
	opAmount := strings.ToUpper(operation.Amount.Operation)
	if opAmount == constant.ONHOLD || opAmount == constant.RELEASE {
		return nil
	}

	direction := strings.ToLower(operation.Amount.Direction)
	opType := strings.ToLower(routeCache.OperationType)

	switch opType {
	case "source":
		if direction != constant.DirectionDebit {
			return pkg.ValidateBusinessError(
				constant.ErrDirectionRouteMismatch,
				reflect.TypeOf(mmodel.OperationRoute{}).Name(),
				operation.Amount.Direction,
				routeCache.OperationType,
				operation.Alias,
			)
		}
	case "destination":
		if direction != constant.DirectionCredit {
			return pkg.ValidateBusinessError(
				constant.ErrDirectionRouteMismatch,
				reflect.TypeOf(mmodel.OperationRoute{}).Name(),
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
			reflect.TypeOf(mmodel.OperationRoute{}).Name(),
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
				reflect.TypeOf(mmodel.OperationRoute{}).Name(),
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
