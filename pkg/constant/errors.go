// Package constant provides error code constants for the Midaz ledger system.
//
// This package defines standardized error codes used throughout the application
// for consistent error handling and client communication. Error codes follow a
// numeric format (0001-9999) that maps to human-readable messages in the API layer.
//
// Error Code Design:
//
// Error codes are designed for:
//   - Unique identification: Each error has a distinct numeric code
//   - Client integration: Clients can map codes to localized messages
//   - API documentation: Codes are documented at https://docs.midaz.io/midaz/api-reference/resources/errors-list
//   - Debugging: Error codes enable quick issue identification
//
// Error Categories:
//
// Codes are loosely grouped by domain:
//   - 0001-0020: Entity validation and uniqueness errors
//   - 0021-0040: Business rule violations
//   - 0041-0050: Authentication and authorization errors
//   - 0051-0070: Entity not found errors
//   - 0071-0090: Query and pagination errors
//   - 0091-0130: Transaction and routing errors
//
// Usage Pattern:
//
// Errors are typically wrapped with business context:
//
//	if err != nil {
//	    return pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, "Account")
//	}
//
// Related Packages:
//   - pkg: ValidateBusinessError function for error wrapping
//   - http/handlers: Map error codes to HTTP status codes
//   - docs: API error documentation generation
package constant

import (
	"errors"
)

// Error code constants for the Midaz ledger system.
//
// Each error is defined as a sentinel error with a numeric string code.
// These codes are used throughout the application and exposed in API responses.
//
// Error Documentation:
//
// For detailed descriptions and resolution steps for each error,
// refer to: https://docs.midaz.io/midaz/api-reference/resources/errors-list
//
// WHY: Standardized error codes enable:
//   - Consistent error handling across all API endpoints
//   - Client-side error message localization
//   - Automated error tracking and alerting
//   - API documentation generation
//
// WHAT: Each constant represents a specific error condition:
//   - Validation failures (duplicate names, invalid formats)
//   - Business rule violations (insufficient funds, immutable fields)
//   - Authentication/authorization failures
//   - Resource not found conditions
//
// HOW: Use with pkg.ValidateBusinessError() to create rich error responses:
//
//	err := pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, "Ledger")
//	// Returns: {"code": "0037", "message": "Ledger not found", ...}
var (
	// ErrDuplicateLedger indicates a ledger with the same identifier already exists.
	// Resolution: Use a different ledger name or ID.
	ErrDuplicateLedger = errors.New("0001")

	// ErrLedgerNameConflict indicates the ledger name is already in use within the organization.
	// Resolution: Choose a unique ledger name.
	ErrLedgerNameConflict = errors.New("0002")

	// ErrAssetNameOrCodeDuplicate indicates an asset with the same name or code already exists.
	// Resolution: Use unique name and code for the asset.
	ErrAssetNameOrCodeDuplicate = errors.New("0003")

	// ErrCodeUppercaseRequirement indicates the code must be uppercase.
	// Resolution: Convert the code to uppercase (e.g., "usd" -> "USD").
	ErrCodeUppercaseRequirement = errors.New("0004")

	// ErrCurrencyCodeStandardCompliance indicates the currency code doesn't comply with ISO 4217.
	// Resolution: Use a valid ISO 4217 currency code (e.g., "USD", "EUR", "BRL").
	ErrCurrencyCodeStandardCompliance = errors.New("0005")

	// ErrUnmodifiableField indicates an attempt to modify a field that cannot be changed.
	// Resolution: Remove the field from the update request.
	ErrUnmodifiableField = errors.New("0006")

	// ErrEntityNotFound indicates the requested entity does not exist.
	// Resolution: Verify the entity ID or create the entity first.
	ErrEntityNotFound = errors.New("0007")

	// ErrActionNotPermitted indicates the requested action is not allowed.
	// Resolution: Check user permissions or entity state.
	ErrActionNotPermitted = errors.New("0008")

	// ErrMissingFieldsInRequest indicates required fields are missing from the request.
	// Resolution: Include all required fields in the request body.
	ErrMissingFieldsInRequest = errors.New("0009")

	// ErrAccountTypeImmutable indicates the account type cannot be changed after creation.
	// Resolution: Create a new account with the desired type.
	ErrAccountTypeImmutable = errors.New("0010")

	// ErrInactiveAccountType indicates the account type is not active.
	// Resolution: Activate the account type or use an active one.
	ErrInactiveAccountType = errors.New("0011")

	// ErrAccountBalanceDeletion indicates an account with balance cannot be deleted.
	// Resolution: Transfer or zero the balance before deletion.
	ErrAccountBalanceDeletion = errors.New("0012")

	// ErrResourceAlreadyDeleted indicates the resource has already been deleted.
	// Resolution: The operation is already complete, no action needed.
	ErrResourceAlreadyDeleted = errors.New("0013")

	// ErrSegmentIDInactive indicates the segment is not active.
	// Resolution: Activate the segment or use an active one.
	ErrSegmentIDInactive = errors.New("0014")

	// ErrDuplicateSegmentName indicates the segment name already exists in the ledger.
	// Resolution: Use a unique segment name.
	ErrDuplicateSegmentName = errors.New("0015")

	// ErrBalanceRemainingDeletion indicates a balance with remaining amount cannot be deleted.
	// Resolution: Zero the balance before deletion.
	ErrBalanceRemainingDeletion = errors.New("0016")

	// ErrInvalidScriptFormat indicates the transaction script format is invalid.
	// Resolution: Review the DSL syntax and fix format errors.
	ErrInvalidScriptFormat = errors.New("0017")

	// ErrInsufficientFunds indicates the account has insufficient funds for the operation.
	// Resolution: Add funds or reduce the transaction amount.
	ErrInsufficientFunds = errors.New("0018")

	// ErrAccountIneligibility indicates the account is not eligible for this operation.
	// Resolution: Check account status and type restrictions.
	ErrAccountIneligibility = errors.New("0019")

	// ErrAliasUnavailability indicates the account alias is already in use.
	// Resolution: Choose a different alias.
	ErrAliasUnavailability = errors.New("0020")

	// ErrParentTransactionIDNotFound indicates the parent transaction does not exist.
	// Resolution: Verify the parent transaction ID.
	ErrParentTransactionIDNotFound = errors.New("0021")

	// ErrImmutableField indicates an attempt to modify an immutable field.
	// Resolution: Remove the field from the update request.
	ErrImmutableField = errors.New("0022")

	// ErrTransactionTimingRestriction indicates the transaction violates timing rules.
	// Resolution: Check transaction date restrictions.
	ErrTransactionTimingRestriction = errors.New("0023")

	// ErrAccountStatusTransactionRestriction indicates the account status prevents transactions.
	// Resolution: Activate the account or resolve status issues.
	ErrAccountStatusTransactionRestriction = errors.New("0024")

	// ErrInsufficientAccountBalance indicates insufficient balance for the operation.
	// Resolution: Add funds or reduce the amount.
	ErrInsufficientAccountBalance = errors.New("0025")

	// ErrTransactionMethodRestriction indicates the transaction method is not allowed.
	// Resolution: Use an allowed transaction method.
	ErrTransactionMethodRestriction = errors.New("0026")

	// ErrDuplicateTransactionTemplateCode indicates the template code already exists.
	// Resolution: Use a unique template code.
	ErrDuplicateTransactionTemplateCode = errors.New("0027")

	// ErrDuplicateAssetPair indicates an asset rate for this pair already exists.
	// Resolution: Update the existing rate instead of creating a new one.
	ErrDuplicateAssetPair = errors.New("0028")

	// ErrInvalidParentAccountID indicates the parent account ID is invalid.
	// Resolution: Verify the parent account exists and is accessible.
	ErrInvalidParentAccountID = errors.New("0029")

	// ErrMismatchedAssetCode indicates the asset code doesn't match expected value.
	// Resolution: Ensure consistent asset codes across the operation.
	ErrMismatchedAssetCode = errors.New("0030")

	// ErrChartTypeNotFound indicates the chart of accounts type was not found.
	// Resolution: Create the chart type or use an existing one.
	ErrChartTypeNotFound = errors.New("0031")

	// ErrInvalidCountryCode indicates the country code is invalid.
	// Resolution: Use a valid ISO 3166-1 alpha-2 country code.
	ErrInvalidCountryCode = errors.New("0032")

	// ErrInvalidCodeFormat indicates the code format is invalid.
	// Resolution: Follow the required code format rules.
	ErrInvalidCodeFormat = errors.New("0033")

	// ErrAssetCodeNotFound indicates the asset code was not found.
	// Resolution: Create the asset or use an existing code.
	ErrAssetCodeNotFound = errors.New("0034")

	// ErrPortfolioIDNotFound indicates the portfolio was not found.
	// Resolution: Verify the portfolio ID or create the portfolio.
	ErrPortfolioIDNotFound = errors.New("0035")

	// ErrSegmentIDNotFound indicates the segment was not found.
	// Resolution: Verify the segment ID or create the segment.
	ErrSegmentIDNotFound = errors.New("0036")

	// ErrLedgerIDNotFound indicates the ledger was not found.
	// Resolution: Verify the ledger ID or create the ledger.
	ErrLedgerIDNotFound = errors.New("0037")

	// ErrOrganizationIDNotFound indicates the organization was not found.
	// Resolution: Verify the organization ID.
	ErrOrganizationIDNotFound = errors.New("0038")

	// ErrParentOrganizationIDNotFound indicates the parent organization was not found.
	// Resolution: Verify the parent organization ID.
	ErrParentOrganizationIDNotFound = errors.New("0039")

	// ErrInvalidType indicates an invalid type was provided.
	// Resolution: Use a valid type from the allowed values.
	ErrInvalidType = errors.New("0040")

	// ErrTokenMissing indicates the authentication token is missing.
	// Resolution: Include the Authorization header with a valid token.
	ErrTokenMissing = errors.New("0041")

	// ErrInvalidToken indicates the authentication token is invalid.
	// Resolution: Obtain a new valid token.
	ErrInvalidToken = errors.New("0042")

	// ErrInsufficientPrivileges indicates the user lacks required permissions.
	// Resolution: Request the necessary permissions.
	ErrInsufficientPrivileges = errors.New("0043")

	// ErrPermissionEnforcement indicates a permission policy violation.
	// Resolution: Contact administrator for permission adjustments.
	ErrPermissionEnforcement = errors.New("0044")

	// ErrJWKFetch indicates failure to fetch JWK for token validation.
	// Resolution: Check network connectivity and auth server status.
	ErrJWKFetch = errors.New("0045")

	// ErrInternalServer indicates an unexpected internal server error.
	// Resolution: Retry the request or contact support.
	ErrInternalServer = errors.New("0046")

	// ErrBadRequest indicates the request is malformed.
	// Resolution: Review and fix the request format.
	ErrBadRequest = errors.New("0047")

	// ErrInvalidDSLFileFormat indicates the DSL file format is invalid.
	// Resolution: Fix the DSL syntax errors.
	ErrInvalidDSLFileFormat = errors.New("0048")

	// ErrEmptyDSLFile indicates the DSL file is empty.
	// Resolution: Provide a non-empty DSL file.
	ErrEmptyDSLFile = errors.New("0049")

	// ErrMetadataKeyLengthExceeded indicates a metadata key exceeds the length limit.
	// Resolution: Shorten the metadata key (max 100 characters).
	ErrMetadataKeyLengthExceeded = errors.New("0050")

	// ErrMetadataValueLengthExceeded indicates a metadata value exceeds the length limit.
	// Resolution: Shorten the metadata value (max 2000 characters).
	ErrMetadataValueLengthExceeded = errors.New("0051")

	// ErrAccountIDNotFound indicates the account was not found.
	// Resolution: Verify the account ID or create the account.
	ErrAccountIDNotFound = errors.New("0052")

	// ErrUnexpectedFieldsInTheRequest indicates unexpected fields in the request.
	// Resolution: Remove unknown fields from the request.
	ErrUnexpectedFieldsInTheRequest = errors.New("0053")

	// ErrIDsNotFoundForAccounts indicates one or more account IDs were not found.
	// Resolution: Verify all account IDs exist.
	ErrIDsNotFoundForAccounts = errors.New("0054")

	// ErrAssetIDNotFound indicates the asset was not found.
	// Resolution: Verify the asset ID or create the asset.
	ErrAssetIDNotFound = errors.New("0055")

	// ErrNoAssetsFound indicates no assets were found for the query.
	// Resolution: Create assets or adjust query filters.
	ErrNoAssetsFound = errors.New("0056")

	// ErrNoSegmentsFound indicates no segments were found for the query.
	// Resolution: Create segments or adjust query filters.
	ErrNoSegmentsFound = errors.New("0057")

	// ErrNoPortfoliosFound indicates no portfolios were found for the query.
	// Resolution: Create portfolios or adjust query filters.
	ErrNoPortfoliosFound = errors.New("0058")

	// ErrNoOrganizationsFound indicates no organizations were found for the query.
	// Resolution: Create organizations or adjust query filters.
	ErrNoOrganizationsFound = errors.New("0059")

	// ErrNoLedgersFound indicates no ledgers were found for the query.
	// Resolution: Create ledgers or adjust query filters.
	ErrNoLedgersFound = errors.New("0060")

	// ErrBalanceUpdateFailed indicates the balance update operation failed.
	// Resolution: Retry the operation or check for conflicts.
	ErrBalanceUpdateFailed = errors.New("0061")

	// ErrNoAccountIDsProvided indicates no account IDs were provided.
	// Resolution: Provide at least one account ID.
	ErrNoAccountIDsProvided = errors.New("0062")

	// ErrFailedToRetrieveAccountsByAliases indicates failure to retrieve accounts by aliases.
	// Resolution: Verify aliases exist and are valid.
	ErrFailedToRetrieveAccountsByAliases = errors.New("0063")

	// ErrNoAccountsFound indicates no accounts were found for the query.
	// Resolution: Create accounts or adjust query filters.
	ErrNoAccountsFound = errors.New("0064")

	// ErrInvalidPathParameter indicates an invalid path parameter.
	// Resolution: Ensure path parameters are valid UUIDs.
	ErrInvalidPathParameter = errors.New("0065")

	// ErrInvalidAccountType indicates the account type is invalid.
	// Resolution: Use a valid account type.
	ErrInvalidAccountType = errors.New("0066")

	// ErrInvalidMetadataNesting indicates metadata contains invalid nesting.
	// Resolution: Flatten metadata structure (no nested objects).
	ErrInvalidMetadataNesting = errors.New("0067")

	// ErrOperationIDNotFound indicates the operation was not found.
	// Resolution: Verify the operation ID.
	ErrOperationIDNotFound = errors.New("0068")

	// ErrNoOperationsFound indicates no operations were found for the query.
	// Resolution: Check transaction ID or adjust filters.
	ErrNoOperationsFound = errors.New("0069")

	// ErrTransactionIDNotFound indicates the transaction was not found.
	// Resolution: Verify the transaction ID.
	ErrTransactionIDNotFound = errors.New("0070")

	// ErrNoTransactionsFound indicates no transactions were found for the query.
	// Resolution: Adjust query filters.
	ErrNoTransactionsFound = errors.New("0071")

	// ErrInvalidTransactionType indicates the transaction type is invalid.
	// Resolution: Use a valid transaction type.
	ErrInvalidTransactionType = errors.New("0072")

	// ErrTransactionValueMismatch indicates debit/credit values don't balance.
	// Resolution: Ensure sum of debits equals sum of credits.
	ErrTransactionValueMismatch = errors.New("0073")

	// ErrForbiddenExternalAccountManipulation indicates external accounts cannot be directly modified.
	// Resolution: Use inflow/outflow endpoints for external account operations.
	ErrForbiddenExternalAccountManipulation = errors.New("0074")

	// ErrAuditRecordNotRetrieved indicates the audit record could not be retrieved.
	// Resolution: Contact support for audit log access.
	ErrAuditRecordNotRetrieved = errors.New("0075")

	// ErrAuditTreeRecordNotFound indicates the audit tree record was not found.
	// Resolution: Verify the audit tree exists.
	ErrAuditTreeRecordNotFound = errors.New("0076")

	// ErrInvalidDateFormat indicates the date format is invalid.
	// Resolution: Use ISO 8601 format (YYYY-MM-DDTHH:MM:SSZ).
	ErrInvalidDateFormat = errors.New("0077")

	// ErrInvalidFinalDate indicates the final date is invalid.
	// Resolution: Ensure final date is after start date.
	ErrInvalidFinalDate = errors.New("0078")

	// ErrDateRangeExceedsLimit indicates the date range exceeds allowed limit.
	// Resolution: Reduce the date range.
	ErrDateRangeExceedsLimit = errors.New("0079")

	// ErrPaginationLimitExceeded indicates the pagination limit exceeds maximum.
	// Resolution: Reduce the limit parameter.
	ErrPaginationLimitExceeded = errors.New("0080")

	// ErrInvalidSortOrder indicates the sort order is invalid.
	// Resolution: Use "asc" or "desc".
	ErrInvalidSortOrder = errors.New("0081")

	// ErrInvalidQueryParameter indicates an invalid query parameter.
	// Resolution: Review allowed query parameters.
	ErrInvalidQueryParameter = errors.New("0082")

	// ErrInvalidDateRange indicates the date range is invalid.
	// Resolution: Ensure start date is before end date.
	ErrInvalidDateRange = errors.New("0083")

	// ErrIdempotencyKey indicates an idempotency key conflict.
	// Resolution: Use a new idempotency key for different requests.
	ErrIdempotencyKey = errors.New("0084")

	// ErrAccountAliasNotFound indicates the account alias was not found.
	// Resolution: Verify the alias or use account ID.
	ErrAccountAliasNotFound = errors.New("0085")

	// ErrLockVersionAccountBalance indicates a balance version conflict (optimistic locking).
	// Resolution: Retry with updated version.
	ErrLockVersionAccountBalance = errors.New("0086")

	// ErrTransactionIDHasAlreadyParentTransaction indicates the transaction already has a parent.
	// Resolution: Cannot add another parent transaction.
	ErrTransactionIDHasAlreadyParentTransaction = errors.New("0087")

	// ErrTransactionIDIsAlreadyARevert indicates the transaction is already a reversal.
	// Resolution: Reversals cannot be reversed.
	ErrTransactionIDIsAlreadyARevert = errors.New("0088")

	// ErrTransactionCantRevert indicates the transaction cannot be reverted.
	// Resolution: Check transaction status and type.
	ErrTransactionCantRevert = errors.New("0089")

	// ErrTransactionAmbiguous indicates ambiguous transaction reference.
	// Resolution: Provide more specific transaction identifier.
	ErrTransactionAmbiguous = errors.New("0090")

	// ErrParentIDSameID indicates parent ID equals the entity ID.
	// Resolution: Parent cannot reference itself.
	ErrParentIDSameID = errors.New("0091")

	// ErrNoBalancesFound indicates no balances were found for the query.
	// Resolution: Create balances or adjust filters.
	ErrNoBalancesFound = errors.New("0092")

	// ErrBalancesCantBeDeleted indicates the balance cannot be deleted.
	// Resolution: Check balance state and dependencies.
	ErrBalancesCantBeDeleted = errors.New("0093")

	// ErrInvalidRequestBody indicates the request body is invalid.
	// Resolution: Fix JSON syntax errors.
	ErrInvalidRequestBody = errors.New("0094")

	// ErrMessageBrokerUnavailable indicates the message broker is unavailable.
	// Resolution: Check RabbitMQ connection and retry.
	ErrMessageBrokerUnavailable = errors.New("0095")

	// ErrAccountAliasInvalid indicates the account alias format is invalid.
	// Resolution: Use valid alias format (@prefix/suffix).
	ErrAccountAliasInvalid = errors.New("0096")

	// ErrOverFlowInt64 indicates an integer overflow.
	// Resolution: Use smaller values within int64 range.
	ErrOverFlowInt64 = errors.New("0097")

	// ErrOnHoldExternalAccount indicates external accounts cannot have on-hold balances.
	// Resolution: Use regular accounts for on-hold operations.
	ErrOnHoldExternalAccount = errors.New("0098")

	// ErrCommitTransactionNotPending indicates only pending transactions can be committed.
	// Resolution: Transaction must be in PENDING status.
	ErrCommitTransactionNotPending = errors.New("0099")

	// ErrOperationRouteTitleAlreadyExists indicates the operation route title already exists.
	// Resolution: Use a unique title.
	ErrOperationRouteTitleAlreadyExists = errors.New("0100")

	// ErrOperationRouteNotFound indicates the operation route was not found.
	// Resolution: Verify the operation route ID.
	ErrOperationRouteNotFound = errors.New("0101")

	// ErrNoOperationRoutesFound indicates no operation routes were found.
	// Resolution: Create operation routes or adjust filters.
	ErrNoOperationRoutesFound = errors.New("0102")

	// ErrInvalidOperationRouteType indicates the operation route type is invalid.
	// Resolution: Use "debit" or "credit".
	ErrInvalidOperationRouteType = errors.New("0103")

	// ErrMissingOperationRoutes indicates required operation routes are missing.
	// Resolution: Configure operation routes for the transaction type.
	ErrMissingOperationRoutes = errors.New("0104")

	// ErrTransactionRouteNotFound indicates the transaction route was not found.
	// Resolution: Verify the transaction route ID.
	ErrTransactionRouteNotFound = errors.New("0105")

	// ErrNoTransactionRoutesFound indicates no transaction routes were found.
	// Resolution: Create transaction routes or adjust filters.
	ErrNoTransactionRoutesFound = errors.New("0106")

	// ErrOperationRouteLinkedToTransactionRoutes indicates the operation route is linked.
	// Resolution: Unlink from transaction routes before deletion.
	ErrOperationRouteLinkedToTransactionRoutes = errors.New("0107")

	// ErrDuplicateAccountTypeKeyValue indicates the account type key_value is duplicate.
	// Resolution: Use a unique key_value.
	ErrDuplicateAccountTypeKeyValue = errors.New("0108")

	// ErrAccountTypeNotFound indicates the account type was not found.
	// Resolution: Verify the account type ID or key_value.
	ErrAccountTypeNotFound = errors.New("0109")

	// ErrNoAccountTypesFound indicates no account types were found.
	// Resolution: Create account types or adjust filters.
	ErrNoAccountTypesFound = errors.New("0110")

	// ErrInvalidAccountRuleType indicates the account rule type is invalid.
	// Resolution: Use valid rule types (account_type, alias, etc.).
	ErrInvalidAccountRuleType = errors.New("0111")

	// ErrInvalidAccountRuleValue indicates the account rule value is invalid.
	// Resolution: Provide valid values for the rule type.
	ErrInvalidAccountRuleValue = errors.New("0112")

	// ErrInvalidAccountingRoute indicates the accounting route is invalid.
	// Resolution: Check route configuration.
	ErrInvalidAccountingRoute = errors.New("0113")

	// ErrTransactionRouteNotInformed indicates the transaction route was not specified.
	// Resolution: Provide the transaction route in the request.
	ErrTransactionRouteNotInformed = errors.New("0114")

	// ErrInvalidTransactionRouteID indicates the transaction route ID is invalid.
	// Resolution: Use a valid UUID.
	ErrInvalidTransactionRouteID = errors.New("0115")

	// ErrAccountingRouteCountMismatch indicates route count doesn't match operations.
	// Resolution: Ensure route count matches operation count.
	ErrAccountingRouteCountMismatch = errors.New("0116")

	// ErrAccountingRouteNotFound indicates the accounting route was not found.
	// Resolution: Configure the required accounting route.
	ErrAccountingRouteNotFound = errors.New("0117")

	// ErrAccountingAliasValidationFailed indicates alias validation failed for routing.
	// Resolution: Check account aliases match route rules.
	ErrAccountingAliasValidationFailed = errors.New("0118")

	// ErrAccountingAccountTypeValidationFailed indicates account type validation failed.
	// Resolution: Check account types match route rules.
	ErrAccountingAccountTypeValidationFailed = errors.New("0119")

	// ErrInvalidAccountTypeKeyValue indicates the account type key_value is invalid.
	// Resolution: Use lowercase alphanumeric with underscores.
	ErrInvalidAccountTypeKeyValue = errors.New("0120")

	// ErrInvalidFutureTransactionDate indicates transaction date is in the future.
	// Resolution: Use current or past date for non-pending transactions.
	ErrInvalidFutureTransactionDate = errors.New("0121")

	// ErrInvalidPendingFutureTransactionDate indicates invalid future date for pending.
	// Resolution: Check date rules for pending transactions.
	ErrInvalidPendingFutureTransactionDate = errors.New("0122")

	// ErrDuplicatedAliasKeyValue indicates duplicate alias key_value.
	// Resolution: Use unique alias key_value combinations.
	ErrDuplicatedAliasKeyValue = errors.New("0123")

	// ErrAdditionalBalanceNotAllowed indicates additional balances are not allowed.
	// Resolution: Check account configuration for balance limits.
	ErrAdditionalBalanceNotAllowed = errors.New("0124")

	// ErrInvalidTransactionNonPositiveValue indicates transaction value must be positive.
	// Resolution: Use a positive value for the transaction.
	ErrInvalidTransactionNonPositiveValue = errors.New("0125")

	// ErrDefaultBalanceNotFound indicates the default balance was not found.
	// Resolution: Ensure account has a default balance created.
	ErrDefaultBalanceNotFound = errors.New("0126")

	// ErrAccountCreationFailed indicates account creation failed.
	// Resolution: Check input data and retry.
	ErrAccountCreationFailed = errors.New("0127")

	// ErrInvalidDatetimeFormat indicates the datetime format is invalid.
	// Resolution: Use ISO 8601 format (YYYY-MM-DDTHH:MM:SSZ).
	ErrInvalidDatetimeFormat = errors.New("0128")
)
