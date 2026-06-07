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

	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ErrValidatorInit is returned when validator initialization fails.
// This is a server-side error and should be mapped to InternalServerError.
var ErrValidatorInit = errors.New("validator initialization failed")

// Validation constants define the limits for rule input fields.
// Note: These values must match the validation tags in the structs below.
const (
	MaxRuleNameLength        = 255
	MaxRuleDescriptionLength = 1000
	MaxRuleExpressionLength  = 5000
	MaxRuleScopesCount       = 100
	MaxRuleSubTypeLength     = 50
)

// registerRuleValidations registers all rule-specific validation functions.
// Returns an error if any validator registration fails.
func registerRuleValidations(v *validator.Validate) error {
	// scopenotempty validates that a ScopeInput has at least one field set
	if err := v.RegisterValidation("scopenotempty", validateScopeNotEmpty); err != nil {
		return fmt.Errorf("failed to register scopenotempty validator: %w", err)
	}

	// transactiontype validates that TransactionType is a valid enum value
	if err := v.RegisterValidation("transactiontype", validateTransactionType); err != nil {
		return fmt.Errorf("failed to register transactiontype validator: %w", err)
	}

	// decision validates that Decision is a valid enum value
	if err := v.RegisterValidation("decision", validateDecision); err != nil {
		return fmt.Errorf("failed to register decision validator: %w", err)
	}

	// rulestatus validates that RuleStatus is a valid enum value
	if err := v.RegisterValidation("rulestatus", validateRuleStatus); err != nil {
		return fmt.Errorf("failed to register rulestatus validator: %w", err)
	}

	return nil
}

// validateDecision validates that the Decision is a valid enum value.
func validateDecision(fl validator.FieldLevel) bool {
	decision := model.Decision(fl.Field().String())

	return decision.IsValid()
}

// validateRuleStatus validates that the RuleStatus is a valid enum value.
func validateRuleStatus(fl validator.FieldLevel) bool {
	field := fl.Field()

	// Handle pointer type
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return true // omitempty handles nil
		}

		field = field.Elem()
	}

	status := model.RuleStatus(field.String())

	return status.IsValid()
}

// validateTransactionType validates that the TransactionType is a valid enum value.
func validateTransactionType(fl validator.FieldLevel) bool {
	field := fl.Field()

	// Handle pointer type
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return true // omitempty handles nil
		}

		field = field.Elem()
	}

	// Get the string value and check if it's a valid TransactionType
	txType := model.TransactionType(field.String())

	return txType.IsValid()
}

// validateScopeNotEmpty validates that a model.Scope has at least one field set.
func validateScopeNotEmpty(fl validator.FieldLevel) bool {
	scope, ok := fl.Field().Interface().(model.Scope)
	if !ok {
		return false
	}

	return !scope.IsEmpty()
}

// CreateRuleInput represents the input for creating a new rule.
// Note: Priority field removed from MVP (TRD v1.2.4) - all rules evaluated, DENY takes precedence.
type CreateRuleInput struct {
	Name        string         `json:"name" validate:"required,min=1,max=255"`
	Description string         `json:"description" validate:"max=1000"`
	Expression  string         `json:"expression" validate:"required,min=1,max=5000"`
	Action      model.Decision `json:"action" validate:"required,decision"`
	Scopes      []model.Scope  `json:"scopes" validate:"max=100,dive,scopenotempty"`
}

// Validate validates the CreateRuleInput struct using validator/v10.
// All validations including scopenotempty are handled by validator tags.
func (c *CreateRuleInput) Validate() error {
	v, err := getValidator()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrValidatorInit, err)
	}

	if err := v.Struct(c); err != nil {
		return formatValidationError(err)
	}

	return nil
}

// UpdateRuleInput represents the input for updating an existing rule.
// All fields are optional (pointers) to support partial updates.
// Note: Priority field removed from MVP (TRD v1.2.4).
type UpdateRuleInput struct {
	Name        *string         `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Description *string         `json:"description,omitempty" validate:"omitempty,max=1000"`
	Expression  *string         `json:"expression,omitempty" validate:"omitempty,min=1,max=5000"`
	Action      *model.Decision `json:"action,omitempty" validate:"omitempty,decision"`
	Scopes      *[]model.Scope  `json:"scopes,omitempty" validate:"omitempty,max=100,dive,scopenotempty"`
}

// Validate validates the UpdateRuleInput struct using validator/v10.
func (u *UpdateRuleInput) Validate() error {
	v, err := getValidator()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrValidatorInit, err)
	}

	if err := v.Struct(u); err != nil {
		return formatValidationError(err)
	}

	return nil
}

// IsEmpty returns true if no fields are set for update.
func (u *UpdateRuleInput) IsEmpty() bool {
	return u.Name == nil &&
		u.Description == nil &&
		u.Expression == nil &&
		u.Action == nil &&
		u.Scopes == nil
}

// ListRulesInput represents the input for listing rules with cursor-based pagination.
// Uses cursor-based pagination for consistent results during navigation.
// Aligned with TRD v1.2.4 - priority removed from sortBy options.
type ListRulesInput struct {
	Name            *string           `query:"name"`
	Status          *model.RuleStatus `query:"status"`
	Action          *model.Decision   `query:"action"`
	AccountID       *string           `query:"account_id"`
	SegmentID       *string           `query:"segment_id"`
	PortfolioID     *string           `query:"portfolio_id"`
	MerchantID      *string           `query:"merchant_id"`
	TransactionType *string           `query:"transaction_type"`
	SubType         *string           `query:"sub_type" validate:"omitempty,max=50"`
	Limit           *int              `query:"limit"`
	Cursor          string            `query:"cursor"`
	SortBy          string            `query:"sort_by" enums:"created_at,updated_at,name,status"`
	SortOrder       string            `query:"sort_order" enums:"ASC,DESC"`
}

// Validate validates the ListRulesInput struct.
// Validates before defaults are applied to ensure fail-fast behavior.
func (l *ListRulesInput) Validate() error {
	// Validate pagination limit (TRC-0040, TRC-0041)
	if err := ValidatePaginationLimit(l.Limit, 100); err != nil {
		return err
	}

	// Validate cursor consistency (TRC-0045)
	if err := ValidateCursorConsistency(l.Cursor, l.SortBy, l.SortOrder); err != nil {
		return err
	}

	// Validate sortBy whitelist (TRC-0043)
	allowedSortFields := []string{"created_at", "updated_at", "name", "status"}
	if err := ValidateSortBy(l.SortBy, allowedSortFields); err != nil {
		return err
	}

	// Validate sortOrder enum (TRC-0042)
	if err := ValidateSortOrder(l.SortOrder); err != nil {
		return err
	}

	// Validate status filter (TRC-0006 for invalid values)
	if l.Status != nil {
		if !l.Status.IsValid() {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityRule, "filters")
		}
		// DELETED status is not allowed as a filter (audit handled separately)
		if *l.Status == model.RuleStatusDeleted {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityRule, "filters")
		}
	}

	// Validate action filter (TRC-0006 for invalid values)
	if l.Action != nil && !l.Action.IsValid() {
		return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityRule, "filters")
	}

	// Validate scope filter fields (TRC-0006 for invalid values)
	if err := l.validateScopeFields(); err != nil {
		return err
	}

	return nil
}

// validateScopeFields validates scope-related query parameters.
func (l *ListRulesInput) validateScopeFields() error {
	// Validate UUID fields
	uuidFields := []struct {
		value *string
		name  string
	}{
		{l.AccountID, "account_id"},
		{l.SegmentID, "segment_id"},
		{l.PortfolioID, "portfolio_id"},
		{l.MerchantID, "merchant_id"},
	}

	for _, f := range uuidFields {
		if f.value != nil && *f.value != "" {
			if _, err := uuid.Parse(*f.value); err != nil {
				return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityRule, "filters")
			}
		}
	}

	// Validate transactionType enum
	if l.TransactionType != nil && *l.TransactionType != "" {
		txType := model.TransactionType(*l.TransactionType)
		if !txType.IsValid() {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityRule, "filters")
		}
	}

	// Validate subType length against trimmed value so whitespace-only input
	// (treated as nil/no-filter by buildScopeFromInput) is not rejected
	// purely because the raw string happens to exceed the limit.
	if l.SubType != nil {
		trimmedSubType := strings.TrimSpace(*l.SubType)
		if trimmedSubType != "" && len(trimmedSubType) > MaxRuleSubTypeLength {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityRule, "filters")
		}
	}

	return nil
}

// SetDefaults sets default values for pagination if not provided.
// Note: SortBy and SortOrder defaults are only applied when cursor is not present,
// because cursor already contains sort configuration (TRC-0045).
func (l *ListRulesInput) SetDefaults() {
	if l.Limit == nil {
		defaultLimit := trcConstant.DefaultPaginationLimit
		l.Limit = &defaultLimit
	}

	// Only apply sort defaults when not using cursor pagination
	// Cursor already contains sort configuration from the original request (TRC-0045)
	if l.Cursor == "" {
		if l.SortBy == "" {
			l.SortBy = "created_at"
		}

		// Normalize sortOrder to uppercase and apply default if empty
		l.SortOrder = NormalizeSortOrder(l.SortOrder, "DESC")
	}
}

// ListRulesResponse represents the response for listing rules.
// Uses cursor-based pagination per PROJECT_RULES.md.
type ListRulesResponse struct {
	Rules      []model.Rule `json:"rules"`
	NextCursor string       `json:"nextCursor,omitempty"`
	HasMore    bool         `json:"hasMore"`
}

// toListFilter converts HTTP ListRulesInput to model.ListRulesFilter.
// SortBy is passed as snake_case and maps directly to DB column names.
// SortOrder is already normalized to uppercase during validation.
// Scope fields are converted from strings to typed values (UUIDs, enums).
// Pre-validated by Validate() - safe to use uuid.MustParse for UUID fields.
func toListFilter(input *ListRulesInput) *model.ListRulesFilter {
	limit := trcConstant.DefaultPaginationLimit
	if input.Limit != nil {
		limit = *input.Limit
	}

	filter := &model.ListRulesFilter{
		Name:      input.Name,
		Status:    input.Status,
		Action:    input.Action,
		Limit:     limit,
		Cursor:    input.Cursor,
		SortBy:    input.SortBy,    // snake_case; maps directly to DB column
		SortOrder: input.SortOrder, // Already normalized to uppercase during validation
	}

	// Build scope filter from individual query parameters
	if scope := buildScopeFromInput(input); scope != nil {
		filter.ScopeFilter = scope
	}

	return filter
}

// buildScopeFromInput constructs a model.Scope from ListRulesInput scope fields.
// Returns nil if no scope fields are provided (all nil or empty strings).
// Uses uuid.MustParse because Validate() has already verified UUID format.
func buildScopeFromInput(input *ListRulesInput) *model.Scope {
	var scope model.Scope

	hasField := false

	if input.AccountID != nil && *input.AccountID != "" {
		id := uuid.MustParse(*input.AccountID)
		scope.AccountID = &id
		hasField = true
	}

	if input.SegmentID != nil && *input.SegmentID != "" {
		id := uuid.MustParse(*input.SegmentID)
		scope.SegmentID = &id
		hasField = true
	}

	if input.PortfolioID != nil && *input.PortfolioID != "" {
		id := uuid.MustParse(*input.PortfolioID)
		scope.PortfolioID = &id
		hasField = true
	}

	if input.MerchantID != nil && *input.MerchantID != "" {
		id := uuid.MustParse(*input.MerchantID)
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

// toListResponse converts model.ListRulesResult to ListRulesResponse.
func toListResponse(result *model.ListRulesResult) *ListRulesResponse {
	// Ensure Rules is never nil to avoid "rules": null in JSON response
	rules := result.Rules
	if rules == nil {
		rules = []model.Rule{}
	}

	return &ListRulesResponse{
		Rules:      rules,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}

// formatValidationError formats validator errors into ValidationError with specific TRC codes.
// Returns the first validation error with its specific TRC code for consistent API responses.
func formatValidationError(err error) error {
	var validationErrors validator.ValidationErrors

	ok := errors.As(err, &validationErrors)
	if !ok {
		return err
	}

	if len(validationErrors) == 0 {
		return nil
	}

	// Return the first error with its specific TRC code
	// This ensures consistent API responses with one error code per request
	for _, fieldError := range validationErrors {
		fieldName := fieldError.Field()
		tag := fieldError.Tag()
		namespace := fieldError.Namespace()

		// Check if this is a scope field error (from dive validation)
		if isScopeFieldError(namespace) {
			return formatScopeFieldErrorWithCode(fieldError)
		}

		jsonFieldName := toJSONFieldName(fieldName)

		switch tag {
		case "required":
			return mapRequiredFieldToError(jsonFieldName)
		case "max":
			return mapMaxFieldToError(jsonFieldName, fieldError.Param())
		case "decision":
			return pkg.ValidateBusinessError(constant.ErrRuleInvalidAction, constant.EntityRule)
		default:
			// Generic validation error for unmapped cases
			return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityRule)
		}
	}

	return nil
}

// mapRequiredFieldToError maps a required field error to its specific TRC code.
func mapRequiredFieldToError(fieldName string) error {
	switch fieldName {
	case "name":
		return pkg.ValidateBusinessError(constant.ErrRuleNameRequired, constant.EntityRule)
	case "expression":
		return pkg.ValidateBusinessError(constant.ErrRuleExpressionRequired, constant.EntityRule)
	case "action":
		return pkg.ValidateBusinessError(constant.ErrRuleInvalidAction, constant.EntityRule)
	default:
		return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityRule)
	}
}

// mapMaxFieldToError maps a max validation error to its specific TRC code.
func mapMaxFieldToError(fieldName, maxValue string) error {
	switch fieldName {
	case "name":
		return pkg.ValidateBusinessError(constant.ErrRuleNameTooLong, constant.EntityRule)
	case "description":
		return pkg.ValidateBusinessError(constant.ErrRuleDescriptionTooLong, constant.EntityRule)
	case "expression":
		return pkg.ValidateBusinessError(constant.ErrRuleExpressionTooLong, constant.EntityRule)
	case "scopes":
		return pkg.ValidateBusinessError(constant.ErrRuleScopesTooMany, constant.EntityRule)
	default:
		return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityRule)
	}
}

// formatScopeFieldErrorWithCode formats a scope field validation error with TRC-0111.
func formatScopeFieldErrorWithCode(fieldError validator.FieldError) error {
	return pkg.ValidateBusinessError(constant.ErrRuleInvalidScope, constant.EntityRule)
}

// isScopeFieldError checks if the error is from a scope field (via dive validation).
func isScopeFieldError(namespace string) bool {
	// Namespace format: "CreateRuleInput.Scopes[0].FieldName"
	return strings.Contains(namespace, "Scopes[")
}

// extractScopeIndex extracts the scope index from the namespace.
// Returns the index and nil error on success, or -1 and an error with details on failure.
func extractScopeIndex(namespace string) (int, error) {
	// Find "Scopes[" and extract the number
	start := strings.Index(namespace, "Scopes[")
	if start == -1 {
		return -1, fmt.Errorf("'Scopes[' not found in namespace: %s", namespace)
	}

	start += len("Scopes[")
	end := strings.Index(namespace[start:], "]")

	if end == -1 {
		return -1, fmt.Errorf("closing bracket ']' not found in namespace: %s", namespace)
	}

	var index int

	n, scanErr := fmt.Sscanf(namespace[start:start+end], "%d", &index)
	if n != 1 {
		return -1, fmt.Errorf("failed to parse index from substring '%s' in namespace: %s (scan error: %w)", namespace[start:start+end], namespace, scanErr)
	}

	if index < 0 {
		return -1, fmt.Errorf("negative index %d is invalid in namespace: %s", index, namespace)
	}

	return index, nil
}

// toJSONFieldName converts struct field name to JSON field name.
func toJSONFieldName(fieldName string) string {
	switch fieldName {
	case "Name":
		return "name"
	case "Description":
		return "description"
	case "Expression":
		return "expression"
	case "Action":
		return "action"
	case "Scopes":
		return "scopes"
	default:
		return fieldName
	}
}

// toScopeJSONFieldName converts Scope field name to JSON field name.
func toScopeJSONFieldName(fieldName string) string {
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
