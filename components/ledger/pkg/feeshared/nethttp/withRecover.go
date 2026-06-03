// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"fmt"
	"net/http"
	"runtime/debug"

	libObservability "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/gofiber/fiber/v2"

	// trace.SpanFromContext has no lib-commons equivalent; direct OTel import is required
	"go.opentelemetry.io/otel/trace"
)

type recoverMiddleware struct {
	Logger libLog.Logger
}

type RecoverMiddlewareOption func(r *recoverMiddleware)

func WithRecoverLogger(logger libLog.Logger) RecoverMiddlewareOption {
	return func(r *recoverMiddleware) {
		r.Logger = logger
	}
}

func buildRecoverOpts(opts ...RecoverMiddlewareOption) *recoverMiddleware {
	mid := &recoverMiddleware{
		Logger: &libLog.GoLogger{},
	}
	for _, opt := range opts {
		opt(mid)
	}

	return mid
}

func WithRecover(opts ...RecoverMiddlewareOption) fiber.Handler {
	return func(c *fiber.Ctx) error {
		mid := buildRecoverOpts(opts...)

		defer func() {
			if r := recover(); r != nil {
				logger := mid.Logger

				if ctxLogger := libObservability.NewLoggerFromContext(c.UserContext()); ctxLogger != nil {
					logger = ctxLogger
				}

				stack := debug.Stack()
				panicErr := fmt.Errorf("panic recovered: %v", r)

				logger.Log(c.UserContext(), libLog.LevelError, "Panic recovered",
					libLog.String("panic", fmt.Sprintf("%v", r)),
					libLog.String("stack", string(stack)))

				span := trace.SpanFromContext(c.UserContext())
				if span.IsRecording() {
					libOpentelemetry.HandleSpanError(span, fmt.Sprintf("Panic: %v", r), panicErr)
				}

				internalErr := pkg.InternalServerError{
					Code:    constant.ErrInternalServer.Error(),
					Title:   "Internal Server Error",
					Message: "The server encountered an unexpected error. Please try again later or contact support.",
				}

				_ = c.Status(http.StatusInternalServerError).JSON(internalErr)
			}
		}()

		return c.Next()
	}
}
