package mlog

import (
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/gofiber/fiber/v2"
)

// responseSizeTrackerKey is the context key for storing the response size tracker.
const responseSizeTrackerKey = "wide_event_response_size_tracker"

// panicValueKey is the context key used to propagate a panic value from upstream middleware.
const panicValueKey = "panic_value"

// ResponseSizeTracker tracks the number of bytes written to the response.
// This is useful for streaming/chunked responses where Content-Length may not be set.
type ResponseSizeTracker struct {
	bytesWritten int64
}

// AddBytes adds to the bytes written counter (thread-safe).
func (t *ResponseSizeTracker) AddBytes(n int64) {
	atomic.AddInt64(&t.bytesWritten, n)
}

// BytesWritten returns the total bytes written (thread-safe).
func (t *ResponseSizeTracker) BytesWritten() int64 {
	return atomic.LoadInt64(&t.bytesWritten)
}

// GetResponseSizeTracker retrieves the response size tracker from the Fiber context.
// Returns nil if no tracker is present.
func GetResponseSizeTracker(c *fiber.Ctx) *ResponseSizeTracker {
	if tracker, ok := c.Locals(responseSizeTrackerKey).(*ResponseSizeTracker); ok {
		return tracker
	}

	return nil
}

// SetResponseSizeTracker stores the response size tracker in the Fiber context.
func SetResponseSizeTracker(c *fiber.Ctx, tracker *ResponseSizeTracker) {
	c.Locals(responseSizeTrackerKey, tracker)
}

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

// shouldSkipPath checks if the given path should be skipped from wide event logging.
func shouldSkipPath(path string, skipMap map[string]struct{}, skipPathFunc func(string) bool) bool {
	if _, skip := skipMap[path]; skip {
		return true
	}

	if skipPathFunc != nil && skipPathFunc(path) {
		return true
	}

	return false
}

// handlePanicContext checks for panic context and sets it on the event.
func handlePanicContext(c *fiber.Ctx, event *WideEvent, didPanic bool, panicVal any) {
	if upstreamPanic := c.Locals(panicValueKey); upstreamPanic != nil {
		event.SetPanic(fmt.Sprintf("%v", upstreamPanic))
	} else if didPanic {
		event.SetPanic(fmt.Sprintf("%v", panicVal))
	}
}

// handleMiddlewareError sets error context on the event based on error type.
func handleMiddlewareError(event *WideEvent, err error) {
	if err == nil {
		return
	}

	fiberErr := &fiber.Error{}
	if errors.As(err, &fiberErr) {
		event.SetError("fiber_error", strconv.Itoa(fiberErr.Code), fiberErr.Message, false)
	} else {
		event.SetError("handler_error", "", err.Error(), false)
	}
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
		if shouldSkipPath(c.Path(), skipMap, cfg.SkipPathFunc) {
			return c.Next()
		}

		// Initialize response size tracker early for accurate streaming response tracking.
		// Handlers can use GetResponseSizeTracker(c).AddBytes(n) when writing streamed data.
		tracker := &ResponseSizeTracker{}
		SetResponseSizeTracker(c, tracker)

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
				//nolint:panicguard // HTTP middleware boundary - must capture panics for logging before re-panicking
				if r := recover(); r != nil {
					didPanic = true
					panicVal = r
				}
			}()

			err = c.Next()
		}()

		handlePanicContext(c, event, didPanic, panicVal)

		// Capture response details with accurate size calculation
		responseSize := calculateResponseSize(c, tracker)
		event.SetResponse(c.Response().StatusCode(), responseSize)

		handleMiddlewareError(event, err)

		// Emit the wide event
		event.Emit(cfg.Logger)

		// Re-panic after logging to preserve stack trace for upstream handlers
		if didPanic {
			//nolint:panicguardwarn // Intentional re-panic after logging - preserves stack trace for upstream recovery handlers
			panic(panicVal)
		}

		//nolint:wrapcheck // Middleware must pass through handler errors without wrapping
		return err
	}
}

// calculateResponseSize determines the response size using the most accurate method available:
// 1. Content-Length header (if explicitly set by the handler)
// 2. ResponseSizeTracker bytes written (for instrumented streaming responses)
// 3. Response body length (fallback for buffered responses)
func calculateResponseSize(c *fiber.Ctx, tracker *ResponseSizeTracker) int64 {
	// First, check for Content-Length header (most authoritative when set)
	if contentLength := c.Response().Header.Peek("Content-Length"); len(contentLength) > 0 {
		if size, err := strconv.ParseInt(string(contentLength), 10, 64); err == nil && size >= 0 {
			return size
		}
	}

	// Second, check if we have tracked bytes from streaming writes
	if tracker != nil {
		if trackedBytes := tracker.BytesWritten(); trackedBytes > 0 {
			return trackedBytes
		}
	}

	// Fallback to body length for buffered responses
	return int64(len(c.Response().Body()))
}
