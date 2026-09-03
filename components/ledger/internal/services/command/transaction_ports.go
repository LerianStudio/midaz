// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// FeesDBResolver resolves a tenant's fee Mongo database. It is the narrow port
// the transaction create path depends on at the fee seam so the concrete
// tenant-manager Mongo manager (*tmmongo.Manager) can be injected at bootstrap
// and faked in tests. The signature mirrors tmmongo.Manager.GetDatabaseForTenant.
type FeesDBResolver interface {
	GetDatabaseForTenant(ctx context.Context, tenantID string) (*mongo.Database, error)
}

// FeeApplier drives the in-process fee engine over a transaction's validated
// send/distribute structure. It is the narrow port the transaction create path
// depends on so the fee use case can be injected at bootstrap and faked in
// tests. The signature mirrors fees services.UseCase.CalculateFee: the engine
// mutates cf.Transaction.Send.* in place (legs are appended to Source.From /
// Distribute.To, and Send.Value moves on deductible fees) and returns a
// business error when a package rule rejects the transaction.
type FeeApplier interface {
	CalculateFee(ctx context.Context, cf *model.FeeCalculate, organizationID uuid.UUID) error
}

// TracerReserver is the narrow port the transaction create seam depends on to
// drive the tracer's two-phase reservation lifecycle. It is declared here, at
// the consuming seam, so the concrete HTTP client can be injected at bootstrap
// and faked in tests — mirroring the FeeApplier precedent.
//
// Reserve holds limit capacity for the fee-inclusive transaction (phase one)
// and returns a handle carrying the reservation ids and the limit-exceeded
// decision. Confirm commits the held capacity on a successful transaction;
// Release returns it on an aborted one. A nil reserver means the tracer
// integration is disabled (tracer.mode=off / unconfigured) and the create path
// stays unchanged — call sites guard with a nil check, mirroring the streaming
// nil-emitter pattern.
//
// Confirm/Release address a single reservation by id, used inline by the direct
// (non-PENDING) create path which still holds the reserve handle. ConfirmByTransaction
// and ReleaseByTransaction address EVERY reservation a transaction holds by the
// transaction id alone — the PENDING lifecycle driver: /commit and /cancel are
// separate requests that carry only the transaction id (the reserve handle from
// create-pending does not survive them), so the tracer resolves and flips every
// RESERVED reservation for the transaction.
//
// Availability failures (timeout, transport error, open breaker) surface as
// tracer.ErrTracerUnavailable so the anchor can branch on tracer.failPosture;
// a DENIED decision is a successful Reserve return (handle.Denied=true), not an
// error.
type TracerReserver interface {
	Reserve(ctx context.Context, req tracer.ReserveRequest) (*tracer.ReserveResult, error)
	Confirm(ctx context.Context, reservationID uuid.UUID) error
	Release(ctx context.Context, reservationID uuid.UUID) error
	ConfirmByTransaction(ctx context.Context, transactionID uuid.UUID) error
	ReleaseByTransaction(ctx context.Context, transactionID uuid.UUID) error
}

// TransactionReader is the narrow read port the transaction create path depends
// on. It is declared here so command never imports the query package: the
// bootstrap wires the query use case in directly, since the signatures match.
type TransactionReader interface {
	// GetParsedLedgerSettings returns the parsed, cached settings for a ledger.
	GetParsedLedgerSettings(ctx context.Context, organizationID, ledgerID uuid.UUID) (mmodel.LedgerSettings, error)

	// GetBalances loads the balances backing the given aliases.
	GetBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error)

	// ValidateAccountingRules enforces the ledger's accounting routes over the
	// balance operations and returns the resolved route cache.
	ValidateAccountingRules(ctx context.Context, organizationID, ledgerID uuid.UUID, operations []mmodel.BalanceOperation, validate *mtransaction.Responses, action string) (*mmodel.TransactionRouteCache, error)

	// GetParentByTransactionID returns the transaction that already reverts the given
	// one, or nil when it has none.
	GetParentByTransactionID(ctx context.Context, organizationID, ledgerID, parentID uuid.UUID) (*transaction.Transaction, error)

	// GetTransactionWithOperationsByID returns a transaction with its operations loaded.
	GetTransactionWithOperationsByID(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error)

	// GetOperationRouteByID returns a single operation route.
	GetOperationRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.OperationRoute, error)
}
