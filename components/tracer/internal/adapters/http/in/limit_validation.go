// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Validation constants define the limits for limit input fields.
// Note: These values must match the validation tags in the structs below.
const (
	MaxLimitNameLength        = 255
	MaxLimitDescriptionLength = 1000
	MaxLimitScopesCount       = 100
	MaxLimitSubTypeLength     = 50
	MaxLimitNameFilterLength  = 255 // Maximum length for name filter query parameter (matches VARCHAR(255))
	// MaxUsageCountersPerLimit bounds the number of usage counters returned for a single limit.
	// Calculated as: max_scopes (100) × reasonable_period_history (~10 months).
	// Provides DoS protection and documents API expectations for tooling validation.
	MaxUsageCountersPerLimit = 1000
)

// registerLimitValidations registers limit-specific validation functions.
// Returns an error if any validator registration fails.
func registerLimitValidations(v *validator.Validate) error {
	// limittype validates that LimitType is a valid enum value
	if err := v.RegisterValidation("limittype", validateLimitType); err != nil {
		return fmt.Errorf("failed to register limittype validator: %w", err)
	}

	// limitstatus validates that LimitStatus is a valid enum value
	if err := v.RegisterValidation("limitstatus", validateLimitStatus); err != nil {
		return fmt.Errorf("failed to register limitstatus validator: %w", err)
	}

	return nil
}

// validateLimitType validates that the LimitType is a valid enum value.
func validateLimitType(fl validator.FieldLevel) bool {
	field := fl.Field()

	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return true
		}

		field = field.Elem()
	}

	limitType := model.LimitType(field.String())

	return limitType.IsValid()
}

// validateLimitStatus validates that the LimitStatus is a valid enum value.
func validateLimitStatus(fl validator.FieldLevel) bool {
	field := fl.Field()

	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return true
		}

		field = field.Elem()
	}

	status := model.LimitStatus(field.String())

	return status.IsValid()
}

// CreateLimitInput represents the HTTP request body for creating a limit.
type CreateLimitInput struct {
	Name            string           `json:"name" validate:"required,min=1,max=255"`
	Description     *string          `json:"description,omitempty" validate:"omitempty,max=1000"`
	LimitType       model.LimitType  `json:"limitType" validate:"required,limittype" swaggertype:"string" enums:"DAILY,MONTHLY,PER_TRANSACTION,WEEKLY,CUSTOM" example:"DAILY"`
	MaxAmount       decimal.Decimal  `json:"maxAmount" validate:"required" swaggertype:"string" example:"1000.00"`
	Currency        string           `json:"currency" validate:"required,len=3,uppercase" minLength:"3" maxLength:"3" example:"USD"`
	Scopes          []model.Scope    `json:"scopes" validate:"required,min=1,max=100,dive,scopenotempty"`
	ActiveTimeStart *model.TimeOfDay `json:"activeTimeStart,omitempty" swaggertype:"string" example:"09:00"`
	ActiveTimeEnd   *model.TimeOfDay `json:"activeTimeEnd,omitempty" swaggertype:"string" example:"17:00"`
	CustomStartDate *string          `json:"customStartDate,omitempty" format:"date-time" example:"2026-11-27T00:00:00Z"`
	CustomEndDate   *string          `json:"customEndDate,omitempty" format:"date-time" example:"2026-11-29T00:00:00Z"`
}

// Validate validates the CreateLimitInput struct using validator/v10.
func (i *CreateLimitInput) Validate() error {
	v, err := getValidator()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrValidatorInit, err)
	}

	if err := v.Struct(i); err != nil {
		return formatLimitValidationError(err)
	}

	// Custom validation for decimal MaxAmount (validator/v10 gt=0 doesn't work with decimal.Decimal)
	if i.MaxAmount.LessThanOrEqual(decimal.Zero) {
		return limitFieldValidationErr("maxAmount must be greater than 0")
	}

	return nil
}

// UpdateLimitInput represents the HTTP request body for updating a limit.
type UpdateLimitInput struct {
	Name            *string          `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Description     *string          `json:"description,omitempty" validate:"omitempty,max=1000"`
	MaxAmount       *decimal.Decimal `json:"maxAmount,omitempty" swaggertype:"string" example:"1000.00"`
	Scopes          *[]model.Scope   `json:"scopes,omitempty" validate:"omitempty,min=1,max=100,dive,scopenotempty"`
	ActiveTimeStart *model.TimeOfDay `json:"activeTimeStart,omitempty" swaggertype:"string" example:"09:00"`
	ActiveTimeEnd   *model.TimeOfDay `json:"activeTimeEnd,omitempty" swaggertype:"string" example:"17:00"`
	CustomStartDate *string          `json:"customStartDate,omitempty" format:"date-time" example:"2026-11-27T00:00:00Z"`
	CustomEndDate   *string          `json:"customEndDate,omitempty" format:"date-time" example:"2026-11-29T00:00:00Z"`
}

// Validate validates the UpdateLimitInput struct using validator/v10.
func (i *UpdateLimitInput) Validate() error {
	v, err := getValidator()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrValidatorInit, err)
	}

	if err := v.Struct(i); err != nil {
		return formatLimitValidationError(err)
	}

	// Custom validation for decimal MaxAmount
	if i.MaxAmount != nil && i.MaxAmount.LessThanOrEqual(decimal.Zero) {
		return limitFieldValidationErr("maxAmount must be greater than 0")
	}

	return nil
}

// IsEmpty returns true if no fields are set for update.
func (i *UpdateLimitInput) IsEmpty() bool {
	return i.Name == nil && i.MaxAmount == nil && i.Description == nil && i.Scopes == nil &&
		i.ActiveTimeStart == nil && i.ActiveTimeEnd == nil && i.CustomStartDate == nil && i.CustomEndDate == nil
}

// ListLimitsInput represents query parameters for listing limits.
type ListLimitsInput struct {
	Name            *string `query:"name"`
	AccountID       *string `query:"account_id"`
	SegmentID       *string `query:"segment_id"`
	PortfolioID     *string `query:"portfolio_id"`
	MerchantID      *string `query:"merchant_id"`
	TransactionType *string `query:"transaction_type"`
	SubType         *string `query:"sub_type"`
	Limit           *int    `query:"limit"`
	Cursor          string  `query:"cursor"`
	Status          string  `query:"status"`
	LimitType       string  `query:"limit_type"`
	SortBy          string  `query:"sort_by"`
	SortOrder       string  `query:"sort_order"`
}

// SetDefaults applies default values.
// Note: SortBy and SortOrder defaults are only applied when cursor is not present,
// because cursor already contains sort configuration (TRC-0045).
func (i *ListLimitsInput) SetDefaults() {
	if i.Limit == nil {
		defaultLimit := trcConstant.DefaultPaginationLimit
		i.Limit = &defaultLimit
	}

	// Only apply sort defaults when not using cursor pagination
	// Cursor already contains sort configuration from the original request (TRC-0045)
	if i.Cursor == "" {
		if i.SortBy == "" {
			i.SortBy = "created_at"
		}

		// Normalize sortOrder to uppercase and apply default if empty
		i.SortOrder = NormalizeSortOrder(i.SortOrder, "DESC")
	}
}

// Validate validates the ListLimitsInput struct.
func (i *ListLimitsInput) Validate() error {
	// Validate pagination limit with specific error codes (TRC-0040, TRC-0041)
	if err := ValidatePaginationLimit(i.Limit, 100); err != nil {
		return err
	}

	// Validate cursor consistency (TRC-0045)
	if err := ValidateCursorConsistency(i.Cursor, i.SortBy, i.SortOrder); err != nil {
		return err
	}

	// Validate sortBy whitelist (TRC-0043)
	allowedSortFields := []string{"created_at", "updated_at", "name", "max_amount"}
	if err := ValidateSortBy(i.SortBy, allowedSortFields); err != nil {
		return err
	}

	// Validate sortOrder enum (TRC-0042)
	if err := ValidateSortOrder(i.SortOrder); err != nil {
		return err
	}

	// Validate status enum using model's IsValid method
	if i.Status != "" {
		status := model.LimitStatus(i.Status)
		if !status.IsValid() {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityLimit, "filters")
		}
		// Prevent DELETED status in list filters (soft-deleted records should not be queried)
		if status == model.LimitStatusDeleted {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityLimit, "filters")
		}
	}

	// Validate limitType enum using model's IsValid method
	if i.LimitType != "" {
		limitType := model.LimitType(i.LimitType)
		if !limitType.IsValid() {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityLimit, "filters")
		}
	}

	// Validate scope filter fields (TRC-0006 for invalid values)
	if err := i.validateScopeFields(); err != nil {
		return err
	}

	return nil
}

// validateScopeFields validates name and scope-related query parameters for listing limits.
func (i *ListLimitsInput) validateScopeFields() error {
	// Validate name length (prevent oversized ILIKE queries)
	if i.Name != nil && len(*i.Name) > MaxLimitNameFilterLength {
		return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityLimit, "filters")
	}

	// Validate UUID fields
	uuidFields := []struct {
		value *string
		name  string
	}{
		{i.AccountID, "account_id"},
		{i.SegmentID, "segment_id"},
		{i.PortfolioID, "portfolio_id"},
		{i.MerchantID, "merchant_id"},
	}

	for _, f := range uuidFields {
		if f.value != nil && *f.value != "" {
			if _, err := uuid.Parse(*f.value); err != nil {
				return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityLimit, "filters")
			}
		}
	}

	// Validate transactionType enum
	if i.TransactionType != nil && *i.TransactionType != "" {
		txType := model.TransactionType(*i.TransactionType)
		if !txType.IsValid() {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityLimit, "filters")
		}
	}

	// Validate subType length against trimmed value so whitespace-only input
	// (treated as nil/no-filter by buildLimitScopeFromInput) is not rejected
	// purely because the raw string happens to exceed the limit.
	if i.SubType != nil {
		trimmedSubType := strings.TrimSpace(*i.SubType)
		if trimmedSubType != "" && len(trimmedSubType) > MaxLimitSubTypeLength {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityLimit, "filters")
		}
	}

	return nil
}

// ListLimitsResponse represents the HTTP response for listing limits.
type ListLimitsResponse struct {
	Limits     []model.Limit `json:"limits"`
	NextCursor string        `json:"nextCursor,omitempty" example:"eyJpZCI6IjAxOTI4In0="`
	HasMore    bool          `json:"hasMore" example:"true"`
}

// Note: UsageSnapshot is now defined in pkg/model/limit.go
// The HTTP response uses model.UsageSnapshot directly.

// ToCreateLimitServiceInput converts HTTP CreateLimitInput to service CreateLimitInput.
func ToCreateLimitServiceInput(input *CreateLimitInput) *command.CreateLimitInput {
	scopes := make([]model.Scope, len(input.Scopes))
	copy(scopes, input.Scopes)

	return &command.CreateLimitInput{
		Name:            input.Name,
		Description:     input.Description,
		LimitType:       input.LimitType,
		MaxAmount:       input.MaxAmount,
		Currency:        input.Currency,
		Scopes:          scopes,
		ActiveTimeStart: input.ActiveTimeStart,
		ActiveTimeEnd:   input.ActiveTimeEnd,
		CustomStartDate: input.CustomStartDate,
		CustomEndDate:   input.CustomEndDate,
	}
}

// ToUpdateLimitServiceInput converts HTTP UpdateLimitInput to service UpdateLimitInput.
func ToUpdateLimitServiceInput(input *UpdateLimitInput) *command.UpdateLimitInput {
	result := &command.UpdateLimitInput{
		Name:            input.Name,
		MaxAmount:       input.MaxAmount,
		Description:     input.Description,
		ActiveTimeStart: input.ActiveTimeStart,
		ActiveTimeEnd:   input.ActiveTimeEnd,
		CustomStartDate: input.CustomStartDate,
		CustomEndDate:   input.CustomEndDate,
	}

	if input.Scopes != nil {
		scopes := make([]model.Scope, len(*input.Scopes))
		copy(scopes, *input.Scopes)

		result.Scopes = &scopes
	}

	return result
}

// ToListLimitsFilter converts HTTP ListLimitsInput to model ListLimitsFilter.
// Safely handles nil input.Limit by defaulting to trcConstant.DefaultPaginationLimit.
// SortBy is passed as snake_case and maps directly to DB column names.
func ToListLimitsFilter(input *ListLimitsInput) *model.ListLimitsFilter {
	limit := trcConstant.DefaultPaginationLimit
	if input.Limit != nil {
		limit = *input.Limit
	}

	filter := &model.ListLimitsFilter{
		Name:      input.Name,
		Limit:     limit,
		Cursor:    input.Cursor,
		SortBy:    input.SortBy, // snake_case; maps directly to DB column
		SortOrder: strings.ToUpper(input.SortOrder),
	}

	if input.Status != "" {
		status := model.LimitStatus(input.Status)
		filter.Status = &status
	}

	if input.LimitType != "" {
		limitType := model.LimitType(input.LimitType)
		filter.LimitType = &limitType
	}

	// Build scope filter from individual query parameters
	if scope := buildLimitScopeFromInput(input); scope != nil {
		filter.ScopeFilter = scope
	}

	return filter
}

// buildLimitScopeFromInput constructs a model.Scope from ListLimitsInput scope fields.
// Returns nil if no scope fields are provided (all nil or empty strings).
// Returns nil defensively if any UUID field fails to parse.
func buildLimitScopeFromInput(input *ListLimitsInput) *model.Scope {
	var scope model.Scope

	hasField := false

	if input.AccountID != nil && *input.AccountID != "" {
		id, err := uuid.Parse(*input.AccountID)
		if err != nil {
			return nil
		}

		scope.AccountID = &id
		hasField = true
	}

	if input.SegmentID != nil && *input.SegmentID != "" {
		id, err := uuid.Parse(*input.SegmentID)
		if err != nil {
			return nil
		}

		scope.SegmentID = &id
		hasField = true
	}

	if input.PortfolioID != nil && *input.PortfolioID != "" {
		id, err := uuid.Parse(*input.PortfolioID)
		if err != nil {
			return nil
		}

		scope.PortfolioID = &id
		hasField = true
	}

	if input.MerchantID != nil && *input.MerchantID != "" {
		id, err := uuid.Parse(*input.MerchantID)
		if err != nil {
			return nil
		}

		scope.MerchantID = &id
		hasField = true
	}

	if input.TransactionType != nil && *input.TransactionType != "" {
		txType := model.TransactionType(*input.TransactionType)
		scope.TransactionType = &txType
		hasField = true
	}

	// Whitespace-only input is treated as no filter: trimming to "" and assigning
	// would produce subType='' in the generated SQL and silently return zero
	// results for a filter the user did not intend.
	if input.SubType != nil {
		normalized := strings.ToLower(strings.TrimSpace(*input.SubType))
		if normalized != "" {
			scope.SubType = &normalized
			hasField = true
		}
	}

	if !hasField {
		return nil
	}

	return &scope
}

// ToListLimitsResponse converts model ListLimitsResult to HTTP ListLimitsResponse.
func ToListLimitsResponse(result *model.ListLimitsResult) *ListLimitsResponse {
	// Ensure Limits is never nil to avoid "limits": null in JSON response
	limits := result.Limits
	if limits == nil {
		limits = []model.Limit{}
	}

	return &ListLimitsResponse{
		Limits:     limits,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}

// limitFieldValidationErr wraps a field-level validation message in the canonical
// 400 ValidationError envelope so the handler can render it via http.WithError.
func limitFieldValidationErr(format string, args ...any) error {
	return pkg.ValidationError{
		Code:    constant.ErrMissingFieldsInRequest.Error(),
		Title:   "Validation Error",
		Message: fmt.Sprintf(format, args...),
	}
}

// formatLimitValidationError formats validator errors into user-friendly messages for limits.
func formatLimitValidationError(err error) error {
	var validationErrors validator.ValidationErrors

	ok := errors.As(err, &validationErrors)
	if !ok {
		return err
	}

	if len(validationErrors) == 0 {
		return nil
	}

	// Process first error only
	fieldError := validationErrors[0]
	fieldName := fieldError.Field()
	tag := fieldError.Tag()
	namespace := fieldError.Namespace()

	// Check if this is a scope field error (from dive validation)
	if isLimitScopeFieldError(namespace) {
		return formatLimitScopeFieldError(fieldError)
	}

	switch tag {
	case "required":
		return limitFieldValidationErr("%s is a required field", toLimitJSONFieldName(fieldName))
	case "min":
		jsonFieldName := toLimitJSONFieldName(fieldName)
		if jsonFieldName == "scopes" {
			return limitFieldValidationErr("%s must have at least %s item(s)", jsonFieldName, fieldError.Param())
		}

		return limitFieldValidationErr("%s must be at least %s characters", jsonFieldName, fieldError.Param())
	case "max":
		jsonFieldName := toLimitJSONFieldName(fieldName)
		if jsonFieldName == "scopes" {
			return limitFieldValidationErr("%s must have a maximum of %s items", jsonFieldName, fieldError.Param())
		}

		return limitFieldValidationErr("%s must be a maximum of %s characters", jsonFieldName, fieldError.Param())
	case "len":
		return limitFieldValidationErr("%s must be exactly %s characters", toLimitJSONFieldName(fieldName), fieldError.Param())
	case "uppercase":
		return limitFieldValidationErr("%s must be uppercase", toLimitJSONFieldName(fieldName))
	case "gt":
		return limitFieldValidationErr("%s must be greater than %s", toLimitJSONFieldName(fieldName), fieldError.Param())
	case "oneof":
		return limitFieldValidationErr("%s must be one of [%s]", toLimitJSONFieldName(fieldName), fieldError.Param())
	case "limittype":
		return limitFieldValidationErr("%s must be one of [DAILY WEEKLY MONTHLY CUSTOM PER_TRANSACTION]", toLimitJSONFieldName(fieldName))
	case "limitstatus":
		return limitFieldValidationErr("%s must be one of [DRAFT ACTIVE INACTIVE]", toLimitJSONFieldName(fieldName))
	default:
		return limitFieldValidationErr("%s validation failed: %s", toLimitJSONFieldName(fieldName), tag)
	}
}

// isLimitScopeFieldError checks if the error is from a scope field (via dive validation).
func isLimitScopeFieldError(namespace string) bool {
	return strings.Contains(namespace, "Scopes[")
}

// formatLimitScopeFieldError formats a scope field validation error for limits.
func formatLimitScopeFieldError(fieldError validator.FieldError) error {
	tag := fieldError.Tag()

	// Handle scopenotempty validation (applies to the whole scope, not a field)
	if tag == "scopenotempty" {
		index := extractLimitScopeIndex(fieldError.Namespace())
		if index == -1 {
			return limitFieldValidationErr("scope must have at least one field set")
		}

		return limitFieldValidationErr("scope at index %d must have at least one field set", index)
	}

	fieldName := toLimitScopeJSONFieldName(fieldError.Field())

	var msg string

	switch tag {
	case "uuid":
		msg = fmt.Sprintf("%s must be a valid UUID", fieldName)
	case "oneof":
		msg = fmt.Sprintf("%s must be one of [%s]", fieldName, fieldError.Param())
	case "transactiontype":
		msg = fmt.Sprintf("%s must be one of [CARD WIRE PIX CRYPTO]", fieldName)
	case "max":
		msg = fmt.Sprintf("%s must be a maximum of %s characters", fieldName, fieldError.Param())
	default:
		msg = fmt.Sprintf("%s validation failed: %s", fieldName, tag)
	}

	// Extract index from namespace (e.g., "CreateLimitInput.Scopes[0].SegmentID")
	index := extractLimitScopeIndex(fieldError.Namespace())
	if index == -1 {
		return limitFieldValidationErr("scope %s", msg)
	}

	return limitFieldValidationErr("scope at index %d: %s", index, msg)
}

// extractLimitScopeIndex extracts the scope index from the namespace.
// Returns -1 if no index is found, allowing 0 to be a valid index.
func extractLimitScopeIndex(namespace string) int {
	start := strings.Index(namespace, "Scopes[")
	if start == -1 {
		return -1
	}

	start += len("Scopes[")
	end := strings.Index(namespace[start:], "]")

	if end == -1 {
		return -1
	}

	var index int

	n, _ := fmt.Sscanf(namespace[start:start+end], "%d", &index)
	if n != 1 {
		return -1
	}

	return index
}

// toLimitJSONFieldName converts struct field name to JSON field name for limits.
func toLimitJSONFieldName(fieldName string) string {
	switch fieldName {
	case "Name":
		return "name"
	case "Description":
		return "description"
	case "LimitType":
		return "limitType"
	case "MaxAmount":
		return "maxAmount"
	case "Currency":
		return "currency"
	case "Scopes":
		return "scopes"
	case "Status":
		return "status"
	case "Limit":
		return "limit"
	case "Cursor":
		return "cursor"
	case "SortBy":
		return "sortBy"
	case "SortOrder":
		return "sortOrder"
	default:
		return fieldName
	}
}

// toLimitScopeJSONFieldName converts LimitScopeInput field name to JSON field name.
func toLimitScopeJSONFieldName(fieldName string) string {
	switch fieldName {
	case "SegmentID":
		return "segmentId"
	case "PortfolioID":
		return "portfolioId"
	case "AccountID":
		return "accountId"
	case "MerchantID":
		return "merchantId"
	case "TransactionType":
		return "transactionType"
	case "SubType":
		return "subType"
	default:
		return fieldName
	}
}
