// Package mbootstrap provides shared interfaces for bootstrapping Midaz components.
// This package enables composition of multiple components (onboarding, transaction)
// into a unified service (ledger) while maintaining encapsulation of internal implementations.
package mbootstrap

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
)

// Runnable represents a component that can be run by the launcher.
// This interface is compatible with libCommons.NewLauncher's RunApp function.
type Runnable interface {
	Run(l *libCommons.Launcher) error
}

// Service represents a bootstrapped service that can be composed into a unified deployment.
// Each component (onboarding, transaction) implements this interface to expose
// its runnable components for composition.
type Service interface {
	// GetRunnables returns all runnable components of this service.
	// These will be passed to libCommons.NewLauncher for execution.
	GetRunnables() []RunnableConfig
}

// RunnableConfig pairs a runnable with its name for the launcher.
type RunnableConfig struct {
	Name     string
	Runnable Runnable
}

// ConsumerTrigger provides on-demand consumer activation for multi-tenant message queues.
// In lazy mode, consumers are not started until the first request arrives for a tenant.
// The tenant middleware calls EnsureConsumerStarted to trigger consumer spawning
// when an HTTP request arrives for a tenant that does not yet have an active consumer.
type ConsumerTrigger interface {
	// EnsureConsumerStarted ensures a message consumer is running for the given tenant.
	// If the consumer is already running, this is a no-op.
	// This method is safe for concurrent use by multiple goroutines.
	EnsureConsumerStarted(ctx context.Context, tenantID string)
}
