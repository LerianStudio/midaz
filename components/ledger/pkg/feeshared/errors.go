// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
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
	Title      string
	Message    string
	Code       string
	Err        error `json:"err,omitempty"`
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
	Code    string `json:"code,omitempty"`
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
		Code:       constant.ErrInternalServer.Error(),
		Title:      "Internal Server Error",
		Message:    "The server encountered an unexpected error. Please try again later or contact support.",
		Err:        err,
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
// error: The appropriate business error with code, title, and message.
func ValidateBusinessError(err error, entityType string, args ...any) error {
	errorMap := map[error]error{
		constant.ErrCalculationFieldType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCalculationFieldType.Error(),
			Title:      "Calculation field type invalid",
			Message:    "The Calculation field type is invalid. Values can only be percentage or flat",
		},
		constant.ErrInvalidRequestBody: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidRequestBody.Error(),
			Title:      "Invalid Request Body",
			Message:    fmt.Sprintf("The request body contains an invalid value: %v. Please check the documentation and try again.", args),
		},
		constant.ErrMissingFieldsInRequest: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMissingFieldsInRequest.Error(),
			Title:      "Missing Fields in Request",
			Message:    fmt.Sprintf("One or more required fields are missing or blank: %v. Please provide all required fields.", args),
		},
		constant.ErrInvalidQueryParameter: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidQueryParameter.Error(),
			Title:      "Invalid Query Parameter",
			Message:    fmt.Sprintf("One or more query parameters are in an incorrect format. Please check the following parameters '%v' and ensure they meet the required format before trying again.", args),
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
		constant.ErrInvalidDateRange: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDateRange.Error(),
			Title:      "Invalid Date Range Error",
			Message:    "Both 'initialDate' and 'finalDate' fields are required and must be in the 'yyyy-mm-dd' format. Please provide valid dates and try again.",
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
			Message:    "Invalid sort_order value. Expected 'asc' or 'desc'.",
		},
		constant.ErrEntityNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrEntityNotFound.Error(),
			Title:      "Entity Not Found",
			Message:    fmt.Sprintf("No %v entity was found for the given ID. Please make sure to use the correct ID for the entity you are trying to manage.", args...),
		},
		constant.ErrPriorityInvalid: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrPriorityInvalid.Error(),
			Title:      "Invalid fee priority",
			Message:    "The priority field in fees is invalid. Field can not be repeated.",
		},
		constant.ErrFindAccountOnMidaz: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrFindAccountOnMidaz.Error(),
			Title:      "Account not found on Midaz",
			Message:    fmt.Sprintf("Failed to find account '%v' on Midaz. Please check the account alias passed.", args...),
		},
		constant.ErrMinAmountGreaterThanMaxAmount: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMinAmountGreaterThanMaxAmount.Error(),
			Title:      "minimumAmount greater than maximumAmount",
			Message:    "minimumAmount value is greater than maximumAmount.",
		},
		constant.ErrInvalidPathParameter: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidPathParameter.Error(),
			Title:      "Invalid Path Parameter",
			Message:    fmt.Sprintf("The path parameter is in an incorrect format. Please check the following parameter %v and ensure it meets the required format before trying again.", args),
		},
		constant.ErrNothingToUpdate: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrNothingToUpdate.Error(),
			Title:      "Nothing to Update",
			Message:    "No updatable fields were provided. Please include at least one field to update.",
		},
		constant.ErrInvalidHeaderParameter: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidHeaderParameter.Error(),
			Title:      "Invalid header parameter",
			Message:    fmt.Sprintf("One or more header parameters are in an incorrect format. Please check the following parameters %v and ensure they meet the required format before trying again.", args),
		},
		constant.ErrHeaderParameterRequired: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrHeaderParameterRequired.Error(),
			Title:      "Missing header",
			Message:    fmt.Sprintf("Header parameters are required. Please check the following header parameters %v and ensure they are passing the values correctly.", args),
		},
		constant.ErrDuplicatePackage: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrDuplicatePackage.Error(),
			Title:      "Package already exists",
			Message:    "A package already exists with the same combination of organizationId, ledgerId, segmentId, transactionRoute, minimumAmount, and maximumAmount.",
		},
		constant.ErrInvalidTransactionType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidTransactionType.Error(),
			Title:      "Invalid Transaction Type",
			Message:    fmt.Sprintf("Only one transaction type ('amount', 'share', or 'remaining') must be specified in the '%v' field for each entry. Please review your input and try again.", args...),
		},
		constant.ErrCalculateFee: ValidationError{
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
		constant.ErrAppRuleFlatFeeAndPercentual: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAppRuleFlatFeeAndPercentual.Error(),
			Title:      "Failed to apply rule: flatFee or percentual",
			Message:    fmt.Sprintf("applicationRule flatFee or percentual must have exactly 1 calculation for Fee %v.", args...),
		},
		constant.ErrAppRuleMaxBetweenTypes: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAppRuleMaxBetweenTypes.Error(),
			Title:      "Failed to apply rule: maxBetweenTypes",
			Message:    fmt.Sprintf("applicationRule maxBetweenTypes must have more than 1 calculation for Fee %v.", args...),
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
		constant.ErrMaxAmountLessThanMinAmount: ValidationError{
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
		constant.ErrPackageRange: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrPackageRange.Error(),
			Title:      "Package amount range overlap",
			Message:    "The maximumAmount and minimumAmount of the new package overlap with the amount range of an existing package.",
		},
		constant.ErrValidateDistributeTransactionValue: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrValidateDistributeTransactionValue.Error(),
			Title:      "Failed to distribute values",
			Message:    "Failed to distribute the transaction values. Please check the data passed.",
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
		constant.ErrConvertToDecimal: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrConvertToDecimal.Error(),
			Title:      "Error to convert values",
			Message:    fmt.Sprintf("The value of the field %s is invalid. Remember to use dot (.) as decimal separator instead of comma (,). Example: use 1000.50 instead of 1000,50.", args...),
		},
		constant.ErrIsDeductibleFrom: ValidationError{
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
		constant.ErrForbiddenAccessMidaz: ForbiddenError{
			EntityType: entityType,
			Code:       constant.ErrForbiddenAccessMidaz.Error(),
			Title:      "Forbidden to access Midaz",
			Message:    fmt.Sprintf("Failed to access Midaz due to insufficient permissions. Please check the client credentials for account '%v' validation.", args...),
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
		constant.ErrAccessMidaz: ValidationError{
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

		constant.ErrUnexpectedFieldsInTheRequest: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrUnexpectedFieldsInTheRequest.Error(),
			Title:      "Unexpected fields in the request",
			Message:    fmt.Sprintf("The request body contains fields that are not allowed for this type of billing package. %v", args...),
		},

		// Motor 2 - Billing CRUD errors
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
		constant.ErrBillingRouteOverlap: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrBillingRouteOverlap.Error(),
			Title:      "Billing route overlap",
			Message:    "A billing package already exists for this organization, ledger, and transaction route combination.",
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
		constant.ErrInvalidFeeAmount: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidFeeAmount.Error(),
			Title:      "Invalid fee amount",
			Message:    "The fee amount is invalid. It must be a positive value greater than zero.",
		},

		// Motor 2 - Billing Calculation errors
		constant.ErrTargetAccountNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrTargetAccountNotFound.Error(),
			Title:      "Target account not found",
			Message:    fmt.Sprintf("The target account '%v' was not found or is inactive in Midaz.", args...),
		},
		constant.ErrBillingCalculationFailed: UnprocessableOperationError{
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

		// Motor 2 - Integration errors.
		// Messages are stable and do not leak internal transport details; original error is preserved in Err for logging.
		constant.ErrSegmentResolutionFailed: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrSegmentResolutionFailed.Error(),
			Title:      "Segment resolution failed",
			Message:    "Failed to resolve accounts for the configured segment. Please verify the segment exists and try again.",
		},
		constant.ErrMidazQueryFailed: UnprocessableOperationError{
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
		constant.ErrMissingSegmentContext: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrMissingSegmentContext.Error(),
			Title:      "Segment context unavailable",
			Message:    "Segment-based waivers are configured but the resolution service is not available. This is an internal configuration issue.",
		},
		constant.ErrMidazRouteNotFound: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrMidazRouteNotFound.Error(),
			Title:      "Midaz service route not found",
			Message:    "The Midaz service endpoint returned 404 (route not found). This usually indicates a misconfigured service URL or an API version mismatch. Please verify the Midaz URL environment variables.",
		},
	}

	if mappedError, found := errorMap[err]; found {
		return mappedError
	}

	return err
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
