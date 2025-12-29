package mlog

import (
	"fmt"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/gofiber/fiber/v2"
)

// DefaultSkipPaths returns paths that should be skipped from wide event logging.
// Returns a fresh copy each time to prevent mutation.
func DefaultSkipPaths() []string {
	return []string{
		"/health",
		"/healthz",
		"/ready",
		"/readyz",
		"/live",
		"/livez",
		"/metrics",
		"/favicon.ico",
	}
}

// Config holds configuration for the WideEvent middleware.
type Config struct {
	// Logger is the logger to use for emitting wide events.
	Logger libLog.Logger

	// Service is the service name to include in events.
	Service string

	// Version is the service version to include in events.
	Version string

	// Environment is the environment name (e.g., "production", "staging").
	Environment string

	// SkipPaths is a list of paths to skip from wide event logging.
	// If nil, DefaultSkipPaths() is used.
	SkipPaths []string

	// SkipPathFunc is a function that returns true for paths that should be skipped.
	// This is called after checking SkipPaths.
	SkipPathFunc func(path string) bool
}

// NewWideEventMiddleware creates a Fiber middleware that initializes WideEvent
// at the start of each request and emits it at the end.
func NewWideEventMiddleware(cfg Config) fiber.Handler {
	skipPaths := cfg.SkipPaths
	if skipPaths == nil {
		skipPaths = DefaultSkipPaths()
	}

	// Build skip path map for O(1) lookup
	skipMap := make(map[string]struct{}, len(skipPaths))
	for _, path := range skipPaths {
		skipMap[path] = struct{}{}
	}

	return func(c *fiber.Ctx) error {
		path := c.Path()

		// Check if path should be skipped
		if _, skip := skipMap[path]; skip {
			return c.Next()
		}

		if cfg.SkipPathFunc != nil && cfg.SkipPathFunc(path) {
			return c.Next()
		}

		// Initialize wide event
		event := NewWideEvent(c)
		event.SetService(cfg.Service, cfg.Version, cfg.Environment)

		// Store in context
		SetWideEvent(c, event)

		// Capture panics to ensure wide event emission
		var err error

		var didPanic bool

		var panicVal any

		func() {
			defer func() {
				if r := recover(); r != nil {
					didPanic = true
					panicVal = r
				}
			}()

			err = c.Next()
		}()

		// Check for panic context from upstream recovery middleware too
		if upstreamPanic := c.Locals("panic_value"); upstreamPanic != nil {
			event.SetPanic(fmt.Sprintf("%v", upstreamPanic))
		} else if didPanic {
			event.SetPanic(fmt.Sprintf("%v", panicVal))
		}

		// Capture response details
		event.SetResponse(c.Response().StatusCode(), int64(len(c.Response().Body())))

		// Handle any error that occurred
		if err != nil {
			if fiberErr, ok := err.(*fiber.Error); ok {
				event.SetError("fiber_error", "", fiberErr.Message, false)
			} else {
				event.SetError("handler_error", "", err.Error(), false)
			}
		}

		// Emit the wide event
		event.Emit(cfg.Logger)

		// Re-panic after logging to preserve stack trace for upstream handlers
		if didPanic {
			panic(panicVal)
		}

		return err
	}
}
