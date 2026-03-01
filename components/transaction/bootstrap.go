// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package transaction provides the public API for initializing the transaction component.
// This package exposes factory functions that allow other components to instantiate
// the transaction service while keeping internal implementation details private.
package transaction

import (
	"fmt"

	"github.com/gofiber/fiber/v2"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
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
}

// Options configures the transaction service initialization behavior.
type Options struct {
	// Logger allows callers to provide a pre-configured logger, avoiding multiple
	// initializations when composing components (e.g. unified ledger).
	Logger libLog.Logger

	// CircuitBreakerStateListener receives notifications when circuit breaker state changes.
	// This is optional - pass nil if you don't need state change notifications.
	CircuitBreakerStateListener libCircuitBreaker.StateChangeListener
}

// InitService initializes the transaction service.
//
// Deprecated: Use InitServiceOrError for proper error handling.
// This function panics on initialization errors.
func InitService() TransactionService {
	service, err := InitServiceOrError()
	if err != nil {
		// Panic is intentional here: this deprecated function's contract is to panic.
		// Use InitServiceOrError for proper error handling.
		panic(fmt.Sprintf("transaction.InitService failed: %v", err)) //nolint:forbidigo
	}

	return service
}

// InitServiceOrError initializes the transaction service with explicit error handling.
// This is the recommended way to initialize the service as it allows callers to handle
// initialization errors gracefully instead of panicking.
func InitServiceOrError() (TransactionService, error) {
	svc, err := bootstrap.InitServers()
	if err != nil {
		return nil, fmt.Errorf("transaction: init servers: %w", err)
	}

	return svc, nil
}

// InitServiceWithOptionsOrError initializes the transaction service with custom options
// and explicit error handling. Use this when composing in unified ledger mode.
func InitServiceWithOptionsOrError(opts *Options) (TransactionService, error) {
	if opts == nil {
		return InitServiceOrError()
	}

	svc, err := bootstrap.InitServersWithOptions(&bootstrap.Options{
		Logger:                      opts.Logger,
		CircuitBreakerStateListener: opts.CircuitBreakerStateListener,
	})
	if err != nil {
		return nil, fmt.Errorf("transaction: init servers with options: %w", err)
	}

	return svc, nil
}

// ConsumerService is a standalone consumer that reads from Redpanda and persists
// to PostgreSQL/MongoDB. It does not include HTTP or gRPC servers. This is used
// by the dedicated consumer binary to cleanly separate the API path (ledger) from
// the persistence path (consumer).
type ConsumerService interface {
	Run()
	Close() error
}

// InitConsumerServiceOrError initializes the standalone consumer service.
// This creates all infrastructure needed for Redpanda consumption and database
// persistence without any HTTP or gRPC servers.
func InitConsumerServiceOrError(opts *Options) (ConsumerService, error) {
	var bopts *bootstrap.Options
	if opts != nil {
		bopts = &bootstrap.Options{
			Logger:                      opts.Logger,
			CircuitBreakerStateListener: opts.CircuitBreakerStateListener,
		}
	}

	svc, err := bootstrap.InitConsumerWithOptions(bopts)
	if err != nil {
		return nil, fmt.Errorf("transaction: init consumer with options: %w", err)
	}

	return svc, nil
}
