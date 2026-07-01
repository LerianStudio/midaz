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

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
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

func (h *LimitHandler) CreateLimit(c *fiber.Ctx) error {
	result, err := h.createLimit(c.UserContext(), c.Body())
	if err != nil {
		return http.WithError(c, err)
	}

	return http.Created(c, result)
}

// createLimit is the transport-agnostic core of the create-limit operation
// shared by the Fiber method (CreateLimit) and the Huma func (CreateLimitHuma).
// It owns the span, imperative parse + Validate(), the service call, and the
// success log, and canonicalizes every error before returning so both
// transports render field/status/code-identical envelopes. See rule_handler.go
// createRule for the split rationale.
func (h *LimitHandler) createLimit(ctx context.Context, rawBody []byte) (*model.Limit, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.create")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	var input CreateLimitInput
	if err := json.Unmarshal(rawBody, &input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse request body", err)
		return nil, pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), Title: "Bad Request", Message: "The request body is malformed or contains invalid JSON. Please verify the syntax and try again."}
	}

	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)
		return nil, err
	}

	// Convert HTTP input to service input
	serviceInput := ToCreateLimitServiceInput(&input)

	result, err := h.service.CreateLimit(ctx, serviceInput)
	if err != nil {
		return nil, classifyLimitServiceError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.create"),
		libLog.String("limit.id", result.ID.String()),
	).Log(ctx, libLog.LevelDebug, "Limit created")

	return result, nil
}

func (h *LimitHandler) GetLimit(c *fiber.Ctx) error {
	result, err := h.getLimit(c.UserContext(), c.Params("id"))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, result)
}

// getLimit is the transport-agnostic core of the get-limit operation shared by
// the Fiber method (GetLimit) and the Huma func (GetLimitHuma). See createLimit
// for the split rationale.
func (h *LimitHandler) getLimit(ctx context.Context, idParam string) (*model.Limit, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.get")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityLimit, "id")
	}

	result, err := h.service.GetLimit(ctx, id)
	if err != nil {
		return nil, classifyLimitServiceError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.get"),
		libLog.String("limit.id", result.ID.String()),
		libLog.String("limit.name", result.Name),
	).Log(ctx, libLog.LevelDebug, "Limit retrieved")

	return result, nil
}

func (h *LimitHandler) ListLimits(c *fiber.Ctx) error {
	// Fiber binds the query with QueryParser; the shared core owns the rest.
	response, err := h.listLimits(c.UserContext(), c.QueryParser)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, response)
}

// listLimits is the transport-agnostic core of the list-limits operation shared
// by the Fiber method (ListLimits) and the Huma func (ListLimitsHuma). Query
// BINDING is the only transport-specific step and stays at the edge: the caller
// passes a bind func that populates *ListLimitsInput from its own query source
// (Fiber's c.QueryParser, or the Huma func's imperative string->typed copy). A
// bind error is canonicalized to ErrInvalidQueryParameter (0082) — the SAME code
// the Fiber QueryParser-failure path produced. Everything after binding
// (Validate -> SetDefaults -> service -> response) is shared. See createLimit
// for the split rationale.
func (h *LimitHandler) listLimits(ctx context.Context, bind func(any) error) (*ListLimitsResponse, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.list")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	var input ListLimitsInput
	if err := bind(&input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse query parameters", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityLimit, "filters")
	}

	// Validate before applying defaults to ensure fail-fast behavior
	if err := input.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Validation failed", err)
		return nil, err
	}

	// Apply defaults after validation
	input.SetDefaults()

	// Convert to service filter
	filter := ToListLimitsFilter(&input)

	result, err := h.service.ListLimits(ctx, filter)
	if err != nil {
		return nil, classifyLimitServiceError(span, err)
	}

	// Convert to response
	response := ToListLimitsResponse(result)

	logger.With(
		libLog.String("operation", "handler.limit.list"),
		libLog.Int("list.count", len(response.Limits)),
		libLog.Bool("list.has_more", response.HasMore),
	).Log(ctx, libLog.LevelDebug, "Limits listed")

	return response, nil
}

func (h *LimitHandler) UpdateLimit(c *fiber.Ctx) error {
	result, err := h.updateLimit(c.UserContext(), c.Params("id"), c.Body())
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, result)
}

// updateLimit is the transport-agnostic core of the update-limit operation
// shared by the Fiber method (UpdateLimit) and the Huma func (UpdateLimitHuma).
// It owns the span + id parse + the immutable-field map-probe (preserved
// VERBATIM from the pre-Huma handler) + imperative parse/Validate/IsEmpty +
// service call + success log, canonicalizing every error before returning. See
// createLimit for the split rationale.
func (h *LimitHandler) updateLimit(ctx context.Context, idParam string, rawBody []byte) (*model.Limit, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.update")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityLimit, "id")
	}

	// Check for immutable fields BEFORE parsing into struct
	// This ensures we detect if limitType or currency was sent in the request
	var rawMap map[string]any
	if err := json.Unmarshal(rawBody, &rawMap); err == nil {
		if _, hasLimitType := rawMap["limitType"]; hasLimitType {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Immutable field limitType in request", constant.ErrLimitImmutableField)
			return nil, pkg.ValidateBusinessError(constant.ErrLimitImmutableField, constant.EntityLimit)
		}

		if _, hasCurrency := rawMap["currency"]; hasCurrency {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Immutable field currency in request", constant.ErrLimitImmutableField)
			return nil, pkg.ValidateBusinessError(constant.ErrLimitImmutableField, constant.EntityLimit)
		}
	}

	var input UpdateLimitInput
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
		return nil, pkg.ValidateBusinessError(constant.ErrNothingToUpdate, constant.EntityLimit)
	}

	// Convert HTTP input to service input
	serviceInput := ToUpdateLimitServiceInput(&input)

	result, err := h.service.UpdateLimit(ctx, id, serviceInput)
	if err != nil {
		return nil, classifyLimitServiceError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.update"),
		libLog.String("limit.id", result.ID.String()),
	).Log(ctx, libLog.LevelDebug, "Limit updated")

	return result, nil
}

func (h *LimitHandler) ActivateLimit(c *fiber.Ctx) error {
	limit, err := h.activateLimit(c.UserContext(), c.Params("id"))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, limit)
}

// activateLimit is the transport-agnostic core of the activate-limit operation
// shared by the Fiber method (ActivateLimit) and the Huma func
// (ActivateLimitHuma). See createLimit for the split rationale.
func (h *LimitHandler) activateLimit(ctx context.Context, idParam string) (*model.Limit, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.activate")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityLimit, "id")
	}

	limit, err := h.service.ActivateLimit(ctx, id)
	if err != nil {
		return nil, classifyLimitServiceError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.activate"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelDebug, "Limit activated")

	return limit, nil
}

func (h *LimitHandler) DeactivateLimit(c *fiber.Ctx) error {
	limit, err := h.deactivateLimit(c.UserContext(), c.Params("id"))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, limit)
}

// deactivateLimit is the transport-agnostic core of the deactivate-limit
// operation shared by the Fiber method (DeactivateLimit) and the Huma func
// (DeactivateLimitHuma). See createLimit for the split rationale.
func (h *LimitHandler) deactivateLimit(ctx context.Context, idParam string) (*model.Limit, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.deactivate")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityLimit, "id")
	}

	limit, err := h.service.DeactivateLimit(ctx, id)
	if err != nil {
		return nil, classifyLimitServiceError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.deactivate"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelDebug, "Limit deactivated")

	return limit, nil
}

func (h *LimitHandler) DraftLimit(c *fiber.Ctx) error {
	limit, err := h.draftLimit(c.UserContext(), c.Params("id"))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, limit)
}

// draftLimit is the transport-agnostic core of the draft-limit operation shared
// by the Fiber method (DraftLimit) and the Huma func (DraftLimitHuma). See
// createLimit for the split rationale.
func (h *LimitHandler) draftLimit(ctx context.Context, idParam string) (*model.Limit, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.draft")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityLimit, "id")
	}

	limit, err := h.service.DraftLimit(ctx, id)
	if err != nil {
		return nil, classifyLimitServiceError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.draft"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelDebug, "Limit transitioned to draft")

	return limit, nil
}

func (h *LimitHandler) DeleteLimit(c *fiber.Ctx) error {
	if err := h.deleteLimit(c.UserContext(), c.Params("id")); err != nil {
		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// deleteLimit is the transport-agnostic core of the delete-limit operation
// shared by the Fiber method (DeleteLimit) and the Huma func (DeleteLimitHuma).
// See createLimit for the split rationale.
func (h *LimitHandler) deleteLimit(ctx context.Context, idParam string) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.delete")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID", err)
		return pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityLimit, "id")
	}

	if err := h.service.DeleteLimit(ctx, id); err != nil {
		return classifyLimitServiceError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.delete"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelDebug, "Limit deleted")

	return nil
}

func (h *LimitHandler) GetLimitUsage(c *fiber.Ctx) error {
	snapshot, err := h.getLimitUsage(c.UserContext(), c.Params("id"))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, snapshot)
}

// getLimitUsage is the transport-agnostic core of the get-limit-usage operation
// shared by the Fiber method (GetLimitUsage) and the Huma func
// (GetLimitUsageHuma). See createLimit for the split rationale.
func (h *LimitHandler) getLimitUsage(ctx context.Context, idParam string) (*model.UsageSnapshot, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.limit.get_usage")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	id, err := uuid.Parse(idParam)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID", err)
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityLimit, "id")
	}

	snapshot, err := h.service.GetLimitUsage(ctx, id)
	if err != nil {
		return nil, classifyLimitServiceError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.limit.get_usage"),
		libLog.String("limit.id", id.String()),
		libLog.Any("current_usage", snapshot.CurrentUsage),
		libLog.Any("utilization_percent", snapshot.UtilizationPercent),
	).Log(ctx, libLog.LevelDebug, "Limit usage retrieved")

	return snapshot, nil
}

// classifyLimitServiceError maps a raw service error to its canonical Midaz
// error, attributing the span, WITHOUT rendering. It is the single
// classification the Fiber wrappers (render via http.WithError) and the Huma
// path (humaProblem -> *http.Detail) both consume, so both transports emit
// field/status/code/type-identical envelopes. The errors.Is cascade and the
// canonical mapping are preserved verbatim from the pre-Huma handler.
func classifyLimitServiceError(span trace.Span, err error) error {
	switch {
	case errors.Is(err, constant.ErrLimitNameAlreadyExists):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit name already exists", err)
		return pkg.ValidateBusinessError(constant.ErrLimitNameAlreadyExists, constant.EntityLimit)
	case errors.Is(err, constant.ErrLimitNotFound):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit not found", err)
		return pkg.ValidateBusinessError(constant.ErrLimitNotFound, constant.EntityLimit)
	case errors.Is(err, constant.ErrLimitAlreadyDeleted):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit already deleted", err)
		return pkg.ValidateBusinessError(constant.ErrLimitAlreadyDeleted, constant.EntityLimit)
	case errors.Is(err, constant.ErrLimitInvalidStatusChange):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid status transition", err)
		return pkg.ValidateBusinessError(constant.ErrLimitInvalidStatusChange, constant.EntityLimit)
	case errors.Is(err, constant.ErrInvalidCursor):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid cursor", err)
		return pkg.ValidateBusinessError(constant.ErrInvalidCursor, constant.EntityLimit)
	case errors.Is(err, constant.ErrInvalidSortColumn):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid sort column", err)
		return pkg.ValidateBusinessError(constant.ErrInvalidSortColumn, constant.EntityLimit)
	case errors.Is(err, constant.ErrInvalidSortOrder):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid sort order", err)
		return pkg.ValidateBusinessError(constant.ErrInvalidSortOrder, constant.EntityLimit)
	case errors.Is(err, constant.ErrLimitNameRequired):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit name required", err)
		return pkg.ValidateBusinessError(constant.ErrLimitNameRequired, constant.EntityLimit)
	case errors.Is(err, constant.ErrLimitNameTooLong):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit name too long", err)
		return pkg.ValidateBusinessError(constant.ErrLimitNameTooLong, constant.EntityLimit)
	case errors.Is(err, constant.ErrLimitNameInvalidChars):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit name invalid chars", err)
		return pkg.ValidateBusinessError(constant.ErrLimitNameInvalidChars, constant.EntityLimit)
	case errors.Is(err, constant.ErrLimitDescriptionInvalidChars):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit description invalid chars", err)
		return pkg.ValidateBusinessError(constant.ErrLimitDescriptionInvalidChars, constant.EntityLimit)
	case errors.Is(err, constant.ErrLimitInvalidType):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit type", err)
		return pkg.ValidateBusinessError(constant.ErrLimitInvalidType, constant.EntityLimit)
	case errors.Is(err, constant.ErrLimitInvalidMaxAmount):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid max amount", err)
		return pkg.ValidateBusinessError(constant.ErrLimitInvalidMaxAmount, constant.EntityLimit)
	case errors.Is(err, constant.ErrLimitInvalidCurrency):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid currency", err)
		return pkg.ValidateBusinessError(constant.ErrLimitInvalidCurrency, constant.EntityLimit)
	case errors.Is(err, constant.ErrLimitInvalidScope):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid scope", err)
		return pkg.ValidateBusinessError(constant.ErrLimitInvalidScope, constant.EntityLimit)
	default:
		libOpentelemetry.HandleSpanError(span, "Operation failed", err)
		return pkg.InternalServerError{Code: constant.ErrInternalServer.Error(), Title: "Internal Server Error", Message: "The server encountered an unexpected error. Please try again later or contact support."}
	}
}
