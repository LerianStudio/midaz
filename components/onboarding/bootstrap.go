// Package onboarding provides the public API for initializing the onboarding component.
// This package exposes factory functions that allow other components to instantiate
// the onboarding service while keeping internal implementation details private.
package onboarding

import (
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/bootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
)

// Options contains configuration options for initializing the onboarding service.
type Options struct {
	// UnifiedMode indicates the service is running as part of the unified ledger.
	// When true, all ports must be provided for in-process communication.
	// When false (or Options is nil), uses gRPC adapters for remote communication.
	UnifiedMode bool

	// BalancePort enables direct in-process communication with the transaction module.
	// Required when UnifiedMode is true. The BalancePort is typically the
	// transaction.UseCase which implements mbootstrap.BalancePort.
	BalancePort mbootstrap.BalancePort
}

// InitService initializes the onboarding service and returns it as the mbootstrap.Service interface.
// This allows other modules to compose the onboarding service without accessing internal packages.
func InitService() mbootstrap.Service {
	return bootstrap.InitServers()
}

// InitServiceWithOptions initializes the onboarding service with custom options.
// Use this when running in unified ledger mode to enable direct in-process calls.
func InitServiceWithOptions(opts *Options) mbootstrap.Service {
	if opts == nil {
		return bootstrap.InitServers()
	}

	return bootstrap.InitServersWithOptions(&bootstrap.Options{
		UnifiedMode: opts.UnifiedMode,
		BalancePort: opts.BalancePort,
	})
}
