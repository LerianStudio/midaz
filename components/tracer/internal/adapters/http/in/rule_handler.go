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
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
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
		return http.WithError(c, pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), Title: "Bad Request", Message: "The request body is malformed or contains invalid JSON. Please verify the syntax and try again."})
	}

	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)

		return http.WithError(c, err)
	}

	// Convert HTTP input to service input
	serviceInput := toServiceInput(&input)

	result, err := h.service.CreateRule(ctx, serviceInput)
	if err != nil {
		return handleServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.create"),
		libLog.String("rule.id", result.ID.String()),
	).Log(ctx, libLog.LevelDebug, "Rule created")

	return http.Created(c, result)
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
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityRule, "id"))
	}

	var input UpdateRuleInput
	if err := c.BodyParser(&input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse request body", err)
		return http.WithError(c, pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), Title: "Bad Request", Message: "The request body is malformed or contains invalid JSON. Please verify the syntax and try again."})
	}

	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)

		return http.WithError(c, err)
	}

	if input.IsEmpty() {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "No fields to update", nil)
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrNothingToUpdate, constant.EntityRule))
	}

	// Convert HTTP input to service input
	serviceInput := toUpdateServiceInput(&input)

	result, err := h.service.UpdateRule(ctx, id, serviceInput)
	if err != nil {
		return handleServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.update"),
		libLog.String("rule.id", result.ID.String()),
	).Log(ctx, libLog.LevelDebug, "Rule updated")

	return http.OK(c, result)
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
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityRule, "id"))
	}

	result, err := h.service.GetRule(ctx, id)
	if err != nil {
		return handleServiceError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.get"),
		libLog.String("rule.id", result.ID.String()),
		libLog.String("rule.name", result.Name),
	).Log(ctx, libLog.LevelDebug, "Rule retrieved")

	return http.OK(c, result)
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
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityRule, "filters"))
	}

	// Validate first (before defaults) to catch explicit invalid values like limit=0
	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)
		// Check for typed validation error with specific code
		return http.WithError(c, err)
	}

	// Apply defaults after validation passes (for non-specified optional fields)
	input.SetDefaults()

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
	).Log(ctx, libLog.LevelDebug, "Rules listed")

	return http.OK(c, response)
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
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityRule, "id"))
	}

	rule, err := h.service.ActivateRule(ctx, id)
	if err != nil {
		return handleLifecycleError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.activate"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelDebug, "Rule activated")

	return http.OK(c, rule)
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
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityRule, "id"))
	}

	rule, err := h.service.DeactivateRule(ctx, id)
	if err != nil {
		return handleLifecycleError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.deactivate"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelDebug, "Rule deactivated")

	return http.OK(c, rule)
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
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityRule, "id"))
	}

	rule, err := h.service.DraftRule(ctx, id)
	if err != nil {
		return handleLifecycleError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.draft"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelDebug, "Rule transitioned to draft")

	return http.OK(c, rule)
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
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityRule, "id"))
	}

	if err := h.service.DeleteRule(ctx, id); err != nil {
		return handleLifecycleError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.delete"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelDebug, "Rule deleted")

	return http.NoContent(c)
}

// handleLifecycleError converts lifecycle service errors to appropriate HTTP responses.
func handleLifecycleError(c *fiber.Ctx, span trace.Span, err error) error {
	var invalidTransitionErr *model.InvalidTransitionError
	if errors.As(err, &invalidTransitionErr) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid state transition", err)
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrRuleInvalidStatus, constant.EntityRule))
	}

	return handleServiceError(c, span, err)
}

// handleServiceError converts service errors to appropriate HTTP responses.
func handleServiceError(c *fiber.Ctx, span trace.Span, err error) error {
	switch {
	case errors.Is(err, constant.ErrRuleNameAlreadyExistsInCtx):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule name already exists in this context", err)
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrRuleNameAlreadyExistsInCtx, constant.EntityRule))
	case errors.Is(err, constant.ErrRuleNameAlreadyExists):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule name already exists", err)
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrRuleNameAlreadyExists, constant.EntityRule))
	case errors.Is(err, constant.ErrExpressionSyntax):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid CEL expression syntax", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrExpressionSyntax, constant.EntityRule))
	case errors.Is(err, constant.ErrExpressionType):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Expression must evaluate to boolean", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrExpressionType, constant.EntityRule))
	case errors.Is(err, constant.ErrExpressionCostExceeded):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Expression cost exceeds limit", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrExpressionCostExceeded, constant.EntityRule))
	case errors.Is(err, constant.ErrExpressionNotModifiable):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Expression cannot be modified for non-DRAFT rules", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrExpressionNotModifiable, constant.EntityRule))
	case errors.Is(err, constant.ErrRuleNotFound):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule not found", err)
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrRuleNotFound, constant.EntityRule))
	case errors.Is(err, constant.ErrInvalidCursor):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid cursor", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidCursor, constant.EntityRule))
	case errors.Is(err, constant.ErrInvalidSortColumn):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid sort column", err)

		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidSortColumn, constant.EntityRule))
	default:
		libOpentelemetry.HandleSpanError(span, "Operation failed", err)
		return http.WithError(c, pkg.InternalServerError{Code: constant.ErrInternalServer.Error(), Title: "Internal Server Error", Message: "The server encountered an unexpected error. Please try again later or contact support."})
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
