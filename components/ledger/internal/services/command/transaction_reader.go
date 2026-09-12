// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// TransactionReader is the narrow read port the transaction write paths
// (create, revert, pending commit/cancel) depend on. It is declared here so
// command never imports the query package: the bootstrap wires the query use
// case in directly, since the signatures match.
type TransactionReader interface {
	SettingsReader

	// GetBalances loads the balances backing the given aliases.
	GetBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error)

	// GetEngineBalances loads the explicitly requested balances separately
	// from the complete set of balances required for engine execution.
	GetEngineBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, explicitAliases []string) (explicitBalances, executionBalances []*mmodel.Balance, err error)

	// ValidateAccountingRules enforces the ledger's accounting routes over the
	// balance operations and returns the resolved route cache.
	ValidateAccountingRules(ctx context.Context, organizationID, ledgerID uuid.UUID, operations []mmodel.BalanceOperation, validate *mtransaction.Responses, action string) (*mmodel.TransactionRouteCache, error)

	// GetParentByTransactionID returns the transaction that already reverts the given
	// one, or nil when it has none.
	GetParentByTransactionID(ctx context.Context, organizationID, ledgerID, parentID uuid.UUID) (*transaction.Transaction, error)

	// GetTransactionWithOperationsByID returns a transaction with its operations loaded.
	GetTransactionWithOperationsByID(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error)

	// GetWriteBehindTransaction returns a transaction from the write-behind cache,
	// with Body and Operations already populated, or an error on a cache miss.
	GetWriteBehindTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error)

	// GetTransactionByID returns a single transaction row, without its operations.
	GetTransactionByID(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error)

	// GetOperationRouteByID returns a single operation route.
	GetOperationRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.OperationRoute, error)
}
