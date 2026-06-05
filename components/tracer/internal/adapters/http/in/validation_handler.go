// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

//go:generate mockgen -source=validation_handler.go -destination=mocks/validation_service_mock.go -package=mocks

import (
	"context"
	"errors"
	"net/http"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/components/tracer/pkg/net/http"
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

// Validate godoc
//
//	@Summary		Validate a transaction
//	@Description	Validates a transaction against configured rules and limits.
//	@ID				validateTransaction
//	@Tags			validations
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			request		body		model.ValidationRequest	true	"Validation request"
//	@Success		200			{object}	model.ValidationResponse	"Duplicate request (idempotent)"
//	@Success		201			{object}	model.ValidationResponse	"New validation created"
//	@Failure		400			{object}	api.ErrorResponse	"Invalid input"
//	@Failure		401			{object}	api.ErrorResponse	"Unauthorized"
//	@Failure		413			{object}	api.ErrorResponse	"Payload too large (exceeds 100KB)"
//	@Failure		500			{object}	api.ErrorResponse	"Internal server error"
//	@Failure		503			{object}	api.ErrorResponse	"Service unavailable"
//	@Failure		504			{object}	api.ErrorResponse	"Gateway timeout"
//	@Router			/v1/validations [post]
func (h *ValidationHandler) Validate(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.validations.validate")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Check payload size (technical error - use HandleSpanError)
	if len(c.Body()) > maxPayloadSize {
		logger.With(
			libLog.String("operation", "handler.validations.validate"),
			libLog.Int("payload_size", len(c.Body())),
			libLog.Int("max_size", maxPayloadSize),
		).Log(ctx, libLog.LevelWarn, "Payload too large")

		libOpentelemetry.HandleSpanError(span, "Payload exceeds size limit", constant.ErrPayloadTooLarge)

		return pkgHTTP.JSONResponse(c, http.StatusRequestEntityTooLarge, libCommons.Response{
			Code:    constant.CodePayloadTooLarge,
			Title:   "Payload Too Large",
			Message: "payload too large: exceeds 100KB limit",
		})
	}

	var request model.ValidationRequest
	if err := c.BodyParser(&request); err != nil {
		logger.With(
			libLog.String("operation", "handler.validations.validate"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelWarn, "Failed to parse request body")

		libOpentelemetry.HandleSpanError(span, "Failed to parse request body", err)

		return pkgHTTP.BadRequestWithMessage(c, "TRC-0003", "Bad Request", h.parseErrorToUserMessage(err))
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

		return h.handleValidationInputError(c, err)
	}

	logger.With(
		libLog.String("operation", "handler.validations.validate"),
		libLog.String("request.id", request.RequestID.String()),
		libLog.Any("request.amount", request.Amount),
		libLog.String("request.currency", request.Currency),
		libLog.String("request.transaction_type", string(request.TransactionType)),
	).Log(ctx, libLog.LevelInfo, "Processing validation request")

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "validation_request", map[string]any{
		"request_id":       request.RequestID.String(),
		"transaction_type": string(request.TransactionType),
		"amount":           request.Amount,
		"currency":         request.Currency,
	}, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set span attributes", err)
	}

	// Call validation service
	result, err := h.service.Validate(ctx, &request)
	if err != nil {
		return h.handleValidationError(c, span, err)
	}

	logger.With(
		libLog.String("operation", "handler.validations.validate"),
		libLog.String("request.id", request.RequestID.String()),
		libLog.String("decision", string(result.Response.Decision)),
		libLog.Any("processing_time_ms", result.Response.ProcessingTimeMs),
		libLog.Bool("is_duplicate", result.IsDuplicate),
	).Log(ctx, libLog.LevelInfo, "Validation completed")

	// Return HTTP 201 for new requests, HTTP 200 for duplicate (idempotent) requests (DD-9)
	if result.IsDuplicate {
		return pkgHTTP.OK(c, result.Response)
	}

	return pkgHTTP.Created(c, result.Response)
}

// validationErrorMapping maps validation errors to their specific error codes and messages.
type validationErrorMapping struct {
	code    string
	message string
}

// validationErrorMappings maps validation errors to specific TRC codes and messages.
var validationErrorMappings = map[error]validationErrorMapping{
	constant.ErrValidationRequestIDRequired:       {code: "TRC-0220", message: "requestId is required"},
	constant.ErrValidationInvalidTransactionType:  {code: "TRC-0221", message: "transactionType must be one of [CARD, WIRE, PIX, CRYPTO]"},
	constant.ErrValidationAmountNonPositive:       {code: "TRC-0222", message: "amount must be positive"},
	constant.ErrValidationCurrencyRequired:        {code: "TRC-0223", message: "currency is required"},
	constant.ErrValidationInvalidCurrency:         {code: "TRC-0224", message: "currency must be valid ISO 4217 code (e.g., BRL, USD)"},
	constant.ErrValidationTimestampRequired:       {code: "TRC-0225", message: "transactionTimestamp is required"},
	constant.ErrValidationTimestampFuture:         {code: "TRC-0226", message: "transactionTimestamp cannot be in the future"},
	constant.ErrValidationAccountRequired:         {code: "TRC-0227", message: "account is required"},
	constant.ErrValidationTimestampPast:           {code: "TRC-0228", message: "transactionTimestamp is too far in the past (max 24h)"},
	constant.ErrValidationSegmentIDRequired:       {code: "TRC-0230", message: "segment.id is required when segment is provided"},
	constant.ErrValidationPortfolioIDRequired:     {code: "TRC-0231", message: "portfolio.id is required when portfolio is provided"},
	constant.ErrValidationSubTypeTooLong:          {code: "TRC-0232", message: "subType exceeds maximum length of 50 characters"},
	constant.ErrValidationInvalidAccountType:      {code: "TRC-0233", message: "account.type must be one of: checking, savings, credit"},
	constant.ErrValidationInvalidAccountStatus:    {code: "TRC-0234", message: "account.status must be one of: active, suspended, closed"},
	constant.ErrValidationInvalidMerchantCategory: {code: "TRC-0235", message: "merchant.category must be a 4-digit MCC code"},
	constant.ErrValidationInvalidMerchantCountry:  {code: "TRC-0236", message: "merchant.country must be ISO 3166-1 alpha-2 code (e.g., BR, US)"},
	constant.ErrValidationMerchantIDRequired:      {code: "TRC-0237", message: "merchant.id is required when merchant object is provided"},
	constant.ErrMetadataEntriesExceeded:           {code: "TRC-0063", message: "metadata exceeds maximum of 50 entries"},
	constant.ErrMetadataKeyLengthExceeded:         {code: "TRC-0060", message: "metadata key exceeds maximum length of 64 characters"},
	constant.ErrMetadataKeyInvalidChars:           {code: "TRC-0064", message: "metadata key contains invalid characters (only alphanumeric and underscore allowed)"},
}

// handleValidationInputError converts input validation errors to appropriate HTTP responses.
// Maps error codes to human-readable messages with field names for better debugging.
func (h *ValidationHandler) handleValidationInputError(c *fiber.Ctx, err error) error {
	for knownErr, mapping := range validationErrorMappings {
		if errors.Is(err, knownErr) {
			return pkgHTTP.BadRequestWithMessage(c, mapping.code, "Validation Error", mapping.message)
		}
	}

	logger, _, _, _ := libObservability.NewTrackingFromContext(c.UserContext()) //nolint:dogsled // only logger needed
	logger.With(libLog.String("error.message", err.Error())).Log(c.UserContext(), libLog.LevelWarn, "Unhandled validation input error")

	return pkgHTTP.BadRequestWithMessage(c, "TRC-0001", "Validation Error", "invalid request")
}

// parseErrorToUserMessage converts JSON parsing errors to user-friendly messages.
// Identifies the field that failed parsing based on error content.
func (h *ValidationHandler) parseErrorToUserMessage(err error) string {
	errMsg := err.Error()

	// UUID parsing errors - generic message since Fiber doesn't include field name
	if strings.Contains(errMsg, "invalid UUID") {
		return "invalid UUID format in request (check requestId, account.id, segment.id, portfolio.id, merchant.id)"
	}

	// Time parsing errors
	if strings.Contains(errMsg, "parsing time") || strings.Contains(errMsg, "cannot parse") {
		return "timestamp: invalid format (expected RFC3339)"
	}

	// Return generic message for other cases to avoid leaking parser details
	return "invalid request body"
}

// handleValidationError converts service errors to appropriate HTTP responses.
func (h *ValidationHandler) handleValidationError(c *fiber.Ctx, span trace.Span, err error) error {
	switch {
	case errors.Is(err, constant.ErrValidationTimeout):
		libOpentelemetry.HandleSpanError(span, "Validation timeout", err)

		return pkgHTTP.JSONResponse(c, http.StatusGatewayTimeout, libCommons.Response{
			Code:    constant.CodeValidationTimeout,
			Title:   "Gateway Timeout",
			Message: "validation timeout",
		})
	case errors.Is(err, context.Canceled):
		libOpentelemetry.HandleSpanError(span, "Context cancelled", err)

		return pkgHTTP.JSONResponse(c, http.StatusServiceUnavailable, libCommons.Response{
			Code:    constant.CodeContextCancelled,
			Title:   "Service Unavailable",
			Message: "request cancelled",
		})
	case errors.Is(err, constant.ErrAmountExceedsPrecision):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Amount exceeds safe precision", err)

		return pkgHTTP.BadRequestWithMessage(c, constant.CodeAmountExceedsPrecision, "Bad Request", "amount exceeds safe precision limit for evaluation (max: ±2^53)")
	case errors.Is(err, constant.ErrRuleEvaluationFailed):
		libOpentelemetry.HandleSpanError(span, "Rule evaluation failed", err)

		return pkgHTTP.InternalServerError(c, constant.CodeRuleEvaluationError, "Internal Server Error", "rule evaluation failed")
	case errors.Is(err, constant.ErrLimitCheckFailed):
		libOpentelemetry.HandleSpanError(span, "Limit check failed", err)

		return pkgHTTP.InternalServerError(c, constant.CodeLimitCheckError, "Internal Server Error", "limit check failed")
	default:
		libOpentelemetry.HandleSpanError(span, "Validation failed", err)

		return pkgHTTP.InternalServerError(c, constant.CodeInternalServer, "Internal Server Error", "validation processing failed")
	}
}
