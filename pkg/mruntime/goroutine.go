package mruntime

import (
	"context"
)

// SafeGo launches a goroutine with panic recovery. If the goroutine panics,
// the panic is handled according to the specified policy.
//
// Parameters:
//   - logger: Logger for recording panic information
//   - name: Descriptive name for the goroutine (used in logs)
//   - policy: How to handle panics (KeepRunning or CrashProcess)
//   - fn: The function to execute in the goroutine
//
// Example:
//
//	mruntime.SafeGo(logger, "email-sender", mruntime.KeepRunning, func() {
//	    sendEmail(to, subject, body)
//	})
func SafeGo(logger Logger, name string, policy PanicPolicy, fn func()) {
	go func() {
		defer RecoverWithPolicy(logger, name, policy)

		fn()
	}()
}

// SafeGoWithContext launches a goroutine with panic recovery and context
// propagation. The context allows for cancellation and deadline propagation.
//
// Parameters:
//   - ctx: Context for cancellation and values
//   - logger: Logger for recording panic information
//   - name: Descriptive name for the goroutine (used in logs)
//   - policy: How to handle panics (KeepRunning or CrashProcess)
//   - fn: The function to execute, receiving the context
//
// Example:
//
//	mruntime.SafeGoWithContext(ctx, logger, "async-processor", mruntime.KeepRunning,
//	    func(ctx context.Context) {
//	        processAsync(ctx, data)
//	    })
func SafeGoWithContext(ctx context.Context, logger Logger, name string, policy PanicPolicy, fn func(context.Context)) {
	go func() {
		defer RecoverWithPolicy(logger, name, policy)

		fn(ctx)
	}()
}
