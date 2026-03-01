// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package onboarding provides the public API for initializing the onboarding component.
// This package exposes factory functions that allow other components to instantiate
// the onboarding service while keeping internal implementation details private.
package onboarding

import (
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/bootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
)

// ErrUnifiedModeRequiresBalancePort is returned when unified mode is enabled but no BalancePort is provided.
var ErrUnifiedModeRequiresBalancePort = errors.New("unified mode requires BalancePort to be provided")

// OnboardingService extends mbootstrap.Service with onboarding-specific methods.
// Use this interface when you need access to route registration for unified ledger mode.
type OnboardingService interface {
	mbootstrap.Service

	// GetRouteRegistrar returns a function that registers onboarding routes to a Fiber app.
	// This is used by the unified ledger server to consolidate all routes on a single port.
	GetRouteRegistrar() func(*fiber.App)

	// GetMetadataIndexPort returns the metadata index port for use by other modules.
	// This allows direct in-process calls for metadata index operations when running in unified mode.
	GetMetadataIndexPort() mbootstrap.MetadataIndexRepository
}

// Options configures the onboarding service initialization behavior.
// It controls whether the service runs in unified mode (part of the ledger monolith)
// or standalone mode (separate microservice).
//
// In unified mode, the onboarding service communicates with other modules
// (such as the transaction module) via direct in-process calls instead of gRPC.
// This improves performance by avoiding network overhead.
//
// Example usage in unified mode:
//
//	opts := &onboarding.Options{
//		UnifiedMode: true,
//		BalancePort: transactionUseCase, // implements mbootstrap.BalancePort
//	}
//	service, err := onboarding.InitServiceWithOptionsOrError(opts)
//
// Example usage in standalone mode:
//
//	// Pass nil or use InitService() directly for gRPC-based communication
//	service := onboarding.InitService()
type Options struct {
	// Logger allows callers to provide a pre-configured logger, avoiding multiple
	// initializations when composing components (e.g. unified ledger).
	Logger libLog.Logger

	// UnifiedMode indicates the service is running as part of the unified ledger.
	// When true, all ports must be provided for in-process communication.
	// When false (or Options is nil), uses gRPC adapters for remote communication.
	UnifiedMode bool

	// BalancePort enables direct in-process communication with the transaction module.
	// Required when UnifiedMode is true. The BalancePort is typically the
	// transaction.UseCase which implements mbootstrap.BalancePort.
	BalancePort mbootstrap.BalancePort
}

// InitService initializes the onboarding service.
//
// Deprecated: Use InitServiceOrError for proper error handling.
// This function panics on initialization errors.
func InitService() mbootstrap.Service {
	service, err := InitServiceOrError()
	if err != nil {
		// Panic is intentional here: this deprecated function's contract is to panic.
		// Use InitServiceOrError for proper error handling.
		panic(fmt.Sprintf("onboarding.InitService failed: %v", err)) //nolint:forbidigo
	}

	return service
}

// InitServiceOrError initializes the onboarding service with explicit error handling.
// This is the recommended way to initialize the service as it allows callers to handle
// initialization errors gracefully instead of panicking.
func InitServiceOrError() (mbootstrap.Service, error) {
	svc, err := bootstrap.InitServers()
	if err != nil {
		return nil, fmt.Errorf("onboarding: init servers: %w", err)
	}

	return svc, nil
}

// InitServiceWithOptionsOrError initializes the onboarding service with custom options
// and explicit error handling. Use this when running in unified ledger mode.
// Returns OnboardingService which provides access to route registration.
func InitServiceWithOptionsOrError(opts *Options) (OnboardingService, error) {
	if opts == nil {
		svc, err := bootstrap.InitServersWithOptions(nil)
		if err != nil {
			return nil, fmt.Errorf("onboarding: init servers with nil options: %w", err)
		}

		return svc, nil
	}

	if opts.UnifiedMode && opts.BalancePort == nil {
		return nil, ErrUnifiedModeRequiresBalancePort
	}

	svc, err := bootstrap.InitServersWithOptions(&bootstrap.Options{
		Logger:      opts.Logger,
		UnifiedMode: opts.UnifiedMode,
		BalancePort: opts.BalancePort,
	})
	if err != nil {
		return nil, fmt.Errorf("onboarding: init servers with options: %w", err)
	}

	return svc, nil
}
