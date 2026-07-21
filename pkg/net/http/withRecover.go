// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/codes"
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

				ctxLogger, _, _, _ := libObservability.NewTrackingFromContext(c.UserContext())

				if ctxLogger != nil {
					logger = ctxLogger
				}

				panicErr := fmt.Errorf("panic recovered")
				panicType := fmt.Sprintf("%T", r)

				logger.Log(c.UserContext(), libLog.LevelError, "panic recovered",
					libLog.String("panic_type", panicType),
				)

				span := trace.SpanFromContext(c.UserContext())
				if span.IsRecording() {
					span.RecordError(panicErr)
					span.SetStatus(codes.Error, "panic recovered")
				}

				// Serialize the RFC 9457 problem+json envelope, identical to the
				// normal error path, so clients parse one error shape everywhere.
				_ = WithError(c, pkg.ValidateInternalError(panicErr, ""))
			}
		}()

		return c.Next()
	}
}
