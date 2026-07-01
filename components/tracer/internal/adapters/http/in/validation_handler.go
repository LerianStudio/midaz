// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

//go:generate mockgen -source=validation_handler.go -destination=mocks/validation_service_mock.go -package=mocks

import (
	"context"
	"encoding/json"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// maxPayloadSize is the maximum allowed request body size in bytes (100KB).
const maxPayloadSize = 100 * 1024

// ValidationService defines the interface for validation operations.
// Interface defined locally per Ring pattern.
type ValidationService interface {
	Validate(ctx context.Context, request *model.ValidationRequest) (*services.ValidateResult, error)
}

// ValidationHandler handles HTTP requests for transaction validation.
type ValidationHandler struct {
	service ValidationService
	clock   clock.Clock
}

// NewValidationHandler creates a new validation handler.
// Returns an error if service or clk is nil.
func NewValidationHandler(service ValidationService, clk clock.Clock) (*ValidationHandler, error) {
	if service == nil {
		return nil, errors.New("nil ValidationService passed to NewValidationHandler")
	}

	if clk == nil {
		return nil, errors.New("nil Clock passed to NewValidationHandler")
	}

	return &ValidationHandler{
		service: service,
		clock:   clk,
	}, nil
}

func (h *ValidationHandler) Validate(c *fiber.Ctx) error {
	result, err := h.validate(c.UserContext(), c.Body())
	if err != nil {
		return pkgHTTP.WithError(c, err)
	}

	// Return HTTP 201 for new requests, HTTP 200 for duplicate (idempotent) requests (DD-9)
	if result.IsDuplicate {
		return pkgHTTP.OK(c, result.Response)
	}

	return pkgHTTP.Created(c, result.Response)
}

// validate is the transport-agnostic core of the validate operation shared by the
// Fiber method (Validate) and the Huma func (ValidateHuma). It owns the span, the
// imperative payload-size check + parse + NormalizeAndValidate, the service call,
// and the success log — and it CANONICALIZES every error before returning it
// (parse/validation errors are already canonical; the service error is run through
// classifyValidationError here). So both callers render the returned error the
// same way — Fiber via pkgHTTP.WithError, Huma via humaProblem — and the two
// transports emit field/status/code/type-identical RFC 9457 envelopes. Choosing
// the success status (200 duplicate vs 201 new) from result.IsDuplicate stays at
// the edge; the core returns the full *services.ValidateResult so each caller can
// map it to its own status.
//
// The payload-size guard is preserved from the Fiber path: Huma has no Fiber-style
// body limit, so the check must live in the core; an oversized body yields the
// canonical ErrPayloadTooLarge, never a native Huma rejection.
func (h *ValidationHandler) validate(ctx context.Context, rawBody []byte) (*services.ValidateResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.validations.validate")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Check payload size (technical error - use HandleSpanError)
	if len(rawBody) > maxPayloadSize {
		logger.With(
			libLog.String("operation", "handler.validations.validate"),
			libLog.Int("payload_size", len(rawBody)),
			libLog.Int("max_size", maxPayloadSize),
		).Log(ctx, libLog.LevelWarn, "Payload too large")

		libOpentelemetry.HandleSpanError(span, "Payload exceeds size limit", constant.ErrPayloadTooLarge)

		return nil, pkg.ValidateBusinessError(constant.ErrPayloadTooLarge, constant.EntityValidationRequest)
	}

	var request model.ValidationRequest
	if err := json.Unmarshal(rawBody, &request); err != nil {
		logger.With(
			libLog.String("operation", "handler.validations.validate"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelWarn, "Failed to parse request body")

		libOpentelemetry.HandleSpanError(span, "Failed to parse request body", err)

		return nil, pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), Title: "Bad Request", Message: "The request body is malformed or contains invalid JSON. Please verify the syntax and try again."}
	}

	// Normalize and validate request (business error - use HandleSpanBusinessErrorEvent)
	// This validates currency is ISO 4217 uppercase (does NOT normalize), trims and lowercases subType (canonical form; matching is case-insensitive), and creates defensive metadata copy
	// Use injected clock for timestamp validation to support MOCK_TIME in tests
	now := h.clock.Now()
	if err := request.NormalizeAndValidate(now); err != nil {
		logger.With(
			libLog.String("operation", "handler.validations.validate"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelWarn, "Request validation failed")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Request validation failed", err)

		return nil, pkg.ValidateBusinessError(err, constant.EntityValidationRequest)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", request.RequestID.String()),
		attribute.String("app.request.transaction_type", string(request.TransactionType)),
		attribute.String("app.request.currency", request.Currency),
	)

	// Call validation service
	result, err := h.service.Validate(ctx, &request)
	if err != nil {
		return nil, h.classifyValidationError(span, err)
	}

	logger.With(
		libLog.String("operation", "handler.validations.validate"),
		libLog.String("request.id", request.RequestID.String()),
		libLog.String("decision", string(result.Response.Decision)),
		libLog.Any("processing_time_ms", result.Response.ProcessingTimeMs),
		libLog.Bool("is_duplicate", result.IsDuplicate),
	).Log(ctx, libLog.LevelDebug, "Validation completed")

	return result, nil
}

// classifyValidationError maps a raw service error to its canonical Midaz error,
// attributing the span, WITHOUT rendering. It is the single classification the
// Fiber method (which renders via pkgHTTP.WithError) and the Huma func
// (humaProblem -> *pkgHTTP.Detail) both consume, so both transports emit
// field/status/code/type-identical envelopes. The errors.Is cascade and the
// canonical mapping are preserved verbatim from the pre-Huma handleValidationError.
func (h *ValidationHandler) classifyValidationError(span trace.Span, err error) error {
	switch {
	case errors.Is(err, constant.ErrValidationTimeout):
		libOpentelemetry.HandleSpanError(span, "Validation timeout", err)

		return pkg.ValidateBusinessError(constant.ErrValidationTimeout, constant.EntityValidationRequest)
	case errors.Is(err, context.Canceled):
		libOpentelemetry.HandleSpanError(span, "Context cancelled", err)

		return pkg.ValidateBusinessError(constant.ErrContextCancelled, constant.EntityValidationRequest)
	case errors.Is(err, constant.ErrAmountExceedsPrecision):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Amount exceeds safe precision", err)

		return pkg.ValidateBusinessError(constant.ErrAmountExceedsPrecision, constant.EntityValidationRequest)
	case errors.Is(err, constant.ErrRuleEvaluationFailed):
		libOpentelemetry.HandleSpanError(span, "Rule evaluation failed", err)

		return pkg.ValidateBusinessError(constant.ErrRuleEvaluationFailed, constant.EntityValidationRequest)
	case errors.Is(err, constant.ErrLimitCheckFailed):
		libOpentelemetry.HandleSpanError(span, "Limit check failed", err)

		return pkg.ValidateBusinessError(constant.ErrLimitCheckFailed, constant.EntityValidationRequest)
	default:
		libOpentelemetry.HandleSpanError(span, "Validation failed", err)

		return pkg.InternalServerError{Code: constant.ErrInternalServer.Error(), Title: "Internal Server Error", Message: "The server encountered an unexpected error. Please try again later or contact support."}
	}
}
