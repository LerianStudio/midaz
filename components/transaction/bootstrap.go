// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package transaction provides the public API for initializing the transaction component.
// This package exposes factory functions that allow other components to instantiate
// the transaction service while keeping internal implementation details private.
package transaction

import (
	"fmt"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v3/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/gofiber/fiber/v2"
)

// TransactionService extends mbootstrap.Service with transaction-specific functionality.
// This interface provides access to the BalancePort for in-process communication
// and route registration for unified ledger mode.
type TransactionService interface {
	mbootstrap.Service
	// GetBalancePort returns the balance port for use by other modules.
	// This allows direct in-process calls instead of gRPC when running in unified mode.
	// The returned BalancePort is the transaction UseCase itself.
	GetBalancePort() mbootstrap.BalancePort

	// GetMetadataIndexPort returns the metadata index repository for use by other modules.
	// This allows direct in-process calls for metadata index operations when running in unified mode.
	GetMetadataIndexPort() mbootstrap.MetadataIndexRepository

	// GetRouteRegistrar returns a function that registers transaction routes to a Fiber app.
	// This is used by the unified ledger server to consolidate all routes on a single port.
	GetRouteRegistrar() func(*fiber.App)

	// SetSettingsPort sets the settings port for querying ledger settings.
	// This is called after initialization in unified ledger mode to wire the onboarding
	// SettingsPort to transaction, resolving the circular dependency between components.
	SetSettingsPort(port mbootstrap.SettingsPort)
}

// Options configures the transaction service initialization behavior.
type Options struct {
	// Logger allows callers to provide a pre-configured logger, avoiding multiple
	// initializations when composing components (e.g. unified ledger).
	Logger libLog.Logger

	// CircuitBreakerStateListener receives notifications when circuit breaker state changes.
	// This is optional - pass nil if you don't need state change notifications.
	CircuitBreakerStateListener libCircuitBreaker.StateChangeListener

	// SettingsPort enables direct in-process communication with the onboarding module
	// for querying ledger settings. Optional - if not provided, settings functionality
	// will not be available.
	SettingsPort mbootstrap.SettingsPort

	// Multi-tenant configuration (only used in unified mode)
	MultiTenantEnabled bool
	TenantClient       *tmclient.Client
	TenantServiceName  string
	TenantEnvironment  string
	TenantManagerURL   string
}

// InitService initializes the transaction service.
//
// Deprecated: Use InitServiceOrError for proper error handling.
// This function panics on initialization errors.
func InitService() TransactionService {
	service, err := InitServiceOrError()
	if err != nil {
		panic(fmt.Sprintf("transaction.InitService failed: %v", err))
	}

	return service
}

// InitServiceOrError initializes the transaction service with explicit error handling.
// This is the recommended way to initialize the service as it allows callers to handle
// initialization errors gracefully instead of panicking.
func InitServiceOrError() (TransactionService, error) {
	return bootstrap.InitServers()
}

// InitServiceWithOptionsOrError initializes the transaction service with custom options
// and explicit error handling. Use this when composing in unified ledger mode.
func InitServiceWithOptionsOrError(opts *Options) (TransactionService, error) {
	if opts == nil {
		return InitServiceOrError()
	}

	return bootstrap.InitServersWithOptions(&bootstrap.Options{
		Logger:                      opts.Logger,
		CircuitBreakerStateListener: opts.CircuitBreakerStateListener,
		SettingsPort:                opts.SettingsPort,
		MultiTenantEnabled:          opts.MultiTenantEnabled,
		TenantClient:                opts.TenantClient,
		TenantServiceName:           opts.TenantServiceName,
		TenantEnvironment:           opts.TenantEnvironment,
		TenantManagerURL:            opts.TenantManagerURL,
	})
}
