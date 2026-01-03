//go:build chaos

package chaos

import "errors"

// Sentinel errors for chaos operations.
var (
	// ErrContainerNotRunning is returned when a container is expected to be running but is not.
	ErrContainerNotRunning = errors.New("container is not running")

	// ErrContainerNotPaused is returned when a container is expected to be paused but is not.
	ErrContainerNotPaused = errors.New("container is not paused")

	// ErrToxiproxyNotConfigured is returned when a network chaos operation is attempted
	// without a configured Toxiproxy client.
	ErrToxiproxyNotConfigured = errors.New("toxiproxy client not configured")

	// ErrRecoveryTimeout is returned when a service fails to recover within the expected time.
	ErrRecoveryTimeout = errors.New("service failed to recover within timeout")

	// ErrDataIntegrityViolation is returned when data integrity checks fail after chaos.
	ErrDataIntegrityViolation = errors.New("data integrity violation detected")
)
