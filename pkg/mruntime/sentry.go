package mruntime

import (
	"context"
	"fmt"
	"sync"
)

// ErrorReporter defines an interface for external error reporting services.
// This abstraction allows integration with services like Sentry without
// creating a hard dependency on any specific SDK.
//
// Implementations should:
//   - Handle nil contexts gracefully
//   - Be safe for concurrent use
//   - Not panic themselves
type ErrorReporter interface {
	// CaptureException reports a panic/exception to the error tracking service.
	// The tags map can include metadata like "component", "goroutine_name", etc.
	CaptureException(ctx context.Context, err error, tags map[string]string)
}

// errorReporterInstance is the singleton error reporter.
// It remains nil unless explicitly configured.
var (
	errorReporterInstance ErrorReporter
	errorReporterMu       sync.RWMutex
)

// SetErrorReporter configures the global error reporter for panic reporting.
// Pass nil to disable error reporting.
//
// This should be called once during application startup if Sentry or another
// error tracking service is desired.
//
// Example with Sentry:
//
//	type sentryReporter struct{}
//
//	func (s *sentryReporter) CaptureException(ctx context.Context, err error, tags map[string]string) {
//	    hub := sentry.GetHubFromContext(ctx)
//	    if hub == nil {
//	        hub = sentry.CurrentHub().Clone()
//	    }
//	    hub.WithScope(func(scope *sentry.Scope) {
//	        for k, v := range tags {
//	            scope.SetTag(k, v)
//	        }
//	        hub.CaptureException(err)
//	    })
//	}
//
//	mruntime.SetErrorReporter(&sentryReporter{})
func SetErrorReporter(reporter ErrorReporter) {
	errorReporterMu.Lock()
	defer errorReporterMu.Unlock()

	errorReporterInstance = reporter
}

// GetErrorReporter returns the currently configured error reporter.
// Returns nil if no reporter has been configured.
func GetErrorReporter() ErrorReporter {
	errorReporterMu.RLock()
	defer errorReporterMu.RUnlock()

	return errorReporterInstance
}

// reportPanicToErrorService reports a panic to the configured error reporter if one exists.
// This is called internally by recovery functions.
func reportPanicToErrorService(ctx context.Context, panicValue any, stack []byte, component, goroutineName string) {
	reporter := GetErrorReporter()
	if reporter == nil {
		return
	}

	// Convert panic value to error
	var err error

	switch v := panicValue.(type) {
	case error:
		err = v
	case string:
		err = &panicError{message: v}
	default:
		err = &panicError{message: "panic: " + formatPanicValue(panicValue)}
	}

	tags := map[string]string{
		"component":      component,
		"goroutine_name": goroutineName,
		"panic_type":     "recovered",
	}

	// Include stack trace if available (truncated to reasonable size for tags)
	if len(stack) > 0 {
		stackStr := string(stack)
		const maxStackLen = 4096
		if len(stackStr) > maxStackLen {
			stackStr = stackStr[:maxStackLen] + "\n...[truncated]"
		}
		tags["stack_trace"] = stackStr
	}

	reporter.CaptureException(ctx, err, tags)
}

// panicError wraps a panic value as an error for reporting.
type panicError struct {
	message string
}

func (e *panicError) Error() string {
	return e.message
}

// formatPanicValue formats a panic value as a string.
func formatPanicValue(v any) string {
	if v == nil {
		return "<nil>"
	}

	switch val := v.(type) {
	case string:
		return val
	case error:
		return val.Error()
	default:
		return fmt.Sprintf("%v", v)
	}
}
