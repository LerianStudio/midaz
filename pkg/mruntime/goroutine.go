package mruntime

import (
	"context"
)

// SafeGo launches a goroutine with panic recovery. If the goroutine panics,
// the panic is handled according to the specified policy.
//
// Note: This function does not record metrics or span events because it lacks
// context. For observability integration, use SafeGoWithContext instead.
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
// propagation.
//
// Note: For better observability labeling, prefer SafeGoWithContextAndComponent.
func SafeGoWithContext(ctx context.Context, logger Logger, name string, policy PanicPolicy, fn func(context.Context)) {
	SafeGoWithContextAndComponent(ctx, logger, "", name, policy, fn)
}

// SafeGoWithContextAndComponent is like SafeGoWithContext but also records the
// provided component name in observability signals.
//
// Parameters:
//   - ctx: Context for cancellation, values, and observability
//   - logger: Logger for recording panic information
//   - component: The service component (e.g., "transaction", "onboarding")
//   - name: Descriptive name for the goroutine (used in logs and metrics)
//   - policy: How to handle panics (KeepRunning or CrashProcess)
//   - fn: The function to execute, receiving the context
func SafeGoWithContextAndComponent(ctx context.Context, logger Logger, component, name string, policy PanicPolicy, fn func(context.Context)) {
	go func() {
		defer RecoverWithPolicyAndContext(ctx, logger, component, name, policy)

		fn(ctx)
	}()
}
