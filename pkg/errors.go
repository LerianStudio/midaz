package pkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/pkg/constant"
)

// EntityNotFoundError records an error indicating an entity was not found in any case that caused it.
// You can use it to representing a Database not found, cache not found or any other repository.
type EntityNotFoundError struct {
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
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
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
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
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
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
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
}

func (e UnprocessableOperationError) Error() string {
	return e.Message
}

// HTTPError indicates an http error raised in an http client.
type HTTPError struct {
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
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
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
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
	Message    string           `json:"message,omitempty"`
	Code       string           `json:"code,omitempty"`
	Err        error            `json:"err,omitempty"`
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
	Message    string        `json:"message,omitempty"`
	Code       string        `json:"code,omitempty"`
	Err        error         `json:"err,omitempty"`
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
		Code:       constant.ErrInternalServer.Error(),
		Title:      "Internal Server Error",
		Message:    "The server encountered an unexpected error. Please try again later or contact support.",
		Err:        err,
	}
}

// ValidateUnmarshallingError validates the error and returns an appropriate ResponseError.
func ValidateUnmarshallingError(err error) error {
	var message = err.Error()

	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		field := ute.Field
		expected := ute.Type.String()
		actual := ute.Value
		message = fmt.Sprintf("invalid value for field '%s': expected type '%s', but got '%s'", field, expected, actual)
	}

	return ResponseError{
		Code:    constant.ErrInvalidRequestBody.Error(),
		Title:   "Unmarshalling error",
		Message: message,
	}
}

// ValidateBadRequestFieldsError validates the error and returns the appropriate bad request error code, title, message, and the invalid fields.
//
// Parameters:
// - requiredFields: A map of missing required fields and their error messages.
// - knownInvalidFields: A map of known invalid fields and their validation errors.
// - entityType: The type of the entity associated with the error.
// - unknownFields: A map of unknown fields and their error messages.
//
// Returns:
// - An error indicating the validation result, which could be a ValidationUnknownFieldsError or a ValidationKnownFieldsError.
func ValidateBadRequestFieldsError(requiredFields, knownInvalidFields map[string]string, entityType string, unknownFields map[string]any) error {
	if len(unknownFields) == 0 && len(knownInvalidFields) == 0 && len(requiredFields) == 0 {
		return errors.New("expected knownInvalidFields, unknownFields and requiredFields to be non-empty")
	}

	if len(unknownFields) > 0 {
		return ValidationUnknownFieldsError{
			EntityType: entityType,
			Code:       constant.ErrUnexpectedFieldsInTheRequest.Error(),
			Title:      "Unexpected Fields in the Request",
			Message:    "The request body contains more fields than expected. Please send only the allowed fields as per the documentation. The unexpected fields are listed in the fields object.",
			Fields:     unknownFields,
		}
	}

	if len(requiredFields) > 0 {
		return ValidationKnownFieldsError{
			EntityType: entityType,
			Code:       constant.ErrMissingFieldsInRequest.Error(),
			Title:      "Missing Fields in Request",
			Message:    "Your request is missing one or more required fields. Please refer to the documentation to ensure all necessary fields are included in your request.",
			Fields:     requiredFields,
		}
	}

	return ValidationKnownFieldsError{
		EntityType: entityType,
		Code:       constant.ErrBadRequest.Error(),
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
func ValidateBusinessError(err error, entityType string, args ...any) error {
	errorMap := map[error]error{
		constant.ErrDuplicateLedger: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicateLedger.Error(),
			Title:      "Duplicate Ledger Error",
			Message:    fmt.Sprintf("A ledger with the name %v already exists in the division %v. Please rename the ledger or choose a different division to attach it to.", args...),
		},
		constant.ErrLedgerNameConflict: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrLedgerNameConflict.Error(),
			Title:      "Ledger Name Conflict",
			Message:    fmt.Sprintf("A ledger named %v already exists in your organization. Please rename the ledger, or if you want to use the same name, consider creating a new ledger for a different division.", args...),
		},
		constant.ErrAssetNameOrCodeDuplicate: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrAssetNameOrCodeDuplicate.Error(),
			Title:      "Asset Name or Code Duplicate",
			Message:    "An asset with the same name or code already exists in your ledger. Please modify the name or code of your new asset.",
		},
		constant.ErrCodeUppercaseRequirement: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCodeUppercaseRequirement.Error(),
			Title:      "Code Uppercase Requirement",
			Message:    "The code must be in uppercase. Please ensure that the code is in uppercase format and try again.",
		},
		constant.ErrCurrencyCodeStandardCompliance: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCurrencyCodeStandardCompliance.Error(),
			Title:      "Currency Code Standard Compliance",
			Message:    "Currency-type assets must comply with the ISO-4217 standard. Please use a currency code that conforms to ISO-4217 guidelines.",
		},
		constant.ErrUnmodifiableField: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrUnmodifiableField.Error(),
			Title:      "Unmodifiable Field Error",
			Message:    "Your request includes a field that cannot be modified. Please review your request and try again, removing any uneditable fields. Please refer to the documentation for guidance.",
		},
		constant.ErrEntityNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrEntityNotFound.Error(),
			Title:      "Entity Not Found",
			Message:    "No entity was found for the given ID. Please make sure to use the correct ID for the entity you are trying to manage.",
		},
		constant.ErrActionNotPermitted: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrActionNotPermitted.Error(),
			Title:      "Action Not Permitted",
			Message:    "The action you are attempting is not allowed in the current environment. Please refer to the documentation for guidance.",
		},
		constant.ErrAccountTypeImmutable: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountTypeImmutable.Error(),
			Title:      "Account Type Immutable",
			Message:    "The account type specified cannot be modified. Please ensure the correct account type is being used and try again.",
		},
		constant.ErrInactiveAccountType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInactiveAccountType.Error(),
			Title:      "Inactive Account Type Error",
			Message:    "The account type specified cannot be set to INACTIVE. Please ensure the correct account type is being used and try again.",
		},
		constant.ErrAccountBalanceDeletion: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountBalanceDeletion.Error(),
			Title:      "Account Balance Deletion Error",
			Message:    "An account or sub-account cannot be deleted if it has a remaining balance. Please ensure all remaining balances are transferred to another account before attempting to delete.",
		},

		constant.ErrAccountBalanceDeletion: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountBalanceDeletion.Error(),
			Title:      "Account Balance Deletion Error",
			Message:    "An account or sub-account cannot be deleted if it has a remaining balance. Please ensure all remaining balances are transferred to another account before attempting to delete.",
		},
		constant.ErrResourceAlreadyDeleted: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrResourceAlreadyDeleted.Error(),
			Title:      "Resource Already Deleted",
			Message:    "The resource you are trying to delete has already been deleted. Ensure you are using the correct ID and try again.",
		},
		constant.ErrSegmentIDInactive: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrSegmentIDInactive.Error(),
			Title:      "Segment ID Inactive",
			Message:    "The Segment ID you are attempting to use is inactive. Please use another Segment ID and try again.",
		},
		constant.ErrDuplicateSegmentName: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicateSegmentName.Error(),
			Title:      "Duplicate Segment Name Error",
			Message:    fmt.Sprintf("A segment with the name %v already exists for this ledger ID %v. Please try again with a different ledger or name.", args...),
		},
		constant.ErrBalanceRemainingDeletion: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrBalanceRemainingDeletion.Error(),
			Title:      "Balance Remaining Deletion Error",
			Message:    "The asset cannot be deleted because there is a remaining balance. Please ensure all balances are cleared before attempting to delete again.",
		},
		constant.ErrInvalidScriptFormat: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrInvalidScriptFormat.Error(),
			Title:      "Invalid Script Format Error",
			Message:    "The script provided in your request is invalid or in an unsupported format. Please verify the script format and try again.",
		},
		constant.ErrInsufficientFunds: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrInsufficientFunds.Error(),
			Title:      "Insufficient Funds Error",
			Message:    "The transaction could not be completed due to insufficient funds in the account. Please add sufficient funds to your account and try again.",
		},
		constant.ErrAccountIneligibility: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrAccountIneligibility.Error(),
			Title:      "Account Ineligibility Error",
			Message:    "One or more accounts listed in the transaction are not eligible to participate. Please review the account statuses and try again.",
		},
		constant.ErrAliasUnavailability: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrAliasUnavailability.Error(),
			Title:      "Alias Unavailability Error",
			Message:    fmt.Sprintf("The alias %v is already in use. Please choose a different alias and try again.", args...),
		},
		constant.ErrParentTransactionIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrParentTransactionIDNotFound.Error(),
			Title:      "Parent Transaction ID Not Found",
			Message:    fmt.Sprintf("The parentTransactionId %v does not correspond to any existing transaction. Please review the ID and try again.", args...),
		},
		constant.ErrImmutableField: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrImmutableField.Error(),
			Title:      "Immutable Field Error",
			Message:    fmt.Sprintf("The %v field cannot be modified. Please remove this field from your request and try again.", args...),
		},
		constant.ErrTransactionTimingRestriction: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionTimingRestriction.Error(),
			Title:      "Transaction Timing Restriction",
			Message:    fmt.Sprintf("You can only perform another transaction using %v of %f from %v to %v after %v. Please wait until the specified time to try again.", args...),
		},
		constant.ErrAccountStatusTransactionRestriction: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountStatusTransactionRestriction.Error(),
			Title:      "Account Status Transaction Restriction",
			Message:    "The current statuses of the source and/or destination accounts do not permit transactions. Change the account status(es) and try again.",
		},
		constant.ErrInsufficientAccountBalance: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInsufficientAccountBalance.Error(),
			Title:      "Insufficient Account Balance Error",
			Message:    fmt.Sprintf("The account %v does not have sufficient balance. Please try again with an amount that is less than or equal to the available balance.", args...),
		},
		constant.ErrTransactionMethodRestriction: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionMethodRestriction.Error(),
			Title:      "Transaction Method Restriction",
			Message:    fmt.Sprintf("Transactions involving %v are not permitted for the specified source and/or destination. Please try again using accounts that allow transactions with %v.", args...),
		},
		constant.ErrDuplicateTransactionTemplateCode: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicateTransactionTemplateCode.Error(),
			Title:      "Duplicate Transaction Template Code Error",
			Message:    fmt.Sprintf("A transaction template with the code %v already exists for your ledger. Please use a different code and try again.", args...),
		},
		constant.ErrDuplicateAssetPair: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicateAssetPair.Error(),
			Title:      "Duplicate Asset Pair Error",
			Message:    fmt.Sprintf("A pair for the assets %v%v already exists with the ID %v. Please update the existing entry instead of creating a new one.", args...),
		},
		constant.ErrInvalidParentAccountID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidParentAccountID.Error(),
			Title:      "Invalid Parent Account ID",
			Message:    "The specified parent account ID does not exist. Please verify the ID is correct and attempt your request again.",
		},
		constant.ErrMismatchedAssetCode: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMismatchedAssetCode.Error(),
			Title:      "Mismatched Asset Code",
			Message:    "The parent account ID you provided is associated with a different asset code than the one specified in your request. Please make sure the asset code matches that of the parent account, or use a different parent account ID and try again.",
		},
		constant.ErrChartTypeNotFound: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrChartTypeNotFound.Error(),
			Title:      "Chart Type Not Found",
			Message:    fmt.Sprintf("The chart type %v does not exist. Please provide a valid chart type and refer to the documentation if you have any questions.", args...),
		},
		constant.ErrInvalidCountryCode: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidCountryCode.Error(),
			Title:      "Invalid Country Code",
			Message:    "The provided country code in the 'address.country' field does not conform to the ISO-3166 alpha-2 standard. Please provide a valid alpha-2 country code.",
		},
		constant.ErrInvalidCodeFormat: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidCodeFormat.Error(),
			Title:      "Invalid Code Format",
			Message:    "The 'code' field must be alphanumeric, in upper case, and must contain at least one letter. Please provide a valid code.",
		},
		constant.ErrAssetCodeNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAssetCodeNotFound.Error(),
			Title:      "Asset Code Not Found",
			Message:    "The provided asset code does not exist in our records. Please verify the asset code and try again.",
		},
		constant.ErrPortfolioIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrPortfolioIDNotFound.Error(),
			Title:      "Portfolio ID Not Found",
			Message:    "The provided portfolio ID does not exist in our records. Please verify the portfolio ID and try again.",
		},
		constant.ErrSegmentIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrSegmentIDNotFound.Error(),
			Title:      "Segment ID Not Found",
			Message:    "The provided segment ID does not exist in our records. Please verify the segment ID and try again.",
		},
		constant.ErrLedgerIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrLedgerIDNotFound.Error(),
			Title:      "Ledger ID Not Found",
			Message:    "The provided ledger ID does not exist in our records. Please verify the ledger ID and try again.",
		},
		constant.ErrOrganizationIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrOrganizationIDNotFound.Error(),
			Title:      "Organization ID Not Found",
			Message:    "The provided organization ID does not exist in our records. Please verify the organization ID and try again.",
		},
		constant.ErrParentOrganizationIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrParentOrganizationIDNotFound.Error(),
			Title:      "Parent Organization ID Not Found",
			Message:    "The provided parent organization ID does not exist in our records. Please verify the parent organization ID and try again.",
		},
		constant.ErrInvalidType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidType.Error(),
			Title:      "Invalid Type",
			Message:    "The provided 'type' is not valid. Accepted types are currency, crypto, commodities, or others. Please provide a valid type.",
		},
		constant.ErrTokenMissing: UnauthorizedError{
			EntityType: entityType,
			Code:       constant.ErrTokenMissing.Error(),
			Title:      "Token Missing",
			Message:    "A valid token must be provided in the request header. Please include a token and try again.",
		},
		constant.ErrInvalidToken: UnauthorizedError{
			EntityType: entityType,
			Code:       constant.ErrInvalidToken.Error(),
			Title:      "Invalid Token",
			Message:    "The provided token is expired, invalid or malformed. Please provide a valid token and try again.",
		},
		constant.ErrInsufficientPrivileges: ForbiddenError{
			EntityType: entityType,
			Code:       constant.ErrInsufficientPrivileges.Error(),
			Title:      "Insufficient Privileges",
			Message:    "You do not have the necessary permissions to perform this action. Please contact your administrator if you believe this is an error.",
		},
		constant.ErrPermissionEnforcement: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrPermissionEnforcement.Error(),
			Title:      "Permission Enforcement Error",
			Message:    "The enforcer is not configured properly. Please contact your administrator if you believe this is an error.",
		},
		constant.ErrJWKFetch: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrJWKFetch.Error(),
			Title:      "JWK Fetch Error",
			Message:    "The JWK keys could not be fetched from the source. Please verify the source environment variable configuration and try again.",
		},
		constant.ErrInvalidDSLFileFormat: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDSLFileFormat.Error(),
			Title:      "Invalid DSL File Format",
			Message:    fmt.Sprintf("The submitted DSL file %v is in an incorrect format. Please ensure that the file follows the expected structure and syntax.", args...),
		},
		constant.ErrEmptyDSLFile: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrEmptyDSLFile.Error(),
			Title:      "Empty DSL File",
			Message:    fmt.Sprintf("The submitted DSL file %v is empty. Please provide a valid file with content.", args...),
		},
		constant.ErrMetadataKeyLengthExceeded: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMetadataKeyLengthExceeded.Error(),
			Title:      "Metadata Key Length Exceeded",
			Message:    fmt.Sprintf("The metadata key %v exceeds the maximum allowed length of %v characters. Please use a shorter key.", args...),
		},
		constant.ErrMetadataValueLengthExceeded: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMetadataValueLengthExceeded.Error(),
			Title:      "Metadata Value Length Exceeded",
			Message:    fmt.Sprintf("The metadata value %v exceeds the maximum allowed length of %v characters. Please use a shorter value.", args...),
		},
		constant.ErrAccountIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAccountIDNotFound.Error(),
			Title:      "Account ID Not Found",
			Message:    "The provided account ID does not exist in our records. Please verify the account ID and try again.",
		},
		constant.ErrIDsNotFoundForAccounts: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrIDsNotFoundForAccounts.Error(),
			Title:      "IDs Not Found for Accounts",
			Message:    "No accounts were found for the provided IDs. Please verify the IDs and try again.",
		},
		constant.ErrAssetIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAssetIDNotFound.Error(),
			Title:      "Asset ID Not Found",
			Message:    "The provided asset ID does not exist in our records. Please verify the asset ID and try again.",
		},
		constant.ErrNoAssetsFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoAssetsFound.Error(),
			Title:      "No Assets Found",
			Message:    "No assets were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrNoSegmentsFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoSegmentsFound.Error(),
			Title:      "No Segments Found",
			Message:    "No segments were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrNoPortfoliosFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoPortfoliosFound.Error(),
			Title:      "No Portfolios Found",
			Message:    "No portfolios were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrNoOrganizationsFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoOrganizationsFound.Error(),
			Title:      "No Organizations Found",
			Message:    "No organizations were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrNoLedgersFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoLedgersFound.Error(),
			Title:      "No Ledgers Found",
			Message:    "No ledgers were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrBalanceUpdateFailed: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrBalanceUpdateFailed.Error(),
			Title:      "Balance Update Failed",
			Message:    "The balance could not be updated for the specified account ID. Please verify the account ID and try again.",
		},
		constant.ErrNoAccountIDsProvided: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoAccountIDsProvided.Error(),
			Title:      "No Account IDs Provided",
			Message:    "No account IDs were provided for the balance update. Please provide valid account IDs and try again.",
		},
		constant.ErrFailedToRetrieveAccountsByAliases: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrFailedToRetrieveAccountsByAliases.Error(),
			Title:      "Failed To Retrieve Accounts By Aliases",
			Message:    "The accounts could not be retrieved using the specified aliases. Please verify the aliases for accuracy and try again.",
		},
		constant.ErrNoAccountsFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoAccountsFound.Error(),
			Title:      "No Accounts Found",
			Message:    "No accounts were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrInvalidPathParameter: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidPathParameter.Error(),
			Title:      "Invalid Path Parameter",
			Message:    fmt.Sprintf("One or more path parameters are in an incorrect format. Please check the following parameters %v and ensure they meet the required format before trying again.", args),
		},
		constant.ErrInvalidAccountType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidAccountType.Error(),
			Title:      "Invalid Account Type",
			Message:    "The provided 'type' is not valid.",
		},
		constant.ErrInvalidMetadataNesting: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidMetadataNesting.Error(),
			Title:      "Invalid Metadata Nesting",
			Message:    fmt.Sprintf("The metadata object cannot contain nested values. Please ensure that the value %v is not nested and try again.", args...),
		},
		constant.ErrOperationIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrOperationIDNotFound.Error(),
			Title:      "Operation ID Not Found",
			Message:    "The provided operation ID does not exist in our records. Please verify the operation ID and try again.",
		},
		constant.ErrNoOperationsFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoOperationsFound.Error(),
			Title:      "No Operations Found",
			Message:    "No operations were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrTransactionIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrTransactionIDNotFound.Error(),
			Title:      "Transaction ID Not Found",
			Message:    "The provided transaction ID does not exist in our records. Please verify the transaction ID and try again.",
		},
		constant.ErrNoTransactionsFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoTransactionsFound.Error(),
			Title:      "No Transactions Found",
			Message:    "No transactions were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrInvalidTransactionType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidTransactionType.Error(),
			Title:      "Invalid Transaction Type",
			Message:    fmt.Sprintf("Only one transaction type ('amount', 'share', or 'remaining') must be specified in the '%v' field for each entry. Please review your input and try again.", args...),
		},
		constant.ErrTransactionValueMismatch: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionValueMismatch.Error(),
			Title:      "Transaction Value Mismatch",
			Message:    "The values for the source, the destination, or both do not match the specified transaction amount. Please verify the values and try again.",
		},
		constant.ErrForbiddenExternalAccountManipulation: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrForbiddenExternalAccountManipulation.Error(),
			Title:      "External Account Modification Prohibited",
			Message:    "Accounts of type 'external' cannot be deleted or modified as they are used for traceability with external systems. Please review your request and ensure operations are only performed on internal accounts.",
		},
		constant.ErrAuditRecordNotRetrieved: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAuditRecordNotRetrieved.Error(),
			Title:      "Audit Record Not Retrieved",
			Message:    fmt.Sprintf("The record %v could not be retrieved for audit. Please verify that the submitted data is correct and try again.", args...),
		},
		constant.ErrAuditTreeRecordNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAuditTreeRecordNotFound.Error(),
			Title:      "Audit Tree Record Not Found",
			Message:    fmt.Sprintf("The record %v does not exist in the audit tree. Please ensure the audit tree is available and try again.", args...),
		},
		constant.ErrInvalidDateFormat: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDateFormat.Error(),
			Title:      "Invalid Date Format Error",
			Message:    "The 'initialDate', 'finalDate', or both are in the incorrect format. Please use the 'yyyy-mm-dd' format and try again.",
		},
		constant.ErrInvalidFinalDate: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidFinalDate.Error(),
			Title:      "Invalid Final Date Error",
			Message:    "The 'finalDate' cannot be earlier than the 'initialDate'. Please verify the dates and try again.",
		},
		constant.ErrDateRangeExceedsLimit: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrDateRangeExceedsLimit.Error(),
			Title:      "Date Range Exceeds Limit Error",
			Message:    fmt.Sprintf("The range between 'initialDate' and 'finalDate' exceeds the permitted limit of %v months. Please adjust the dates and try again.", args...),
		},
		constant.ErrPaginationLimitExceeded: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrPaginationLimitExceeded.Error(),
			Title:      "Pagination Limit Exceeded",
			Message:    fmt.Sprintf("The pagination limit exceeds the maximum allowed of %v items per page. Please verify the limit and try again.", args...),
		},
		constant.ErrInvalidSortOrder: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidSortOrder.Error(),
			Title:      "Invalid Sort Order",
			Message:    "The 'sort_order' field must be 'asc' or 'desc'. Please provide a valid sort order and try again.",
		},
		constant.ErrInvalidQueryParameter: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidQueryParameter.Error(),
			Title:      "Invalid Query Parameter",
			Message:    fmt.Sprintf("One or more query parameters are in an incorrect format. Please check the following parameters '%v' and ensure they meet the required format before trying again.", args),
		},
		constant.ErrInvalidDateRange: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDateRange.Error(),
			Title:      "Invalid Date Range Error",
			Message:    "Both 'initialDate' and 'finalDate' fields are required and must be in the 'yyyy-mm-dd' format. Please provide valid dates and try again.",
		},
		constant.ErrIdempotencyKey: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrIdempotencyKey.Error(),
			Title:      "Duplicate Idempotency Key",
			Message:    fmt.Sprintf("The idempotency key %v is already in use. Please provide a unique key and try again.", args),
		},
		constant.ErrAccountAliasNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAccountAliasNotFound.Error(),
			Title:      "Account Alias Not Found",
			Message:    "The provided account Alias does not exist in our records. Please verify the account Alias and try again.",
		},
		constant.ErrLockVersionAccountBalance: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLockVersionAccountBalance.Error(),
			Title:      "Race conditioning detected",
			Message:    "A race condition was detected while processing your request. Please try again",
		},
		constant.ErrTransactionIDHasAlreadyParentTransaction: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionIDHasAlreadyParentTransaction.Error(),
			Title:      "Transaction Revert already exist",
			Message:    "Transaction revert already exists. Please try again.",
		},
		constant.ErrTransactionIDIsAlreadyARevert: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionIDIsAlreadyARevert.Error(),
			Title:      "Transaction is already a reversal",
			Message:    "Transaction is already a reversal. Please try again",
		},
		constant.ErrTransactionCantRevert: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionCantRevert.Error(),
			Title:      "Transaction can't be reverted",
			Message:    "Transaction can't be reverted. Please try again",
		},
		constant.ErrTransactionAmbiguous: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionAmbiguous.Error(),
			Title:      "Transaction ambiguous account",
			Message:    "Transaction can't be used same account in sources ans destinations",
		},
		constant.ErrBalancesCantDeleted: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrBalancesCantDeleted.Error(),
			Title:      "Balance cannot be deleted",
			Message:    "Balance cannot be deleted because it still has funds in it.",
		},
		constant.ErrParentIDSameID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrParentIDSameID.Error(),
			Title:      "ID cannot be used as the parent ID",
			Message:    "The provided ID cannot be used as the parent ID. Please choose a different one.",
		},
		constant.ErrMessageBrokerUnavailable: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrMessageBrokerUnavailable.Error(),
			Title:      "Message Broker Unavailable",
			Message:    "The server encountered an unexpected error while connecting to Message Broker. Please try again later or contact support."},
		constant.ErrAccountAliasInvalid: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrAccountAliasInvalid.Error(),
			Title:      "Invalid Account Alias",
			Message:    "The alias contains invalid characters. Please verify the alias value and try again.",
		},
		constant.ErrOverFlowInt64: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrOverFlowInt64.Error(),
			Title:      "Overflow Error",
			Message:    "The request could not be completed due to an overflow. Please check the values, and try again.",
		},
	}

	if mappedError, found := errorMap[err]; found {
		return mappedError
	}

	return err
}
