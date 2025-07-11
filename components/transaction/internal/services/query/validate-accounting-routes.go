package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// validateAccountingRules validates the accounting rules for the given operations
func (uc *UseCase) ValidateAccountingRules(ctx context.Context, organizationID, ledgerID uuid.UUID, operations []lockOperation, validate *libTransaction.Responses) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.validate_accounting_rules")
	defer span.End()

	settings, err := uc.GetOrCreateSettingsCache(ctx, organizationID, ledgerID, "accounting_validation_enabled")
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get settings cache", err)

		logger.Errorf("Failed to get settings cache: %v", err)

		return err
	}

	if settings == nil || settings.Active == nil || !*settings.Active {
		return nil
	}

	if libCommons.IsNilOrEmpty(&validate.TransactionRoute) {
		err := pkg.ValidateBusinessError(constant.ErrTransactionRouteNotInformed, "")
		libOpentelemetry.HandleSpanError(&span, "Transaction route is empty", err)

		logger.Errorf("Transaction route is empty")

		return err
	}

	transactionRouteID, err := uuid.Parse(validate.TransactionRoute)
	if err != nil {
		validationErr := pkg.ValidateBusinessError(constant.ErrInvalidTransactionRouteID, "")

		libOpentelemetry.HandleSpanError(&span, "Invalid transaction route ID format", validationErr)

		logger.Errorf("Invalid transaction route ID format: %v", err)

		return validationErr
	}

	transactionRouteCache, err := uc.GetOrCreateTransactionRouteCache(ctx, organizationID, ledgerID, transactionRouteID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to load transaction route cache", err)

		logger.Errorf("Failed to load transaction route cache: %v", err)

		return err
	}

	uniqueFromCount := uniqueValues(validate.OperationRoutesFrom)
	uniqueToCount := uniqueValues(validate.OperationRoutesTo)
	sourceRoutesCount := len(transactionRouteCache.Source)
	destinationRoutesCount := len(transactionRouteCache.Destination)

	if uniqueFromCount != sourceRoutesCount || uniqueToCount != destinationRoutesCount {
		err := pkg.ValidateBusinessError(constant.ErrAccountingRouteCountMismatch, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), uniqueFromCount, uniqueToCount, sourceRoutesCount, destinationRoutesCount)
		libOpentelemetry.HandleSpanError(&span, "Accounting route count mismatch", err)

		logger.Errorf("Route count mismatch: expected %d source, %d destination; got %d source, %d destination", sourceRoutesCount, destinationRoutesCount, uniqueFromCount, uniqueToCount)

		return err
	}

	return validateAccountRules(ctx, transactionRouteCache, validate, operations)
}

// validateAccountRules validates each operation against its corresponding route rule
func validateAccountRules(ctx context.Context, transactionRouteCache mmodel.TransactionRouteCache, validate *libTransaction.Responses, operations []lockOperation) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	_, span := tracer.Start(ctx, "usecase.validate_accounting_rules")
	defer span.End()

	for _, operation := range operations {
		// Get route ID and determine if operation is source or destination
		var routeID string

		var isSource bool

		if _, exists := validate.From[operation.alias]; exists {
			index := findKeyIndex(validate.From, operation.alias)
			routeID = validate.OperationRoutesFrom[libTransaction.ConcatAlias(index, operation.alias)]
			isSource = true
		} else if _, existsTo := validate.To[operation.alias]; existsTo {
			index := findKeyIndex(validate.To, operation.alias)
			routeID = validate.OperationRoutesTo[libTransaction.ConcatAlias(index, operation.alias)]
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
			err := pkg.ValidateBusinessError(constant.ErrAccountingRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name(), routeID, operation.alias)
			libOpentelemetry.HandleSpanError(&span, "Accounting route not found", err)

			logger.Errorf("Route ID '%s' not found in cache for operation '%s'", routeID, operation.alias)

			return err
		}

		if cacheRule.Account != nil {
			if err := validateSingleOperationRule(operation, cacheRule.Account); err != nil {
				libOpentelemetry.HandleSpanError(&span, "Operation failed validation against route rules", err)

				logger.Errorf("Operation '%s' failed validation against route rules: %v", operation.alias, err)

				return err
			}
		}
	}

	return nil
}

// findKeyIndex finds the index of a key in a map based on iteration order
func findKeyIndex(m map[string]libTransaction.Amount, key string) int {
	index := 0

	for k := range m {
		if k == key {
			break
		}

		index++
	}

	return index
}

// validateSingleOperationRule validates if an operation matches the account rule defined in the transaction route
func validateSingleOperationRule(op lockOperation, account *mmodel.AccountCache) error {
	switch account.RuleType {
	case "alias":
		expected, ok := account.ValidIf.(string)
		if !ok {
			return pkg.ValidateBusinessError(constant.ErrInvalidAccountingRoute, reflect.TypeOf(mmodel.AccountRule{}).Name())
		}

		if op.alias != expected {
			return pkg.ValidateBusinessError(
				constant.ErrAccountingAliasValidationFailed,
				reflect.TypeOf(mmodel.AccountRule{}).Name(),
				op.alias,
				expected,
			)
		}

	case "account_type":
		allowedTypes := extractStringSlice(account.ValidIf)
		if allowedTypes == nil {
			return pkg.ValidateBusinessError(constant.ErrInvalidAccountingRoute, reflect.TypeOf(mmodel.AccountRule{}).Name())
		}

		for _, allowedType := range allowedTypes {
			if op.balance.AccountType == allowedType {
				return nil
			}
		}

		return pkg.ValidateBusinessError(
			constant.ErrAccountingAccountTypeValidationFailed,
			reflect.TypeOf(mmodel.AccountRule{}).Name(),
			op.balance.AccountType,
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
