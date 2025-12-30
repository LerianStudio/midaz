// Package safego provides panic-safe goroutine execution utilities.
// This package is intentionally isolated within the reconciliation component
// to avoid dependencies on the main pkg/ directory.
//
//nolint:panicguard // This package IS the SafeGo implementation - it must use raw goroutines and recover()
package safego

import (
	"fmt"
	"runtime/debug"
)

// Logger interface for panic logging
type Logger interface {
	Errorf(format string, args ...any)
}

// noopLogger is a no-op implementation of Logger used when nil is passed.
type noopLogger struct{}

//nolint:revive // Errorf intentionally does nothing in this noop implementation.
func (noopLogger) Errorf(string, ...any) {}

// Go executes a function in a goroutine with panic recovery.
// If the function panics, the panic is recovered and logged,
// preventing the panic from crashing the entire service.
func Go(logger Logger, name string, fn func()) {
	// Guard against nil logger to prevent nil-pointer panic inside the goroutine
	if logger == nil {
		logger = noopLogger{}
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				logger.Errorf("panic recovered in goroutine %q: %v\n%s", name, r, string(stack))
			}
		}()

		fn()
	}()
}

// GoWithCallback executes a function in a goroutine with panic recovery
// and calls the onPanic callback if a panic occurs.
func GoWithCallback(logger Logger, name string, fn func(), onPanic func(recovered any)) {
	// Guard against nil logger to prevent nil-pointer panic inside the goroutine
	if logger == nil {
		logger = noopLogger{}
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				logger.Errorf("panic recovered in goroutine %q: %v\n%s", name, r, string(stack))

				if onPanic != nil {
					onPanic(r)
				}
			}
		}()

		fn()
	}()
}

// RecoveredError wraps a recovered panic value as an error
type RecoveredError struct {
	Recovered any
	Stack     string
}

func (e *RecoveredError) Error() string {
	return fmt.Sprintf("panic: %v", e.Recovered)
}

// RunWithRecovery executes a function synchronously with panic recovery.
// Returns an error if the function panics.
func RunWithRecovery(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &RecoveredError{
				Recovered: r,
				Stack:     string(debug.Stack()),
			}
		}
	}()

	fn()

	return nil
}
