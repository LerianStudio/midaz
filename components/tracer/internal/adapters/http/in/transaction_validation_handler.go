// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

//go:generate mockgen -source=transaction_validation_handler.go -destination=mocks/transaction_validation_service_mock.go -package=mocks

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/validation"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// TransactionValidationService defines the interface for transaction validation operations.
// Interface defined locally per Ring pattern.
// NOTE: Transaction validations are immutable per SOX/GLBA requirements - only read operations.
type TransactionValidationService interface {
	GetTransactionValidation(ctx context.Context, id uuid.UUID) (*model.TransactionValidation, error)
	ListTransactionValidations(ctx context.Context, filters *model.TransactionValidationFilters) (*query.ListTransactionValidationsResult, error)
}

// TransactionValidationHandler handles HTTP requests for transaction validation operations.
type TransactionValidationHandler struct {
	service TransactionValidationService
}

// NewTransactionValidationHandler creates a new transaction validation handler.
func NewTransactionValidationHandler(service TransactionValidationService) *TransactionValidationHandler {
	return &TransactionValidationHandler{
		service: service,
	}
}

// GetTransactionValidation godoc
//
//	@Summary		Get a transaction validation record by ID
//	@Description	Retrieves a transaction validation record by its unique identifier.
//	@ID				getValidation
//	@Tags			validations
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string					true	"Transaction Validation ID (UUID)"	Format(uuid)
//	@Success		200			{object}	model.TransactionValidation	"Transaction validation retrieved successfully"
//	@Failure		400			{object}	api.ErrorResponse		"Invalid transaction validation ID"
//	@Failure		401			{object}	api.ErrorResponse		"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse		"Transaction validation not found"
//	@Failure		500			{object}	api.ErrorResponse		"Internal server error"
//	@Router			/v1/validations/{id} [get]
func (h *TransactionValidationHandler) GetTransactionValidation(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.transaction-validation.get")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Parse transaction validation ID from path
	idParam := c.Params("id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid transaction validation ID", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityTransactionValidation, "id"))
	}

	logger.With(
		libLog.String("operation", "handler.transaction-validation.get"),
		libLog.String("transaction_validation.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Getting transaction validation record")

	result, err := h.service.GetTransactionValidation(ctx, id)
	if err != nil {
		return handleTransactionValidationServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.transaction-validation.get"),
		libLog.String("transaction_validation.id", result.ID.String()),
		libLog.String("transaction_validation.decision", string(result.Decision)),
	).Log(ctx, libLog.LevelInfo, "Transaction validation record retrieved")

	return http.OK(c, result)
}

// ListTransactionValidations godoc
//
//	@Summary		List transaction validation records
//	@Description	Lists transaction validation records with cursor-based pagination and filters.
//	@ID				listValidations
//	@Tags			validations
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			limit			query		int								false	"Max items per page (1-1000, default: 100)"		minimum(1)	maximum(1000)
//	@Param			cursor			query		string							false	"Pagination cursor from previous response"
//	@Param			sort_by			query		string							false	"Field to sort by (default: created_at)"		Enums(created_at, processing_time_ms)
//	@Param			sort_order		query		string							false	"Sort direction (default: DESC)"				Enums(ASC, DESC)
//	@Param			start_date		query		string							false	"Filter from this date (RFC3339)"				Format(date-time)
//	@Param			end_date		query		string							false	"Filter to this date (RFC3339)"					Format(date-time)
//	@Param			decision		query		string							false	"Filter by decision"							Enums(ALLOW, DENY, REVIEW)
//	@Param			account_id		query		string							false	"Filter by account ID (UUID)"					Format(uuid)
//	@Param			matched_rule_id	query		string							false	"Filter by matched rule ID (UUID)"				Format(uuid)
//	@Param			exceeded_limit_id	query		string							false	"Filter by exceeded limit ID (UUID)"			Format(uuid)
//	@Param			segment_id		query		string							false	"Filter by segment ID (UUID)"					Format(uuid)
//	@Param			portfolio_id	query		string							false	"Filter by portfolio ID (UUID)"					Format(uuid)
//	@Param			transaction_type	query		string							false	"Filter by transaction type"					Enums(CARD, WIRE, PIX, CRYPTO)
//	@Success		200				{object}	ListTransactionValidationsResponse	"Transaction validations listed successfully"
//	@Failure		400				{object}	api.ErrorResponse				"Invalid parameters"
//	@Failure		401				{object}	api.ErrorResponse				"Unauthorized"
//	@Failure		500				{object}	api.ErrorResponse				"Internal server error"
//	@Router			/v1/validations [get]
func (h *TransactionValidationHandler) ListTransactionValidations(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.transaction-validation.list")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Parse query parameters into input struct
	var input ListTransactionValidationsInput

	if err := c.QueryParser(&input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse query parameters", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityTransactionValidation, "filters"))
	}

	// Validate before applying defaults to ensure fail-fast behavior
	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)

		return http.WithError(c, err)
	}

	// Apply defaults after validation
	input.SetDefaults()

	logger.With(
		libLog.String("operation", "handler.transaction-validation.list"),
		libLog.Any("list.limit", input.Limit),
		libLog.String("list.cursor", input.Cursor),
		libLog.String("list.sort_by", input.SortBy),
		libLog.String("list.sort_order", input.SortOrder),
	).Log(ctx, libLog.LevelInfo, "Listing transaction validation records")

	// Convert to service filter
	filters, err := ToTransactionValidationFilters(&input)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid filters", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidTransactionValidationFilters, constant.EntityTransactionValidation))
	}

	result, err := h.service.ListTransactionValidations(ctx, filters)
	if err != nil {
		return handleTransactionValidationServiceError(c, span, err)
	}

	// Convert to response
	response := ToListTransactionValidationsResponse(result)

	logger.With(
		libLog.String("operation", "handler.transaction-validation.list"),
		libLog.Int("list.count", len(response.TransactionValidations)),
		libLog.Bool("list.has_more", response.HasMore),
	).Log(ctx, libLog.LevelInfo, "Transaction validation records listed")

	return http.OK(c, response)
}

// handleTransactionValidationServiceError converts service errors to appropriate HTTP responses.
func handleTransactionValidationServiceError(c *fiber.Ctx, span trace.Span, err error) error {
	switch {
	case errors.Is(err, constant.ErrTransactionValidationNotFound):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction validation not found", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrTransactionValidationNotFound, constant.EntityTransactionValidation))
	case errors.Is(err, constant.ErrInvalidTransactionValidationFilters):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid transaction validation filters", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidTransactionValidationFilters, constant.EntityTransactionValidation))
	case errors.Is(err, constant.ErrInvalidCursor):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid cursor", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidCursor, constant.EntityTransactionValidation))
	default:
		libOpentelemetry.HandleSpanError(span, "Operation failed", err)

		return http.WithError(c, pkg.InternalServerError{Code: constant.ErrInternalServer.Error(), Title: "Internal Server Error", Message: "The server encountered an unexpected error. Please try again later or contact support."})
	}
}

// ListTransactionValidationsInput represents query parameters for listing transaction validation records.
// Uses cursor-based pagination for consistent, efficient pagination with large datasets.
type ListTransactionValidationsInput struct {
	Limit           *int   `query:"limit"`
	Cursor          string `query:"cursor"`
	SortBy          string `query:"sort_by"`
	SortOrder       string `query:"sort_order"`
	StartDate       string `query:"start_date"`
	EndDate         string `query:"end_date"`
	Decision        string `query:"decision"`
	AccountID       string `query:"account_id"`
	MatchedRuleID   string `query:"matched_rule_id"`
	ExceededLimitID string `query:"exceeded_limit_id"`
	SegmentID       string `query:"segment_id"`
	PortfolioID     string `query:"portfolio_id"`
	TransactionType string `query:"transaction_type"`
}

// validateUUID checks if a string is a valid UUID format.
// Returns nil if value is empty (optional field) or valid UUID.
func validateUUID(value, fieldName string) error {
	if value == "" {
		return nil
	}

	if _, err := uuid.Parse(value); err != nil {
		return pkg.ValidateBusinessError(constant.ErrInvalidTransactionValidationFilters, constant.EntityTransactionValidation)
	}

	return nil
}

// SetDefaults sets default values for optional fields after validation.
// Note: Only applies sort defaults when cursor is not present, as cursor already contains sort configuration.
func (i *ListTransactionValidationsInput) SetDefaults() {
	if i.Limit == nil {
		defaultLimit := model.DefaultTransactionValidationFilterLimit
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

// Validate checks if the input is valid.
// Validates before defaults are applied to ensure fail-fast behavior.
func (i *ListTransactionValidationsInput) Validate() error {
	// Validate pagination limit (TRC-0040, TRC-0041)
	if err := ValidatePaginationLimit(i.Limit, model.MaxTransactionValidationFilterLimit); err != nil {
		return err
	}

	// Validate cursor consistency (TRC-0045)
	if err := ValidateCursorConsistency(i.Cursor, i.SortBy, i.SortOrder); err != nil {
		return err
	}

	// Validate sortBy whitelist (TRC-0043)
	allowedSortFields := []string{"created_at", "processing_time_ms"}
	if err := ValidateSortBy(i.SortBy, allowedSortFields); err != nil {
		return err
	}

	// Validate sortOrder enum (TRC-0042)
	if err := ValidateSortOrder(i.SortOrder); err != nil {
		return err
	}

	if err := i.validateDecision(); err != nil {
		return err
	}

	if err := i.validateDates(); err != nil {
		return err
	}

	if err := i.validateUUIDFilters(); err != nil {
		return err
	}

	return i.validateTransactionType()
}

func (i *ListTransactionValidationsInput) validateDecision() error {
	if i.Decision != "" && !model.Decision(i.Decision).IsValid() {
		return pkg.ValidateBusinessError(constant.ErrInvalidTransactionValidationFilters, constant.EntityTransactionValidation)
	}

	return nil
}

func (i *ListTransactionValidationsInput) validateDates() error {
	startDate, err := validation.ParseRFC3339Timestamp(i.StartDate, "start_date")
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrInvalidDateFormat, constant.EntityTransactionValidation)
	}

	endDate, err := validation.ParseRFC3339Timestamp(i.EndDate, "end_date")
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrInvalidDateFormat, constant.EntityTransactionValidation)
	}

	// Use user-friendly error message for date range validation
	if !startDate.IsZero() && !endDate.IsZero() && startDate.After(endDate) {
		return pkg.ValidateBusinessError(constant.ErrInvalidDateRange, constant.EntityTransactionValidation)
	}

	return nil
}

func (i *ListTransactionValidationsInput) validateUUIDFilters() error {
	if err := validateUUID(i.AccountID, "account_id"); err != nil {
		return err
	}

	if err := validateUUID(i.MatchedRuleID, "matched_rule_id"); err != nil {
		return err
	}

	if err := validateUUID(i.ExceededLimitID, "exceeded_limit_id"); err != nil {
		return err
	}

	if err := validateUUID(i.SegmentID, "segment_id"); err != nil {
		return err
	}

	return validateUUID(i.PortfolioID, "portfolio_id")
}

func (i *ListTransactionValidationsInput) validateTransactionType() error {
	if i.TransactionType != "" && !model.TransactionType(i.TransactionType).IsValid() {
		return pkg.ValidateBusinessError(constant.ErrInvalidTransactionValidationFilters, constant.EntityTransactionValidation)
	}

	return nil
}

// ToTransactionValidationFilters converts the HTTP input to service filters.
func ToTransactionValidationFilters(input *ListTransactionValidationsInput) (*model.TransactionValidationFilters, error) {
	limit := model.DefaultTransactionValidationFilterLimit
	if input.Limit != nil {
		limit = *input.Limit
	}

	filters := &model.TransactionValidationFilters{
		Limit:     limit,
		Cursor:    input.Cursor,
		SortBy:    input.SortBy,
		SortOrder: input.SortOrder,
	}

	// Parse start date
	if input.StartDate != "" {
		startDate, err := time.Parse(time.RFC3339, input.StartDate)
		if err != nil {
			return nil, err
		}

		filters.StartDate = startDate
	}

	// Parse end date
	if input.EndDate != "" {
		endDate, err := time.Parse(time.RFC3339, input.EndDate)
		if err != nil {
			return nil, err
		}

		filters.EndDate = endDate
	}

	// Parse decision
	if input.Decision != "" {
		decision := model.Decision(input.Decision)
		filters.Decision = &decision
	}

	// Parse account ID
	if input.AccountID != "" {
		accountID, err := uuid.Parse(input.AccountID)
		if err != nil {
			return nil, err
		}

		filters.AccountID = &accountID
	}

	// Parse matched rule ID
	if input.MatchedRuleID != "" {
		matchedRuleID, err := uuid.Parse(input.MatchedRuleID)
		if err != nil {
			return nil, err
		}

		filters.MatchedRuleID = &matchedRuleID
	}

	// Parse exceeded limit ID
	if input.ExceededLimitID != "" {
		exceededLimitID, err := uuid.Parse(input.ExceededLimitID)
		if err != nil {
			return nil, err
		}

		filters.ExceededLimitID = &exceededLimitID
	}

	// Parse segment ID
	if input.SegmentID != "" {
		segmentID, err := uuid.Parse(input.SegmentID)
		if err != nil {
			return nil, err
		}

		filters.SegmentID = &segmentID
	}

	// Parse portfolio ID
	if input.PortfolioID != "" {
		portfolioID, err := uuid.Parse(input.PortfolioID)
		if err != nil {
			return nil, err
		}

		filters.PortfolioID = &portfolioID
	}

	// Parse transaction type
	if input.TransactionType != "" {
		transactionType := model.TransactionType(input.TransactionType)
		filters.TransactionType = &transactionType
	}

	return filters, nil
}

// ValidationSummary represents a summary of a validation record in list responses.
// Per API Design v1.3.2: List endpoint returns ValidationSummary, not full TransactionValidation.
// Fields are flattened (accountId at root, not nested in account object).
type ValidationSummary struct {
	ID               uuid.UUID             `json:"validationId" swaggertype:"string" format:"uuid"`
	Decision         model.Decision        `json:"decision"`
	Reason           string                `json:"reason"`
	Amount           decimal.Decimal       `json:"amount" swaggertype:"string" example:"100.00"`
	Currency         string                `json:"currency"`
	TransactionType  model.TransactionType `json:"transactionType"`
	AccountID        uuid.UUID             `json:"accountId" swaggertype:"string" format:"uuid"`
	SegmentID        *uuid.UUID            `json:"segmentId,omitempty" swaggertype:"string" format:"uuid"`
	PortfolioID      *uuid.UUID            `json:"portfolioId,omitempty" swaggertype:"string" format:"uuid"`
	MatchedRuleIDs   []uuid.UUID           `json:"matchedRuleIds" swaggertype:"array,string" format:"uuid"`
	ExceededLimitIDs []uuid.UUID           `json:"exceededLimitIds" swaggertype:"array,string" format:"uuid"`
	ProcessingTimeMs float64               `json:"processingTimeMs"`
	CreatedAt        string                `json:"createdAt" format:"date-time"`
}

// ToValidationSummary converts a TransactionValidation to a ValidationSummary.
func ToValidationSummary(tv *model.TransactionValidation) *ValidationSummary {
	if tv == nil {
		return nil
	}

	summary := &ValidationSummary{
		ID:               tv.ID,
		Decision:         tv.Decision,
		Reason:           tv.Reason,
		Amount:           tv.Amount,
		Currency:         tv.Currency,
		TransactionType:  tv.TransactionType,
		AccountID:        tv.Account.ID,
		MatchedRuleIDs:   ensureUUIDSlice(tv.MatchedRuleIDs),
		ProcessingTimeMs: tv.ProcessingTimeMs,
		CreatedAt:        tv.CreatedAt.Format(time.RFC3339),
	}

	// Extract exceeded limit IDs from limit usage details
	exceededLimitIDs := make([]uuid.UUID, 0)

	for _, detail := range tv.LimitUsageDetails {
		if detail.Exceeded {
			exceededLimitIDs = append(exceededLimitIDs, detail.LimitID)
		}
	}

	summary.ExceededLimitIDs = exceededLimitIDs

	// Extract optional segment and portfolio IDs
	if tv.Segment != nil {
		summary.SegmentID = &tv.Segment.ID
	}

	if tv.Portfolio != nil {
		summary.PortfolioID = &tv.Portfolio.ID
	}

	return summary
}

// ListTransactionValidationsResponse represents the response for listing transaction validation records.
// Uses cursor-based pagination with nextCursor and hasMore fields.
// Per API Design v1.3.2: Returns ValidationSummary objects with flattened fields.
type ListTransactionValidationsResponse struct {
	TransactionValidations []*ValidationSummary `json:"transactionValidations"`
	NextCursor             string               `json:"nextCursor,omitempty"`
	HasMore                bool                 `json:"hasMore"`
}

// ToListTransactionValidationsResponse converts the service result to HTTP response.
func ToListTransactionValidationsResponse(result *query.ListTransactionValidationsResult) *ListTransactionValidationsResponse {
	summaries := make([]*ValidationSummary, len(result.TransactionValidations))
	for i, tv := range result.TransactionValidations {
		summaries[i] = ToValidationSummary(tv)
	}

	return &ListTransactionValidationsResponse{
		TransactionValidations: summaries,
		NextCursor:             result.NextCursor,
		HasMore:                result.HasMore,
	}
}

// ensureUUIDSlice returns an empty slice if input is nil, ensuring JSON serializes as [] not null.
func ensureUUIDSlice(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}

	return ids
}
