// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	stdhttp "net/http"

	libObs "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/trace"
)

func legacyFiberErrorHandler(c *fiber.Ctx, err error) error {
	ctx := c.UserContext()
	if ctx != nil {
		span := trace.SpanFromContext(ctx)
		libOpentelemetry.HandleSpanError(span, "handler error", err)
	}

	statusCode := fiber.StatusInternalServerError

	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		statusCode = fiberErr.Code
	}

	if statusCode == fiber.StatusInternalServerError {
		if ctx == nil {
			ctx = context.Background()
		}

		logger := libObs.NewLoggerFromContext(ctx)
		logger.Log(ctx, libLog.LevelError,
			"handler error",
			libLog.String("method", c.Method()),
			libLog.String("path", c.Path()),
			libLog.Err(err),
		)

		// Keep the legacy response envelope for compatibility with older clients.
		return c.Status(statusCode).JSON(fiber.Map{"error": stdhttp.StatusText(statusCode)})
	}

	// Keep the legacy response envelope for compatibility with older clients.
	return c.Status(statusCode).JSON(fiber.Map{"error": stdhttp.StatusText(statusCode)})
}

func LegacyErrorBoundary() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := c.Next(); err != nil {
			return legacyFiberErrorHandler(c, err)
		}

		return nil
	}
}
