package mruntime

import (
	"context"
	"runtime/debug"
)

// Logger defines the minimal logging interface required by mruntime.
// This interface is satisfied by github.com/LerianStudio/lib-commons/v2/commons/log.Logger.
type Logger interface {
	Errorf(format string, args ...any)
}

// RecoverAndLog recovers from a panic, logs it with the stack trace, and
// continues execution. Use this in defer statements for handlers and workers
// where you want to prevent crashes.
//
// Note: This function does not record metrics or span events because it lacks
// context. For observability integration, use RecoverAndLogWithContext instead.
//
// Example:
//
//	func worker() {
//	    defer mruntime.RecoverAndLog(logger, "worker")
//	    // ...
//	}
func RecoverAndLog(logger Logger, name string) {
	if r := recover(); r != nil {
		logPanic(logger, name, r)
	}
}

// RecoverAndLogWithContext is like RecoverAndLog but with full observability integration.
// It records metrics, span events, and reports to error tracking services.
//
// Parameters:
//   - ctx: Context for observability (metrics, tracing, error reporting)
//   - logger: Logger for structured logging
//   - component: The service component (e.g., "transaction", "onboarding")
//   - name: Descriptive name for the goroutine or handler
//
// Example:
//
//	func handler(ctx context.Context) {
//	    defer mruntime.RecoverAndLogWithContext(ctx, logger, "transaction", "create_handler")
//	    // ...
//	}
func RecoverAndLogWithContext(ctx context.Context, logger Logger, component, name string) {
	if r := recover(); r != nil {
		stack := debug.Stack()
		logPanicWithStack(logger, name, r, stack)
		recordPanicObservability(ctx, r, stack, component, name)
	}
}

// RecoverAndCrash recovers from a panic, logs it with the stack trace, and
// re-panics to crash the process. Use this in defer statements for critical
// operations where continuing after a panic would be dangerous.
//
// Example:
//
//	func criticalOperation() {
//	    defer mruntime.RecoverAndCrash(logger, "critical-op")
//	    // ...
//	}
func RecoverAndCrash(logger Logger, name string) {
	if r := recover(); r != nil {
		logPanic(logger, name, r)
		panic(r)
	}
}

// RecoverAndCrashWithContext is like RecoverAndCrash but with full observability integration.
// It records metrics and span events before re-panicking.
//
// Parameters:
//   - ctx: Context for observability (metrics, tracing, error reporting)
//   - logger: Logger for structured logging
//   - component: The service component (e.g., "transaction", "onboarding")
//   - name: Descriptive name for the goroutine or handler
func RecoverAndCrashWithContext(ctx context.Context, logger Logger, component, name string) {
	if r := recover(); r != nil {
		stack := debug.Stack()
		logPanicWithStack(logger, name, r, stack)
		recordPanicObservability(ctx, r, stack, component, name)
		panic(r)
	}
}

// RecoverWithPolicy recovers from a panic and handles it according to the
// specified policy. Use this when the recovery behavior needs to be determined
// at runtime.
//
// Note: This function does not record metrics or span events because it lacks
// context. For observability integration, use RecoverWithPolicyAndContext instead.
//
// Example:
//
//	func flexibleHandler(policy mruntime.PanicPolicy) {
//	    defer mruntime.RecoverWithPolicy(logger, "handler", policy)
//	    // ...
//	}
func RecoverWithPolicy(logger Logger, name string, policy PanicPolicy) {
	if r := recover(); r != nil {
		logPanic(logger, name, r)

		if policy == CrashProcess {
			panic(r)
		}
	}
}

// RecoverWithPolicyAndContext is like RecoverWithPolicy but with full observability integration.
// It records metrics, span events, and reports to error tracking services.
//
// Parameters:
//   - ctx: Context for observability (metrics, tracing, error reporting)
//   - logger: Logger for structured logging
//   - component: The service component (e.g., "transaction", "onboarding")
//   - name: Descriptive name for the goroutine or handler
//   - policy: How to handle the panic after logging/recording
//
// Example:
//
//	func worker(ctx context.Context, policy mruntime.PanicPolicy) {
//	    defer mruntime.RecoverWithPolicyAndContext(ctx, logger, "transaction", "balance_worker", policy)
//	    // ...
//	}
func RecoverWithPolicyAndContext(ctx context.Context, logger Logger, component, name string, policy PanicPolicy) {
	if r := recover(); r != nil {
		stack := debug.Stack()
		logPanicWithStack(logger, name, r, stack)
		recordPanicObservability(ctx, r, stack, component, name)

		if policy == CrashProcess {
			panic(r)
		}
	}
}

// logPanic logs the panic value and stack trace using the provided logger.
// This is the legacy function that captures stack internally.
func logPanic(logger Logger, name string, panicValue any) {
	stack := debug.Stack()
	logPanicWithStack(logger, name, panicValue, stack)
}

// logPanicWithStack logs the panic with a pre-captured stack trace.
func logPanicWithStack(logger Logger, name string, panicValue any, stack []byte) {
	if logger == nil {
		// Last resort fallback - should never happen in production
		return
	}

	logger.Errorf("panic recovered: source=%s value=%v\nstack_trace:\n%s",
		name, panicValue, string(stack))
}

// recordPanicObservability records panic information to all configured observability systems.
// This includes metrics, distributed tracing, and error reporting services.
func recordPanicObservability(ctx context.Context, panicValue any, stack []byte, component, name string) {
	// Record metric
	recordPanicMetric(ctx, component, name)

	// Record span event
	RecordPanicToSpanWithComponent(ctx, panicValue, stack, component, name)

	// Report to error tracking service (e.g., Sentry) if configured
	reportPanicToErrorService(ctx, panicValue, stack, component, name)
}
