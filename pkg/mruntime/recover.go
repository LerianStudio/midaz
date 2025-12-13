package mruntime

import (
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

// RecoverWithPolicy recovers from a panic and handles it according to the
// specified policy. Use this when the recovery behavior needs to be determined
// at runtime.
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

// logPanic logs the panic value and stack trace using the provided logger.
func logPanic(logger Logger, name string, panicValue any) {
	stack := debug.Stack()

	if logger == nil {
		// Last resort fallback - should never happen in production
		return
	}

	logger.Errorf("panic recovered in %s: %v\npanic_source=%s panic_value=%v\nstack_trace:\n%s",
		name, panicValue, name, panicValue, string(stack))
}
