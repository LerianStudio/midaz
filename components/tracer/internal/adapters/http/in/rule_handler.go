// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

//go:generate mockgen -source=rule_handler.go -destination=rule_service_mock.go -package=in

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

func (h *Handler) CreateRule(c *fiber.Ctx) error {
	result, err := h.createRule(c.UserContext(), c.Body())
	if err != nil {
		return http.WithError(c, err)
	}

	return http.Created(c, result)
}

// createRule is the transport-agnostic core of the create-rule operation shared
// by the Fiber method (CreateRule) and the Huma func (CreateRuleHuma). It owns
// the span, the imperative parse + Validate(), the service call, and the
// success log — and it CANONICALIZES every error before returning it (parse /
// validation errors are already canonical; the service error is run through
// classifyServiceError here). So both callers render the returned error the same
// way — Fiber via http.WithError, Huma via ProblemDetail — and the two
// transports emit field/status/code/type-identical RFC 9457 envelopes (decoded
// bodies match exactly; raw bytes differ only by Huma's encoder newline +
// HTML-escaping, invisible to any JSON parser). Error rendering is the ONLY thing
// that differs between transports; it stays at the edge.
//
// On error the returned *model.Rule is nil and err is the canonical Midaz error.
func (h *Handler) createRule(ctx context.Context, rawBody []byte) (*model.Rule, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.create")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	var input CreateRuleInput
	if err := json.Unmarshal(rawBody, &input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse request body", err)
		return nil, pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), Title: "Bad Request", Message: "The request body is malformed or contains invalid JSON. Please verify the syntax and try again."}
	}

	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)
		return nil, err
	}

	// Convert HTTP input to service input
	serviceInput := toServiceInput(&input)

	result, err := h.service.CreateRule(ctx, serviceInput)
	if err != nil {
		return nil, classifyServiceError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.create"),
		libLog.String("rule.id", result.ID.String()),
	).Log(ctx, libLog.LevelDebug, "Rule created")

	return result, nil
}

func (h *Handler) UpdateRule(c *fiber.Ctx) error {
	result, err := h.updateRule(c.UserContext(), c.Params("id"), c.Body())
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, result)
}

// updateRule is the transport-agnostic core of the update-rule operation shared
// by the Fiber method (UpdateRule) and the Huma func (UpdateRuleHuma). See
// createRule for the split rationale: it owns the span + id parse + imperative
// parse/Validate/IsEmpty + service call + success log, and canonicalizes every
// error before returning so both transports render the identical body.
func (h *Handler) updateRule(ctx context.Context, idParam string, rawBody []byte) (*model.Rule, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.update")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid rule ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityRule, "id")
	}

	var input UpdateRuleInput
	if err := json.Unmarshal(rawBody, &input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse request body", err)
		return nil, pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), Title: "Bad Request", Message: "The request body is malformed or contains invalid JSON. Please verify the syntax and try again."}
	}

	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)
		return nil, err
	}

	if input.IsEmpty() {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "No fields to update", nil)
		return nil, pkg.ValidateBusinessError(constant.ErrNothingToUpdate, constant.EntityRule)
	}

	// Convert HTTP input to service input
	serviceInput := toUpdateServiceInput(&input)

	result, err := h.service.UpdateRule(ctx, id, serviceInput)
	if err != nil {
		return nil, classifyServiceError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.update"),
		libLog.String("rule.id", result.ID.String()),
	).Log(ctx, libLog.LevelDebug, "Rule updated")

	return result, nil
}

func (h *Handler) GetRule(c *fiber.Ctx) error {
	result, err := h.getRule(c.UserContext(), c.Params("id"))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, result)
}

// getRule is the transport-agnostic core of the get-rule operation shared by
// the Fiber method (GetRule) and the Huma func (GetRuleHuma). See createRule for
// the split rationale: it owns the span + id parse + service call + success log,
// and canonicalizes every error before returning so both transports render the
// identical body.
func (h *Handler) getRule(ctx context.Context, idParam string) (*model.Rule, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.get")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid rule ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityRule, "id")
	}

	result, err := h.service.GetRule(ctx, id)
	if err != nil {
		return nil, classifyServiceError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.get"),
		libLog.String("rule.id", result.ID.String()),
		libLog.String("rule.name", result.Name),
	).Log(ctx, libLog.LevelDebug, "Rule retrieved")

	return result, nil
}

func (h *Handler) ListRules(c *fiber.Ctx) error {
	// Fiber binds the query with QueryParser; the shared core owns the rest.
	response, err := h.listRules(c.UserContext(), c.QueryParser)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, response)
}

// listRules is the transport-agnostic core of the list-rules operation shared by
// the Fiber method (ListRules) and the Huma func (ListRulesHuma). See createRule
// for the split rationale.
//
// Query BINDING is the only transport-specific step and stays at the edge: the
// caller passes a bind func that populates *ListRulesInput from its own query
// source (Fiber's c.QueryParser, or the Huma func's imperative string->typed
// copy). A bind error is canonicalized to ErrInvalidQueryParameter (0082) — the
// SAME code the Fiber QueryParser-failure path produced. Everything after
// binding (Validate -> SetDefaults -> service -> response) is shared, so both
// transports emit field/status/code-identical results.
func (h *Handler) listRules(ctx context.Context, bind func(any) error) (*ListRulesResponse, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.list")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	var input ListRulesInput
	if err := bind(&input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse query parameters", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityRule, "filters")
	}

	// Validate first (before defaults) to catch explicit invalid values like limit=0
	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)
		return nil, err
	}

	// Apply defaults after validation passes (for non-specified optional fields)
	input.SetDefaults()

	// Convert to service filter
	filter := toListFilter(&input)

	result, err := h.service.ListRules(ctx, filter)
	if err != nil {
		return nil, classifyServiceError(span, err)
	}

	// Convert to response
	response := toListResponse(result)

	logger.With(
		libLog.String("operation", "handler.rule.list"),
		libLog.Int("list.count", len(response.Rules)),
		libLog.Bool("list.has_more", response.HasMore),
	).Log(ctx, libLog.LevelDebug, "Rules listed")

	return response, nil
}

func (h *Handler) ActivateRule(c *fiber.Ctx) error {
	rule, err := h.activateRule(c.UserContext(), c.Params("id"))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, rule)
}

// activateRule is the transport-agnostic core of the activate-rule operation
// shared by the Fiber method (ActivateRule) and the Huma func (ActivateRuleHuma).
// It owns the span + id parse + service call + success log, and canonicalizes
// every error via classifyLifecycleError before returning.
func (h *Handler) activateRule(ctx context.Context, idParam string) (*model.Rule, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.activate")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid rule ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityRule, "id")
	}

	rule, err := h.service.ActivateRule(ctx, id)
	if err != nil {
		return nil, classifyLifecycleError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.activate"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelDebug, "Rule activated")

	return rule, nil
}

func (h *Handler) DeactivateRule(c *fiber.Ctx) error {
	rule, err := h.deactivateRule(c.UserContext(), c.Params("id"))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, rule)
}

// deactivateRule is the transport-agnostic core of the deactivate-rule operation
// shared by the Fiber method (DeactivateRule) and the Huma func
// (DeactivateRuleHuma). See activateRule for the split rationale.
func (h *Handler) deactivateRule(ctx context.Context, idParam string) (*model.Rule, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.deactivate")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid rule ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityRule, "id")
	}

	rule, err := h.service.DeactivateRule(ctx, id)
	if err != nil {
		return nil, classifyLifecycleError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.deactivate"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelDebug, "Rule deactivated")

	return rule, nil
}

func (h *Handler) DraftRule(c *fiber.Ctx) error {
	rule, err := h.draftRule(c.UserContext(), c.Params("id"))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, rule)
}

// draftRule is the transport-agnostic core of the draft-rule operation shared by
// the Fiber method (DraftRule) and the Huma func (DraftRuleHuma). See
// activateRule for the split rationale.
func (h *Handler) draftRule(ctx context.Context, idParam string) (*model.Rule, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.draft")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid rule ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityRule, "id")
	}

	rule, err := h.service.DraftRule(ctx, id)
	if err != nil {
		return nil, classifyLifecycleError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.draft"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelDebug, "Rule transitioned to draft")

	return rule, nil
}

func (h *Handler) DeleteRule(c *fiber.Ctx) error {
	if err := h.deleteRule(c.UserContext(), c.Params("id")); err != nil {
		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// deleteRule is the transport-agnostic core of the delete-rule operation shared
// by the Fiber method (DeleteRule) and the Huma func (DeleteRuleHuma). It owns
// the span + id parse + service call + success log, and canonicalizes every
// error via classifyLifecycleError before returning.
func (h *Handler) deleteRule(ctx context.Context, idParam string) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.rule.delete")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid rule ID", err)
		return pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityRule, "id")
	}

	if err := h.service.DeleteRule(ctx, id); err != nil {
		return classifyLifecycleError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.rule.delete"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelDebug, "Rule deleted")

	return nil
}

// classifyLifecycleError maps a raw lifecycle error to its canonical Midaz
// error, attributing the span, WITHOUT rendering. Shared by every rule
// lifecycle core (activate/deactivate/draft/delete) — both the Fiber method and
// the Huma func route their errors through it, so the two transports emit
// field/status/code-identical envelopes.
func classifyLifecycleError(span trace.Span, err error) error {
	var invalidTransitionErr *model.InvalidTransitionError
	if errors.As(err, &invalidTransitionErr) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid state transition", err)
		return pkg.ValidateBusinessError(constant.ErrRuleInvalidStatus, constant.EntityRule)
	}

	return classifyServiceError(span, err)
}

// classifyServiceError maps a raw service error to its canonical Midaz error,
// attributing the span, WITHOUT rendering. It is the single classification the
// Fiber wrappers (which render via http.WithError) and the Huma path
// (humaProblem -> *http.Detail) both consume, so both transports emit
// field/status/code/type-identical envelopes (decoded bodies match exactly; raw
// bytes differ only by Huma's encoder newline + HTML-escaping, invisible to any
// JSON parser). The errors.Is cascade and the canonical mapping are preserved
// verbatim from the pre-Huma handler.
func classifyServiceError(span trace.Span, err error) error {
	// Services pre-classify domain failures into typed business errors via
	// pkg.ValidateBusinessError (leaving Err nil, so errors.Is on the raw
	// sentinel below cannot re-match them). Pass those through unchanged so the
	// service's status/code survives instead of collapsing to a 500.
	if pkg.IsBusinessError(err) {
		return err
	}

	switch {
	case errors.Is(err, constant.ErrRuleNameAlreadyExistsInCtx):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule name already exists in this context", err)
		return pkg.ValidateBusinessError(constant.ErrRuleNameAlreadyExistsInCtx, constant.EntityRule)
	case errors.Is(err, constant.ErrRuleNameAlreadyExists):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule name already exists", err)
		return pkg.ValidateBusinessError(constant.ErrRuleNameAlreadyExists, constant.EntityRule)
	case errors.Is(err, constant.ErrExpressionSyntax):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid CEL expression syntax", err)

		return pkg.ValidateBusinessError(constant.ErrExpressionSyntax, constant.EntityRule)
	case errors.Is(err, constant.ErrExpressionType):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Expression must evaluate to boolean", err)

		return pkg.ValidateBusinessError(constant.ErrExpressionType, constant.EntityRule)
	case errors.Is(err, constant.ErrExpressionCostExceeded):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Expression cost exceeds limit", err)

		return pkg.ValidateBusinessError(constant.ErrExpressionCostExceeded, constant.EntityRule)
	case errors.Is(err, constant.ErrExpressionNotModifiable):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Expression cannot be modified for non-DRAFT rules", err)

		return pkg.ValidateBusinessError(constant.ErrExpressionNotModifiable, constant.EntityRule)
	case errors.Is(err, constant.ErrRuleNotFound):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule not found", err)
		return pkg.ValidateBusinessError(constant.ErrRuleNotFound, constant.EntityRule)
	case errors.Is(err, constant.ErrInvalidCursor):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid cursor", err)

		return pkg.ValidateBusinessError(constant.ErrInvalidCursor, constant.EntityRule)
	case errors.Is(err, constant.ErrInvalidSortColumn):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid sort column", err)

		return pkg.ValidateBusinessError(constant.ErrInvalidSortColumn, constant.EntityRule)
	default:
		libOpentelemetry.HandleSpanError(span, "Operation failed", err)
		return pkg.InternalServerError{Code: constant.ErrInternalServer.Error(), Title: "Internal Server Error", Message: "The server encountered an unexpected error. Please try again later or contact support."}
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
