// Package constant provides system-wide constant values used across the Midaz ledger system.
// This file contains error code constants that map to specific error conditions in the system.
// Each error is represented by a unique numeric code that corresponds to detailed error
// information in the API documentation.
package constant

import (
	"errors"
)

// Error Code Constants
//
// This file defines all error codes used throughout the Midaz ledger system.
// Each error code is a unique 4-digit identifier (e.g., "0001", "0002") that maps to
// a specific error condition. These codes are used for:
//   - Consistent error identification across services
//   - API error responses with standardized error codes
//   - Internationalization and localization of error messages
//   - Error tracking and monitoring
//
// For complete error descriptions, HTTP status codes, and resolution guidance,
// refer to the API documentation: https://docs.midaz.io/midaz/api-reference/resources/errors-list
var (
	// Ledger Errors (0001-0002)

	// ErrDuplicateLedger indicates an attempt to create a ledger that already exists.
	ErrDuplicateLedger = errors.New("0001")

	// ErrLedgerNameConflict indicates a ledger name collision within the same organization.
	ErrLedgerNameConflict = errors.New("0002")

	// Asset Errors (0003-0005, 0034, 0055-0056)

	// ErrAssetNameOrCodeDuplicate indicates an asset with the same name or code already exists.
	ErrAssetNameOrCodeDuplicate = errors.New("0003")

	// ErrCodeUppercaseRequirement indicates asset codes must be in uppercase format.
	ErrCodeUppercaseRequirement = errors.New("0004")

	// ErrCurrencyCodeStandardCompliance indicates the currency code doesn't comply with ISO 4217 or similar standards.
	ErrCurrencyCodeStandardCompliance = errors.New("0005")

	// ErrAssetCodeNotFound indicates the specified asset code does not exist in the system.
	ErrAssetCodeNotFound = errors.New("0034")

	// ErrAssetIDNotFound indicates the specified asset ID does not exist.
	ErrAssetIDNotFound = errors.New("0055")

	// ErrNoAssetsFound indicates no assets were found matching the query criteria.
	ErrNoAssetsFound = errors.New("0056")

	// ErrDuplicateAssetPair indicates an attempt to create a duplicate asset rate pair.
	ErrDuplicateAssetPair = errors.New("0028")

	// General Entity Errors (0006-0009, 0013, 0022)

	// ErrUnmodifiableField indicates an attempt to modify a field that cannot be changed after creation.
	ErrUnmodifiableField = errors.New("0006")

	// ErrEntityNotFound indicates the requested entity does not exist in the system.
	ErrEntityNotFound = errors.New("0007")

	// ErrActionNotPermitted indicates the requested action is not allowed for the current entity state.
	ErrActionNotPermitted = errors.New("0008")

	// ErrMissingFieldsInRequest indicates required fields are missing from the request.
	ErrMissingFieldsInRequest = errors.New("0009")

	// ErrResourceAlreadyDeleted indicates an attempt to delete a resource that has already been deleted.
	ErrResourceAlreadyDeleted = errors.New("0013")

	// ErrImmutableField indicates an attempt to modify an immutable field.
	ErrImmutableField = errors.New("0022")

	// Account Type Errors (0010-0011, 0066, 0108-0112, 0120)

	// ErrAccountTypeImmutable indicates the account type cannot be changed after account creation.
	ErrAccountTypeImmutable = errors.New("0010")

	// ErrInactiveAccountType indicates an attempt to use an inactive or disabled account type.
	ErrInactiveAccountType = errors.New("0011")

	// ErrInvalidAccountType indicates the specified account type is not valid or recognized.
	ErrInvalidAccountType = errors.New("0066")

	// ErrDuplicateAccountTypeKeyValue indicates a duplicate key-value pair in account type configuration.
	ErrDuplicateAccountTypeKeyValue = errors.New("0108")

	// ErrAccountTypeNotFound indicates the specified account type does not exist.
	ErrAccountTypeNotFound = errors.New("0109")

	// ErrNoAccountTypesFound indicates no account types were found matching the query.
	ErrNoAccountTypesFound = errors.New("0110")

	// ErrInvalidAccountRuleType indicates an invalid rule type for account configuration.
	ErrInvalidAccountRuleType = errors.New("0111")

	// ErrInvalidAccountRuleValue indicates an invalid rule value for account configuration.
	ErrInvalidAccountRuleValue = errors.New("0112")

	// ErrInvalidAccountTypeKeyValue indicates an invalid key-value pair in account type metadata.
	ErrInvalidAccountTypeKeyValue = errors.New("0120")

	// Account Errors (0012, 0019-0020, 0029, 0052, 0054, 0062-0064, 0074, 0085, 0096, 0098)

	// ErrAccountBalanceDeletion indicates an account with balances cannot be deleted.
	ErrAccountBalanceDeletion = errors.New("0012")

	// ErrAccountIneligibility indicates the account does not meet requirements for the requested operation.
	ErrAccountIneligibility = errors.New("0019")

	// ErrAliasUnavailability indicates the requested account alias is already in use.
	ErrAliasUnavailability = errors.New("0020")

	// ErrInvalidParentAccountID indicates the specified parent account ID is invalid or not found.
	ErrInvalidParentAccountID = errors.New("0029")

	// ErrAccountIDNotFound indicates the specified account ID does not exist.
	ErrAccountIDNotFound = errors.New("0052")

	// ErrIDsNotFoundForAccounts indicates one or more account IDs in the request were not found.
	ErrIDsNotFoundForAccounts = errors.New("0054")

	// ErrNoAccountIDsProvided indicates the request requires account IDs but none were provided.
	ErrNoAccountIDsProvided = errors.New("0062")

	// ErrFailedToRetrieveAccountsByAliases indicates an error occurred while fetching accounts by their aliases.
	ErrFailedToRetrieveAccountsByAliases = errors.New("0063")

	// ErrNoAccountsFound indicates no accounts were found matching the query criteria.
	ErrNoAccountsFound = errors.New("0064")

	// ErrForbiddenExternalAccountManipulation indicates direct manipulation of external accounts is not allowed.
	ErrForbiddenExternalAccountManipulation = errors.New("0074")

	// ErrAccountAliasNotFound indicates the specified account alias does not exist.
	ErrAccountAliasNotFound = errors.New("0085")

	// ErrAccountAliasInvalid indicates the account alias format is invalid.
	ErrAccountAliasInvalid = errors.New("0096")

	// ErrOnHoldExternalAccount indicates an attempt to put funds on hold in an external account, which is not allowed.
	ErrOnHoldExternalAccount = errors.New("0098")

	// Segment Errors (0014-0015, 0036, 0057)

	// ErrSegmentIDInactive indicates the specified segment is inactive and cannot be used.
	ErrSegmentIDInactive = errors.New("0014")

	// ErrDuplicateSegmentName indicates a segment with the same name already exists.
	ErrDuplicateSegmentName = errors.New("0015")

	// ErrSegmentIDNotFound indicates the specified segment ID does not exist.
	ErrSegmentIDNotFound = errors.New("0036")

	// ErrNoSegmentsFound indicates no segments were found matching the query criteria.
	ErrNoSegmentsFound = errors.New("0057")

	// Balance Errors (0016, 0018, 0025, 0061, 0086, 0092-0093, 0124)

	// ErrBalanceRemainingDeletion indicates an account with remaining balance cannot be deleted.
	ErrBalanceRemainingDeletion = errors.New("0016")

	// ErrInsufficientFunds indicates the account does not have sufficient funds for the operation.
	ErrInsufficientFunds = errors.New("0018")

	// ErrInsufficientAccountBalance indicates the account balance is insufficient for the transaction.
	ErrInsufficientAccountBalance = errors.New("0025")

	// ErrBalanceUpdateFailed indicates a failure occurred while updating account balances.
	ErrBalanceUpdateFailed = errors.New("0061")

	// ErrLockVersionAccountBalance indicates a version conflict in balance update (optimistic locking failure).
	ErrLockVersionAccountBalance = errors.New("0086")

	// ErrNoBalancesFound indicates no balances were found for the specified criteria.
	ErrNoBalancesFound = errors.New("0092")

	// ErrBalancesCantDeleted indicates balances cannot be deleted due to business rules.
	ErrBalancesCantDeleted = errors.New("0093")

	// ErrAdditionalBalanceNotAllowed indicates creating additional balances is not permitted for this account.
	ErrAdditionalBalanceNotAllowed = errors.New("0124")

	// Transaction DSL Errors (0017, 0048-0049)

	// ErrInvalidScriptFormat indicates the transaction DSL script has invalid syntax or format.
	ErrInvalidScriptFormat = errors.New("0017")

	// ErrInvalidDSLFileFormat indicates the DSL file format is invalid or corrupted.
	ErrInvalidDSLFileFormat = errors.New("0048")

	// ErrEmptyDSLFile indicates the DSL file is empty or contains no valid content.
	ErrEmptyDSLFile = errors.New("0049")

	// Transaction Errors (0021, 0023-0027, 0030, 0070-0073, 0087-0091, 0099, 0121-0122)

	// ErrParentTransactionIDNotFound indicates the specified parent transaction ID does not exist.
	ErrParentTransactionIDNotFound = errors.New("0021")

	// ErrTransactionTimingRestriction indicates the transaction timing violates business rules.
	ErrTransactionTimingRestriction = errors.New("0023")

	// ErrAccountStatusTransactionRestriction indicates the account status prevents transaction processing.
	ErrAccountStatusTransactionRestriction = errors.New("0024")

	// ErrTransactionMethodRestriction indicates the transaction method is not allowed for this operation.
	ErrTransactionMethodRestriction = errors.New("0026")

	// ErrDuplicateTransactionTemplateCode indicates a transaction template with the same code already exists.
	ErrDuplicateTransactionTemplateCode = errors.New("0027")

	// ErrMismatchedAssetCode indicates asset codes in the transaction do not match expected values.
	ErrMismatchedAssetCode = errors.New("0030")

	// ErrTransactionIDNotFound indicates the specified transaction ID does not exist.
	ErrTransactionIDNotFound = errors.New("0070")

	// ErrNoTransactionsFound indicates no transactions were found matching the query criteria.
	ErrNoTransactionsFound = errors.New("0071")

	// ErrInvalidTransactionType indicates the transaction type is invalid or not recognized.
	ErrInvalidTransactionType = errors.New("0072")

	// ErrTransactionValueMismatch indicates transaction values do not balance (debits != credits).
	ErrTransactionValueMismatch = errors.New("0073")

	// ErrTransactionIDHasAlreadyParentTransaction indicates the transaction already has a parent and cannot be modified.
	ErrTransactionIDHasAlreadyParentTransaction = errors.New("0087")

	// ErrTransactionIDIsAlreadyARevert indicates the transaction is already a revert transaction.
	ErrTransactionIDIsAlreadyARevert = errors.New("0088")

	// ErrTransactionCantRevert indicates the transaction cannot be reverted due to its current state.
	ErrTransactionCantRevert = errors.New("0089")

	// ErrTransactionAmbiguous indicates the transaction reference is ambiguous and cannot be uniquely identified.
	ErrTransactionAmbiguous = errors.New("0090")

	// ErrParentIDSameID indicates the parent ID cannot be the same as the entity ID.
	ErrParentIDSameID = errors.New("0091")

	// ErrCommitTransactionNotPending indicates only pending transactions can be committed.
	ErrCommitTransactionNotPending = errors.New("0099")

	// ErrInvalidFutureTransactionDate indicates the future transaction date is invalid or outside allowed range.
	ErrInvalidFutureTransactionDate = errors.New("0121")

	// ErrInvalidPendingFutureTransactionDate indicates the pending transaction date is invalid for future transactions.
	ErrInvalidPendingFutureTransactionDate = errors.New("0122")

	// Chart of Accounts Errors (0031-0033)

	// ErrChartTypeNotFound indicates the specified chart of accounts type does not exist.
	ErrChartTypeNotFound = errors.New("0031")

	// ErrInvalidCountryCode indicates the country code is invalid or not recognized.
	ErrInvalidCountryCode = errors.New("0032")

	// ErrInvalidCodeFormat indicates the code format does not meet requirements.
	ErrInvalidCodeFormat = errors.New("0033")

	// Portfolio Errors (0035, 0058)

	// ErrPortfolioIDNotFound indicates the specified portfolio ID does not exist.
	ErrPortfolioIDNotFound = errors.New("0035")

	// ErrNoPortfoliosFound indicates no portfolios were found matching the query criteria.
	ErrNoPortfoliosFound = errors.New("0058")

	// Ledger Reference Errors (0037, 0060)

	// ErrLedgerIDNotFound indicates the specified ledger ID does not exist.
	ErrLedgerIDNotFound = errors.New("0037")

	// ErrNoLedgersFound indicates no ledgers were found matching the query criteria.
	ErrNoLedgersFound = errors.New("0060")

	// Organization Errors (0038-0039, 0059)

	// ErrOrganizationIDNotFound indicates the specified organization ID does not exist.
	ErrOrganizationIDNotFound = errors.New("0038")

	// ErrParentOrganizationIDNotFound indicates the specified parent organization ID does not exist.
	ErrParentOrganizationIDNotFound = errors.New("0039")

	// ErrNoOrganizationsFound indicates no organizations were found matching the query criteria.
	ErrNoOrganizationsFound = errors.New("0059")

	// Type Validation Errors (0040)

	// ErrInvalidType indicates the specified type is invalid or not recognized by the system.
	ErrInvalidType = errors.New("0040")

	// Authentication & Authorization Errors (0041-0045)

	// ErrTokenMissing indicates the required authentication token is missing from the request.
	ErrTokenMissing = errors.New("0041")

	// ErrInvalidToken indicates the authentication token is invalid, expired, or malformed.
	ErrInvalidToken = errors.New("0042")

	// ErrInsufficientPrivileges indicates the authenticated user lacks required permissions.
	ErrInsufficientPrivileges = errors.New("0043")

	// ErrPermissionEnforcement indicates an error occurred while enforcing permission rules.
	ErrPermissionEnforcement = errors.New("0044")

	// ErrJWKFetch indicates an error occurred while fetching JSON Web Keys for token validation.
	ErrJWKFetch = errors.New("0045")

	// General HTTP Errors (0046-0047, 0053, 0065, 0094)

	// ErrInternalServer indicates an unexpected internal server error occurred.
	ErrInternalServer = errors.New("0046")

	// ErrBadRequest indicates the request is malformed or contains invalid data.
	ErrBadRequest = errors.New("0047")

	// ErrUnexpectedFieldsInTheRequest indicates the request contains fields that are not expected or allowed.
	ErrUnexpectedFieldsInTheRequest = errors.New("0053")

	// ErrInvalidPathParameter indicates a path parameter has an invalid format or value.
	ErrInvalidPathParameter = errors.New("0065")

	// ErrInvalidRequestBody indicates the request body is invalid or cannot be parsed.
	ErrInvalidRequestBody = errors.New("0094")

	// Metadata Errors (0050-0051, 0067)

	// ErrMetadataKeyLengthExceeded indicates a metadata key exceeds the maximum allowed length.
	ErrMetadataKeyLengthExceeded = errors.New("0050")

	// ErrMetadataValueLengthExceeded indicates a metadata value exceeds the maximum allowed length.
	ErrMetadataValueLengthExceeded = errors.New("0051")

	// ErrInvalidMetadataNesting indicates metadata nesting exceeds allowed depth or has invalid structure.
	ErrInvalidMetadataNesting = errors.New("0067")

	// Operation Errors (0068-0069)

	// ErrOperationIDNotFound indicates the specified operation ID does not exist.
	ErrOperationIDNotFound = errors.New("0068")

	// ErrNoOperationsFound indicates no operations were found matching the query criteria.
	ErrNoOperationsFound = errors.New("0069")

	// Audit Errors (0075-0076)

	// ErrAuditRecordNotRetrieved indicates an error occurred while retrieving audit records.
	ErrAuditRecordNotRetrieved = errors.New("0075")

	// ErrAuditTreeRecordNotFound indicates the audit tree record does not exist.
	ErrAuditTreeRecordNotFound = errors.New("0076")

	// Date & Time Validation Errors (0077-0079, 0083)

	// ErrInvalidDateFormat indicates the date format is invalid or not recognized.
	ErrInvalidDateFormat = errors.New("0077")

	// ErrInvalidFinalDate indicates the final date is invalid or before the start date.
	ErrInvalidFinalDate = errors.New("0078")

	// ErrDateRangeExceedsLimit indicates the date range exceeds the maximum allowed period.
	ErrDateRangeExceedsLimit = errors.New("0079")

	// ErrInvalidDateRange indicates the date range is invalid or logically inconsistent.
	ErrInvalidDateRange = errors.New("0083")

	// Query & Pagination Errors (0080-0082)

	// ErrPaginationLimitExceeded indicates the pagination limit exceeds the maximum allowed value.
	ErrPaginationLimitExceeded = errors.New("0080")

	// ErrInvalidSortOrder indicates the sort order parameter is invalid (must be 'asc' or 'desc').
	ErrInvalidSortOrder = errors.New("0081")

	// ErrInvalidQueryParameter indicates a query parameter has an invalid value or format.
	ErrInvalidQueryParameter = errors.New("0082")

	// Idempotency Errors (0084)

	// ErrIdempotencyKey indicates an idempotency key conflict or validation error.
	ErrIdempotencyKey = errors.New("0084")

	// Message Broker Errors (0095)

	// ErrMessageBrokerUnavailable indicates the message broker (RabbitMQ) is unavailable or unreachable.
	ErrMessageBrokerUnavailable = errors.New("0095")

	// Numeric Overflow Errors (0097)

	// ErrOverFlowInt64 indicates a numeric value exceeds the maximum int64 value.
	ErrOverFlowInt64 = errors.New("0097")

	// Operation Route Errors (0100-0104, 0107)

	// ErrOperationRouteTitleAlreadyExists indicates an operation route with the same title already exists.
	ErrOperationRouteTitleAlreadyExists = errors.New("0100")

	// ErrOperationRouteNotFound indicates the specified operation route does not exist.
	ErrOperationRouteNotFound = errors.New("0101")

	// ErrNoOperationRoutesFound indicates no operation routes were found matching the query.
	ErrNoOperationRoutesFound = errors.New("0102")

	// ErrInvalidOperationRouteType indicates the operation route type is invalid.
	ErrInvalidOperationRouteType = errors.New("0103")

	// ErrMissingOperationRoutes indicates required operation routes are missing from the request.
	ErrMissingOperationRoutes = errors.New("0104")

	// ErrOperationRouteLinkedToTransactionRoutes indicates the operation route cannot be modified because it's linked to transaction routes.
	ErrOperationRouteLinkedToTransactionRoutes = errors.New("0107")

	// Transaction Route Errors (0105-0106, 0113-0119)

	// ErrTransactionRouteNotFound indicates the specified transaction route does not exist.
	ErrTransactionRouteNotFound = errors.New("0105")

	// ErrNoTransactionRoutesFound indicates no transaction routes were found matching the query.
	ErrNoTransactionRoutesFound = errors.New("0106")

	// ErrInvalidAccountingRoute indicates the accounting route configuration is invalid.
	ErrInvalidAccountingRoute = errors.New("0113")

	// ErrTransactionRouteNotInformed indicates the transaction route was not provided in the request.
	ErrTransactionRouteNotInformed = errors.New("0114")

	// ErrInvalidTransactionRouteID indicates the transaction route ID is invalid or malformed.
	ErrInvalidTransactionRouteID = errors.New("0115")

	// ErrAccountingRouteCountMismatch indicates the number of accounting routes doesn't match expected count.
	ErrAccountingRouteCountMismatch = errors.New("0116")

	// ErrAccountingRouteNotFound indicates the specified accounting route does not exist.
	ErrAccountingRouteNotFound = errors.New("0117")

	// ErrAccountingAliasValidationFailed indicates the accounting alias validation failed.
	ErrAccountingAliasValidationFailed = errors.New("0118")

	// ErrAccountingAccountTypeValidationFailed indicates the accounting account type validation failed.
	ErrAccountingAccountTypeValidationFailed = errors.New("0119")

	// Alias Errors (0123)

	// ErrDuplicatedAliasKeyValue indicates a duplicate alias key-value pair was detected.
	ErrDuplicatedAliasKeyValue = errors.New("0123")
)
