package http

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Static errors for panic recovery.
var (
	ErrPanicRecovered = errors.New("panic recovered")
)

type recoverMiddleware struct {
	Logger libLog.Logger
}

// RecoverMiddlewareOption configures the recover middleware.
type RecoverMiddlewareOption func(r *recoverMiddleware)

// WithRecoverLogger sets the logger for the recover middleware.
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

// WithRecover returns a Fiber middleware that recovers from panics.
//
//nolint:panicguard // This is an HTTP boundary middleware that handles panics at the edge
func WithRecover(opts ...RecoverMiddlewareOption) fiber.Handler {
	return func(c *fiber.Ctx) error {
		mid := buildRecoverOpts(opts...)

		defer func() {
			if r := recover(); r != nil {
				logger := mid.Logger

				ctxLogger, _, _, _ := libCommons.NewTrackingFromContext(c.UserContext())

				if ctxLogger != nil {
					logger = ctxLogger
				}

				stack := debug.Stack()
				panicErr := fmt.Errorf("%w: %v", ErrPanicRecovered, r)

				logger.Errorf("Panic recovered: %v\nStack trace:\n%s", r, string(stack))

				span := trace.SpanFromContext(c.UserContext())
				if span.IsRecording() {
					span.RecordError(panicErr)
					span.SetStatus(codes.Error, fmt.Sprintf("Panic: %v", r))
				}

				_ = c.Status(http.StatusInternalServerError).JSON(fiber.Map{
					"code":    "INTERNAL_SERVER_ERROR",
					"title":   "Internal Server Error",
					"message": "An unexpected error occurred. Please try again later.",
				})
			}
		}()

		return c.Next()
	}
}
