// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

// EntityNotFoundError records an error indicating an entity was not found in any case that caused it.
// You can use it to represent a Database not found, cache not found or any other repository.
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

// ValidationError records an error indicating some validation have failed in any case that caused it.
// You can use it to represent a validation error or any other repository.
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
// You can use it to represent a Database conflict, cache or any other repository.
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

// UnauthorizedError indicates an operation that couldn't be performed because there's no user authenticated.
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

// ForbiddenError indicates an operation that couldn't be performed because the authenticated user has no sufficient privileges.
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

// UnprocessableOperationError indicates an operation that couldn't be performed because it's invalid.
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

// ServiceUnavailableError indicates a dependent service is temporarily unavailable.
type ServiceUnavailableError struct {
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
}

func (e ServiceUnavailableError) Error() string {
	return e.Message
}

// InternalServerError indicates midaz has an unexpected failure during an operation.
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

// ValidationUnknownFieldsError records an error that occurred during a validation of unknown fields.
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
	message := err.Error()

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
//   - err: The error to be validated (ref: https://github.com/LerianStudio/midaz/v3/common/constant/errors.go).
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
		constant.ErrMissingFieldsInRequest: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMissingFieldsInRequest.Error(),
			Title:      "Missing Fields in Request",
			Message:    fmt.Sprintf("Your request is missing one or more required fields: %v. Please refer to the documentation to ensure all necessary fields are included in your request.", args...),
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
			Message:    fmt.Sprintf("One or more path parameters are in an incorrect format. Please check the following parameters %v and ensure they meet the required format before trying again.", args...),
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
			Message:    fmt.Sprintf("One or more query parameters are in an incorrect format. Please check the following parameters '%v' and ensure they meet the required format before trying again.", args...),
		},
		constant.ErrInvalidDateRange: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDateRange.Error(),
			Title:      "Invalid Date Range Error",
			Message:    "Both 'initialDate' and 'finalDate' fields are required and must be in the 'yyyy-mm-dd' format. Please provide valid dates and try again.",
		},
		constant.ErrIdempotencyKey: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrIdempotencyKey.Error(),
			Title:      "Duplicate Idempotency Key",
			Message:    fmt.Sprintf("The idempotency key %v is already in use. Please provide a unique key and try again.", args...),
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
			Title:      "Race condition detected",
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
			Message:    "Transaction can't use the same account in sources and destinations",
		},
		constant.ErrBalancesCantBeDeleted: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrBalancesCantBeDeleted.Error(),
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
			Message:    "The server encountered an unexpected error while connecting to Message Broker. Please try again later or contact support.",
		},
		constant.ErrAccountAliasInvalid: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrAccountAliasInvalid.Error(),
			Title:      "Invalid Account Alias",
			Message:    "The alias contains invalid characters. Please verify the alias value and try again.",
		},
		constant.ErrOnHoldExternalAccount: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrOnHoldExternalAccount.Error(),
			Title:      "Invalid Pending Transaction",
			Message:    "External accounts cannot be used for pending transactions in source operations. Please check the accounts and try again.",
		},
		constant.ErrCommitTransactionNotPending: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrCommitTransactionNotPending.Error(),
			Title:      "Invalid Transaction Status",
			Message:    "The transaction status does not allow the requested action. Please check the transaction status.",
		},
		constant.ErrOverFlowInt64: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrOverFlowInt64.Error(),
			Title:      "Overflow Error",
			Message:    "The request could not be completed due to an overflow. Please check the values, and try again.",
		},
		constant.ErrOperationRouteTitleAlreadyExists: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrOperationRouteTitleAlreadyExists.Error(),
			Title:      "Operation Route Title Already Exists",
			Message:    "The 'title' provided already exists for the 'type' provided. Please redefine the operation route title.",
		},
		constant.ErrOperationRouteNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrOperationRouteNotFound.Error(),
			Title:      "Operation Route Not Found",
			Message:    "The provided operation route does not exist in our records. Please verify the operation route and try again.",
		},
		constant.ErrNoOperationRoutesFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoOperationRoutesFound.Error(),
			Title:      "No Operation Routes Found",
			Message:    "No operation routes were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrInvalidOperationRouteType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidOperationRouteType.Error(),
			Title:      "Invalid Operation Route Type",
			Message:    "The provided 'type' is not valid. Accepted types are 'debit' or 'credit'. Please provide a valid type.",
		},
		constant.ErrMissingOperationRoutes: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMissingOperationRoutes.Error(),
			Title:      "Missing Operation Routes in Request",
			Message:    "Your request must include at least one operation route of each type (debit and credit). Please refer to the documentation to ensure these fields are properly populated.",
		},
		constant.ErrTransactionRouteNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrTransactionRouteNotFound.Error(),
			Title:      "Transaction Route Not Found",
			Message:    "The provided transaction route does not exist in our records. Please verify the transaction route and try again.",
		},
		constant.ErrNoTransactionRoutesFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoTransactionRoutesFound.Error(),
			Title:      "No Transaction Routes Found",
			Message:    "No transaction routes were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrOperationRouteLinkedToTransactionRoutes: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrOperationRouteLinkedToTransactionRoutes.Error(),
			Title:      "Operation Route Linked to Transaction Routes",
			Message:    "The operation route cannot be deleted because it is linked to one or more transaction routes. Please remove the operation route from all transaction routes before attempting to delete it.",
		},
		constant.ErrInvalidAccountRuleType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidAccountRuleType.Error(),
			Title:      "Invalid Account Rule Type",
			Message:    "The provided 'account.ruleType' is not valid. Accepted types are 'alias' or 'account_type'. Please provide a valid rule type.",
		},
		constant.ErrInvalidAccountRuleValue: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidAccountRuleValue.Error(),
			Title:      "Invalid Account Rule Value",
			Message:    "The provided 'account.validIf' is not valid. Please provide a string for 'alias' or an array of strings for 'account_type'.",
		},
		constant.ErrInvalidAccountingRoute: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidAccountingRoute.Error(),
			Title:      "Invalid Accounting Route",
			Message:    "The transaction does not comply with the defined accounting route rules. Please verify that the transaction matches the expected operation types and account validation rules.",
		},
		constant.ErrTransactionRouteNotInformed: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionRouteNotInformed.Error(),
			Title:      "Transaction Route Not Informed",
			Message:    "The transaction route is not informed. Please inform the transaction route for this transaction.",
		},
		constant.ErrInvalidTransactionRouteID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidTransactionRouteID.Error(),
			Title:      "Invalid Transaction Route ID",
			Message:    "The provided transaction route ID is not a valid UUID format. Please provide a valid UUID for the transaction route.",
		},
		constant.ErrInvalidTransactionNonPositiveValue: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidTransactionNonPositiveValue.Error(),
			Title:      "Invalid Transaction Value",
			Message:    "Negative or zero transaction values are not allowed. The 'send.value' must be greater than zero.",
		},
		constant.ErrAccountingRouteCountMismatch: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountingRouteCountMismatch.Error(),
			Title:      "Accounting Route Count Mismatch",
			Message:    fmt.Sprintf("The operation routes count does not match the transaction route cache. Expected %v source routes and %v destination routes, but found %v source routes and %v destination routes in the transaction route.", args...),
		},
		constant.ErrAccountingRouteNotFound: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountingRouteNotFound.Error(),
			Title:      "Accounting Route Not Found",
			Message:    fmt.Sprintf("The operation route ID '%v' was not found in the transaction route cache for operation '%v'. Please verify the route configuration.", args...),
		},
		constant.ErrAccountingAliasValidationFailed: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountingAliasValidationFailed.Error(),
			Title:      "Accounting Alias Validation Failed",
			Message:    fmt.Sprintf("The operation alias '%v' does not match the expected alias '%v' defined in the accounting route rule.", args...),
		},
		constant.ErrAccountingAccountTypeValidationFailed: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountingAccountTypeValidationFailed.Error(),
			Title:      "Accounting Account Type Validation Failed",
			Message:    fmt.Sprintf("The account type '%v' does not match any of the expected account types %v defined in the accounting route rule.", args...),
		},
		constant.ErrInvalidAccountTypeKeyValue: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidAccountTypeKeyValue.Error(),
			Title:      "Invalid Characters",
			Message:    "The field 'keyValue' contains invalid characters. Use only letters, numbers, underscores and hyphens.",
		},
		constant.ErrDuplicateAccountTypeKeyValue: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicateAccountTypeKeyValue.Error(),
			Title:      "Duplicate Account Type Key Value Error",
			Message:    "An account type with the specified key value already exists for this organization and ledger. Please use a different key value or update the existing account type.",
		},
		constant.ErrAccountTypeNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAccountTypeNotFound.Error(),
			Title:      "Account Type Not Found Error",
			Message:    "The account type you are trying to access does not exist or has been removed.",
		},
		constant.ErrNoAccountTypesFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoAccountTypesFound.Error(),
			Title:      "No Account Types Found",
			Message:    "No account types were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrInvalidFutureTransactionDate: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidFutureTransactionDate.Error(),
			Title:      "Invalid Future Date Error",
			Message:    "The 'transactionDate' cannot be a future date. Please provide a valid date.",
		},
		constant.ErrInvalidPendingFutureTransactionDate: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidPendingFutureTransactionDate.Error(),
			Title:      "Invalid Field for Pending Transaction Error",
			Message:    "Pending transactions do not support the 'transactionDate' field. To proceed, please remove it from your request.",
		},
		constant.ErrDefaultBalanceNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrDefaultBalanceNotFound.Error(),
			Title:      "Default Balance Not Found",
			Message:    "Default balance must be created first for this account.",
		},
		constant.ErrDuplicatedAliasKeyValue: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicatedAliasKeyValue.Error(),
			Title:      "Duplicated Alias Key Value Error",
			Message:    "An account alias with the specified key value already exists for this organization and ledger. Please use a different key value.",
		},
		constant.ErrAdditionalBalanceNotAllowed: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAdditionalBalanceNotAllowed.Error(),
			Title:      "Additional Balance Creation Not Allowed",
			Message:    "Additional balances are not allowed for external account type.",
		},
		constant.ErrAccountCreationFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrAccountCreationFailed.Error(),
			Title:      "Account Creation Failed",
			Message:    "The account could not be created because the default balance could not be created. Please try again.",
		},
		constant.ErrInvalidDatetimeFormat: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDatetimeFormat.Error(),
			Title:      "Invalid Datetime Format Error",
			Message:    fmt.Sprintf("The '%v' parameter is in the incorrect format. Please use the '%v' format and try again.", args...),
		},
		constant.ErrHolderNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrHolderNotFound.Error(),
			Title:      "Holder ID Not Found",
			Message:    "The provided holder ID does not exist in our records. Please verify the holder ID and try again.",
		},
		constant.ErrAliasNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAliasNotFound.Error(),
			Title:      "Alias ID Not Found",
			Message:    "The provided alias ID does not exist in our records. Please verify the alias ID and try again.",
		},
		constant.ErrDocumentAssociationError: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDocumentAssociationError.Error(),
			Title:      "Document Association Error",
			Message:    "A document can only be associated with one holder.",
		},
		constant.ErrAccountAlreadyAssociated: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrAccountAlreadyAssociated.Error(),
			Title:      "Account Already Associated",
			Message:    "An accountId from ledger can only be associated with a single related account on CRM.",
		},
		constant.ErrHolderHasAliases: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrHolderHasAliases.Error(),
			Title:      "Unable to Delete Holder",
			Message:    "The holder cannot be deleted because it has one or more associated aliases.",
		},
		constant.ErrAliasClosingDateBeforeCreation: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAliasClosingDateBeforeCreation.Error(),
			Title:      "Alias Closing Date Before Creation Date",
			Message:    "The alias closing date cannot be before the creation date. Please provide a valid closing date.",
		},
		constant.ErrRelatedPartyNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrRelatedPartyNotFound.Error(),
			Title:      "Related Party Not Found",
			Message:    "The specified related party does not exist. Please verify the related party ID and try again.",
		},
		constant.ErrInvalidRelatedPartyRole: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidRelatedPartyRole.Error(),
			Title:      "Invalid Related Party Role",
			Message:    "The provided related party role is not valid. Accepted roles are: PRIMARY_HOLDER, LEGAL_REPRESENTATIVE, or RESPONSIBLE_PARTY.",
		},
		constant.ErrRelatedPartyDocumentRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrRelatedPartyDocumentRequired.Error(),
			Title:      "Related Party Document Required",
			Message:    "The related party document is required. Please provide a valid document.",
		},
		constant.ErrRelatedPartyNameRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrRelatedPartyNameRequired.Error(),
			Title:      "Related Party Name Required",
			Message:    "The related party name is required. Please provide a valid name.",
		},
		constant.ErrRelatedPartyStartDateRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrRelatedPartyStartDateRequired.Error(),
			Title:      "Related Party Start Date Required",
			Message:    "The related party start date is required. Please provide a valid start date.",
		},
		constant.ErrRelatedPartyEndDateInvalid: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrRelatedPartyEndDateInvalid.Error(),
			Title:      "Related Party End Date Invalid",
			Message:    "The related party end date must be after the start date. Please provide a valid end date.",
		},
		constant.ErrMetadataIndexAlreadyExists: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrMetadataIndexAlreadyExists.Error(),
			Title:      "Metadata Index Already Exists",
			Message:    "A metadata index with the same key already exists for this entity. Please use a different key from the existing index.",
		},
		constant.ErrMetadataIndexNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrMetadataIndexNotFound.Error(),
			Title:      "Metadata Index Not Found",
			Message:    "The specified metadata index does not exist. Please verify the index name and try again.",
		},
		constant.ErrMetadataIndexInvalidKey: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMetadataIndexInvalidKey.Error(),
			Title:      "Invalid Metadata Key Format",
			Message:    "The metadata key format is invalid. Keys must start with a letter and contain only alphanumeric characters and underscores.",
		},
		constant.ErrMetadataIndexLimitExceeded: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMetadataIndexLimitExceeded.Error(),
			Title:      "Metadata Index Limit Exceeded",
			Message:    "The maximum number of metadata indexes has been reached for this entity. Please delete unused indexes before creating new ones.",
		},
		constant.ErrMetadataIndexCreationFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrMetadataIndexCreationFailed.Error(),
			Title:      "Metadata Index Creation Failed",
			Message:    "The metadata index could not be created. Please try again later or contact support.",
		},
		constant.ErrMetadataIndexDeletionForbidden: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMetadataIndexDeletionForbidden.Error(),
			Title:      "Metadata Index Deletion Forbidden",
			Message:    "System indexes cannot be deleted. Please ensure you are deleting a custom metadata index.",
		},
		constant.ErrInvalidEntityName: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidEntityName.Error(),
			Title:      "Invalid Entity Name",
			Message:    "The provided entity name is not valid.",
		},
		constant.ErrTransactionBackupCacheFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrTransactionBackupCacheFailed.Error(),
			Title:      "Transaction Backup Cache Failed",
			Message:    "The server encountered an unexpected error while adding the transaction to the backup cache. Please try again later or contact support.",
		},
		constant.ErrTransactionBackupCacheMarshalFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrTransactionBackupCacheMarshalFailed.Error(),
			Title:      "Transaction Backup Cache Marshal Failed",
			Message:    "The server encountered an unexpected error while serializing the transaction for the backup cache. This uses the same backup mechanism. Please try again later or contact support.",
		},
		constant.ErrGRPCServiceUnavailable: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrGRPCServiceUnavailable.Error(),
			Title:      "gRPC Service Unavailable",
			Message:    "The balance service is temporarily unavailable. Please try again later.",
		},
		constant.ErrMissingRequiredQueryParameter: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMissingRequiredQueryParameter.Error(),
			Title:      "Missing Required Query Parameter",
			Message:    fmt.Sprintf("The required query parameter '%v' is missing from the request.", args...),
		},
		constant.ErrInvalidTimestamp: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidTimestamp.Error(),
			Title:      "Invalid Timestamp",
			Message:    fmt.Sprintf("The provided timestamp '%v' is invalid. Timestamps cannot be in the future.", args...),
		},
		constant.ErrNoBalanceDataAtTimestamp: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoBalanceDataAtTimestamp.Error(),
			Title:      "No Balance Data at Date",
			Message:    "No balance data is available at the specified date.",
		},
		constant.ErrJSONNestingDepthExceeded: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrJSONNestingDepthExceeded.Error(),
			Title:      "JSON Nesting Depth Exceeded",
			Message:    "The JSON payload exceeds the maximum allowed nesting depth of 10 levels. Please flatten your data structure.",
		},
		constant.ErrJSONKeyCountExceeded: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrJSONKeyCountExceeded.Error(),
			Title:      "JSON Key Count Exceeded",
			Message:    "The JSON payload exceeds the maximum allowed number of keys (100). Please reduce the number of keys in your payload.",
		},
	}

	if mappedError, found := errorMap[err]; found {
		return mappedError
	}

	return err
}

func HandleKnownBusinessValidationErrors(err error) error {
	switch {
	case err.Error() == constant.ErrTransactionAmbiguous.Error():
		return ValidateBusinessError(constant.ErrTransactionAmbiguous, "ValidateSendSourceAndDistribute")
	case err.Error() == constant.ErrTransactionValueMismatch.Error():
		return ValidateBusinessError(constant.ErrTransactionValueMismatch, "ValidateSendSourceAndDistribute")
	default:
		return err
	}
}
