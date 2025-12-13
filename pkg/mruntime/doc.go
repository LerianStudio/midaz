// Package mruntime provides panic recovery utilities for Midaz services.
//
// This package offers policy-based panic recovery primitives that integrate
// with lib-commons logging. It supports two panic policies:
//
//   - KeepRunning: Log the panic and stack trace, then continue execution.
//     Use this for worker goroutines and any recovery boundaries you control.
//     (HTTP/gRPC handlers use framework-level recovery in this phase.)
//
//   - CrashProcess: Log the panic and stack trace, then re-panic to crash
//     the process. Use this for critical invariant violations where continuing
//     would cause data corruption or undefined behavior.
//
// # Safe Goroutine Launching
//
// Use SafeGo and SafeGoWithContext to launch goroutines with automatic panic
// recovery:
//
//	mruntime.SafeGo(logger, "background-task", mruntime.KeepRunning, func() {
//	    // This panic will be caught and logged, not crash the process
//	    doWork()
//	})
//
// # Deferred Recovery
//
// Use RecoverAndLog, RecoverAndCrash, or RecoverWithPolicy in defer statements:
//
//	func worker() {
//	    defer mruntime.RecoverAndLog(logger, "worker")
//	    // Panics here will be logged but not crash the process
//	}
//
// # Stack Traces
//
// All recovery functions capture and log the full stack trace using
// runtime/debug.Stack() for debugging purposes.
package mruntime
