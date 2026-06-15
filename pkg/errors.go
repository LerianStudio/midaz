// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
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
	if strings.TrimSpace(e.Code) != "" {
		return fmt.Sprintf("%s - %s", e.Code, e.Message)
	}

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

// IsBusinessError reports whether err is a business/domain error (validation, not-found,
// conflict, auth) as opposed to a technical/infrastructure error. Business errors should
// use HandleSpanBusinessErrorEvent so they don't pollute error-rate metrics with expected
// conditions. Wrapped errors are unwrapped via errors.As.
func IsBusinessError(err error) bool {
	if err == nil {
		return false
	}

	var notFoundErr EntityNotFoundError
	if errors.As(err, &notFoundErr) {
		return true
	}

	var validationErr ValidationError
	if errors.As(err, &validationErr) {
		return true
	}

	var conflictErr EntityConflictError
	if errors.As(err, &conflictErr) {
		return true
	}

	var unauthorizedErr UnauthorizedError
	if errors.As(err, &unauthorizedErr) {
		return true
	}

	var forbiddenErr ForbiddenError
	if errors.As(err, &forbiddenErr) {
		return true
	}

	var unprocessableErr UnprocessableOperationError
	if errors.As(err, &unprocessableErr) {
		return true
	}

	var validationKnownFieldsErr ValidationKnownFieldsError
	if errors.As(err, &validationKnownFieldsErr) {
		return true
	}

	var validationUnknownFieldsErr ValidationUnknownFieldsError

	return errors.As(err, &validationUnknownFieldsErr)
}

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
//   - err: The error to be validated (ref: https://github.com/LerianStudio/midaz/v4/common/constant/errors.go).
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
		constant.ErrActionNotPermitted: UnprocessableOperationError{
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
		constant.ErrAccountTypeImmutable: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrAccountTypeImmutable.Error(),
			Title:      "Account Type Immutable",
			Message:    "The account type specified cannot be modified. Please ensure the correct account type is being used and try again.",
		},
		constant.ErrInactiveAccountType: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrInactiveAccountType.Error(),
			Title:      "Inactive Account Type Error",
			Message:    "The account type specified cannot be set to INACTIVE. Please ensure the correct account type is being used and try again.",
		},
		constant.ErrAccountBalanceDeletion: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrAccountBalanceDeletion.Error(),
			Title:      "Account Balance Deletion Error",
			Message:    "An account or sub-account cannot be deleted if it has a remaining balance. Please ensure all remaining balances are transferred to another account before attempting to delete.",
		},
		constant.ErrResourceAlreadyDeleted: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrResourceAlreadyDeleted.Error(),
			Title:      "Resource Already Deleted",
			Message:    "The resource you are trying to delete has already been deleted. Ensure you are using the correct ID and try again.",
		},
		constant.ErrSegmentIDInactive: UnprocessableOperationError{
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
		constant.ErrBalanceRemainingDeletion: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrBalanceRemainingDeletion.Error(),
			Title:      "Balance Remaining Deletion Error",
			Message:    "The asset cannot be deleted because there is a remaining balance. Please ensure all balances are cleared before attempting to delete again.",
		},
		constant.ErrInvalidScriptFormat: ValidationError{
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
		constant.ErrOverdraftLimitExceeded: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrOverdraftLimitExceeded.Error(),
			Title:      "Overdraft Limit Exceeded Error",
			Message:    "The transaction could not be completed because it would exceed the configured overdraft limit for the balance. Please reduce the amount or increase the overdraft limit and try again.",
		},
		constant.ErrTransactionReservationDenied: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionReservationDenied.Error(),
			Title:      "Transaction Reservation Denied Error",
			Message:    "The transaction could not be completed because it would exceed a configured usage limit. Please reduce the amount or wait for the limit window to reset and try again.",
		},
		constant.ErrTransactionReservationUnavailable: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrTransactionReservationUnavailable.Error(),
			Title:      "Transaction Reservation Unavailable Error",
			Message:    "The transaction could not be completed because the usage-limit service is temporarily unavailable and this ledger is configured to reject transactions when it cannot be reached. Please retry shortly.",
		},
		constant.ErrDirectOperationOnInternalBalance: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrDirectOperationOnInternalBalance.Error(),
			Title:      "Direct Operation On Internal Balance Error",
			Message:    "Direct operations on internal-scope balances are not permitted. Internal balances are managed exclusively by the system. Please target a transactional balance and try again.",
		},
		constant.ErrDeletionOfInternalBalance: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrDeletionOfInternalBalance.Error(),
			Title:      "Deletion Of Internal Balance Error",
			Message:    "Internal-scope balances cannot be deleted. They are managed by the system and must remain for accounting consistency.",
		},
		constant.ErrReservedBalanceKey: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrReservedBalanceKey.Error(),
			Title:      "Reserved Balance Key Error",
			Message:    fmt.Sprintf("The balance key %v is reserved for system use and cannot be created through the public API. Please choose a different key and try again.", args...),
		},
		constant.ErrInvalidBalanceDirection: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidBalanceDirection.Error(),
			Title:      "Invalid Balance Direction Error",
			Message:    fmt.Sprintf("The balance direction %v is not supported. Allowed values are \"credit\" and \"debit\".", args...),
		},
		constant.ErrInvalidBalanceSettings: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidBalanceSettings.Error(),
			Title:      "Invalid Balance Settings Error",
			Message:    "The balance settings payload is invalid. Please review overdraft, limit, and scope fields and try again.",
		},
		constant.ErrOverdraftLimitBelowUsage: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrOverdraftLimitBelowUsage.Error(),
			Title:      "Overdraft Limit Below Usage Error",
			Message:    "The new overdraft limit is below the amount currently used. Raise the limit above the current usage or repay the outstanding amount before reducing the limit.",
		},
		constant.ErrStaleBalanceVersion: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrStaleBalanceVersion.Error(),
			Title:      "Stale Balance Version Error",
			Message:    "The balance was modified by another transaction between read and write. Please retry the operation after reading the latest balance state.",
		},
		constant.ErrUpdateOfInternalBalance: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrUpdateOfInternalBalance.Error(),
			Title:      "Update Of Internal Balance Error",
			Message:    "Internal balances are system-managed and cannot be updated through the public API. They are maintained by the system for accounting consistency.",
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
		constant.ErrAccountStatusTransactionRestriction: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrAccountStatusTransactionRestriction.Error(),
			Title:      "Account Status Transaction Restriction",
			Message:    "The current statuses of the source and/or destination accounts do not permit transactions. Change the account status(es) and try again.",
		},
		constant.ErrInsufficientAccountBalance: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrInsufficientAccountBalance.Error(),
			Title:      "Insufficient Account Balance Error",
			Message:    fmt.Sprintf("The account %v does not have sufficient balance. Please try again with an amount that is less than or equal to the available balance.", args...),
		},
		constant.ErrTransactionMethodRestriction: UnprocessableOperationError{
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
		constant.ErrMismatchedAssetCode: UnprocessableOperationError{
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
		constant.ErrTransactionValueMismatch: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionValueMismatch.Error(),
			Title:      "Transaction Value Mismatch",
			Message:    "The values for the source, the destination, or both do not match the specified transaction amount. Please verify the values and try again.",
		},
		constant.ErrForbiddenExternalAccountManipulation: UnprocessableOperationError{
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
		constant.ErrLockVersionAccountBalance: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrLockVersionAccountBalance.Error(),
			Title:      "Race condition detected",
			Message:    "A race condition was detected while processing your request. Please try again",
		},
		constant.ErrTransactionIDHasAlreadyParentTransaction: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrTransactionIDHasAlreadyParentTransaction.Error(),
			Title:      "Transaction Revert already exist",
			Message:    "Transaction revert already exists. Please try again.",
		},
		constant.ErrTransactionIDIsAlreadyARevert: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrTransactionIDIsAlreadyARevert.Error(),
			Title:      "Transaction is already a reversal",
			Message:    "Transaction is already a reversal. Please try again",
		},
		constant.ErrTransactionCantRevert: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionCantRevert.Error(),
			Title:      "Transaction can't be reverted",
			Message:    "Transaction can't be reverted. Please try again",
		},
		constant.ErrTransactionAmbiguous: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionAmbiguous.Error(),
			Title:      "Transaction ambiguous account",
			Message:    "Transaction can't use the same account in sources and destinations",
		},
		constant.ErrBalancesCantBeDeleted: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrBalancesCantBeDeleted.Error(),
			Title:      "Balance cannot be deleted",
			Message:    "Balance cannot be deleted because it still has funds in it.",
		},
		constant.ErrParentIDSameID: UnprocessableOperationError{
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
		constant.ErrAccountAliasInvalid: ValidationError{
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
		constant.ErrCommitTransactionNotPending: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrCommitTransactionNotPending.Error(),
			Title:      "Invalid Transaction Status",
			Message:    "The transaction status does not allow the requested action. Please check the transaction status.",
		},
		constant.ErrPendingTransactionLocked: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrPendingTransactionLocked.Error(),
			Title:      "Transaction Locked",
			Message:    "This transaction is currently being processed by another request. Please retry shortly.",
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
			Message:    "The provided 'operationType' is not valid. Accepted types are 'source', 'destination', or 'bidirectional'. Please provide a valid type.",
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
		constant.ErrCorruptedAccountRule: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrCorruptedAccountRule.Error(),
			Title:      "Corrupted Account Rule",
			Message:    "The account rule data in the operation route is internally inconsistent (unknown rule type or malformed validIf value). This indicates a data integrity issue — please verify the operation route configuration.",
		},
		constant.ErrTransactionRouteNotInformed: UnprocessableOperationError{
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
		constant.ErrAccountingRouteCountMismatch: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrAccountingRouteCountMismatch.Error(),
			Title:      "Accounting Route Count Mismatch",
			Message:    fmt.Sprintf("The operation routes count does not match the transaction route cache. Expected %v source and %v destination operations, but the route has %v source, %v destination, and %v bidirectional operation routes.", args...),
		},
		constant.ErrAccountingRouteNotFound: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrAccountingRouteNotFound.Error(),
			Title:      "Accounting Route Not Found",
			Message:    fmt.Sprintf("The operation route ID '%v' was not found in the transaction route cache for operation '%v'. Please verify the route configuration.", args...),
		},
		constant.ErrAccountingAliasValidationFailed: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrAccountingAliasValidationFailed.Error(),
			Title:      "Accounting Alias Validation Failed",
			Message:    fmt.Sprintf("The operation alias '%v' does not match the expected alias '%v' defined in the accounting route rule.", args...),
		},
		constant.ErrAccountingAccountTypeValidationFailed: UnprocessableOperationError{
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
		constant.ErrAdditionalBalanceNotAllowed: UnprocessableOperationError{
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
		constant.ErrInstrumentNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrInstrumentNotFound.Error(),
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
		constant.ErrHolderHasInstruments: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrHolderHasInstruments.Error(),
			Title:      "Unable to Delete Holder",
			Message:    "The holder cannot be deleted because it has one or more associated aliases.",
		},
		constant.ErrInstrumentClosingDateBeforeCreation: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInstrumentClosingDateBeforeCreation.Error(),
			Title:      "Alias Closing Date Before Creation Date",
			Message:    "The alias closing date cannot be before the creation date. Please provide a valid closing date.",
		},
		constant.ErrInstrumentLedgerReferenceNotFound: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrInstrumentLedgerReferenceNotFound.Error(),
			Title:      "Instrument Ledger Reference Not Found",
			Message:    "The ledger referenced by this instrument does not exist in this organization. Please provide a ledgerId that belongs to the organization and try again.",
		},
		constant.ErrInstrumentAccountReferenceNotFound: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrInstrumentAccountReferenceNotFound.Error(),
			Title:      "Instrument Account Reference Not Found",
			Message:    "The account referenced by this instrument does not exist in the referenced ledger. Please provide an accountId that belongs to the ledger and try again.",
		},
		constant.ErrSkipNotPermitted: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrSkipNotPermitted.Error(),
			Title:      "Skip Not Permitted",
			Message:    fmt.Sprintf("The %v skip requested for this operation is not permitted on this ledger. Enable the matching ledger override to allow it, or remove the skip from your request.", args...),
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
		constant.ErrMetadataIndexLimitExceeded: UnprocessableOperationError{
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
		constant.ErrMetadataIndexDeletionForbidden: UnprocessableOperationError{
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
		constant.ErrTransactionBackupCacheRetrievalFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrTransactionBackupCacheRetrievalFailed.Error(),
			Title:      "Transaction Backup Cache Retrieval Failed",
			Message:    "The transaction could not be retrieved from the backup cache internal function. Please ensure the transaction exists in the cache before processing balances.",
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
		constant.ErrRouteNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrRouteNotFound.Error(),
			Title:      "Route Not Found",
			Message:    "The requested route does not exist. Please verify the HTTP method and path and try again.",
		},
		constant.ErrMethodNotAllowed: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMethodNotAllowed.Error(),
			Title:      "Method Not Allowed",
			Message:    "The HTTP method is not allowed for the requested route. Please verify the method and try again.",
		},
		constant.ErrPayloadTooLarge: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrPayloadTooLarge.Error(),
			Title:      "Payload Too Large",
			Message:    "The request payload exceeds the maximum allowed size of 64KB.",
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
		constant.ErrUnknownSettingsField: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrUnknownSettingsField.Error(),
			Title:      "Unknown Settings Field",
			Message:    fmt.Sprintf("The settings contain an unknown field: '%v'. Only known settings fields are allowed.", args...),
		},
		constant.ErrInvalidSettingsFieldType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidSettingsFieldType.Error(),
			Title:      "Invalid Settings Field Type",
			Message:    fmt.Sprintf("The settings field '%v' has an invalid type. Expected %v.", args...),
		},
		constant.ErrInvalidSettingsFieldValue: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidSettingsFieldValue.Error(),
			Title:      "Invalid Settings Field Value",
			Message:    fmt.Sprintf("The settings field '%v' has an invalid value. Allowed values are: %v.", args...),
		},
		constant.ErrSettingsRootLevelField: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrSettingsRootLevelField.Error(),
			Title:      "Settings Field at Root Level",
			Message:    fmt.Sprintf("The settings field '%v' must be nested under '%v'. Expected structure: {\"%v\": {\"%v\": value}}.", args...),
		},
		constant.ErrRouteNotBidirectional: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrRouteNotBidirectional.Error(),
			Title:      "Route Not Bidirectional",
			Message:    "The operation route does not allow bidirectional transactions. Only routes with operation type 'bidirectional' can be reverted.",
		},
		constant.ErrMissingCounterpart: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrMissingCounterpart.Error(),
			Title:      "Missing Counterpart",
			Message:    fmt.Sprintf("Route '%v' requires at least one debit and one credit operation (counterpart validation).", args...),
		},
		constant.ErrDirectionRouteMismatch: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrDirectionRouteMismatch.Error(),
			Title:      "Direction Route Mismatch",
			Message:    fmt.Sprintf("Operation direction '%v' is not compatible with route operation type '%v' for operation '%v'.", args...),
		},
		constant.ErrNoSourceForAction: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrNoSourceForAction.Error(),
			Title:      "No Source for Action",
			Message:    fmt.Sprintf("The action '%v' requires at least one source operation route. Please add a source route for this action.", args...),
		},
		constant.ErrNoDestinationForAction: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrNoDestinationForAction.Error(),
			Title:      "No Destination for Action",
			Message:    fmt.Sprintf("The action '%v' requires at least one destination operation route. Please add a destination route for this action.", args...),
		},
		constant.ErrInvalidRouteAction: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidRouteAction.Error(),
			Title:      "Invalid Route Action",
			Message:    fmt.Sprintf("The action '%v' is not a valid route action. Please provide a valid action value.", args...),
		},
		constant.ErrDuplicateActionRoute: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicateActionRoute.Error(),
			Title:      "Duplicate Action Route",
			Message:    fmt.Sprintf("The operation route '%v' is already assigned to the action '%v'. Please remove the duplicate entry.", args...),
		},
		constant.ErrNoRoutesForAction: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrNoRoutesForAction.Error(),
			Title:      "No Routes for Action",
			Message:    fmt.Sprintf("No routes found for action '%v'. Please configure operation routes for this action in the transaction route.", args...),
		},
		constant.ErrTooManyOperationRoutes: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTooManyOperationRoutes.Error(),
			Title:      "Too Many Operation Routes",
			Message:    "The number of operation routes exceeds the maximum allowed. Please reduce the number of operation routes and try again.",
		},
		// Accounting Rules Validation Errors (0162-0166)
		constant.ErrScenarioNotAllowedForDirection: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrScenarioNotAllowedForDirection.Error(),
			Title:      "Scenario Not Allowed For Direction",
			Message:    fmt.Sprintf("The accounting scenario is not allowed for the specified operation direction. %v", args...),
		},
		constant.ErrReserveGroupIncomplete: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrReserveGroupIncomplete.Error(),
			Title:      "Reserve Group Incomplete",
			Message:    fmt.Sprintf("The reserve group (hold, commit, cancel) must be complete. %v", args...),
		},
		constant.ErrDirectScenarioRequired: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrDirectScenarioRequired.Error(),
			Title:      "Direct Scenario Required",
			Message:    fmt.Sprintf("The direct scenario is required when other scenarios are present. %v", args...),
		},
		constant.ErrRevertOnlyBidirectional: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrRevertOnlyBidirectional.Error(),
			Title:      "Revert Only Bidirectional",
			Message:    fmt.Sprintf("The revert scenario is only allowed for bidirectional operation routes. %v", args...),
		},
		constant.ErrAccountingEntryFieldRequired: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrAccountingEntryFieldRequired.Error(),
			Title:      "Accounting Entry Field Required",
			Message:    fmt.Sprintf("A required field is missing in the accounting entry. %v", args...),
		},
		// Fee platform codes (migrated from FEE-xxxx, see docs/plans/2026-06-07-error-code-migration.md).
		constant.ErrFeeCalculationFieldType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrFeeCalculationFieldType.Error(),
			Title:      "Calculation field type invalid",
			Message:    "The Calculation field type is invalid. Values can only be percentage or flat",
		},
		constant.ErrPriorityInvalid: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrPriorityInvalid.Error(),
			Title:      "Invalid fee priority",
			Message:    "The priority field in fees is invalid. Field can not be repeated.",
		},
		constant.ErrFindAccountOnMidaz: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrFindAccountOnMidaz.Error(),
			Title:      "Account not found on Midaz",
			Message:    fmt.Sprintf("Failed to find account '%v' on Midaz. Please check the account alias passed.", args...),
		},
		constant.ErrMinAmountGreaterThanMaxAmount: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrMinAmountGreaterThanMaxAmount.Error(),
			Title:      "minimumAmount greater than maximumAmount",
			Message:    "minimumAmount value is greater than maximumAmount.",
		},
		constant.ErrNothingToUpdate: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrNothingToUpdate.Error(),
			Title:      "Nothing to Update",
			Message:    "No updatable fields were provided. Please include at least one field to update.",
		},
		constant.ErrDuplicatePackage: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicatePackage.Error(),
			Title:      "Package already exists",
			Message:    "A package already exists with the same combination of organizationId, ledgerId, segmentId, transactionRoute, minimumAmount, and maximumAmount.",
		},
		constant.ErrFeeInvalidHeaderParameter: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrFeeInvalidHeaderParameter.Error(),
			Title:      "Invalid header parameter",
			Message:    fmt.Sprintf("One or more header parameters are in an incorrect format. Please check the following parameters %v and ensure they meet the required format before trying again.", args),
		},
		constant.ErrCalculateFee: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrCalculateFee.Error(),
			Title:      "Failed to calculate fee",
			Message:    "Failed to calculate the fee for the transaction. Please check the fee configuration and try again.",
		},
		constant.ErrCalculationRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCalculationRequired.Error(),
			Title:      "Missing calculation model",
			Message:    fmt.Sprintf("The calculation model is required for fee %v.", args...),
		},
		constant.ErrPriorityOne: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrPriorityOne.Error(),
			Title:      "originalAmount is required when priority is one",
			Message:    fmt.Sprintf("For Priority equals to one, referenceAmount must be 'originalAmount' for fee %v.", args...),
		},
		constant.ErrAppRuleFlatFeeAndPercentual: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrAppRuleFlatFeeAndPercentual.Error(),
			Title:      "Failed to apply rule: flatFee or percentual",
			Message:    fmt.Sprintf("applicationRule flatFee or percentual must have exactly 1 calculation for Fee %v.", args...),
		},
		constant.ErrCalculationTypePercentual: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCalculationTypePercentual.Error(),
			Title:      "Invalid calculation type: percentual",
			Message:    fmt.Sprintf("The calculation type percentual must be 'percentage' for Fee %v.", args...),
		},
		constant.ErrCalculationTypeFlatFee: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCalculationTypeFlatFee.Error(),
			Title:      "Invalid calculation type: flatFee",
			Message:    fmt.Sprintf("The calculation type flatFee must be 'flat' for Fee %v.", args...),
		},
		constant.ErrFeeFieldsRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrFeeFieldsRequired.Error(),
			Title:      "Missing required fee fields",
			Message:    "All fields of a new Fee must be filled. Please check again the payload passed.",
		},
		constant.ErrCalculationFieldOfFeeRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCalculationFieldOfFeeRequired.Error(),
			Title:      "Calculation field is required for fee",
			Message:    "Please fill the Calculation object correctly. All calculation fields must be filled.",
		},
		constant.ErrReferenceAmountInvalid: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrReferenceAmountInvalid.Error(),
			Title:      "referenceAmount is not valid",
			Message:    "Field reference amount must be originalAmount or afterFeesAmount.",
		},
		constant.ErrAppRuleInvalid: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAppRuleInvalid.Error(),
			Title:      "Invalid applicationRule",
			Message:    "Field application rule must be maxBetweenTypes, flatFee or percentual.",
		},
		constant.ErrCalculationTypeInvalid: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCalculationTypeInvalid.Error(),
			Title:      "Invalid calculation type",
			Message:    "Field calculation type must be percentage or flat.",
		},
		constant.ErrMaxAmountLessThanMinAmount: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrMaxAmountLessThanMinAmount.Error(),
			Title:      "maximumAmount less than minimumAmount",
			Message:    "maximumAmount value is less than minimumAmount.",
		},
		constant.ErrFilterPackage: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrFilterPackage.Error(),
			Title:      "Package filtering error",
			Message:    "Failed to filter a single package by transactionRoute, segmentID, and maximum/minimum amount. Either no package was found or multiple packages matched the criteria.",
		},
		constant.ErrPackageRange: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrPackageRange.Error(),
			Title:      "Package amount range overlap",
			Message:    "The maximumAmount and minimumAmount of the new package overlap with the amount range of an existing package.",
		},
		constant.ErrValidateDistributeTransactionValue: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrValidateDistributeTransactionValue.Error(),
			Title:      "Failed to distribute values",
			Message:    "Failed to distribute the transaction values. Please check the data passed.",
		},
		constant.ErrAppRuleMaxBetweenTypes: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrAppRuleMaxBetweenTypes.Error(),
			Title:      "Failed to apply rule: maxBetweenTypes",
			Message:    fmt.Sprintf("applicationRule maxBetweenTypes must have more than 1 calculation for Fee %v.", args...),
		},
		constant.ErrInvalidSegmentID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidSegmentID.Error(),
			Title:      "Invalid segmentID",
			Message:    "The specified segmentID is not a valid UUID. Please check the value passed.",
		},
		constant.ErrInvalidLedgerID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidLedgerID.Error(),
			Title:      "Invalid ledgerID",
			Message:    "The specified ledgerID is not a valid UUID. Please check the value passed.",
		},
		constant.ErrConvertToDecimal: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrConvertToDecimal.Error(),
			Title:      "Error to convert values",
			Message:    fmt.Sprintf("The value of the field %s is invalid. Remember to use dot (.) as decimal separator instead of comma (,). Example: use 1000.50 instead of 1000,50.", args...),
		},
		constant.ErrIsDeductibleFrom: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrIsDeductibleFrom.Error(),
			Title:      "originalAmount is required when isDeductibleFrom is true",
			Message:    fmt.Sprintf("For isDeductibleFrom `true`, referenceAmount must be 'originalAmount' for fee %v.", args...),
		},
		constant.ErrApplicationRule: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrApplicationRule.Error(),
			Title:      "applicationRule invalid value",
			Message:    fmt.Sprintf("applicationRule is invalid, Err: %v.", args...),
		},
		constant.ErrCalculationValuePercentage: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCalculationValuePercentage.Error(),
			Title:      "calculation value percentage invalid",
			Message:    fmt.Sprintf("Calculation value is invalid, it cannot exceed 100%%. Please check the calculation value for Fee %v.", args...),
		},
		constant.ErrCalculationValueFlatFee: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCalculationValueFlatFee.Error(),
			Title:      "calculation value flat invalid",
			Message:    fmt.Sprintf("Calculation value is invalid, it cannot exceed the minimum amount %v. Please check the calculation value for Fee %v.", args...),
		},
		constant.ErrAccessMidaz: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrAccessMidaz.Error(),
			Title:      "Failed to access Midaz",
			Message:    fmt.Sprintf("Failed to access Midaz to validate account '%v'. Please check the service configuration and client credentials.", args...),
		},
		constant.ErrDeductibleCalculationValuePercentage: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrDeductibleCalculationValuePercentage.Error(),
			Title:      "deductible value forbidden",
			Message:    fmt.Sprintf("Can not update deductible value to true. The calculation value is bigger than 100%% for Fee %v.", args...),
		},
		constant.ErrDeductibleCalculationValueFlatFee: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrDeductibleCalculationValueFlatFee.Error(),
			Title:      "deductible value forbidden",
			Message:    fmt.Sprintf("Can not update deductible value to true. Calculation value is bigger than the minimum amount %v for Fee %v.", args...),
		},
		constant.ErrInvalidQueryParameterPage: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidQueryParameterPage.Error(),
			Title:      "Invalid Page",
			Message:    "Query parameter page is invalid. The page must be greater than 0.",
		},
		constant.ErrBillingPackageNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrBillingPackageNotFound.Error(),
			Title:      "Billing package not found",
			Message:    fmt.Sprintf("No billing package was found for the given ID '%v'.", args...),
		},
		constant.ErrInvalidBillingPackageType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidBillingPackageType.Error(),
			Title:      "Invalid billing package type",
			Message:    "The billing package type is invalid. Valid types are 'volume' and 'maintenance'.",
		},
		constant.ErrMissingVolumeFields: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMissingVolumeFields.Error(),
			Title:      "Missing volume fields",
			Message:    "Volume billing packages require: eventFilter (transactionRoute, status), pricingModel, tiers, assetCode, debitAccountAlias, and creditAccountAlias.",
		},
		constant.ErrMissingMaintenanceFields: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMissingMaintenanceFields.Error(),
			Title:      "Missing maintenance fields",
			Message:    "Maintenance billing packages require: feeAmount, assetCode, maintenanceCreditAccount, and accountTarget.",
		},
		constant.ErrInvalidPricingModel: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidPricingModel.Error(),
			Title:      "Invalid pricing model",
			Message:    "The pricing model is invalid. Valid models are 'tiered' and 'fixed'.",
		},
		constant.ErrInvalidPricingTier: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidPricingTier.Error(),
			Title:      "Invalid pricing tier",
			Message:    formatPricingTierError(args),
		},
		constant.ErrBillingRouteOverlap: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrBillingRouteOverlap.Error(),
			Title:      "Billing route overlap",
			Message:    "A billing package already exists for this organization, ledger, and transaction route combination.",
		},
		constant.ErrTargetAccountNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrTargetAccountNotFound.Error(),
			Title:      "Target account not found",
			Message:    fmt.Sprintf("The target account '%v' was not found or is inactive in Midaz.", args...),
		},
		constant.ErrBillingCalculationFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrBillingCalculationFailed.Error(),
			Title:      "Billing calculation failed",
			Message:    fmt.Sprintf("Failed to calculate billing: %v", args...),
		},
		constant.ErrNoActiveBillingPackages: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoActiveBillingPackages.Error(),
			Title:      "No active billing packages",
			Message:    "No active billing packages were found for the specified organization and ledger.",
		},
		constant.ErrSegmentResolutionFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrSegmentResolutionFailed.Error(),
			Title:      "Segment resolution failed",
			Message:    "Failed to resolve accounts for the configured segment. Please verify the segment exists and try again.",
		},
		constant.ErrInvalidBillingPeriod: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidBillingPeriod.Error(),
			Title:      "Invalid billing period",
			Message:    "The billing period format is invalid. Use 'YYYY-MM' (monthly), 'YYYY-Www' (weekly, e.g. '2026-W13'), or 'YYYY-MM-DD' (daily) format (e.g., '2026-01' or '2026-01-15').",
		},
		constant.ErrInvalidFreeQuota: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidFreeQuota.Error(),
			Title:      "Invalid free quota",
			Message:    "The free quota value is invalid. Must be a non-negative integer.",
		},
		constant.ErrInvalidDiscountTier: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDiscountTier.Error(),
			Title:      "Invalid discount tier",
			Message:    "Discount tier configuration is invalid. Each tier must have minQuantity and discountPercentage between 0 and 100.",
		},
		constant.ErrInvalidCountMode: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidCountMode.Error(),
			Title:      "Invalid count mode",
			Message:    "The count mode is invalid. Valid modes are 'perRoute' and 'perAccount'.",
		},
		constant.ErrMidazQueryFailed: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrMidazQueryFailed.Error(),
			Title:      "Service dependency unavailable",
			Message:    "A required service is temporarily unavailable. Please try again later.",
		},
		constant.ErrInvalidAccountTarget: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidAccountTarget.Error(),
			Title:      "Invalid account target",
			Message:    "The account target is invalid. Exactly one of segmentId, portfolioId, or aliases must be provided.",
		},
		constant.ErrInvalidFeeAmount: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidFeeAmount.Error(),
			Title:      "Invalid fee amount",
			Message:    "The fee amount is invalid. It must be a positive value greater than zero.",
		},
		constant.ErrMissingSegmentContext: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrMissingSegmentContext.Error(),
			Title:      "Segment context unavailable",
			Message:    "Segment-based waivers are configured but the resolution service is not available. This is an internal configuration issue.",
		},
		constant.ErrMidazRouteNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrMidazRouteNotFound.Error(),
			Title:      "Midaz service route not found",
			Message:    "The Midaz service endpoint returned 404 (route not found). This usually indicates a misconfigured service URL or an API version mismatch. Please verify the Midaz URL environment variables.",
		},
		// Reporter template platform codes (migrated from TPL-xxxx).
		constant.ErrInvalidFileFormat: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidFileFormat.Error(),
			Title:      "Invalid file format",
			Message:    "The uploaded file must be a .tpl file. Other formats are not supported.",
		},
		constant.ErrInvalidOutputFormat: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidOutputFormat.Error(),
			Title:      "Invalid output format",
			Message:    "The outputFormat field must be one of: html, csv, or xml.",
		},
		constant.ErrTemplateInvalidHeaderParameter: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTemplateInvalidHeaderParameter.Error(),
			Title:      "Invalid header",
			Message:    fmt.Sprintf("One or more header values are missing or incorrectly formatted. Please verify required headers %v.", args),
		},
		constant.ErrInvalidFileUploaded: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidFileUploaded.Error(),
			Title:      "Invalid File Uploaded",
			Message:    fmt.Sprintf("The file you submitted is invalid. Please check the uploaded file with error: %v", args),
		},
		constant.ErrEmptyFile: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrEmptyFile.Error(),
			Title:      "Error File Empty",
			Message:    "The file you submitted is empty. Please check the uploaded file.",
		},
		constant.ErrFileContentInvalid: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrFileContentInvalid.Error(),
			Title:      "Error File Content Invalid",
			Message:    fmt.Sprintf("The file content is invalid because is not %s. Please check the uploaded file.", args),
		},
		constant.ErrInvalidMapFields: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidMapFields.Error(),
			Title:      "Invalid Map Fields",
			Message:    fmt.Sprintf("The field on template file is invalid. Invalid field %s on %s.", args...),
		},
		constant.ErrOutputFormatWithoutTemplateFile: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrOutputFormatWithoutTemplateFile.Error(),
			Title:      "Update Output format without template File",
			Message:    "Can not update output format without passing template file. Please check information passed and try again.",
		},
		constant.ErrInvalidTemplateID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidTemplateID.Error(),
			Title:      "Invalid templateID",
			Message:    "The specified templateID is not a valid UUID. Please check the value passed.",
		},
		constant.ErrInvalidLedgerIDList: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidLedgerIDList.Error(),
			Title:      "Invalid ledgerID",
			Message:    fmt.Sprintf("The specified ledgerID inside ledger ID list is not a valid UUID. Please check the value passed %v.", args),
		},
		constant.ErrMissingTableFields: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMissingTableFields.Error(),
			Title:      "Missing required fields",
			Message:    fmt.Sprintf("The fields mapped on template file are missing in the table schema or may be empty. Please check the fields passed: '%v'.", args...),
		},
		constant.ErrReportStatusNotFinished: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrReportStatusNotFinished.Error(),
			Title:      "Report status not Finished",
			Message:    "The Report is not ready to download. Report is processing yet.",
		},
		constant.ErrMissingSchemaTable: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMissingSchemaTable.Error(),
			Title:      "Missing Schema Table",
			Message:    fmt.Sprintf("The schema table %v is missing for data source '%v'. Please check the information passed.", args...),
		},
		constant.ErrMissingDataSource: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMissingDataSource.Error(),
			Title:      "Missing Data Source Table",
			Message:    fmt.Sprintf("The data source %v is missing. Please check the value passed.", args),
		},
		constant.ErrScriptTagDetected: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrScriptTagDetected.Error(),
			Title:      "Script Tag Detected",
			Message:    "The template file contains a script tag and is not allowed. Please check the template file and try again.",
		},
		constant.ErrDecryptionData: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrDecryptionData.Error(),
			Title:      "Encryption data error Tag Detected",
			Message:    fmt.Sprintf("Error to make the encryption of CRM data. Err: %v", args...),
		},
		constant.ErrCommunicateSeaweedFS: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrCommunicateSeaweedFS.Error(),
			Title:      "Communication Error with SeaweedFS",
			Message:    "Error to communicate with SeaweedFS to download or upload file. Please try again.",
		},
		constant.ErrSchemaAmbiguous: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrSchemaAmbiguous.Error(),
			Title:      "Ambiguous Schema Reference",
			Message:    fmt.Sprintf("The table '%v' exists in multiple schemas: %v. Please use explicit schema syntax: database:schema.table", args...),
		},
		constant.ErrSchemaNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrSchemaNotFound.Error(),
			Title:      "Schema Not Found",
			Message:    fmt.Sprintf("The schema '%v' was not found in database '%v'. Please verify the schema name.", args...),
		},
		constant.ErrTableNotFoundInSchema: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrTableNotFoundInSchema.Error(),
			Title:      "Table Not Found in Schema",
			Message:    fmt.Sprintf("The table '%v' was not found in schema '%v' of database '%v'. Please verify the table name and schema.", args...),
		},
		constant.ErrDatabaseNotRegistered: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrDatabaseNotRegistered.Error(),
			Title:      "Database Not Registered",
			Message:    fmt.Sprintf("The database '%v' is not registered. Please verify the datasource configuration.", args...),
		},
		constant.ErrDuplicateRequestInFlight: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicateRequestInFlight.Error(),
			Title:      "Duplicate Request In Flight",
			Message:    "A duplicate request is currently being processed. Please wait and try again.",
		},
		constant.ErrBucketRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrBucketRequired.Error(),
			Title:      "Bucket Required",
			Message:    "The storage bucket name is required. Please check the storage configuration.",
		},
		constant.ErrObjectKeyRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrObjectKeyRequired.Error(),
			Title:      "Object Key Required",
			Message:    "The object key is required for the storage operation.",
		},
		constant.ErrObjectNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrObjectNotFound.Error(),
			Title:      "Object Not Found",
			Message:    "The requested object was not found in storage.",
		},
		constant.ErrTTLNotSupported: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTTLNotSupported.Error(),
			Title:      "TTL Not Supported",
			Message:    "TTL parameter is not supported in S3 mode. Use bucket lifecycle policies instead.",
		},
		constant.ErrDuplicateDeadline: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicateDeadline.Error(),
			Title:      "Duplicate Deadline",
			Message:    "A deadline with the same name, type, due date, and frequency already exists. Please use different values or update the existing deadline.",
		},
		constant.ErrInvalidDeadlineType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDeadlineType.Error(),
			Title:      "Invalid Deadline Type",
			Message:    "The 'type' field must be 'regulatory' or 'custom'. Please provide a valid deadline type and try again.",
		},
		constant.ErrInvalidDeadlineFrequency: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDeadlineFrequency.Error(),
			Title:      "Invalid Deadline Frequency",
			Message:    "The 'frequency' field must be one of: 'once', 'daily', 'weekly', 'monthly', 'semiannual', 'annual'. Please provide a valid frequency and try again.",
		},
		constant.ErrInvalidDeadlineColor: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDeadlineColor.Error(),
			Title:      "Invalid Deadline Color",
			Message:    "The 'color' field must be a valid hex color code (e.g., '#FF5733'). Please provide a valid color and try again.",
		},
		constant.ErrMonthsOfYearNotApplicable: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMonthsOfYearNotApplicable.Error(),
			Title:      "Months of Year Not Applicable",
			Message:    fmt.Sprintf("The 'monthsOfYear' field is not applicable for frequency '%v'. It can only be used with 'semiannual' or 'annual' frequencies.", args...),
		},
		constant.ErrMonthsOfYearRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMonthsOfYearRequired.Error(),
			Title:      "Months of Year Required",
			Message:    fmt.Sprintf("The 'monthsOfYear' field is required for frequency '%v'. Please specify which months of the year the deadline should recur on.", args...),
		},
		constant.ErrMonthsOfYearOutOfRange: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMonthsOfYearOutOfRange.Error(),
			Title:      "Months of Year Out of Range",
			Message:    fmt.Sprintf("Each value in 'monthsOfYear' must be between 1 and 12. Received invalid value: %v.", args...),
		},
		constant.ErrDueDateInPast: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrDueDateInPast.Error(),
			Title:      "Due Date in the Past",
			Message:    "The 'dueDate' must be today or a future date. Please provide a date that is not in the past.",
		},
		constant.ErrMonthsOfYearCountMismatch: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMonthsOfYearCountMismatch.Error(),
			Title:      "Months of Year Count Mismatch",
			Message:    fmt.Sprintf("The number of months in 'monthsOfYear' does not match the '%v' frequency. 'semiannual' requires exactly 2 months and 'annual' requires exactly 1 month.", args...),
		},
		constant.ErrDataSourceNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrDataSourceNotFound.Error(),
			Title:      "Data Source Not Found",
			Message:    "The requested data source was not found. Please verify the data source ID.",
		},
		constant.ErrDataSourceUnavailable: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrDataSourceUnavailable.Error(),
			Title:      "Data Source Unavailable",
			Message:    "The data source is currently unavailable. Results may be incomplete.",
		},
		constant.ErrSchemaValidationFailed: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrSchemaValidationFailed.Error(),
			Title:      "Schema Validation Failed",
			Message:    "The schema validation failed. Please verify the fields against the data source schema.",
		},
		constant.ErrExtractionJobFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrExtractionJobFailed.Error(),
			Title:      "Extraction Job Failed",
			Message:    "The extraction job failed. Please try again later or contact support.",
		},
		constant.ErrInvalidUTF8: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidUTF8.Error(),
			Title:      "Invalid UTF-8 Encoding",
			Message:    fmt.Sprintf("The '%v' field contains invalid UTF-8 byte sequences. Please provide valid UTF-8 text and try again.", args...),
		},
		constant.ErrTemplateRenderFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrTemplateRenderFailed.Error(),
			Title:      "Template Rendering Failed",
			Message:    fmt.Sprintf("The template could not be rendered with the provided data. This is a permanent error and will not succeed on retry. Detail: %v.", args...),
		},
		// Reporter worker-internal pipeline codes (migrated from REP-xxxx).
		constant.ErrCodeDataSourceNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrCodeDataSourceNotFound.Error(),
			Title:      "Data Source Not Found",
			Message:    "Data source not found.",
		},
		constant.ErrCodeDataSourceUnavailable: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrCodeDataSourceUnavailable.Error(),
			Title:      "Data Source Unavailable",
			Message:    "Data source unavailable.",
		},
		constant.ErrCodeUnsupportedDatabaseType: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrCodeUnsupportedDatabaseType.Error(),
			Title:      "Unsupported Database Type",
			Message:    "Unsupported database type.",
		},
		constant.ErrCodeUnexpectedSchemaResult: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrCodeUnexpectedSchemaResult.Error(),
			Title:      "Unexpected Schema Result",
			Message:    "Unexpected schema result.",
		},
		constant.ErrCodeUnexpectedTableResult: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrCodeUnexpectedTableResult.Error(),
			Title:      "Unexpected Table Result",
			Message:    "Unexpected table result.",
		},
		constant.ErrCodeUnexpectedCollectionResult: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrCodeUnexpectedCollectionResult.Error(),
			Title:      "Unexpected Collection Result",
			Message:    "Unexpected collection result.",
		},
		constant.ErrCodeCRMHashKeyNotConfigured: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrCodeCRMHashKeyNotConfigured.Error(),
			Title:      "CRMHash Key Not Configured",
			Message:    "CRM hash key not configured.",
		},
		constant.ErrCodeCRMEncryptKeyNotConfigured: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrCodeCRMEncryptKeyNotConfigured.Error(),
			Title:      "CRMEncrypt Key Not Configured",
			Message:    "CRM encrypt key not configured.",
		},
		constant.ErrCodeCipherInitFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrCodeCipherInitFailed.Error(),
			Title:      "Cipher Init Failed",
			Message:    "Cipher init failed.",
		},
		constant.ErrCodeRecordDecryptionFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrCodeRecordDecryptionFailed.Error(),
			Title:      "Record Decryption Failed",
			Message:    "Record decryption failed.",
		},
		constant.ErrCodeStorageNotConfigured: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrCodeStorageNotConfigured.Error(),
			Title:      "Storage Not Configured",
			Message:    "Storage not configured.",
		},
		constant.ErrCodeInvalidExtractedData: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrCodeInvalidExtractedData.Error(),
			Title:      "Invalid Extracted Data",
			Message:    "Invalid extracted data.",
		},
		constant.ErrCodeEmptyEncryptedData: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrCodeEmptyEncryptedData.Error(),
			Title:      "Empty Encrypted Data",
			Message:    "Empty encrypted data.",
		},
		constant.ErrCodeDecryptionKeyNotConfigured: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrCodeDecryptionKeyNotConfigured.Error(),
			Title:      "Decryption Key Not Configured",
			Message:    "Decryption key not configured.",
		},
		constant.ErrCodeInvalidEncryptedData: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrCodeInvalidEncryptedData.Error(),
			Title:      "Invalid Encrypted Data",
			Message:    "Invalid encrypted data.",
		},
		constant.ErrCodeAESCipherCreationFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrCodeAESCipherCreationFailed.Error(),
			Title:      "AESCipher Creation Failed",
			Message:    "AES cipher creation failed.",
		},
		constant.ErrCodeGCMCreationFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrCodeGCMCreationFailed.Error(),
			Title:      "GCMCreation Failed",
			Message:    "GCM creation failed.",
		},
		constant.ErrCodeCorruptEncryptedData: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrCodeCorruptEncryptedData.Error(),
			Title:      "Corrupt Encrypted Data",
			Message:    "Corrupt encrypted data.",
		},
		constant.ErrCodeAESGCMDecryptionFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrCodeAESGCMDecryptionFailed.Error(),
			Title:      "AESGCMDecryption Failed",
			Message:    "AES-GCM decryption failed.",
		},
		constant.ErrCodeInvalidFetcherResponse: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrCodeInvalidFetcherResponse.Error(),
			Title:      "Invalid Fetcher Response",
			Message:    "Invalid fetcher response.",
		},
		// Tracer platform codes (migrated from TRC-xxxx).
		constant.ErrRuleCalculationFieldType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrRuleCalculationFieldType.Error(),
			Title:      "Calculation Field Type",
			Message:    "Invalid calculation field type.",
		},
		constant.ErrParentIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrParentIDNotFound.Error(),
			Title:      "Parent IDNot Found",
			Message:    "Parent ID not found.",
		},
		constant.ErrContextCancelled: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrContextCancelled.Error(),
			Title:      "Context Cancelled",
			Message:    "Context cancelled / service unavailable.",
		},
		constant.ErrPaginationLimitInvalid: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrPaginationLimitInvalid.Error(),
			Title:      "Pagination Limit Invalid",
			Message:    "Pagination limit must be positive.",
		},
		constant.ErrInvalidSortColumn: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidSortColumn.Error(),
			Title:      "Invalid Sort Column",
			Message:    "Sort column not in allowed list.",
		},
		constant.ErrInvalidCursor: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidCursor.Error(),
			Title:      "Invalid Cursor",
			Message:    "Invalid or corrupted pagination cursor.",
		},
		constant.ErrCursorWithSortParams: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCursorWithSortParams.Error(),
			Title:      "Cursor With Sort Params",
			Message:    "Cursor and sort parameters are mutually exclusive.",
		},
		constant.ErrMetadataEntriesExceeded: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMetadataEntriesExceeded.Error(),
			Title:      "Metadata Entries Exceeded",
			Message:    "Metadata entries exceed maximum of 50.",
		},
		constant.ErrMetadataKeyInvalidChars: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMetadataKeyInvalidChars.Error(),
			Title:      "Metadata Key Invalid Chars",
			Message:    "Metadata key contains invalid characters.",
		},
		constant.ErrInvalidDecision: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDecision.Error(),
			Title:      "Invalid Decision",
			Message:    "Invalid decision value.",
		},
		constant.ErrReasonRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrReasonRequired.Error(),
			Title:      "Reason Required",
			Message:    "Reason is required.",
		},
		constant.ErrInvalidDefaultDecision: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDefaultDecision.Error(),
			Title:      "Invalid Default Decision",
			Message:    "Invalid default decision value.",
		},
		constant.ErrExpressionSyntax: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrExpressionSyntax.Error(),
			Title:      "Expression Syntax",
			Message:    "Invalid CEL syntax.",
		},
		constant.ErrExpressionType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrExpressionType.Error(),
			Title:      "Expression Type",
			Message:    "Expression must return boolean.",
		},
		constant.ErrExpressionCostExceeded: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrExpressionCostExceeded.Error(),
			Title:      "Expression Cost Exceeded",
			Message:    "Cost limit exceeded (cost computed and above threshold).",
		},
		constant.ErrExpressionEvaluation: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrExpressionEvaluation.Error(),
			Title:      "Expression Evaluation",
			Message:    "Runtime evaluation error.",
		},
		constant.ErrExpressionProgram: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrExpressionProgram.Error(),
			Title:      "Expression Program",
			Message:    "Program creation failed (compilation phase).",
		},
		constant.ErrExpressionCostEstimation: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrExpressionCostEstimation.Error(),
			Title:      "Expression Cost Estimation",
			Message:    "Failed to estimate expression cost.",
		},
		constant.ErrAmountExceedsPrecision: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrAmountExceedsPrecision.Error(),
			Title:      "Amount Exceeds Precision",
			Message:    "Amount exceeds safe precision for CEL float64 evaluation (max: ±2^53).",
		},
		constant.ErrRuleNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrRuleNotFound.Error(),
			Title:      "Rule Not Found",
			Message:    "Rule not found by ID.",
		},
		constant.ErrRuleNameAlreadyExists: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrRuleNameAlreadyExists.Error(),
			Title:      "Rule Name Already Exists",
			Message:    "Rule name must be unique.",
		},
		constant.ErrRuleInvalidStatus: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrRuleInvalidStatus.Error(),
			Title:      "Rule Invalid Status",
			Message:    "Invalid rule status transition.",
		},
		constant.ErrRuleEvaluationFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrRuleEvaluationFailed.Error(),
			Title:      "Rule Evaluation Failed",
			Message:    "Rule evaluation failed.",
		},
		constant.ErrExpressionNotModifiable: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrExpressionNotModifiable.Error(),
			Title:      "Expression Not Modifiable",
			Message:    "Expression cannot be modified for non-DRAFT rules.",
		},
		constant.ErrRuleNilInput: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrRuleNilInput.Error(),
			Title:      "Rule Nil Input",
			Message:    "Rule input cannot be nil.",
		},
		constant.ErrRuleNameRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrRuleNameRequired.Error(),
			Title:      "Rule Name Required",
			Message:    "Rule name is required.",
		},
		constant.ErrRuleNameTooLong: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrRuleNameTooLong.Error(),
			Title:      "Rule Name Too Long",
			Message:    "Rule name exceeds max length (255).",
		},
		constant.ErrRuleExpressionRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrRuleExpressionRequired.Error(),
			Title:      "Rule Expression Required",
			Message:    "Rule expression is required.",
		},
		constant.ErrRuleExpressionTooLong: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrRuleExpressionTooLong.Error(),
			Title:      "Rule Expression Too Long",
			Message:    "Rule expression exceeds max length (5000).",
		},
		constant.ErrRuleInvalidAction: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrRuleInvalidAction.Error(),
			Title:      "Rule Invalid Action",
			Message:    "Action must be one of [ALLOW, DENY, REVIEW].",
		},
		constant.ErrRuleInvalidScope: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrRuleInvalidScope.Error(),
			Title:      "Rule Invalid Scope",
			Message:    "Scope must have at least one field set.",
		},
		constant.ErrRuleDescriptionTooLong: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrRuleDescriptionTooLong.Error(),
			Title:      "Rule Description Too Long",
			Message:    "Rule description exceeds max length (1000).",
		},
		constant.ErrRuleScopesTooMany: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrRuleScopesTooMany.Error(),
			Title:      "Rule Scopes Too Many",
			Message:    "Rule scopes exceed maximum (100).",
		},
		constant.ErrRuleInvalidTransition: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrRuleInvalidTransition.Error(),
			Title:      "Rule Invalid Transition",
			Message:    "Status transition not allowed.",
		},
		constant.ErrLimitNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrLimitNotFound.Error(),
			Title:      "Limit Not Found",
			Message:    "Limit not found by ID.",
		},
		constant.ErrLimitInvalidStatusChange: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrLimitInvalidStatusChange.Error(),
			Title:      "Limit Invalid Status Change",
			Message:    "Invalid limit status transition.",
		},
		constant.ErrLimitInvalidType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitInvalidType.Error(),
			Title:      "Limit Invalid Type",
			Message:    "Invalid limit type.",
		},
		constant.ErrLimitInvalidMaxAmount: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitInvalidMaxAmount.Error(),
			Title:      "Limit Invalid Max Amount",
			Message:    "MaxAmount must be positive.",
		},
		constant.ErrLimitInvalidCurrency: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitInvalidCurrency.Error(),
			Title:      "Limit Invalid Currency",
			Message:    "Currency must be valid ISO 4217.",
		},
		constant.ErrLimitInvalidScope: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitInvalidScope.Error(),
			Title:      "Limit Invalid Scope",
			Message:    "Scope validation failed.",
		},
		constant.ErrLimitNameRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitNameRequired.Error(),
			Title:      "Limit Name Required",
			Message:    "Limit name is required.",
		},
		constant.ErrLimitNameTooLong: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitNameTooLong.Error(),
			Title:      "Limit Name Too Long",
			Message:    "Limit name exceeds max length.",
		},
		constant.ErrLimitAlreadyDeleted: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrLimitAlreadyDeleted.Error(),
			Title:      "Limit Already Deleted",
			Message:    "Limit is already in DELETED state.",
		},
		constant.ErrLimitNameInvalidChars: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitNameInvalidChars.Error(),
			Title:      "Limit Name Invalid Chars",
			Message:    "Limit name contains invalid characters.",
		},
		constant.ErrLimitDescriptionInvalidChars: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitDescriptionInvalidChars.Error(),
			Title:      "Limit Description Invalid Chars",
			Message:    "Limit description contains invalid characters.",
		},
		constant.ErrLimitInvalidID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitInvalidID.Error(),
			Title:      "Limit Invalid ID",
			Message:    "Limit ID is invalid or nil.",
		},
		constant.ErrLimitDescriptionTooLong: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitDescriptionTooLong.Error(),
			Title:      "Limit Description Too Long",
			Message:    "Limit description exceeds max length.",
		},
		constant.ErrLimitInvalidStatusFilter: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitInvalidStatusFilter.Error(),
			Title:      "Limit Invalid Status Filter",
			Message:    "Invalid status filter value.",
		},
		constant.ErrLimitInvalidTypeFilter: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitInvalidTypeFilter.Error(),
			Title:      "Limit Invalid Type Filter",
			Message:    "Invalid limitType filter value.",
		},
		constant.ErrLimitDeletedAtInvariant: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrLimitDeletedAtInvariant.Error(),
			Title:      "Limit Deleted At Invariant",
			Message:    "DeletedAt must be set iff status is DELETED.",
		},
		constant.ErrLimitCheckFailed: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrLimitCheckFailed.Error(),
			Title:      "Limit Check Failed",
			Message:    "Limit check failed.",
		},
		constant.ErrLimitNilInput: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitNilInput.Error(),
			Title:      "Limit Nil Input",
			Message:    "Limit input cannot be nil.",
		},
		constant.ErrLimitImmutableField: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrLimitImmutableField.Error(),
			Title:      "Limit Immutable Field",
			Message:    "Cannot modify immutable field (limitType, currency).",
		},
		constant.ErrAuditEventNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAuditEventNotFound.Error(),
			Title:      "Audit Event Not Found",
			Message:    "Audit event not found.",
		},
		constant.ErrInvalidAuditEventFilters: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidAuditEventFilters.Error(),
			Title:      "Invalid Audit Event Filters",
			Message:    "Invalid audit event filter parameters.",
		},
		constant.ErrAuditEventInvalidType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAuditEventInvalidType.Error(),
			Title:      "Audit Event Invalid Type",
			Message:    "Invalid audit event type.",
		},
		constant.ErrAuditEventInvalidAction: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAuditEventInvalidAction.Error(),
			Title:      "Audit Event Invalid Action",
			Message:    "Invalid audit action.",
		},
		constant.ErrAuditEventInvalidResult: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAuditEventInvalidResult.Error(),
			Title:      "Audit Event Invalid Result",
			Message:    "Invalid audit result.",
		},
		constant.ErrAuditEventResourceIDRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAuditEventResourceIDRequired.Error(),
			Title:      "Audit Event Resource IDRequired",
			Message:    "Resource ID is required.",
		},
		constant.ErrAuditEventInvalidResourceType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAuditEventInvalidResourceType.Error(),
			Title:      "Audit Event Invalid Resource Type",
			Message:    "Invalid resource type.",
		},
		constant.ErrAuditEventActorIDRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAuditEventActorIDRequired.Error(),
			Title:      "Audit Event Actor IDRequired",
			Message:    "Actor ID is required.",
		},
		constant.ErrAuditEventActorTypeInvalid: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAuditEventActorTypeInvalid.Error(),
			Title:      "Audit Event Actor Type Invalid",
			Message:    "Actor type must be 'user' or 'system'.",
		},
		constant.ErrUsageCounterOverflow: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrUsageCounterOverflow.Error(),
			Title:      "Usage Counter Overflow",
			Message:    "Usage counter would overflow.",
		},
		constant.ErrUsageCounterLimitIDRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrUsageCounterLimitIDRequired.Error(),
			Title:      "Usage Counter Limit IDRequired",
			Message:    "Usage counter limitID is required.",
		},
		constant.ErrUsageCounterScopeKeyRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrUsageCounterScopeKeyRequired.Error(),
			Title:      "Usage Counter Scope Key Required",
			Message:    "Usage counter scopeKey is required.",
		},
		constant.ErrUsageCounterPeriodKeyRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrUsageCounterPeriodKeyRequired.Error(),
			Title:      "Usage Counter Period Key Required",
			Message:    "Usage counter periodKey is required.",
		},
		constant.ErrUsageCounterCurrentUsageNegative: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrUsageCounterCurrentUsageNegative.Error(),
			Title:      "Usage Counter Current Usage Negative",
			Message:    "Usage counter currentUsage must be non-negative.",
		},
		constant.ErrUsageCounterIncrementNonNegative: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrUsageCounterIncrementNonNegative.Error(),
			Title:      "Usage Counter Increment Non Negative",
			Message:    "Increment amount must be non-negative.",
		},
		constant.ErrUsageCounterNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrUsageCounterNotFound.Error(),
			Title:      "Usage Counter Not Found",
			Message:    "Usage counter not found.",
		},
		constant.ErrUsageCounterExceedsLimit: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrUsageCounterExceedsLimit.Error(),
			Title:      "Usage Counter Exceeds Limit",
			Message:    "Usage counter increment would exceed limit maximum.",
		},
		constant.ErrUsageCounterDecrementNonNegative: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrUsageCounterDecrementNonNegative.Error(),
			Title:      "Usage Counter Decrement Non Negative",
			Message:    "Decrement amount must be non-negative.",
		},
		constant.ErrCheckLimitsInvalidAmount: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCheckLimitsInvalidAmount.Error(),
			Title:      "Check Limits Invalid Amount",
			Message:    "Check limits amount must be positive.",
		},
		constant.ErrCheckLimitsInvalidCurrency: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCheckLimitsInvalidCurrency.Error(),
			Title:      "Check Limits Invalid Currency",
			Message:    "Check limits currency must be valid ISO 4217.",
		},
		constant.ErrCheckLimitsUnknownLimitType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCheckLimitsUnknownLimitType.Error(),
			Title:      "Check Limits Unknown Limit Type",
			Message:    "Unknown limit type for period key calculation.",
		},
		constant.ErrCheckLimitsInvalidTimestamp: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCheckLimitsInvalidTimestamp.Error(),
			Title:      "Check Limits Invalid Timestamp",
			Message:    "Check limits timestamp must not be zero.",
		},
		constant.ErrCheckLimitsNilInput: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCheckLimitsNilInput.Error(),
			Title:      "Check Limits Nil Input",
			Message:    "Check limits input cannot be nil.",
		},
		constant.ErrCheckLimitsInvalidAccountID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCheckLimitsInvalidAccountID.Error(),
			Title:      "Check Limits Invalid Account ID",
			Message:    "Check limits accountId is required.",
		},
		constant.ErrCheckLimitsInvalidTransactionType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCheckLimitsInvalidTransactionType.Error(),
			Title:      "Check Limits Invalid Transaction Type",
			Message:    "Check limits transactionType must be valid.",
		},
		constant.ErrCheckLimitsInvalidSubType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCheckLimitsInvalidSubType.Error(),
			Title:      "Check Limits Invalid Sub Type",
			Message:    "Check limits subType exceeds maximum length.",
		},
		constant.ErrCheckLimitsInvalidSegmentID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCheckLimitsInvalidSegmentID.Error(),
			Title:      "Check Limits Invalid Segment ID",
			Message:    "Check limits segmentId must not be zero UUID.",
		},
		constant.ErrCheckLimitsInvalidPortfolioID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCheckLimitsInvalidPortfolioID.Error(),
			Title:      "Check Limits Invalid Portfolio ID",
			Message:    "Check limits portfolioId must not be zero UUID.",
		},
		constant.ErrCheckLimitsInvalidMerchantID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCheckLimitsInvalidMerchantID.Error(),
			Title:      "Check Limits Invalid Merchant ID",
			Message:    "Check limits merchantId must not be zero UUID.",
		},
		constant.ErrLimitCheckerNilLimitRepo: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrLimitCheckerNilLimitRepo.Error(),
			Title:      "Limit Checker Nil Limit Repo",
			Message:    "Limit checker: limit repository cannot be nil.",
		},
		constant.ErrLimitCheckerNilUsageCounterRepo: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrLimitCheckerNilUsageCounterRepo.Error(),
			Title:      "Limit Checker Nil Usage Counter Repo",
			Message:    "Limit checker: usage counter repository cannot be nil.",
		},
		constant.ErrLimitCheckerNilClock: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrLimitCheckerNilClock.Error(),
			Title:      "Limit Checker Nil Clock",
			Message:    "Limit checker: clock cannot be nil.",
		},
		constant.ErrValidationRequestIDRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationRequestIDRequired.Error(),
			Title:      "Validation Request IDRequired",
			Message:    "RequestId is required.",
		},
		constant.ErrValidationInvalidTransactionType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationInvalidTransactionType.Error(),
			Title:      "Validation Invalid Transaction Type",
			Message:    "Invalid transactionType.",
		},
		constant.ErrValidationAmountNonPositive: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationAmountNonPositive.Error(),
			Title:      "Validation Amount Non Positive",
			Message:    "Amount must be positive.",
		},
		constant.ErrValidationCurrencyRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationCurrencyRequired.Error(),
			Title:      "Validation Currency Required",
			Message:    "Currency is required.",
		},
		constant.ErrValidationInvalidCurrency: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationInvalidCurrency.Error(),
			Title:      "Validation Invalid Currency",
			Message:    "Currency must be valid ISO 4217.",
		},
		constant.ErrValidationTimestampRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationTimestampRequired.Error(),
			Title:      "Validation Timestamp Required",
			Message:    "Timestamp is required.",
		},
		constant.ErrValidationTimestampFuture: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationTimestampFuture.Error(),
			Title:      "Validation Timestamp Future",
			Message:    "Timestamp cannot be in the future.",
		},
		constant.ErrValidationAccountRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationAccountRequired.Error(),
			Title:      "Validation Account Required",
			Message:    "Account is required.",
		},
		constant.ErrValidationTimestampPast: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationTimestampPast.Error(),
			Title:      "Validation Timestamp Past",
			Message:    "Timestamp is too far in the past.",
		},
		constant.ErrValidationTimeout: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrValidationTimeout.Error(),
			Title:      "Validation Timeout",
			Message:    "Validation timeout.",
		},
		constant.ErrValidationSegmentIDRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationSegmentIDRequired.Error(),
			Title:      "Validation Segment IDRequired",
			Message:    "SegmentId is required when segment is provided.",
		},
		constant.ErrValidationPortfolioIDRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationPortfolioIDRequired.Error(),
			Title:      "Validation Portfolio IDRequired",
			Message:    "PortfolioId is required when portfolio is provided.",
		},
		constant.ErrValidationSubTypeTooLong: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationSubTypeTooLong.Error(),
			Title:      "Validation Sub Type Too Long",
			Message:    "SubType exceeds maximum length of 50 characters.",
		},
		constant.ErrValidationInvalidAccountType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationInvalidAccountType.Error(),
			Title:      "Validation Invalid Account Type",
			Message:    "Account.type must be checking, savings, or credit.",
		},
		constant.ErrValidationInvalidAccountStatus: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationInvalidAccountStatus.Error(),
			Title:      "Validation Invalid Account Status",
			Message:    "Account.status must be active, suspended, or closed.",
		},
		constant.ErrValidationInvalidMerchantCategory: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationInvalidMerchantCategory.Error(),
			Title:      "Validation Invalid Merchant Category",
			Message:    "Merchant.category must be 4-digit MCC code.",
		},
		constant.ErrValidationInvalidMerchantCountry: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationInvalidMerchantCountry.Error(),
			Title:      "Validation Invalid Merchant Country",
			Message:    "Merchant.country must be ISO 3166-1 alpha-2.",
		},
		constant.ErrValidationMerchantIDRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidationMerchantIDRequired.Error(),
			Title:      "Validation Merchant IDRequired",
			Message:    "Merchant.id is required when merchant is provided.",
		},
		constant.ErrInvalidTransactionValidationFilters: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidTransactionValidationFilters.Error(),
			Title:      "Invalid Transaction Validation Filters",
			Message:    "Invalid transaction validation filter parameters.",
		},
		constant.ErrTransactionValidationNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrTransactionValidationNotFound.Error(),
			Title:      "Transaction Validation Not Found",
			Message:    "Transaction validation record not found.",
		},
		constant.ErrListValidationsTimeout: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrListValidationsTimeout.Error(),
			Title:      "List Validations Timeout",
			Message:    "List validations query timeout (deadline exceeded).",
		},
		constant.ErrTransactionValidationIDRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionValidationIDRequired.Error(),
			Title:      "Transaction Validation IDRequired",
			Message:    "Validation ID is required.",
		},
		constant.ErrTransactionValidationCreatedAtRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionValidationCreatedAtRequired.Error(),
			Title:      "Transaction Validation Created At Required",
			Message:    "CreatedAt is required.",
		},
		constant.ErrRuleCacheWarmUpFailed: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrRuleCacheWarmUpFailed.Error(),
			Title:      "Rule Cache Warm Up Failed",
			Message:    "Rule cache warm-up failed.",
		},
		constant.ErrRuleCacheNotReady: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrRuleCacheNotReady.Error(),
			Title:      "Rule Cache Not Ready",
			Message:    "Rule cache is not ready.",
		},
		constant.ErrLimitTimeWindowMismatch: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitTimeWindowMismatch.Error(),
			Title:      "Limit Time Window Mismatch",
			Message:    "ActiveTimeStart and activeTimeEnd must both be set or both be nil.",
		},
		constant.ErrLimitTimeWindowZeroWidth: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitTimeWindowZeroWidth.Error(),
			Title:      "Limit Time Window Zero Width",
			Message:    "ActiveTimeStart cannot equal activeTimeEnd.",
		},
		constant.ErrTimeOfDayInvalidFormat: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTimeOfDayInvalidFormat.Error(),
			Title:      "Time Of Day Invalid Format",
			Message:    "Invalid time of day format, expected HH:MM.",
		},
		constant.ErrRuleNameAlreadyExistsInCtx: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrRuleNameAlreadyExistsInCtx.Error(),
			Title:      "Rule Name Already Exists In Ctx",
			Message:    "Rule name already exists in this context.",
		},
		constant.ErrLimitNameAlreadyExists: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrLimitNameAlreadyExists.Error(),
			Title:      "Limit Name Already Exists",
			Message:    "Limit name already exists.",
		},
		constant.ErrLimitCustomDatesNotAllowed: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitCustomDatesNotAllowed.Error(),
			Title:      "Limit Custom Dates Not Allowed",
			Message:    "CustomStartDate/customEndDate only allowed for CUSTOM limitType.",
		},
		constant.ErrLimitUnknownType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitUnknownType.Error(),
			Title:      "Limit Unknown Type",
			Message:    "Unknown limit type.",
		},
		constant.ErrLimitCustomPeriodTooLong: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrLimitCustomPeriodTooLong.Error(),
			Title:      "Limit Custom Period Too Long",
			Message:    "Custom period cannot exceed 5 years.",
		},
		constant.ErrLimitCustomPeriodExpired: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrLimitCustomPeriodExpired.Error(),
			Title:      "Limit Custom Period Expired",
			Message:    "Custom period end date must be in the future.",
		},
		constant.ErrLimitInvalidCustomStartFormat: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitInvalidCustomStartFormat.Error(),
			Title:      "Limit Invalid Custom Start Format",
			Message:    "Invalid customStartDate format, expected RFC3339.",
		},
		constant.ErrLimitInvalidCustomEndFormat: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitInvalidCustomEndFormat.Error(),
			Title:      "Limit Invalid Custom End Format",
			Message:    "Invalid customEndDate format, expected RFC3339.",
		},
		constant.ErrLimitCustomDatesRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitCustomDatesRequired.Error(),
			Title:      "Limit Custom Dates Required",
			Message:    "CustomStartDate and customEndDate required for CUSTOM limitType.",
		},
		constant.ErrLimitCustomDatesOrder: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLimitCustomDatesOrder.Error(),
			Title:      "Limit Custom Dates Order",
			Message:    "CustomStartDate must be before customEndDate.",
		},
		constant.ErrMTConfigRequired: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrMTConfigRequired.Error(),
			Title:      "MTConfig Required",
			Message:    "Multi-tenant config: cfg is required.",
		},
		constant.ErrMTLoggerRequired: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrMTLoggerRequired.Error(),
			Title:      "MTLogger Required",
			Message:    "Multi-tenant config: logger is required.",
		},
		constant.ErrMTURLRequired: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrMTURLRequired.Error(),
			Title:      "MTURLRequired",
			Message:    "MULTI_TENANT_URL must be set when MULTI_TENANT_ENABLED=true.",
		},
		constant.ErrMTURLInvalid: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrMTURLInvalid.Error(),
			Title:      "MTURLInvalid",
			Message:    "MULTI_TENANT_URL must be a valid absolute URL with scheme and host.",
		},
		constant.ErrMTServiceAPIKeyRequired: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrMTServiceAPIKeyRequired.Error(),
			Title:      "MTService APIKey Required",
			Message:    "MULTI_TENANT_SERVICE_API_KEY must be set when MULTI_TENANT_ENABLED=true.",
		},
		constant.ErrMTRedisHostRequired: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrMTRedisHostRequired.Error(),
			Title:      "MTRedis Host Required",
			Message:    "MULTI_TENANT_REDIS_HOST must be set when MULTI_TENANT_ENABLED=true.",
		},
		constant.ErrMTPluginAuthRequired: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrMTPluginAuthRequired.Error(),
			Title:      "MTPlugin Auth Required",
			Message:    "MULTI_TENANT_ENABLED=true requires PLUGIN_AUTH_ENABLED=true.",
		},
		constant.ErrMTAPIKeyOnlyValidationConfl: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrMTAPIKeyOnlyValidationConfl.Error(),
			Title:      "MTAPIKey Only Validation Confl",
			Message:    "MULTI_TENANT_ENABLED=true is incompatible with API_KEY_ENABLED_ONLY_VALIDATION=true.",
		},
		constant.ErrReadyzPgConnectionNotEstablished: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrReadyzPgConnectionNotEstablished.Error(),
			Title:      "Readyz Pg Connection Not Established",
			Message:    "Postgres readyz: connection not established.",
		},
		constant.ErrReadyzPgConnectionFailed: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrReadyzPgConnectionFailed.Error(),
			Title:      "Readyz Pg Connection Failed",
			Message:    "Postgres readyz: connection failed.",
		},
		constant.ErrReadyzPgPingFailed: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrReadyzPgPingFailed.Error(),
			Title:      "Readyz Pg Ping Failed",
			Message:    "Postgres readyz: ping failed.",
		},
		constant.ErrReadyzDependenciesUnhealthy: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrReadyzDependenciesUnhealthy.Error(),
			Title:      "Readyz Dependencies Unhealthy",
			Message:    "/readyz aggregate: one or more dependencies unhealthy.",
		},
		constant.ErrReadyzCacheNotReady: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrReadyzCacheNotReady.Error(),
			Title:      "Readyz Cache Not Ready",
			Message:    "Rule_cache readyz: cache not ready.",
		},
		constant.ErrReadyzCacheStale: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrReadyzCacheStale.Error(),
			Title:      "Readyz Cache Stale",
			Message:    "Rule_cache readyz: cache data stale.",
		},
		constant.ErrSupervisorShuttingDown: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrSupervisorShuttingDown.Error(),
			Title:      "Supervisor Shutting Down",
			Message:    "Worker supervisor: shutting down, refusing to spawn new tenant workers.",
		},
		constant.ErrTenantCapReached: ServiceUnavailableError{
			EntityType: entityType,
			Code:       constant.ErrTenantCapReached.Error(),
			Title:      "Tenant Cap Reached",
			Message:    "Tenant worker cap reached; client should retry after backoff.",
		},
		constant.ErrSupervisorNilRuleCache: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrSupervisorNilRuleCache.Error(),
			Title:      "Supervisor Nil Rule Cache",
			Message:    "Worker supervisor: rule cache is required.",
		},
		constant.ErrSupervisorNilSyncRepo: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrSupervisorNilSyncRepo.Error(),
			Title:      "Supervisor Nil Sync Repo",
			Message:    "Worker supervisor: sync repo is required.",
		},
		constant.ErrSupervisorNilUsageRepo: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrSupervisorNilUsageRepo.Error(),
			Title:      "Supervisor Nil Usage Repo",
			Message:    "Worker supervisor: usage repo is required when cleanup workers are enabled.",
		},
		constant.ErrSupervisorNilCompiler: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrSupervisorNilCompiler.Error(),
			Title:      "Supervisor Nil Compiler",
			Message:    "Worker supervisor: compiler is required.",
		},
		constant.ErrSupervisorNilLogger: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrSupervisorNilLogger.Error(),
			Title:      "Supervisor Nil Logger",
			Message:    "Worker supervisor: logger is required.",
		},
		constant.ErrSupervisorNilReaperRepo: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrSupervisorNilReaperRepo.Error(),
			Title:      "Supervisor Nil Reaper Repo",
			Message:    "Worker supervisor: reservation reaper repo is required when reaper workers are enabled.",
		},
		constant.ErrSupervisorNilReaperAuditor: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrSupervisorNilReaperAuditor.Error(),
			Title:      "Supervisor Nil Reaper Auditor",
			Message:    "Worker supervisor: reservation reaper auditor is required when reaper workers are enabled.",
		},
		constant.ErrUnauthorizedMissingSub: UnauthorizedError{
			EntityType: entityType,
			Code:       constant.ErrUnauthorizedMissingSub.Error(),
			Title:      "Unauthorized Missing Sub",
			Message:    "JWT lacks required 'sub' claim — identity cannot be attributed.",
		},
		constant.ErrReservationLimitIDRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrReservationLimitIDRequired.Error(),
			Title:      "Reservation Limit IDRequired",
			Message:    "Reservation: limitId is required.",
		},
		constant.ErrReservationTransactionIDReq: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrReservationTransactionIDReq.Error(),
			Title:      "Reservation Transaction IDReq",
			Message:    "Reservation: transactionId is required.",
		},
		constant.ErrReservationTenantRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrReservationTenantRequired.Error(),
			Title:      "Reservation Tenant Required",
			Message:    "Reservation: tenant id is required on the multi-tenant reservation surface.",
		},
		constant.ErrReservationScopeKeyRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrReservationScopeKeyRequired.Error(),
			Title:      "Reservation Scope Key Required",
			Message:    "Reservation: scopeKey is required.",
		},
		constant.ErrReservationPeriodKeyRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrReservationPeriodKeyRequired.Error(),
			Title:      "Reservation Period Key Required",
			Message:    "Reservation: periodKey is required.",
		},
		constant.ErrReservationAmountInvalid: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrReservationAmountInvalid.Error(),
			Title:      "Reservation Amount Invalid",
			Message:    "Reservation: amount must be non-negative.",
		},
		constant.ErrReservationInvalidStatus: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrReservationInvalidStatus.Error(),
			Title:      "Reservation Invalid Status",
			Message:    "Reservation: status must be one of RESERVED, CONFIRMED, RELEASED, EXPIRED.",
		},
		constant.ErrReservationExpiresAtRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrReservationExpiresAtRequired.Error(),
			Title:      "Reservation Expires At Required",
			Message:    "Reservation: reservationExpiresAt is required.",
		},
		constant.ErrReservationNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrReservationNotFound.Error(),
			Title:      "Reservation Not Found",
			Message:    "Reservation: reservation not found.",
		},
		constant.ErrReservationAlreadyTerminal: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrReservationAlreadyTerminal.Error(),
			Title:      "Reservation Already Terminal",
			Message:    "Reservation: reservation is already in a terminal state.",
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

// formatPricingTierError builds a clean error message for pricing tier validation failures.
func formatPricingTierError(args []any) string {
	if len(args) == 1 {
		return fmt.Sprintf("Pricing tier configuration is invalid: %v.", args[0])
	}

	if len(args) > 1 {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = fmt.Sprint(a)
		}

		return fmt.Sprintf("Pricing tier configuration is invalid: %s.", strings.Join(parts, "; "))
	}

	return "Pricing tier configuration is invalid."
}
