// Package query implements read operations (queries) for the transaction service.
// This file contains query implementation.

package query

import (
	"context"
	"os"
	"reflect"
	"slices"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// ValidateAccountingRules validates operations against transaction route rules.
//
// This method implements transaction route validation, which ensures operations comply
// with configured routing rules. It:
// 1. Checks if validation is enabled for this org:ledger pair (TRANSACTION_ROUTE_VALIDATION env)
// 2. Validates transaction route ID is provided
// 3. Retrieves transaction route cache from Redis
// 4. Validates operation count matches route count (source and destination)
// 5. Validates each operation against its route's account rules
//
// Accounting Route Validation:
//   - Enabled per org:ledger via TRANSACTION_ROUTE_VALIDATION env variable
//   - Format: "orgID:ledgerID,orgID2:ledgerID2"
//   - Validates account alias or account type matches route rules
//   - Ensures correct number of source and destination operations
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - operations: Array of balance operations to validate
//   - validate: Validation responses with route IDs
//
// Returns:
//   - error: nil if valid, business error if validation fails
//
// OpenTelemetry: Creates span "usecase.validate_accounting_rules"
func (uc *UseCase) ValidateAccountingRules(ctx context.Context, organizationID, ledgerID uuid.UUID, operations []mmodel.BalanceOperation, validate *libTransaction.Responses) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	accountingValidation := os.Getenv("TRANSACTION_ROUTE_VALIDATION")
	if !strings.Contains(accountingValidation, organizationID.String()+":"+ledgerID.String()) {
		return nil
	}

	ctx, span := tracer.Start(ctx, "usecase.validate_accounting_rules")
	defer span.End()

	if libCommons.IsNilOrEmpty(&validate.TransactionRoute) {
		err := pkg.ValidateBusinessError(constant.ErrTransactionRouteNotInformed, "")
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Transaction route is empty", err)

		logger.Warnf("Transaction route is empty")

		return err
	}

	transactionRouteID, err := uuid.Parse(validate.TransactionRoute)
	if err != nil {
		validationErr := pkg.ValidateBusinessError(constant.ErrInvalidTransactionRouteID, "")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid transaction route ID format", validationErr)

		logger.Warnf("Invalid transaction route ID format: %v", err)

		return validationErr
	}

	transactionRouteCache, err := uc.GetOrCreateTransactionRouteCache(ctx, organizationID, ledgerID, transactionRouteID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to load transaction route cache", err)

		logger.Errorf("Failed to load transaction route cache: %v", err)

		return err
	}

	uniqueFromCount := uniqueValues(validate.OperationRoutesFrom)
	uniqueToCount := uniqueValues(validate.OperationRoutesTo)
	sourceRoutesCount := len(transactionRouteCache.Source)
	destinationRoutesCount := len(transactionRouteCache.Destination)

	if uniqueFromCount != sourceRoutesCount || uniqueToCount != destinationRoutesCount {
		err := pkg.ValidateBusinessError(constant.ErrAccountingRouteCountMismatch, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), uniqueFromCount, uniqueToCount, sourceRoutesCount, destinationRoutesCount)
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Accounting route count mismatch", err)

		logger.Warnf("Route count mismatch: expected %d source, %d destination; got %d source, %d destination", sourceRoutesCount, destinationRoutesCount, uniqueFromCount, uniqueToCount)

		return err
	}

	return validateAccountRules(ctx, transactionRouteCache, validate, operations)
}

// validateAccountRules validates each operation against its corresponding route rule.
//
// This helper function iterates through all operations and validates that each one
// matches the account rule defined in its operation route. It:
// 1. Determines if operation is source or destination
// 2. Looks up the operation route in cache
// 3. Validates operation against the route's account rule
//
// Account Rule Types:
//   - alias: Exact alias match
//   - account_type: Account type must be in allowed list
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - transactionRouteCache: Cached transaction route with operation routes
//   - validate: Validation responses with route mappings
//   - operations: Array of balance operations to validate
//
// Returns:
//   - error: nil if all operations valid, business error if any validation fails
//
// OpenTelemetry: Creates span "usecase.validate_account_rules"
func validateAccountRules(ctx context.Context, transactionRouteCache mmodel.TransactionRouteCache, validate *libTransaction.Responses, operations []mmodel.BalanceOperation) error {
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
			cacheRule, found = transactionRouteCache.Source[routeID]
		} else {
			cacheRule, found = transactionRouteCache.Destination[routeID]
		}

		if !found {
			err := pkg.ValidateBusinessError(constant.ErrAccountingRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name(), routeID, operation.Alias)
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Accounting route not found", err)

			logger.Warnf("Route ID '%s' not found in cache for operation '%s'", routeID, operation.Alias)

			return err
		}

		if cacheRule.Account != nil {
			if err := validateSingleOperationRule(operation, cacheRule.Account); err != nil {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Operation failed validation against route rules", err)

				logger.Warnf("Operation '%s' failed validation against route rules: %v", operation.Alias, err)

				return err
			}
		}
	}

	return nil
}

// validateSingleOperationRule validates a single operation against an account rule.
//
// This function checks if an operation's account matches the rule defined in the
// operation route. It supports two rule types:
//
// 1. Alias Rule: Exact alias match
//   - ValidIf contains expected alias string
//   - Operation alias must match exactly
//
// 2. Account Type Rule: Account type must be in allowed list
//   - ValidIf contains array of allowed account types
//   - Operation's account type must be in the list
//
// Parameters:
//   - op: Balance operation to validate
//   - account: Account rule from operation route
//
// Returns:
//   - error: nil if valid, business error if validation fails
//
// Possible Errors:
//   - ErrInvalidAccountingRoute: Invalid rule configuration
//   - ErrAccountingAliasValidationFailed: Alias doesn't match
//   - ErrAccountingAccountTypeValidationFailed: Account type not allowed
func validateSingleOperationRule(op mmodel.BalanceOperation, account *mmodel.AccountCache) error {
	switch account.RuleType {
	case constant.AccountRuleTypeAlias:
		expected, ok := account.ValidIf.(string)
		if !ok {
			return pkg.ValidateBusinessError(constant.ErrInvalidAccountingRoute, reflect.TypeOf(mmodel.AccountRule{}).Name())
		}

		alias := libTransaction.SplitAlias(op.Alias)

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

// uniqueValues counts the number of unique operation route IDs in a map.
//
// This helper function counts distinct operation route IDs to validate that the
// correct number of routes are being used in a transaction.
//
// Parameters:
//   - m: Map of alias to operation route ID
//
// Returns:
//   - int: Number of unique operation route IDs
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

// extractStringSlice converts interface{} to []string for account type validation.
//
// This helper function handles type conversion for ValidIf values, which can be
// either []string or []any depending on JSON unmarshaling.
//
// Parameters:
//   - value: ValidIf value (expected to be string array)
//
// Returns:
//   - []string: Converted string slice, or nil if conversion fails
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
