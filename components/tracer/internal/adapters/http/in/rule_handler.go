// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

//go:generate mockgen -source=rule_handler.go -destination=rule_service_mock.go -package=in

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/components/tracer/pkg/net/http"
)

// RuleService defines the interface for rule operations.
// Interface defined locally per Ring pattern.
type RuleService interface {
	CreateRule(ctx context.Context, input *command.CreateRuleInput) (*model.Rule, error)
	UpdateRule(ctx context.Context, id uuid.UUID, input *command.UpdateRuleInput) (*model.Rule, error)
	GetRule(ctx context.Context, id uuid.UUID) (*model.Rule, error)
	ListRules(ctx context.Context, filter *model.ListRulesFilter) (*model.ListRulesResult, error)
	ActivateRule(ctx context.Context, id uuid.UUID) (*model.Rule, error)
	DeactivateRule(ctx context.Context, id uuid.UUID) (*model.Rule, error)
	DraftRule(ctx context.Context, id uuid.UUID) (*model.Rule, error)
	DeleteRule(ctx context.Context, id uuid.UUID) error
}

// Handler handles HTTP requests for rule operations.
type Handler struct {
	service RuleService
}

// NewHandler creates a new rule handler.
func NewHandler(service RuleService) *Handler {
	return &Handler{
		service: service,
	}
}

// CreateRule godoc
//
//	@Summary		Create a new fraud rule
//	@Description	Creates a rule with CEL expression and scopes array. Rules are created in DRAFT status.
//	@ID				createRule
//	@Tags			rules
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			rule		body		CreateRuleInput	true	"Rule details"
//	@Success		201			{object}	model.Rule		"Rule created successfully"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid input"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		409			{object}	api.ErrorResponse	"Rule name already exists"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/rules [post]
func (h *Handler) CreateRule(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.create")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	var input CreateRuleInput
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

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0001", "Validation Error", "Validation error")
	}

	logger.With(
		libLog.String("operation", "handler.rule.create"),
		libLog.String("rule.name", input.Name),
	).Log(ctx, libLog.LevelInfo, "Creating rule")

	// Convert HTTP input to service input
	serviceInput := toServiceInput(&input)

	result, err := h.service.CreateRule(ctx, serviceInput)
	if err != nil {
		return handleServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.create"),
		libLog.String("rule.id", result.ID.String()),
	).Log(ctx, libLog.LevelInfo, "Rule created")

	return pkgHTTP.Created(c, result)
}

// UpdateRule godoc
//
//	@Summary		Partially update an existing fraud rule
//	@Description	Partially updates a rule. Only provided fields are updated, omitted fields remain unchanged.
//	@ID				updateRule
//	@Tags			rules
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string	true	"Rule ID (UUID)"	Format(uuid)
//	@Param			rule		body		UpdateRuleInput	true	"Fields to update"
//	@Success		200			{object}	model.Rule		"Rule updated successfully"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid input"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Rule not found"
//	@Failure		409			{object}	api.ErrorResponse	"Rule name already exists"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/rules/{id} [patch]
func (h *Handler) UpdateRule(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.update")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Parse rule ID from path
	idParam := c.Params("id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid rule ID", err)
		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid rule ID format")
	}

	var input UpdateRuleInput
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

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0001", "Validation Error", "Validation error")
	}

	if input.IsEmpty() {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "No fields to update", nil)
		return pkgHTTP.BadRequestWithMessage(c, "TRC-0002", "Validation Error", "At least one field must be provided for update")
	}

	logger.With(
		libLog.String("operation", "handler.rule.update"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Updating rule")

	// Convert HTTP input to service input
	serviceInput := toUpdateServiceInput(&input)

	result, err := h.service.UpdateRule(ctx, id, serviceInput)
	if err != nil {
		return handleServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.update"),
		libLog.String("rule.id", result.ID.String()),
	).Log(ctx, libLog.LevelInfo, "Rule updated")

	return pkgHTTP.OK(c, result)
}

// GetRule godoc
//
//	@Summary		Get a fraud rule by ID
//	@Description	Retrieves a rule by its unique identifier.
//	@ID				getRule
//	@Tags			rules
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string	true	"Rule ID (UUID)"	Format(uuid)
//	@Success		200			{object}	model.Rule		"Rule retrieved successfully"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid rule ID"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Rule not found"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/rules/{id} [get]
func (h *Handler) GetRule(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.get")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Parse rule ID from path
	idParam := c.Params("id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid rule ID", err)
		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid rule ID format")
	}

	logger.With(
		libLog.String("operation", "handler.rule.get"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Getting rule")

	result, err := h.service.GetRule(ctx, id)
	if err != nil {
		return handleServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.get"),
		libLog.String("rule.id", result.ID.String()),
		libLog.String("rule.name", result.Name),
	).Log(ctx, libLog.LevelInfo, "Rule retrieved")

	return pkgHTTP.OK(c, result)
}

// ListRules godoc
//
//	@Summary		List fraud rules
//	@Description	Lists rules with cursor-based pagination and optional filters. Supports filtering by scope fields to find rules applicable to specific contexts. Global rules (empty scopes) are always included in scope-filtered results.
//	@ID				listRules
//	@Tags			rules
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			limit			query		int		false	"Max items per page (1-100, default: 10)"	minimum(1)	maximum(100)
//	@Param			cursor			query		string	false	"Pagination cursor (empty for first page)"
//	@Param			name			query		string	false	"Filter by name (case-insensitive partial match)"
//	@Param			status			query		string	false	"Filter by status (DELETED not allowed)"	Enums(DRAFT, ACTIVE, INACTIVE)
//	@Param			action			query		string	false	"Filter by action"	Enums(ALLOW, DENY, REVIEW)
//	@Param			account_id		query		string	false	"Filter by scope account_id (UUID)"	Format(uuid)
//	@Param			segment_id		query		string	false	"Filter by scope segment_id (UUID)"	Format(uuid)
//	@Param			portfolio_id	query		string	false	"Filter by scope portfolio_id (UUID)"	Format(uuid)
//	@Param			merchant_id		query		string	false	"Filter by scope merchant_id (UUID)"	Format(uuid)
//	@Param			transaction_type	query		string	false	"Filter by scope transaction_type"	Enums(CARD, WIRE, PIX, CRYPTO)
//	@Param			sub_type		query		string	false	"Filter by scope sub_type (case-insensitive, normalized to lowercase; max 50 chars)"	maxLength(50)
//	@Param			sort_by			query		string	false	"Sort field"	Enums(created_at, updated_at, name, status)
//	@Param			sort_order		query		string	false	"Sort direction"	Enums(ASC, DESC)
//	@Success		200				{object}	ListRulesResponse	"Rules listed successfully"
//	@Failure		400				{object}	api.ErrorResponse	"Invalid parameters"
//	@Failure		401				{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		500				{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/rules [get]
func (h *Handler) ListRules(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.list")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Parse query parameters into input struct
	var input ListRulesInput

	if err := c.QueryParser(&input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse query parameters", err)
		return pkgHTTP.BadRequestWithMessage(c, "TRC-0006", "Invalid Query Parameter", "Invalid query parameters")
	}

	// Validate first (before defaults) to catch explicit invalid values like limit=0
	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)
		// Check for typed validation error with specific code
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			return pkgHTTP.BadRequestWithMessage(c, validationErr.Code, "Bad Request", validationErr.Message)
		}

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0001", "Validation Error", "Validation error")
	}

	// Apply defaults after validation passes (for non-specified optional fields)
	input.SetDefaults()

	logger.With(
		libLog.String("operation", "handler.rule.list"),
		libLog.Any("list.limit", input.Limit),
		libLog.String("list.cursor", input.Cursor),
		libLog.String("list.sort_by", input.SortBy),
		libLog.String("list.sort_order", input.SortOrder),
	).Log(ctx, libLog.LevelInfo, "Listing rules")

	// Convert to service filter
	filter := toListFilter(&input)

	result, err := h.service.ListRules(ctx, filter)
	if err != nil {
		return handleServiceError(c, span, err)
	}

	// Convert to response
	response := toListResponse(result)

	logger.With(
		libLog.String("operation", "handler.rule.list"),
		libLog.Int("list.count", len(response.Rules)),
		libLog.Bool("list.has_more", response.HasMore),
	).Log(ctx, libLog.LevelInfo, "Rules listed")

	return pkgHTTP.OK(c, response)
}

// ActivateRule godoc
//
//	@Summary		Activate a fraud rule
//	@Description	Activates a rule (DRAFT/INACTIVE → ACTIVE). Validates CEL expression before activation.
//	@ID				activateRule
//	@Tags			rules
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string	true	"Rule ID (UUID)"	Format(uuid)
//	@Success		200			{object}	model.Rule		"Rule activated successfully"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid rule ID or transition"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Rule not found"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/rules/{id}/activate [post]
func (h *Handler) ActivateRule(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.activate")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	idParam := c.Params("id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid rule ID", err)
		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid rule ID format")
	}

	logger.With(
		libLog.String("operation", "handler.rule.activate"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Activating rule")

	rule, err := h.service.ActivateRule(ctx, id)
	if err != nil {
		return handleLifecycleError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.activate"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Rule activated")

	return pkgHTTP.OK(c, rule)
}

// DeactivateRule godoc
//
//	@Summary		Deactivate a fraud rule
//	@Description	Deactivates a rule (ACTIVE/DRAFT → INACTIVE).
//	@ID				deactivateRule
//	@Tags			rules
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string	true	"Rule ID (UUID)"	Format(uuid)
//	@Success		200			{object}	model.Rule		"Rule deactivated successfully"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid rule ID or transition"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Rule not found"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/rules/{id}/deactivate [post]
func (h *Handler) DeactivateRule(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.deactivate")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	idParam := c.Params("id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid rule ID", err)
		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid rule ID format")
	}

	logger.With(
		libLog.String("operation", "handler.rule.deactivate"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Deactivating rule")

	rule, err := h.service.DeactivateRule(ctx, id)
	if err != nil {
		return handleLifecycleError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.deactivate"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Rule deactivated")

	return pkgHTTP.OK(c, rule)
}

// DraftRule godoc
//
//	@Summary		Transition a rule back to draft
//	@Description	Transitions a rule from INACTIVE to DRAFT status. Allows re-editing a previously deactivated rule.
//	@ID				draftRule
//	@Tags			rules
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string	true	"Rule ID (UUID)"	Format(uuid)
//	@Success		200			{object}	model.Rule		"Rule transitioned to draft successfully"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid rule ID or transition"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Rule not found"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/rules/{id}/draft [post]
func (h *Handler) DraftRule(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.draft")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	idParam := c.Params("id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid rule ID", err)
		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid rule ID format")
	}

	logger.With(
		libLog.String("operation", "handler.rule.draft"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Transitioning rule to draft")

	rule, err := h.service.DraftRule(ctx, id)
	if err != nil {
		return handleLifecycleError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.draft"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Rule transitioned to draft")

	return pkgHTTP.OK(c, rule)
}

// DeleteRule godoc
//
//	@Summary		Delete a fraud rule
//	@Description	Soft-deletes a rule (transitions to DELETED status). Only DRAFT and INACTIVE rules can be deleted. ACTIVE rules must be deactivated first.
//	@ID				deleteRule
//	@Tags			rules
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			id			path		string	true	"Rule ID (UUID)"	Format(uuid)
//	@Success		204			"Rule deleted successfully"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid rule ID or transition"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		404			{object}	api.ErrorResponse	"Rule not found"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Router			/v1/rules/{id} [delete]
func (h *Handler) DeleteRule(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.delete")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	idParam := c.Params("id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid rule ID", err)
		return pkgHTTP.BadRequestWithMessage(c, "TRC-0007", "Invalid Path Parameter", "Invalid rule ID format")
	}

	logger.With(
		libLog.String("operation", "handler.rule.delete"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Deleting rule")

	if err := h.service.DeleteRule(ctx, id); err != nil {
		return handleLifecycleError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.delete"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Rule deleted")

	return pkgHTTP.NoContent(c)
}

// handleLifecycleError converts lifecycle service errors to appropriate HTTP responses.
func handleLifecycleError(c *fiber.Ctx, span trace.Span, err error) error {
	var invalidTransitionErr *model.InvalidTransitionError
	if errors.As(err, &invalidTransitionErr) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid state transition", err)
		return pkgHTTP.BadRequestWithMessage(c, "TRC-0102", "Invalid State Transition", "Invalid state transition")
	}

	return handleServiceError(c, span, err)
}

// handleServiceError converts service errors to appropriate HTTP responses.
func handleServiceError(c *fiber.Ctx, span trace.Span, err error) error {
	switch {
	case errors.Is(err, constant.ErrRuleNameAlreadyExistsInCtx):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule name already exists in this context", err)
		return pkgHTTP.Conflict(c, "TRC-0303", "Conflict", "Rule name already exists in this context")
	case errors.Is(err, constant.ErrRuleNameAlreadyExists):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule name already exists", err)
		return pkgHTTP.Conflict(c, "TRC-0101", "Conflict", "Rule name already exists")
	case errors.Is(err, constant.ErrExpressionSyntax):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid CEL expression syntax", err)

		return pkgHTTP.BadRequest(c, fiber.Map{
			"code":    "TRC-0083",
			"title":   "Bad Request",
			"message": "Invalid CEL expression syntax",
		})
	case errors.Is(err, constant.ErrExpressionType):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Expression must evaluate to boolean", err)

		return pkgHTTP.BadRequest(c, fiber.Map{
			"code":    "TRC-0084",
			"title":   "Bad Request",
			"message": "Expression must evaluate to boolean",
		})
	case errors.Is(err, constant.ErrExpressionCostExceeded):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Expression cost exceeds limit", err)

		return pkgHTTP.BadRequest(c, fiber.Map{
			"code":    "TRC-0085",
			"title":   "Bad Request",
			"message": "Expression cost exceeds limit",
		})
	case errors.Is(err, constant.ErrExpressionNotModifiable):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Expression cannot be modified for non-DRAFT rules", err)

		return pkgHTTP.BadRequest(c, fiber.Map{
			"code":    "TRC-0104",
			"title":   "Bad Request",
			"message": "Expression cannot be modified for non-DRAFT rules",
		})
	case errors.Is(err, constant.ErrRuleNotFound):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule not found", err)
		return pkgHTTP.NotFound(c, "TRC-0100", "Not Found", "Rule not found")
	case errors.Is(err, constant.ErrInvalidCursor):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid cursor", err)

		return pkgHTTP.BadRequest(c, fiber.Map{
			"code":    "TRC-0044",
			"title":   "Bad Request",
			"message": "Invalid pagination cursor",
		})
	case errors.Is(err, constant.ErrInvalidSortColumn):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid sort column", err)

		return pkgHTTP.BadRequest(c, fiber.Map{
			"code":    "TRC-0043",
			"title":   "Bad Request",
			"message": "Invalid sort column",
		})
	default:
		libOpentelemetry.HandleSpanError(span, "Operation failed", err)
		return pkgHTTP.InternalServerError(c, "TRC-0004", "Internal Server Error", "An unexpected error occurred")
	}
}

// toServiceInput converts HTTP CreateRuleInput to service CreateRuleInput.
func toServiceInput(input *CreateRuleInput) *command.CreateRuleInput {
	return &command.CreateRuleInput{
		Name:        input.Name,
		Description: input.Description,
		Expression:  input.Expression,
		Action:      input.Action,
		Scopes:      input.Scopes,
	}
}

// toUpdateServiceInput converts HTTP UpdateRuleInput to service UpdateRuleInput.
func toUpdateServiceInput(input *UpdateRuleInput) *command.UpdateRuleInput {
	return &command.UpdateRuleInput{
		Name:        input.Name,
		Description: input.Description,
		Expression:  input.Expression,
		Action:      input.Action,
		Scopes:      input.Scopes,
	}
}
