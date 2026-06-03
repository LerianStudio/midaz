// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

//go:generate mockgen -source=limit_handler.go -destination=limit_service_mock.go -package=in

import (
	"context"
	"encoding/json"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"tracer/internal/services/command"
	"tracer/pkg/constant"
	"tracer/pkg/logging"
	"tracer/pkg/model"
	pkgHTTP "tracer/pkg/net/http"
)

// LimitService defines the interface for limit operations.
// Interface defined locally per Ring pattern.
type LimitService interface {
	CreateLimit(ctx context.Context, input *command.CreateLimitInput) (*model.Limit, error)
	GetLimit(ctx context.Context, id uuid.UUID) (*model.Limit, error)
	ListLimits(ctx context.Context, filter *model.ListLimitsFilter) (*model.ListLimitsResult, error)
	UpdateLimit(ctx context.Context, id uuid.UUID, input *command.UpdateLimitInput) (*model.Limit, error)
	ActivateLimit(ctx context.Context, id uuid.UUID) (*model.Limit, error)
	DeactivateLimit(ctx context.Context, id uuid.UUID) (*model.Limit, error)
	DraftLimit(ctx context.Context, id uuid.UUID) (*model.Limit, error)
	DeleteLimit(ctx context.Context, id uuid.UUID) error
	GetLimitUsage(ctx context.Context, limitID uuid.UUID) (*model.UsageSnapshot, error)
}

// LimitHandler handles HTTP requests for limit operations.
type LimitHandler struct {
	service LimitService
}

// NewLimitHandler creates a new limit handler.
func NewLimitHandler(service LimitService) *LimitHandler {
	return &LimitHandler{
		service: service,
	}
}

// CreateLimit godoc
//
//	@Summary		Create a new spending limit
//	@Description	Creates a limit with scopes array. Limits are created in DRAFT status.
//	@ID				createLimit
//	@Tags			limits
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			limit		body		CreateLimitInput	true	"Limit details"
//	@Success		201			{object}	model.Limit				"Limit created successfully"
//	@Failure		400			{object}	api.ErrorResponse		"Invalid input"
//	@Failure		401			{object}	api.ErrorResponse		"Unauthorized"
//	@Failure		409			{object}	api.ErrorResponse		"Limit name already exists"
//	@Failure		500			{object}	api.ErrorResponse		"Internal server error"
//	@Router			/v1/limits [post]
func (h *LimitHandler) CreateLimit(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.create")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	var input CreateLimitInput
	if err := c.BodyParser(&input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse request body", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0003", "Bad Request", "Invalid request body")
	}

	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)

		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			return pkgHTTP.BadRequestWithMessage(c, validationErr.Code, "Validation Error", validationErr.Message)
		}

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0001", "Validation Error", formatValidationMessage(err))
	}

	logger.With(
		libLog.String("operation", "handler.limit.create"),
		libLog.String("limit.name", input.Name),
	).Log(ctx, libLog.LevelInfo, "Creating limit")

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "limit_input", input, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set span attributes", err)
	}

	// Convert HTTP input to service input
	serviceInput := ToCreateLimitServiceInput(&input)

	result, err := h.service.CreateLimit(ctx, serviceInput)
	if err != nil {
		return handleLimitServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.create"),
		libLog.String("limit.id", result.ID.String()),
	).Log(ctx, libLog.LevelInfo, "Limit created")

	return pkgHTTP.Created(c, result)
}

// GetLimit godoc
//
//	@Summary		Get a spending limit by ID
//	@Description	Retrieves a limit by its unique identifier.
//	@ID				getLimit
//	@Tags			limits
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string				true	"Limit ID (UUID)"	Format(uuid)
//	@Success		200			{object}	model.Limit			"Limit retrieved successfully"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid limit ID"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Limit not found"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/limits/{id} [get]
func (h *LimitHandler) GetLimit(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.get")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Parse limit ID from path
	idParam := c.Params("id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid limit ID format")
	}

	logger.With(
		libLog.String("operation", "handler.limit.get"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Getting limit")

	result, err := h.service.GetLimit(ctx, id)
	if err != nil {
		return handleLimitServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.get"),
		libLog.String("limit.id", result.ID.String()),
		libLog.String("limit.name", result.Name),
	).Log(ctx, libLog.LevelInfo, "Limit retrieved")

	return pkgHTTP.OK(c, result)
}

// ListLimits godoc
//
//	@Summary		List spending limits
//	@Description	Lists limits with cursor-based pagination and optional filters.
//	@ID				listLimits
//	@Tags			limits
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			limit			query		int						false	"Max items per page (1-100, default: 10)"	minimum(1)	maximum(100)
//	@Param			cursor			query		string					false	"Pagination cursor (empty for first page)"
//	@Param			name			query		string					false	"Filter by name (case-insensitive partial match)"	maxLength(255)
//	@Param			status			query		string					false	"Filter by status"			Enums(DRAFT, ACTIVE, INACTIVE)
//	@Param			limit_type		query		string					false	"Filter by limit type"		Enums(DAILY, WEEKLY, MONTHLY, CUSTOM, PER_TRANSACTION)
//	@Param			account_id		query		string					false	"Filter by scope account_id (UUID)"	Format(uuid)
//	@Param			segment_id		query		string					false	"Filter by scope segment_id (UUID)"	Format(uuid)
//	@Param			portfolio_id	query		string					false	"Filter by scope portfolio_id (UUID)"	Format(uuid)
//	@Param			merchant_id		query		string					false	"Filter by scope merchant_id (UUID)"	Format(uuid)
//	@Param			transaction_type	query		string					false	"Filter by scope transaction_type"	Enums(CARD, WIRE, PIX, CRYPTO)
//	@Param			sub_type		query		string					false	"Filter by scope sub_type (case-insensitive, normalized to lowercase; max 50 chars)"	maxLength(50)
//	@Param			sort_by			query		string					false	"Sort field"				Enums(created_at, updated_at, name, max_amount)
//	@Param			sort_order		query		string					false	"Sort direction"			Enums(ASC, DESC)
//	@Success		200			{object}	ListLimitsResponse	"Limits listed successfully"
//	@Failure		400			{object}	api.ErrorResponse		"Invalid parameters"
//	@Failure		401			{object}	api.ErrorResponse		"Unauthorized"
//	@Failure		500			{object}	api.ErrorResponse		"Internal server error"
//	@Router			/v1/limits [get]
func (h *LimitHandler) ListLimits(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.list")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Parse query parameters into input struct
	var input ListLimitsInput

	if err := c.QueryParser(&input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse query parameters", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0006", "Invalid Query Parameter", "Invalid query parameters")
	}

	// Validate before applying defaults to ensure fail-fast behavior
	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)

		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			return pkgHTTP.BadRequestWithMessage(c, validationErr.Code, "Validation Error", validationErr.Message)
		}

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0001", "Validation Error", formatValidationMessage(err))
	}

	// Apply defaults after validation
	input.SetDefaults()

	logger.With(
		libLog.String("operation", "handler.limit.list"),
		libLog.Any("list.limit", input.Limit),
		libLog.String("list.cursor", input.Cursor),
		libLog.Any("list.name", input.Name),
		libLog.String("list.sort_by", input.SortBy),
		libLog.String("list.sort_order", input.SortOrder),
	).Log(ctx, libLog.LevelInfo, "Listing limits")

	// Convert to service filter
	filter := ToListLimitsFilter(&input)

	result, err := h.service.ListLimits(ctx, filter)
	if err != nil {
		return handleLimitServiceError(c, span, err)
	}

	// Convert to response
	response := ToListLimitsResponse(result)

	logger.With(
		libLog.String("operation", "handler.limit.list"),
		libLog.Int("list.count", len(response.Limits)),
		libLog.Bool("list.has_more", response.HasMore),
	).Log(ctx, libLog.LevelInfo, "Limits listed")

	return pkgHTTP.OK(c, response)
}

// UpdateLimit godoc
//
//	@Summary		Partially update an existing spending limit
//	@Description	Partially updates a limit. Only provided fields are updated, omitted fields remain unchanged.
//	@ID				updateLimit
//	@Tags			limits
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string					true	"Limit ID (UUID)"	Format(uuid)
//	@Param			limit		body		UpdateLimitInput	true	"Fields to update"
//	@Success		200			{object}	model.Limit				"Limit updated successfully"
//	@Failure		400			{object}	api.ErrorResponse		"Invalid input"
//	@Failure		401			{object}	api.ErrorResponse		"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse		"Limit not found"
//	@Failure		409			{object}	api.ErrorResponse		"Limit name already exists"
//	@Failure		500			{object}	api.ErrorResponse		"Internal server error"
//	@Router			/v1/limits/{id} [patch]
func (h *LimitHandler) UpdateLimit(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.update")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Parse limit ID from path
	idParam := c.Params("id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid limit ID format")
	}

	// Check for immutable fields BEFORE parsing into struct
	// This ensures we detect if limitType or currency was sent in the request
	var rawBody map[string]any
	if err := json.Unmarshal(c.Body(), &rawBody); err == nil {
		if _, hasLimitType := rawBody["limitType"]; hasLimitType {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Immutable field limitType in request", constant.ErrLimitImmutableField)

			return pkgHTTP.BadRequestWithMessage(c, "TRC-0138", "Immutable Field", "limitType cannot be modified after creation")
		}

		if _, hasCurrency := rawBody["currency"]; hasCurrency {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Immutable field currency in request", constant.ErrLimitImmutableField)

			return pkgHTTP.BadRequestWithMessage(c, "TRC-0138", "Immutable Field", "currency cannot be modified after creation")
		}
	}

	var input UpdateLimitInput
	if err := c.BodyParser(&input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse request body", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0003", "Bad Request", "Invalid request body")
	}

	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)

		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			return pkgHTTP.BadRequestWithMessage(c, validationErr.Code, "Validation Error", validationErr.Message)
		}

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0001", "Validation Error", formatValidationMessage(err))
	}

	if input.IsEmpty() {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "No fields to update", nil)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0002", "Validation Error", "At least one field must be provided for update")
	}

	logger.With(
		libLog.String("operation", "handler.limit.update"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Updating limit")

	err = libOpentelemetry.SetSpanAttributesFromValue(span, "limit_update", input, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set span attributes", err)
	}

	// Convert HTTP input to service input
	serviceInput := ToUpdateLimitServiceInput(&input)

	result, err := h.service.UpdateLimit(ctx, id, serviceInput)
	if err != nil {
		return handleLimitServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.update"),
		libLog.String("limit.id", result.ID.String()),
	).Log(ctx, libLog.LevelInfo, "Limit updated")

	return pkgHTTP.OK(c, result)
}

// ActivateLimit godoc
//
//	@Summary		Activate a spending limit
//	@Description	Activates an inactive limit (INACTIVE → ACTIVE).
//	@ID				activateLimit
//	@Tags			limits
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string				true	"Limit ID (UUID)"	Format(uuid)
//	@Success		200			{object}	model.Limit			"Limit activated successfully"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid limit ID or transition"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Limit not found"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/limits/{id}/activate [post]
func (h *LimitHandler) ActivateLimit(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.activate")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	idParam := c.Params("id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid limit ID format")
	}

	logger.With(
		libLog.String("operation", "handler.limit.activate"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Activating limit")

	limit, err := h.service.ActivateLimit(ctx, id)
	if err != nil {
		return handleLimitServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.activate"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Limit activated")

	return pkgHTTP.OK(c, limit)
}

// DeactivateLimit godoc
//
//	@Summary		Deactivate a spending limit
//	@Description	Deactivates an active limit (ACTIVE → INACTIVE).
//	@ID				deactivateLimit
//	@Tags			limits
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string				true	"Limit ID (UUID)"	Format(uuid)
//	@Success		200			{object}	model.Limit			"Limit deactivated successfully"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid limit ID or transition"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Limit not found"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/limits/{id}/deactivate [post]
func (h *LimitHandler) DeactivateLimit(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.deactivate")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	idParam := c.Params("id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid limit ID format")
	}

	logger.With(
		libLog.String("operation", "handler.limit.deactivate"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Deactivating limit")

	limit, err := h.service.DeactivateLimit(ctx, id)
	if err != nil {
		return handleLimitServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.deactivate"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Limit deactivated")

	return pkgHTTP.OK(c, limit)
}

// DraftLimit godoc
//
//	@Summary		Transition a limit back to draft
//	@Description	Transitions a limit from INACTIVE to DRAFT status. Allows re-editing a previously deactivated limit.
//	@ID				draftLimit
//	@Tags			limits
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string				true	"Limit ID (UUID)"	Format(uuid)
//	@Success		200			{object}	model.Limit			"Limit transitioned to draft successfully"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid limit ID or transition"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Limit not found"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/limits/{id}/draft [post]
func (h *LimitHandler) DraftLimit(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.draft")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	idParam := c.Params("id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid limit ID format")
	}

	logger.With(
		libLog.String("operation", "handler.limit.draft"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Transitioning limit to draft")

	limit, err := h.service.DraftLimit(ctx, id)
	if err != nil {
		return handleLimitServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.draft"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Limit transitioned to draft")

	return pkgHTTP.OK(c, limit)
}

// DeleteLimit godoc
//
//	@Summary		Delete a spending limit
//	@Description	Soft-deletes a limit (transitions to DELETED status). Only DRAFT and INACTIVE limits can be deleted. ACTIVE limits must be deactivated first.
//	@ID				deleteLimit
//	@Tags			limits
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string				true	"Limit ID (UUID)"	Format(uuid)
//	@Success		204			"Limit deleted successfully"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid limit ID or transition"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Limit not found"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/limits/{id} [delete]
func (h *LimitHandler) DeleteLimit(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.delete")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	idParam := c.Params("id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid limit ID format")
	}

	logger.With(
		libLog.String("operation", "handler.limit.delete"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Deleting limit")

	if err := h.service.DeleteLimit(ctx, id); err != nil {
		return handleLimitServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.delete"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Limit deleted")

	return pkgHTTP.NoContent(c)
}

// GetLimitUsage godoc
//
//	@Summary		Get usage snapshot for a limit
//	@Description	Retrieves current usage snapshot for a limit, showing aggregated usage, utilization percentage, and reset time.
//	@ID				getLimitUsage
//	@Tags			limits
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string			true	"Limit ID (UUID)"	Format(uuid)
//	@Success		200			{object}	model.UsageSnapshot	"Usage snapshot retrieved successfully"
//	@Failure		400			{object}	api.ErrorResponse		"Invalid limit ID"
//	@Failure		401			{object}	api.ErrorResponse		"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse		"Limit not found"
//	@Failure		500			{object}	api.ErrorResponse		"Internal server error"
//	@Router			/v1/limits/{id}/usage [get]
func (h *LimitHandler) GetLimitUsage(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.get_usage")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	idParam := c.Params("id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid limit ID format")
	}

	logger.With(
		libLog.String("operation", "handler.limit.get_usage"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Getting limit usage")

	snapshot, err := h.service.GetLimitUsage(ctx, id)
	if err != nil {
		return handleLimitServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.get_usage"),
		libLog.String("limit.id", id.String()),
		libLog.Any("current_usage", snapshot.CurrentUsage),
		libLog.Any("utilization_percent", snapshot.UtilizationPercent),
	).Log(ctx, libLog.LevelInfo, "Limit usage retrieved")

	return pkgHTTP.OK(c, snapshot)
}

// handleLimitServiceError converts service errors to appropriate HTTP responses.
func handleLimitServiceError(c *fiber.Ctx, span trace.Span, err error) error {
	switch {
	case errors.Is(err, constant.ErrLimitNameAlreadyExists):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit name already exists", err)

		return pkgHTTP.Conflict(c, "TRC-0304", "Conflict", "Limit name already exists")
	case errors.Is(err, constant.ErrLimitNotFound):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit not found", err)

		return pkgHTTP.NotFound(c, "TRC-0120", "Not Found", "Limit not found")
	case errors.Is(err, constant.ErrLimitAlreadyDeleted):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit already deleted", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0128", "Bad Request", "Cannot modify deleted limit")
	case errors.Is(err, constant.ErrLimitInvalidStatusChange):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid status transition", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0121", "Bad Request", "Invalid status transition")
	case errors.Is(err, constant.ErrInvalidCursor):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid cursor", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0044", "Bad Request", "Invalid pagination cursor")
	case errors.Is(err, constant.ErrInvalidSortColumn):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid sort column", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0043", "Bad Request", "Invalid sort column")
	case errors.Is(err, constant.ErrInvalidSortOrder):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid sort order", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0042", "Bad Request", "Invalid sort order")
	case errors.Is(err, constant.ErrLimitNameRequired):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit name required", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0126", "Validation Error", "name is required")
	case errors.Is(err, constant.ErrLimitNameTooLong):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit name too long", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0127", "Validation Error", "name exceeds maximum length")
	case errors.Is(err, constant.ErrLimitNameInvalidChars):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit name invalid chars", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0129", "Validation Error", "name contains invalid characters")
	case errors.Is(err, constant.ErrLimitDescriptionInvalidChars):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit description invalid chars", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0130", "Validation Error", "description contains invalid characters")
	case errors.Is(err, constant.ErrLimitInvalidType):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit type", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0122", "Validation Error", "limitType must be one of [DAILY, WEEKLY, MONTHLY, CUSTOM, PER_TRANSACTION]")
	case errors.Is(err, constant.ErrLimitInvalidMaxAmount):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid max amount", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0123", "Validation Error", "maxAmount must be positive")
	case errors.Is(err, constant.ErrLimitInvalidCurrency):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid currency", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0124", "Validation Error", "currency must be a valid 3-letter ISO 4217 code")
	case errors.Is(err, constant.ErrLimitInvalidScope):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid scope", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0125", "Validation Error", "scopes must have at least one entry with at least one field set")
	default:
		libOpentelemetry.HandleSpanError(span, "Operation failed", err)

		return pkgHTTP.InternalServerError(c, "TRC-0004", "Internal Server Error", "An unexpected error occurred")
	}
}

// formatValidationMessage extracts user-friendly message from validation error.
// Falls back to generic message to avoid exposing internal error details.
func formatValidationMessage(err error) string {
	if err == nil {
		return "Invalid input"
	}

	// The error message from formatLimitValidationError is already user-friendly
	// but we add a safety net in case of unexpected errors
	msg := err.Error()
	if msg == "" {
		return "Invalid input"
	}

	return msg
}
