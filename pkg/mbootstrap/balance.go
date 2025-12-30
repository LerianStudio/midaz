package mbootstrap

import (
	"context"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

//go:generate mockgen --destination=balance_mock.go --package=mbootstrap . BalancePort

// BalancePort defines the interface for balance operations.
// This is a transport-agnostic "port" that abstracts
// how the onboarding module communicates with the transaction module.
//
// This interface is implemented by:
//   - transaction.UseCase: Direct implementation (unified ledger mode)
//   - GRPCBalanceAdapter: Network calls via gRPC (separate services mode)
//
// The onboarding module's UseCase depends on this port, allowing it to work
// with either implementation without knowing the underlying transport mechanism.
//
// In unified ledger mode, the transaction.UseCase is passed directly to onboarding,
// eliminating the need for intermediate adapters and enabling zero-overhead in-process calls.
type BalancePort interface {
	// CreateBalanceSync creates a balance synchronously and returns the created balance.
	// Named "Sync" to distinguish from the async queue-based CreateBalance in transaction module.
	CreateBalanceSync(ctx context.Context, input mmodel.CreateBalanceInput) (*mmodel.Balance, error)

	// DeleteAllBalancesByAccountID deletes all balances for a given account.
	DeleteAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, requestID string) error

	// CheckHealth verifies the balance service is available.
	// In unified mode (in-process), returns nil immediately.
	// In microservices mode (gRPC), checks gRPC connection health.
	CheckHealth(ctx context.Context) error
}
