// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
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

	// GetOperationRouteByID returns an active operation route of the organization, whatever
	// ledger it was created under.
	GetOperationRouteByID(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.OperationRoute, error)
}

// TransactionProjectionResolution describes the freshest transaction view
// available to lifecycle writes. ExecutionID is present when the view came
// from authenticated engine evidence and must be retained as a causal dependency.
type TransactionProjectionResolution struct {
	Transaction *transaction.Transaction
	ExecutionID uuid.UUID
	Pending     bool
}

// TransactionProjectionResolver is optional so non-engine readers and older
// test doubles keep the narrow TransactionReader contract. Bootstrap's query
// use case implements it with Redis evidence first and primary SQL fallback.
type TransactionProjectionResolver interface {
	ResolveTransactionProjection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*transaction.Transaction, uuid.UUID, bool, error)
}

// TransactionGroupReader is the cross-ledger extension implemented by the
// production query use case. It stays separate from TransactionReader so
// singular transaction readers and focused test doubles do not gain an
// unrelated cross-scope method.
type TransactionGroupReader interface {
	FindTransactionsByGroupID(context.Context, uuid.UUID) ([]*transaction.Transaction, error)
}

// TransactionGroupMemberResolver locates the members of a cross-ledger group
// for the lifecycle paths, which run before the asynchronous projection may
// have reached PostgreSQL.
type TransactionGroupMemberResolver interface {
	// ResolveTransactionGroupMembers returns the members of the grouped
	// execution that last applied the addressed transaction, the addressed one
	// included: a hold's origins, or every part of a direct group or a commit.
	// When the addressed transaction has no engine index, or its execution
	// recorded no member manifest, it returns the group's rows read from the
	// primary by groupID. A manifest member that cannot be found answers
	// ErrCrossLedgerGroupIncomplete; the result never mixes the manifest with a
	// partial primary listing.
	ResolveTransactionGroupMembers(ctx context.Context, organizationID, ledgerID, transactionID, groupID uuid.UUID) ([]*transaction.Transaction, error)
}

func resolveTransactionProjection(
	ctx context.Context,
	reader TransactionReader,
	organizationID, ledgerID, transactionID uuid.UUID,
) (*TransactionProjectionResolution, error) {
	if resolver, ok := reader.(TransactionProjectionResolver); ok {
		tran, executionID, pending, err := resolver.ResolveTransactionProjection(ctx, organizationID, ledgerID, transactionID)
		if err != nil {
			return nil, err
		}

		return &TransactionProjectionResolution{Transaction: tran, ExecutionID: executionID, Pending: pending}, nil
	}

	tran, err := reader.GetTransactionWithOperationsByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	return &TransactionProjectionResolution{Transaction: tran}, nil
}

// loadLifecycleTransaction returns the transaction a commit, cancel, or revert
// acts on: the engine index, then the primary PostgreSQL, then the legacy
// write-behind entry. The engine never indexes an annotation, so until its
// projection lands only the legacy entry can name it. That entry is consulted
// only once both other sources answer not-found, so it never shadows a newer
// engine or persisted state; any other error propagates unchanged.
func (uc *UseCase) loadLifecycleTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	resolution, err := uc.loadLifecycleResolution(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	return resolution.Transaction, nil
}

// loadLifecycleResolution is loadLifecycleTransaction plus the engine index state
// the transaction was read from. ExecutionID is nil when the primary or the legacy
// entry answered.
func (uc *UseCase) loadLifecycleResolution(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*TransactionProjectionResolution, error) {
	resolution, err := uc.loadIndexedOrPersistedResolution(ctx, organizationID, ledgerID, transactionID)

	var notFound pkg.EntityNotFoundError
	if !errors.As(err, &notFound) {
		return resolution, err
	}

	legacy, legacyErr := uc.TransactionReader.GetWriteBehindTransaction(ctx, organizationID, ledgerID, transactionID)
	if legacyErr != nil || legacy == nil || legacy.ID == "" {
		return nil, err
	}

	return &TransactionProjectionResolution{Transaction: legacy}, nil
}

func (uc *UseCase) loadIndexedOrPersistedResolution(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*TransactionProjectionResolution, error) {
	resolution, err := resolveTransactionProjection(ctx, uc.TransactionReader, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	if resolution.Transaction != nil && resolution.Transaction.ID != "" {
		return resolution, nil
	}

	// A reader without the engine index answers from FindWithOperations, which
	// joins on operations, so a transaction with no rows comes back as an empty
	// value with no error. The row-only read tells a missing transaction from an
	// operationless one.
	tran, err := uc.TransactionReader.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	return &TransactionProjectionResolution{Transaction: tran}, nil
}
