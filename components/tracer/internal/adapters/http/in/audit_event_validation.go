// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"fmt"
	"reflect"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Validation constants for audit event input fields.
const (
	MaxAuditEventLimit     = 1000
	DefaultAuditEventLimit = 100
)

// registerAuditEventValidations registers all audit-event-specific validation functions.
func registerAuditEventValidations(v *validator.Validate) error {
	validations := map[string]validator.Func{
		"auditeventtype":  validateAuditEventType,
		"auditaction":     validateAuditAction,
		"auditresult":     validateAuditResult,
		"resourcetype":    validateResourceType,
		"actortype":       validateActorType,
		"transactiontype": validateTransactionType,
	}

	for name, fn := range validations {
		if err := v.RegisterValidation(name, fn); err != nil {
			return fmt.Errorf("failed to register %s validator: %w", name, err)
		}
	}

	return nil
}

// validateAuditEventType validates that the AuditEventType is a valid enum value.
func validateAuditEventType(fl validator.FieldLevel) bool {
	field := fl.Field()
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return true
		}

		field = field.Elem()
	}

	eventType := model.AuditEventType(field.String())
	switch eventType {
	case model.AuditEventTransactionValidated,
		model.AuditEventRuleCreated, model.AuditEventRuleUpdated,
		model.AuditEventRuleActivated, model.AuditEventRuleDeactivated,
		model.AuditEventRuleDrafted, model.AuditEventRuleDeleted,
		model.AuditEventLimitCreated, model.AuditEventLimitUpdated,
		model.AuditEventLimitActivated, model.AuditEventLimitDeactivated,
		model.AuditEventLimitDrafted, model.AuditEventLimitDeleted:
		return true
	default:
		return false
	}
}

// validateAuditAction validates that the AuditAction is a valid enum value.
func validateAuditAction(fl validator.FieldLevel) bool {
	field := fl.Field()
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return true
		}

		field = field.Elem()
	}

	action := model.AuditAction(field.String())
	switch action {
	case model.AuditActionValidate, model.AuditActionCreate,
		model.AuditActionUpdate, model.AuditActionDelete,
		model.AuditActionActivate, model.AuditActionDeactivate,
		model.AuditActionDraft:
		return true
	default:
		return false
	}
}

// validateAuditResult validates that the AuditResult is a valid enum value.
func validateAuditResult(fl validator.FieldLevel) bool {
	field := fl.Field()
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return true
		}

		field = field.Elem()
	}

	result := model.AuditResult(field.String())
	switch result {
	case model.AuditResultSuccess, model.AuditResultFailed,
		model.AuditResultAllow, model.AuditResultDeny,
		model.AuditResultReview:
		return true
	default:
		return false
	}
}

// validateResourceType validates that the ResourceType is a valid enum value.
func validateResourceType(fl validator.FieldLevel) bool {
	field := fl.Field()
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return true
		}

		field = field.Elem()
	}

	resourceType := model.ResourceType(field.String())
	switch resourceType {
	case model.ResourceTypeTransaction, model.ResourceTypeRule,
		model.ResourceTypeLimit:
		return true
	default:
		return false
	}
}

// validateActorType validates that the ActorType is a valid enum value.
func validateActorType(fl validator.FieldLevel) bool {
	field := fl.Field()
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return true
		}

		field = field.Elem()
	}

	actorType := model.ActorType(field.String())
	switch actorType {
	case model.ActorTypeUser, model.ActorTypeSystem:
		return true
	default:
		return false
	}
}

// ListAuditEventsInput represents the input for listing audit events with filters and pagination.
type ListAuditEventsInput struct {
	// Date range filters
	StartDate string `query:"start_date,omitempty"` // RFC3339 format
	EndDate   string `query:"end_date,omitempty"`   // RFC3339 format

	// Core filters
	EventType    *model.AuditEventType `query:"event_type,omitempty" validate:"omitempty,auditeventtype"`
	Action       *model.AuditAction    `query:"action,omitempty" validate:"omitempty,auditaction"`
	Result       *model.AuditResult    `query:"result,omitempty" validate:"omitempty,auditresult"`
	ResourceType *model.ResourceType   `query:"resource_type,omitempty" validate:"omitempty,resourcetype"`
	ResourceID   *string               `query:"resource_id,omitempty" validate:"omitempty,uuid"`

	// Actor filters
	ActorType *model.ActorType `query:"actor_type,omitempty" validate:"omitempty,actortype"`
	ActorID   *string          `query:"actor_id,omitempty"`
	// JSONB filters
	AccountID       *string `query:"account_id,omitempty" validate:"omitempty,uuid"`
	SegmentID       *string `query:"segment_id,omitempty" validate:"omitempty,uuid"`
	PortfolioID     *string `query:"portfolio_id,omitempty" validate:"omitempty,uuid"`
	TransactionType *string `query:"transaction_type,omitempty" validate:"omitempty,transactiontype"`
	MatchedRuleID   *string `query:"matched_rule_id,omitempty" validate:"omitempty,uuid"`
	// Pagination
	Limit     *int   `query:"limit"`
	Cursor    string `query:"cursor"`
	SortBy    string `query:"sort_by" enums:"created_at,event_type"`
	SortOrder string `query:"sort_order" enums:"ASC,DESC"`
}

// Validate validates the ListAuditEventsInput struct.
func (l *ListAuditEventsInput) Validate() error {
	// Validate pagination limit with specific error codes (TRC-0040, TRC-0041)
	if err := ValidatePaginationLimit(l.Limit, MaxAuditEventLimit); err != nil {
		return err
	}

	// Validate cursor consistency (TRC-0045)
	if err := ValidateCursorConsistency(l.Cursor, l.SortBy, l.SortOrder); err != nil {
		return err
	}

	// Validate sortBy whitelist (TRC-0043)
	allowedSortFields := []string{"created_at", "event_type"}
	if err := ValidateSortBy(l.SortBy, allowedSortFields); err != nil {
		return err
	}

	// Validate sortOrder enum (TRC-0042)
	if err := ValidateSortOrder(l.SortOrder); err != nil {
		return err
	}

	// Validate remaining fields using go-playground validator
	v, err := getValidator()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrValidatorInit, err)
	}

	if err := v.Struct(l); err != nil {
		return formatValidationError(err)
	}

	// Validate date formats if provided (TRC-0020)
	if l.StartDate != "" {
		if _, err := time.Parse(time.RFC3339, l.StartDate); err != nil {
			return pkg.ValidateBusinessError(constant.ErrInvalidDateFormat, constant.EntityAuditEvent)
		}
	}

	if l.EndDate != "" {
		if _, err := time.Parse(time.RFC3339, l.EndDate); err != nil {
			return pkg.ValidateBusinessError(constant.ErrInvalidDateFormat, constant.EntityAuditEvent)
		}
	}

	// Validate date range logic
	if l.StartDate != "" && l.EndDate != "" {
		start, _ := time.Parse(time.RFC3339, l.StartDate)

		end, _ := time.Parse(time.RFC3339, l.EndDate)
		if end.Before(start) {
			return pkg.ValidateBusinessError(constant.ErrInvalidDateRange, constant.EntityAuditEvent)
		}
	}

	return nil
}

// SetDefaults sets default values for pagination if not provided.
// Note: SortBy and SortOrder defaults are only applied when cursor is not present,
// because cursor already contains sort configuration (TRC-0045).
func (l *ListAuditEventsInput) SetDefaults() {
	if l.Limit == nil {
		defaultLimit := DefaultAuditEventLimit
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

// ListAuditEventsResponse represents the response for listing audit events.
type ListAuditEventsResponse struct {
	AuditEvents []*model.AuditEvent `json:"auditEvents" validate:"max=1000" maxItems:"1000"`
	NextCursor  string              `json:"nextCursor,omitempty" example:"eyJpZCI6IjAxOTI4In0="`
	HasMore     bool                `json:"hasMore" example:"true"`
}

// toAuditEventFilters converts ListAuditEventsInput to model.AuditEventFilters.
func toAuditEventFilters(input *ListAuditEventsInput) (*model.AuditEventFilters, error) {
	limit := DefaultAuditEventLimit
	if input.Limit != nil {
		limit = *input.Limit
	}

	filters := &model.AuditEventFilters{
		EventType:    input.EventType,
		Action:       input.Action,
		Result:       input.Result,
		ResourceType: input.ResourceType,
		ResourceID:   input.ResourceID,
		ActorType:    input.ActorType,
		ActorID:      input.ActorID,
		Limit:        limit,
		Cursor:       input.Cursor,
		SortBy:       input.SortBy,
		SortOrder:    input.SortOrder,
	}

	// Parse dates
	if input.StartDate != "" {
		startDate, err := time.Parse(time.RFC3339, input.StartDate)
		if err != nil {
			return nil, fmt.Errorf("invalid start_date format: %w", err)
		}

		filters.StartDate = startDate
	}

	if input.EndDate != "" {
		endDate, err := time.Parse(time.RFC3339, input.EndDate)
		if err != nil {
			return nil, fmt.Errorf("invalid end_date format: %w", err)
		}

		filters.EndDate = endDate
	}

	// Parse UUIDs for JSONB filters (pre-validated, safe to use MustParse)
	if input.AccountID != nil {
		accountID := uuid.MustParse(*input.AccountID)
		filters.AccountID = &accountID
	}

	if input.SegmentID != nil {
		segmentID := uuid.MustParse(*input.SegmentID)
		filters.SegmentID = &segmentID
	}

	if input.PortfolioID != nil {
		portfolioID := uuid.MustParse(*input.PortfolioID)
		filters.PortfolioID = &portfolioID
	}

	if input.TransactionType != nil {
		txType := model.TransactionType(*input.TransactionType)
		filters.TransactionType = &txType
	}

	if input.MatchedRuleID != nil {
		matchedRuleID := uuid.MustParse(*input.MatchedRuleID)
		filters.MatchedRuleID = &matchedRuleID
	}

	return filters, nil
}

// toListAuditEventsResponse converts model.ListAuditEventsResult to ListAuditEventsResponse.
func toListAuditEventsResponse(result *model.ListAuditEventsResult) *ListAuditEventsResponse {
	// Ensure AuditEvents is never nil to avoid "auditEvents": null in JSON response
	auditEvents := result.AuditEvents
	if auditEvents == nil {
		auditEvents = []*model.AuditEvent{}
	}

	return &ListAuditEventsResponse{
		AuditEvents: auditEvents,
		NextCursor:  result.NextCursor,
		HasMore:     result.HasMore,
	}
}
