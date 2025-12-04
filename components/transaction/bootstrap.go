// Package transaction provides the public API for initializing the transaction component.
// This package exposes factory functions that allow other components to instantiate
// the transaction service while keeping internal implementation details private.
package transaction

import (
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
)

// TransactionService extends mbootstrap.Service with transaction-specific functionality.
// This interface provides access to the BalancePort for in-process communication.
type TransactionService interface {
	mbootstrap.Service
	// GetBalancePort returns the balance port for use by other modules.
	// This allows direct in-process calls instead of gRPC when running in unified mode.
	// The returned BalancePort is the transaction UseCase itself.
	GetBalancePort() mbootstrap.BalancePort
}

// InitService initializes the transaction service and returns it as the TransactionService interface.
// This allows other modules to compose the transaction service and access its BalanceRepository
// without accessing internal packages.
func InitService() TransactionService {
	return bootstrap.InitServers()
}
