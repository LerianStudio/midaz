package common

import (
	"errors"
	"fmt"
	"strings"

	cn "github.com/LerianStudio/midaz/common/constant"
)

// EntityNotFoundError records an error indicating an entity was not found in any case that caused it.
// You can use it to representing a Database not found, cache not found or any other repository.
type EntityNotFoundError struct {
	EntityType string
	Title      string
	Message    string
	Code       string
	Err        error
}

// NewEntityNotFoundError creates an instance of EntityNotFoundError.
func NewEntityNotFoundError(entityType string) EntityNotFoundError {
	return EntityNotFoundError{
		EntityType: entityType,
		Code:       "",
		Title:      "",
		Message:    "",
		Err:        nil,
	}
}

// WrapEntityNotFoundError creates an instance of EntityNotFoundError.
func WrapEntityNotFoundError(entityType string, err error) EntityNotFoundError {
	return EntityNotFoundError{
		EntityType: entityType,
		Code:       "",
		Title:      "",
		Message:    "",
		Err:        err,
	}
}

// Error implements the error interface.
func (e EntityNotFoundError) Error() string {
	if strings.TrimSpace(e.Message) == "" {
		if strings.TrimSpace(e.EntityType) != "" {
			return fmt.Sprintf("Entity %s not found", e.EntityType)
		}

		if e.Err != nil && strings.TrimSpace(e.Message) == "" {
			return e.Err.Error()
		}

		return "entity not found"
	}

	return e.Message
}

// Unwrap implements the error interface introduced in Go 1.13 to unwrap the internal error.
func (e EntityNotFoundError) Unwrap() error {
	return e.Err
}

// ValidationError records an error indicating an entity was not found in any case that caused it.
// You can use it to representing a Database not found, cache not found or any other repository.
type ValidationError struct {
	EntityType string
	Title      string
	Message    string
	Code       string
	Err        error
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	if strings.TrimSpace(e.Code) != "" {
		return fmt.Sprintf("%s - %s", e.Code, e.Message)
	}

	return e.Message
}

// Unwrap implements the error interface introduced in Go 1.13 to unwrap the internal error.
func (e ValidationError) Unwrap() error {
	return e.Err
}

// EntityConflictError records an error indicating an entity already exists in some repository
// You can use it to representing a Database conflict, cache or any other repository.
type EntityConflictError struct {
	EntityType string
	Title      string
	Message    string
	Code       string
	Err        error
}

// Error implements the error interface.
func (e EntityConflictError) Error() string {
	if e.Err != nil && strings.TrimSpace(e.Message) == "" {
		return e.Err.Error()
	}

	return e.Message
}

// Unwrap implements the error interface introduced in Go 1.13 to unwrap the internal error.
func (e EntityConflictError) Unwrap() error {
	return e.Err
}

// UnauthorizedError indicates an operation that couldn't be performant because there's no user authenticated.
type UnauthorizedError struct {
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
}

func (e UnauthorizedError) Error() string {
	return e.Message
}

// ForbiddenError indicates an operation that couldn't be performant because the authenticated user has no sufficient privileges.
type ForbiddenError struct {
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
}

func (e ForbiddenError) Error() string {
	return e.Message
}

// UnprocessableOperationError indicates an operation that couldn't be performant because it's invalid.
type UnprocessableOperationError struct {
	EntityType string
	Title      string
	Message    string
	Code       string
	Err        error
}

func (e UnprocessableOperationError) Error() string {
	return e.Message
}

// HTTPError indicates a http error raised in a http client.
type HTTPError struct {
	EntityType string
	Title      string
	Message    string
	Code       string
	Err        error
}

func (e HTTPError) Error() string {
	return e.Message
}

// FailedPreconditionError indicates a precondition failed during an operation.
type FailedPreconditionError struct {
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
}

func (e FailedPreconditionError) Error() string {
	return e.Message
}

// InternalServerError indicates a precondition failed during an operation.
type InternalServerError struct {
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
}

func (e InternalServerError) Error() string {
	return e.Message
}

// ResponseError is a struct used to return errors to the client.
type ResponseError struct {
	Code    int    `json:"code,omitempty"`
	Title   string `json:"title,omitempty"`
	Message string `json:"message,omitempty"`
}

// Error returns the message of the ResponseError.
//
// No parameters.
// Returns a string.
func (r ResponseError) Error() string {
	return r.Message
}

// ValidationKnownFieldsError records an error that occurred during a validation of known fields.
type ValidationKnownFieldsError struct {
	EntityType string           `json:"entityType,omitempty"`
	Title      string           `json:"title,omitempty"`
	Code       string           `json:"code,omitempty"`
	Message    string           `json:"message,omitempty"`
	Fields     FieldValidations `json:"fields,omitempty"`
}

// Error returns the error message for a ValidationKnownFieldsError.
//
// No parameters.
// Returns a string.
func (r ValidationKnownFieldsError) Error() string {
	return r.Message
}

// FieldValidations is a map of known fields and their validation errors.
type FieldValidations map[string]string

// ValidationUnknownFieldsError records an error that occurred during a validation of known fields.
type ValidationUnknownFieldsError struct {
	EntityType string        `json:"entityType,omitempty"`
	Title      string        `json:"title,omitempty"`
	Code       string        `json:"code,omitempty"`
	Message    string        `json:"message,omitempty"`
	Fields     UnknownFields `json:"fields,omitempty"`
}

// Error returns the error message for a ValidationUnknownFieldsError.
//
// No parameters.
// Returns a string.
func (r ValidationUnknownFieldsError) Error() string {
	return r.Message
}

// UnknownFields is a map of unknown fields and their error messages.
type UnknownFields map[string]any

// Methods to create errors for different scenarios:

// ValidateInternalError validates the error and returns an appropriate InternalServerError.
//
// Parameters:
// - err: The error to be validated.
// - entityType: The type of the entity associated with the error.
//
// Returns:
// - An InternalServerError with the appropriate code, title, message.
func ValidateInternalError(err error, entityType string) error {
	return InternalServerError{
		EntityType: entityType,
		Code:       cn.ErrInternalServer.Error(),
		Title:      "Internal Server Error",
		Message:    "The server encountered an unexpected error. Please try again later or contact support.",
		Err:        err,
	}
}

// ValidateBadRequestFieldsError validates the error and returns the appropriate bad request error code, title, message, and the invalid fields.
//
// Parameters:
// - knownInvalidFields: A map of known invalid fields and their validation errors.
// - entityType: The type of the entity associated with the error.
// - unknownFields: A map of unknown fields and their error messages.
//
// Returns:
// - An error indicating the validation result, which could be a ValidationUnknownFieldsError or a ValidationKnownFieldsError.
func ValidateBadRequestFieldsError(knownInvalidFields map[string]string, entityType string, unknownFields map[string]any) error {
	if len(unknownFields) == 0 && len(knownInvalidFields) == 0 {
		return errors.New("expected knownInvalidFields and unknownFields to be non-empty")
	}

	if len(unknownFields) > 0 {
		return ValidationUnknownFieldsError{
			EntityType: entityType,
			Code:       cn.ErrUnexpectedFieldsInTheRequest.Error(),
			Title:      "Unexpected Fields in the Request",
			Message:    "The request body contains more fields than expected. Please send only the allowed fields as per the documentation. The unexpected fields are listed in the fields object.",
			Fields:     unknownFields,
		}
	}

	return ValidationKnownFieldsError{
		EntityType: entityType,
		Code:       cn.ErrBadRequest.Error(),
		Title:      "Bad Request",
		Message:    "The server could not understand the request due to malformed syntax. Please check the listed fields and try again.",
		Fields:     knownInvalidFields,
	}
}

// ValidateBusinessError validates the error and returns the appropriate business error code, title, and message.
//
// Parameters:
//   - err: The error to be validated (ref: https://github.com/LerianStudio/midaz/common/constant/errors.go).
//   - entityType: The type of the entity related to the error.
//   - args: Additional arguments for formatting error messages.
//
// Returns:
//   - error: The appropriate business error with code, title, and message.
//
//nolint:gocyclo
func ValidateBusinessError(err error, entityType string, args ...any) error {
	switch {
	case errors.Is(err, cn.ErrDuplicateLedger):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrDuplicateLedger.Error(),
			Title:      "Duplicate Ledger Error",
			Message:    fmt.Sprintf("A ledger with the name %s already exists in the division %s. Please rename the ledger or choose a different division to attach it to.", args...),
		}
	case errors.Is(err, cn.ErrLedgerNameConflict):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrLedgerNameConflict.Error(),
			Title:      "Ledger Name Conflict",
			Message:    fmt.Sprintf("A ledger named %s already exists in your organization. Please rename the ledger, or if you want to use the same name, consider creating a new ledger for a different division.", args...),
		}
	case errors.Is(err, cn.ErrAssetNameOrCodeDuplicate):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrAssetNameOrCodeDuplicate.Error(),
			Title:      "Asset Name or Code Duplicate",
			Message:    "An asset with the same name or code already exists in your ledger. Please modify the name or code of your new asset.",
		}
	case errors.Is(err, cn.ErrCodeUppercaseRequirement):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrCodeUppercaseRequirement.Error(),
			Title:      "Code Uppercase Requirement",
			Message:    "The code must be in uppercase. Please ensure that the code is in uppercase format and try again.",
		}
	case errors.Is(err, cn.ErrCurrencyCodeStandardCompliance):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrCurrencyCodeStandardCompliance.Error(),
			Title:      "Currency Code Standard Compliance",
			Message:    "Currency-type assets must comply with the ISO-4217 standard. Please use a currency code that conforms to ISO-4217 guidelines.",
		}
	case errors.Is(err, cn.ErrUnmodifiableField):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrUnmodifiableField.Error(),
			Title:      "Unmodifiable Field Error",
			Message:    "Your request includes a field that cannot be modified. Please review your request and try again, removing any uneditable fields. Please refer to the documentation for guidance.",
		}
	case errors.Is(err, cn.ErrEntityNotFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrEntityNotFound.Error(),
			Title:      "Entity Not Found",
			Message:    "No entity was found for the given ID. Please make sure to use the correct ID for the entity you are trying to manage.",
		}
	case errors.Is(err, cn.ErrActionNotPermitted):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrActionNotPermitted.Error(),
			Title:      "Action Not Permitted",
			Message:    "The action you are attempting is not allowed in the current environment. Please refer to the documentation for guidance.",
		}
	case errors.Is(err, cn.ErrMissingFieldsInRequest):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrMissingFieldsInRequest.Error(),
			Title:      "Missing Fields in Request",
			Message:    "Your request is missing one or more required fields. Please refer to the documentation to ensure all necessary fields are included in your request.",
		}
	case errors.Is(err, cn.ErrAccountTypeImmutable):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrAccountTypeImmutable.Error(),
			Title:      "Account Type Immutable",
			Message:    "The account type specified cannot be modified. Please ensure the correct account type is being used and try again.",
		}
	case errors.Is(err, cn.ErrInactiveAccountType):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrInactiveAccountType.Error(),
			Title:      "Inactive Account Type Error",
			Message:    "The account type specified cannot be set to INACTIVE. Please ensure the correct account type is being used and try again.",
		}
	case errors.Is(err, cn.ErrAccountBalanceDeletion):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrAccountBalanceDeletion.Error(),
			Title:      "Account Balance Deletion Error",
			Message:    "An account or sub-account cannot be deleted if it has a remaining balance. Please ensure all remaining balances are transferred to another account before attempting to delete.",
		}
	case errors.Is(err, cn.ErrResourceAlreadyDeleted):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrResourceAlreadyDeleted.Error(),
			Title:      "Resource Already Deleted",
			Message:    "The resource you are trying to delete has already been deleted. Ensure you are using the correct ID and try again.",
		}
	case errors.Is(err, cn.ErrProductIDInactive):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrProductIDInactive.Error(),
			Title:      "Product ID Inactive",
			Message:    "The Product ID you are attempting to use is inactive. Please use another Product ID and try again.",
		}
	case errors.Is(err, cn.ErrDuplicateProductName):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrDuplicateProductName.Error(),
			Title:      "Duplicate Product Name Error",
			Message:    fmt.Sprintf("A product with the name %s already exists for this ledger ID %s. Please try again with a different ledger or name.", args...),
		}
	case errors.Is(err, cn.ErrBalanceRemainingDeletion):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrBalanceRemainingDeletion.Error(),
			Title:      "Balance Remaining Deletion Error",
			Message:    "The asset cannot be deleted because there is a remaining balance. Please ensure all balances are cleared before attempting to delete again.",
		}
	case errors.Is(err, cn.ErrInvalidScriptFormat):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrInvalidScriptFormat.Error(),
			Title:      "Invalid Script Format Error",
			Message:    "The script provided in your request is invalid or in an unsupported format. Please verify the script format and try again.",
		}
	case errors.Is(err, cn.ErrInsufficientFunds):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrInsufficientFunds.Error(),
			Title:      "Insufficient Funds Error",
			Message:    "The transaction could not be completed due to insufficient funds in the account. Please add sufficient funds to your account and try again.",
		}
	case errors.Is(err, cn.ErrAccountIneligibility):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrAccountIneligibility.Error(),
			Title:      "Account Ineligibility Error",
			Message:    "One or more accounts listed in the transaction are not eligible to participate. Please review the account statuses and try again.",
		}
	case errors.Is(err, cn.ErrAliasUnavailability):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrAliasUnavailability.Error(),
			Title:      "Alias Unavailability Error",
			Message:    fmt.Sprintf("The alias %s is already in use. Please choose a different alias and try again.", args...),
		}
	case errors.Is(err, cn.ErrParentTransactionIDNotFound):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrParentTransactionIDNotFound.Error(),
			Title:      "Parent Transaction ID Not Found",
			Message:    fmt.Sprintf("The parentTransactionId %s does not correspond to any existing transaction. Please review the ID and try again.", args...),
		}
	case errors.Is(err, cn.ErrImmutableField):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrImmutableField.Error(),
			Title:      "Immutable Field Error",
			Message:    fmt.Sprintf("The %s field cannot be modified. Please remove this field from your request and try again.", args...),
		}
	case errors.Is(err, cn.ErrTransactionTimingRestriction):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrTransactionTimingRestriction.Error(),
			Title:      "Transaction Timing Restriction",
			Message:    fmt.Sprintf("You can only perform another transaction using %s of %f from %s to %s after %s. Please wait until the specified time to try again.", args...),
		}
	case errors.Is(err, cn.ErrAccountStatusTransactionRestriction):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrAccountStatusTransactionRestriction.Error(),
			Title:      "Account Status Transaction Restriction",
			Message:    "The current statuses of the source and/or destination accounts do not permit transactions. Change the account status(es) and try again.",
		}
	case errors.Is(err, cn.ErrInsufficientAccountBalance):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrInsufficientAccountBalance.Error(),
			Title:      "Insufficient Account Balance Error",
			Message:    fmt.Sprintf("The account %s does not have sufficient balance. Please try again with an amount that is less than or equal to the available balance.", args...),
		}
	case errors.Is(err, cn.ErrTransactionMethodRestriction):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrTransactionMethodRestriction.Error(),
			Title:      "Transaction Method Restriction",
			Message:    fmt.Sprintf("Transactions involving %s are not permitted for the specified source and/or destination. Please try again using accounts that allow transactions with %s.", args...),
		}
	case errors.Is(err, cn.ErrDuplicateTransactionTemplateCode):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrDuplicateTransactionTemplateCode.Error(),
			Title:      "Duplicate Transaction Template Code Error",
			Message:    fmt.Sprintf("A transaction template with the code %s already exists for your ledger. Please use a different code and try again.", args...),
		}
	case errors.Is(err, cn.ErrDuplicateAssetPair):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ErrDuplicateAssetPair.Error(),
			Title:      "Duplicate Asset Pair Error",
			Message:    fmt.Sprintf("A pair for the assets %s%s already exists with the ID %s. Please update the existing entry instead of creating a new one.", args...),
		}
	case errors.Is(err, cn.ErrInvalidParentAccountID):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrInvalidParentAccountID.Error(),
			Title:      "Invalid Parent Account ID",
			Message:    "The specified parent account ID does not exist. Please verify the ID is correct and attempt your request again.",
		}
	case errors.Is(err, cn.ErrMismatchedAssetCode):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrMismatchedAssetCode.Error(),
			Title:      "Mismatched Asset Code",
			Message:    "The parent account ID you provided is associated with a different asset code than the one specified in your request. Please make sure the asset code matches that of the parent account, or use a different parent account ID and try again.",
		}
	case errors.Is(err, cn.ErrChartTypeNotFound):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrChartTypeNotFound.Error(),
			Title:      "Chart Type Not Found",
			Message:    fmt.Sprintf("The chart type %s does not exist. Please provide a valid chart type and refer to the documentation if you have any questions.", args...),
		}
	case errors.Is(err, cn.ErrInvalidCountryCode):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrInvalidCountryCode.Error(),
			Title:      "Invalid Country Code",
			Message:    "The provided country code in the 'address.country' field does not conform to the ISO-3166 alpha-2 standard. Please provide a valid alpha-2 country code.",
		}
	case errors.Is(err, cn.ErrInvalidCodeFormat):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrInvalidCodeFormat.Error(),
			Title:      "Invalid Code Format",
			Message:    "The 'code' field must be alphanumeric, in upper case, and must contain at least one letter. Please provide a valid code.",
		}
	case errors.Is(err, cn.ErrAssetCodeNotFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrAssetCodeNotFound.Error(),
			Title:      "Asset Code Not Found",
			Message:    "The provided asset code does not exist in our records. Please verify the asset code and try again.",
		}
	case errors.Is(err, cn.ErrPortfolioIDNotFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrPortfolioIDNotFound.Error(),
			Title:      "Portfolio ID Not Found",
			Message:    "The provided portfolio ID does not exist in our records. Please verify the portfolio ID and try again.",
		}
	case errors.Is(err, cn.ErrProductIDNotFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrProductIDNotFound.Error(),
			Title:      "Product ID Not Found",
			Message:    "The provided product ID does not exist in our records. Please verify the product ID and try again.",
		}
	case errors.Is(err, cn.ErrLedgerIDNotFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrLedgerIDNotFound.Error(),
			Title:      "Ledger ID Not Found",
			Message:    "The provided ledger ID does not exist in our records. Please verify the ledger ID and try again.",
		}
	case errors.Is(err, cn.ErrOrganizationIDNotFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrOrganizationIDNotFound.Error(),
			Title:      "Organization ID Not Found",
			Message:    "The provided organization ID does not exist in our records. Please verify the organization ID and try again.",
		}
	case errors.Is(err, cn.ErrParentOrganizationIDNotFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrParentOrganizationIDNotFound.Error(),
			Title:      "Parent Organization ID Not Found",
			Message:    "The provided parent organization ID does not exist in our records. Please verify the parent organization ID and try again.",
		}
	case errors.Is(err, cn.ErrInvalidType):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrInvalidType.Error(),
			Title:      "Invalid Type",
			Message:    "The provided 'type' is not valid. Accepted types are currency, crypto, commodities, or others. Please provide a valid type.",
		}
	case errors.Is(err, cn.ErrTokenMissing):
		return UnauthorizedError{
			EntityType: entityType,
			Code:       cn.ErrTokenMissing.Error(),
			Title:      "Token Missing",
			Message:    "A valid token must be provided in the request header. Please include a token and try again.",
		}
	case errors.Is(err, cn.ErrInvalidToken):
		return UnauthorizedError{
			EntityType: entityType,
			Code:       cn.ErrInvalidToken.Error(),
			Title:      "Invalid Token",
			Message:    "The provided token is expired, invalid or malformed. Please provide a valid token and try again.",
		}
	case errors.Is(err, cn.ErrInsufficientPrivileges):
		return ForbiddenError{
			EntityType: entityType,
			Code:       cn.ErrInsufficientPrivileges.Error(),
			Title:      "Insufficient Privileges",
			Message:    "You do not have the necessary permissions to perform this action. Please contact your administrator if you believe this is an error.",
		}
	case errors.Is(err, cn.ErrPermissionEnforcement):
		return FailedPreconditionError{
			EntityType: entityType,
			Code:       cn.ErrPermissionEnforcement.Error(),
			Title:      "Permission Enforcement Error",
			Message:    "The enforcer is not configured properly. Please contact your administrator if you believe this is an error.",
		}
	case errors.Is(err, cn.ErrJWKFetch):
		return FailedPreconditionError{
			EntityType: entityType,
			Code:       cn.ErrJWKFetch.Error(),
			Title:      "JWK Fetch Error",
			Message:    "The JWK keys could not be fetched from the source. Please verify the source environment variable configuration and try again.",
		}
	case errors.Is(err, cn.ErrInvalidDSLFileFormat):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrInvalidDSLFileFormat.Error(),
			Title:      "Invalid DSL File Format",
			Message:    fmt.Sprintf("The submitted DSL file %s is in an incorrect format. Please ensure that the file follows the expected structure and syntax.", args...),
		}
	case errors.Is(err, cn.ErrEmptyDSLFile):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrEmptyDSLFile.Error(),
			Title:      "Empty DSL File",
			Message:    fmt.Sprintf("The submitted DSL file %s is empty. Please provide a valid file with content.", args...),
		}
	case errors.Is(err, cn.ErrMetadataKeyLengthExceeded):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrMetadataKeyLengthExceeded.Error(),
			Title:      "Metadata Key Length Exceeded",
			Message:    fmt.Sprintf("The metadata key %s exceeds the maximum allowed length of 100 characters. Please use a shorter key.", args...),
		}
	case errors.Is(err, cn.ErrMetadataValueLengthExceeded):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ErrMetadataValueLengthExceeded.Error(),
			Title:      "Metadata Value Length Exceeded",
			Message:    fmt.Sprintf("The metadata value %s exceeds the maximum allowed length of 100 characters. Please use a shorter value.", args...),
		}
	case errors.Is(err, cn.ErrAccountIDNotFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrAccountIDNotFound.Error(),
			Title:      "Account ID Not Found",
			Message:    "The provided account ID does not exist in our records. Please verify the account ID and try again.",
		}
	case errors.Is(err, cn.ErrIDsNotFoundForAccounts):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrIDsNotFoundForAccounts.Error(),
			Title:      "IDs Not Found for Accounts",
			Message:    "No accounts were found for the provided IDs. Please verify the IDs and try again.",
		}
	case errors.Is(err, cn.ErrAssetIDNotFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrAssetIDNotFound.Error(),
			Title:      "Asset ID Not Found",
			Message:    "The provided asset ID does not exist in our records. Please verify the asset ID and try again.",
		}
	case errors.Is(err, cn.ErrNoAssetsFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrNoAssetsFound.Error(),
			Title:      "No Assets Found",
			Message:    "No assets were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, cn.ErrNoProductsFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrNoProductsFound.Error(),
			Title:      "No Products Found",
			Message:    "No products were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, cn.ErrNoPortfoliosFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrNoPortfoliosFound.Error(),
			Title:      "No Portfolios Found",
			Message:    "No portfolios were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, cn.ErrNoOrganizationsFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrNoOrganizationsFound.Error(),
			Title:      "No Organizations Found",
			Message:    "No organizations were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, cn.ErrNoLedgersFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrNoLedgersFound.Error(),
			Title:      "No Ledgers Found",
			Message:    "No ledgers were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, cn.ErrBalanceUpdateFailed):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrBalanceUpdateFailed.Error(),
			Title:      "Balance Update Failed",
			Message:    "The balance could not be updated for the specified account ID. Please verify the account ID and try again.",
		}
	case errors.Is(err, cn.ErrNoAccountIDsProvided):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrNoAccountIDsProvided.Error(),
			Title:      "No Account IDs Provided",
			Message:    "No account IDs were provided for the balance update. Please provide valid account IDs and try again.",
		}
	case errors.Is(err, cn.ErrFailedToRetrieveAccountsByAliases):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrFailedToRetrieveAccountsByAliases.Error(),
			Title:      "Failed To Retrieve Accounts By Aliases",
			Message:    "The accounts could not be retrieved using the specified aliases. Please verify the aliases for accuracy and try again.",
		}
	case errors.Is(err, cn.ErrNoAccountsFound):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ErrNoAccountsFound.Error(),
			Title:      "No Accounts Found",
			Message:    "No accounts were found in the search. Please review the search criteria and try again.",
		}
	default:
		return err
	}
}
