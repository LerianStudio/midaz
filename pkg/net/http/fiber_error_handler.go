// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"context"
	"errors"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// CanonicalFiberErrorHandler is the Fiber ErrorHandler that renders the canonical
// {code,title,message} envelope (E13) for errors that escape the handler chain —
// chiefly *fiber.Error producers: authorization refusals returned by lib-auth
// (401, 403, 503 when the Access Manager never decided, and any 4xx the Access
// Manager itself answered), Fiber's router (404/405), the body-limit guard (413),
// and the header-size guard (431). Any unmapped error degrades to a generic 500
// with no raw error text (E9).
//
// A returned refusal can carry 404 or 405, so "no such route" and "wrong method"
// are recognised only by the identity of Fiber's router singletons, never by
// status alone.
//
// Reuse this handler in every fiber.Config{ErrorHandler: ...} so all Midaz fiber
// apps share one error envelope.
func CanonicalFiberErrorHandler(c fiber.Ctx, err error) error {
	ctx := c.Context()
	if ctx != nil {
		span := trace.SpanFromContext(ctx)
		libOpentelemetry.HandleSpanError(span, "handler error", err)
		span.End()
	}

	// Only the router's singletons mean "no such route" or "wrong method"; the
	// Access Manager answers 404 when the token's subject does not exist, and that
	// is a refusal.
	if errors.Is(err, fiber.ErrNotFound) {
		return WithError(c, pkg.ValidateBusinessError(constant.ErrRouteNotFound, ""))
	}

	if errors.Is(err, fiber.ErrMethodNotAllowed) {
		return renderCanonical(c, fiber.StatusMethodNotAllowed, pkg.ValidateBusinessError(constant.ErrMethodNotAllowed, ""))
	}

	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		// A refusal the Access Manager itself answered carries that service's own
		// code, which is what the Console reads, at the status it chose.
		if isClientError(fiberErr.Code) && accessManagerCode(err) != "" {
			return withProblemStatus(c, fiberErr.Code, err)
		}

		switch fiberErr.Code {
		case fiber.StatusUnauthorized:
			return WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidToken, ""))
		case fiber.StatusForbidden:
			return WithError(c, pkg.ValidateBusinessError(constant.ErrInsufficientPrivileges, ""))
		case fiber.StatusServiceUnavailable:
			return WithError(c, pkg.ValidateBusinessError(constant.ErrAuthorizationServiceUnavailable, ""))
		case fiber.StatusRequestEntityTooLarge:
			return renderCanonical(c, fiber.StatusRequestEntityTooLarge, pkg.ValidateBusinessError(constant.ErrPayloadTooLarge, ""))
		case fiber.StatusRequestHeaderFieldsTooLarge:
			return renderCanonical(c, fiber.StatusRequestHeaderFieldsTooLarge, pkg.ValidateBusinessError(constant.ErrRequestHeaderFieldsTooLarge, ""))
		}

		// Every remaining 4xx is a refusal lib-auth returned without a usable
		// Access Manager code; it renders the generic client-error code at that
		// code's own status (FC-A item 4), never a 500. Typed: ErrBadRequest has
		// no business-error map entry.
		if isClientError(fiberErr.Code) {
			return renderCanonical(c, fiber.StatusBadRequest, pkg.ValidationError{
				Code:    constant.ErrBadRequest.Error(),
				Message: "The authorization service refused the request.",
			})
		}
	}

	logError(ctx, c, err)

	return WithError(c, pkg.ValidateInternalError(err, ""))
}

func isClientError(status int) bool {
	return status >= fiber.StatusBadRequest && status < fiber.StatusInternalServerError
}

// accessManagerCode is the code the Access Manager sent on a decoded refusal
// (AUT-xxxx), or "" when there is none to carry. An all-digit code is dropped:
// Midaz catalog codes are four digits, so passing one through would have clients
// read it as a Midaz code.
func accessManagerCode(err error) string {
	response := libCommons.Response{}
	if !errors.As(err, &response) || response.Code == "" {
		return ""
	}

	if strings.Trim(response.Code, "0123456789") == "" {
		return ""
	}

	return response.Code
}

// renderCanonical emits the RFC 9457 problem+json envelope at an explicit status
// for classes (405, 413) that the WithError status table does not produce. The
// explicit status overrides the code->status table (r3 §0, §1.3); the code is
// still carried verbatim (money path).
func renderCanonical(c fiber.Ctx, status int, err error) error {
	if responseErr := (pkg.ResponseError{}); errors.As(err, &responseErr) {
		return withProblemStatus(c, status, pkg.ValidationError{
			EntityType: responseErr.EntityType,
			Code:       responseErr.Code,
			Title:      responseErr.Title,
			Message:    responseErr.Message,
		})
	}

	return withProblemStatus(c, status, err)
}

func logError(ctx context.Context, c fiber.Ctx, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	logger := libObservability.NewLoggerFromContext(ctx)
	logger.Log(
		ctx, libLog.LevelError,
		"handler error",
		libLog.String("method", c.Method()),
		libLog.String("path", c.Path()),
		libLog.Err(err),
	)
}
