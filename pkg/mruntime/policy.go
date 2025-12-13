package mruntime

// PanicPolicy determines how a recovered panic should be handled.
type PanicPolicy int

const (
	// KeepRunning logs the panic and stack trace, then continues execution.
	// Use for HTTP/gRPC handlers and worker goroutines where crashing would
	// affect other requests or tasks.
	KeepRunning PanicPolicy = iota

	// CrashProcess logs the panic and stack trace, then re-panics to crash
	// the process. Use for critical invariant violations where continuing
	// would cause data corruption or undefined behavior.
	CrashProcess
)

// String returns the string representation of the PanicPolicy.
func (p PanicPolicy) String() string {
	switch p {
	case KeepRunning:
		return "KeepRunning"
	case CrashProcess:
		return "CrashProcess"
	default:
		return "Unknown"
	}
}
