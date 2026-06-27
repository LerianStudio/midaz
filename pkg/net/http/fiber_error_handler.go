// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/trace"
)

// CanonicalFiberErrorHandler is the Fiber ErrorHandler that renders the canonical
// {code,title,message} envelope (E13) for errors that escape the handler chain —
// chiefly *fiber.Error producers: auth assertions (401), Fiber's router (404/405),
// and the body-limit guard (413). Any unmapped error degrades to a generic 500
// with no raw error text (E9).
//
// Reuse this handler in every fiber.Config{ErrorHandler: ...} so all Midaz fiber
// apps share one error envelope.
func CanonicalFiberErrorHandler(c *fiber.Ctx, err error) error {
	ctx := c.UserContext()
	if ctx != nil {
		span := trace.SpanFromContext(ctx)
		libOpentelemetry.HandleSpanError(span, "handler error", err)
		span.End()
	}

	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		switch fiberErr.Code {
		case fiber.StatusUnauthorized:
			return WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidToken, ""))
		case fiber.StatusNotFound:
			return WithError(c, pkg.ValidateBusinessError(constant.ErrRouteNotFound, ""))
		case fiber.StatusMethodNotAllowed:
			return renderCanonical(c, fiber.StatusMethodNotAllowed, pkg.ValidateBusinessError(constant.ErrMethodNotAllowed, ""))
		case fiber.StatusRequestEntityTooLarge:
			return renderCanonical(c, fiber.StatusRequestEntityTooLarge, pkg.ValidateBusinessError(constant.ErrPayloadTooLarge, ""))
		}
	}

	logError(ctx, c, err)

	return WithError(c, pkg.ValidateInternalError(err, ""))
}

// renderCanonical emits the canonical {code,title,message} envelope at an explicit
// status for classes (405, 413) that the WithError status table does not produce.
func renderCanonical(c *fiber.Ctx, status int, err error) error {
	if validationErr := (pkg.ValidationError{}); errors.As(err, &validationErr) {
		return c.Status(status).JSON(fiber.Map{
			"code":    validationErr.Code,
			"title":   validationErr.Title,
			"message": validationErr.Message,
		})
	}

	if responseErr := (pkg.ResponseError{}); errors.As(err, &responseErr) {
		return c.Status(status).JSON(fiber.Map{
			"code":    responseErr.Code,
			"title":   responseErr.Title,
			"message": responseErr.Message,
		})
	}

	return WithError(c, err)
}

func logError(ctx context.Context, c *fiber.Ctx, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	logger := libObservability.NewLoggerFromContext(ctx)
	logger.Log(ctx, libLog.LevelError,
		"handler error",
		libLog.String("method", c.Method()),
		libLog.String("path", c.Path()),
		libLog.Err(err),
	)
}
