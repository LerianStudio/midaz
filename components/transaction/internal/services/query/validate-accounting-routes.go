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

// ValidateAccountingRules validates transaction operations against configured accounting routes.
//
// This method enforces accounting rules defined in transaction routes, ensuring that
// operations comply with configured account restrictions. It is a critical validation
// step that prevents invalid transactions from being recorded in the ledger.
//
// Feature Flag:
//
// Validation is controlled by the TRANSACTION_ROUTE_VALIDATION environment variable.
// Format: "orgID1:ledgerID1,orgID2:ledgerID2" (comma-separated org:ledger pairs)
// If the current org:ledger is not in the list, validation is skipped (returns nil).
//
// Validation Process:
//
//	Step 1: Check Feature Flag
//	  - Parse TRANSACTION_ROUTE_VALIDATION environment variable
//	  - Skip validation if org:ledger pair not enabled
//	  - This allows gradual rollout of route validation
//
//	Step 2: Validate Transaction Route Presence
//	  - Ensure transaction route ID is provided
//	  - Parse and validate UUID format
//	  - Return business error if invalid or missing
//
//	Step 3: Load Transaction Route Cache
//	  - Retrieve cached route configuration (or create from DB)
//	  - Cache contains source/destination operation routes
//	  - Fail if route not found in database
//
//	Step 4: Validate Route Counts
//	  - Count unique source routes from request
//	  - Count unique destination routes from request
//	  - Compare against configured route counts
//	  - Mismatch indicates incorrect transaction structure
//
//	Step 5: Validate Account Rules
//	  - Delegate to validateAccountRules for per-operation checks
//	  - Each operation validated against its assigned route
//
// Parameters:
//   - ctx: Request context with tenant and tracing information
//   - organizationID: Organization UUID for route lookup
//   - ledgerID: Ledger UUID for route lookup
//   - operations: Balance operations to validate
//   - validate: Transaction validation context with route assignments
//
// Returns:
//   - error: Validation failure with specific error code, nil if valid
//
// Error Scenarios:
//   - ErrTransactionRouteNotInformed: No route ID provided
//   - ErrInvalidTransactionRouteID: Malformed UUID
//   - ErrAccountingRouteCountMismatch: Wrong number of source/destination routes
//   - ErrAccountingRouteNotFound: Operation references non-existent route
//   - ErrAccountingAliasValidationFailed: Account alias doesn't match rule
//   - ErrAccountingAccountTypeValidationFailed: Account type not in allowed list
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
// This function iterates through operations and validates each against the accounting
// rules defined in the transaction route cache. It determines whether an operation
// is a source (debit) or destination (credit) and applies the appropriate rule set.
//
// Validation Logic:
//
//	Step 1: Determine Operation Direction
//	  - Check if alias exists in From map (source/debit)
//	  - Check if alias exists in To map (destination/credit)
//	  - Skip if operation not in either (shouldn't happen)
//
//	Step 2: Locate Route Rule
//	  - Find route ID from operation routes map
//	  - Lookup rule in cache (source or destination)
//	  - Error if route not found in cache
//
//	Step 3: Apply Account Rule
//	  - Validate operation against account rule if defined
//	  - Delegate to validateSingleOperationRule
//	  - Return first validation failure
//
// Parameters:
//   - ctx: Request context for tracing
//   - transactionRouteCache: Cached route with source/destination rules
//   - validate: Validation context with alias-to-route mappings
//   - operations: Operations to validate
//
// Returns:
//   - error: First validation failure, nil if all pass
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

// validateSingleOperationRule validates if an operation matches the account rule defined in the transaction route.
//
// Account rules define constraints on which accounts can participate in operations.
// Two rule types are supported:
//
// Rule Types:
//
//	AccountRuleTypeAlias: Exact alias match
//	  - ValidIf contains expected alias string
//	  - Operation alias must exactly match
//	  - Use case: Specific account requirements (e.g., "settlement_account")
//
//	AccountRuleTypeAccountType: Account type whitelist
//	  - ValidIf contains []string of allowed types
//	  - Operation's account type must be in list
//	  - Use case: Category restrictions (e.g., only "ASSET" or "LIABILITY")
//
// Parameters:
//   - op: Balance operation to validate
//   - account: Account rule from route configuration
//
// Returns:
//   - error: Validation failure with specific error code
//
// Error Scenarios:
//   - ErrInvalidAccountingRoute: Rule type unknown or ValidIf malformed
//   - ErrAccountingAliasValidationFailed: Alias doesn't match expected value
//   - ErrAccountingAccountTypeValidationFailed: Type not in allowed list
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

// uniqueValues counts the number of unique values in a map.
//
// This helper function is used to count distinct route IDs in operation assignments,
// ensuring the correct number of unique routes are specified.
//
// Optimization:
//   - Early return for empty map (0)
//   - Early return for single-entry map (1)
//   - Uses map[string]struct{} for O(1) deduplication
//
// Parameters:
//   - m: Map with string values to deduplicate
//
// Returns:
//   - int: Count of unique values
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

// extractStringSlice helper function to handle []string and []any conversion.
//
// MongoDB may return arrays as []any instead of []string. This helper normalizes
// both formats to []string for consistent rule validation.
//
// Supported Input Types:
//   - []string: Returned as-is
//   - []any: Converted element-by-element (returns nil if any element not string)
//   - Other: Returns nil
//
// Parameters:
//   - value: Value to convert (typically from account rule ValidIf)
//
// Returns:
//   - []string: Converted slice, nil if conversion fails
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
