// Package query implements read operations (queries) for the transaction service.
// This file contains the logic for validating accounting routes.
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

// ValidateAccountingRules validates a set of operations against transaction route rules.
//
// This use case ensures that all operations in a transaction comply with the configured
// routing rules. Validation can be enabled on a per-organization and per-ledger basis
// via the `TRANSACTION_ROUTE_VALIDATION` environment variable.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - operations: A slice of balance operations to validate.
//   - validate: The validation response containing the route IDs.
//
// Returns:
//   - error: An error if validation fails.
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
// matches the account rule defined in its operation route.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - transactionRouteCache: The cached transaction route with its operation routes.
//   - validate: The validation response with route mappings.
//   - operations: A slice of balance operations to validate.
//
// Returns:
//   - error: An error if any operation fails validation.
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
// operation route. It supports two rule types: alias matching and account type matching.
//
// Parameters:
//   - op: The balance operation to validate.
//   - account: The account rule from the operation route.
//
// Returns:
//   - error: An error if the validation fails.
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
// This helper function is used to validate that the correct number of distinct
// operation routes are being used in a transaction.
//
// Parameters:
//   - m: A map of aliases to operation route IDs.
//
// Returns:
//   - int: The number of unique operation route IDs.
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

// extractStringSlice converts an interface{} to a slice of strings.
//
// This helper function handles the type conversion for the `ValidIf` field, which
// can be either a `[]string` or a `[]any` depending on how it is unmarshaled.
//
// Parameters:
//   - value: The `ValidIf` value, expected to be a slice of strings.
//
// Returns:
//   - []string: The converted slice of strings, or nil if the conversion fails.
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
