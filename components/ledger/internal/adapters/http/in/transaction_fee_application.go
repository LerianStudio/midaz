// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// feesDBResolver resolves a tenant's fee Mongo database. It is the narrow port
// the transaction handler depends on at the fee seam so the concrete
// tenant-manager Mongo manager (*tmmongo.Manager) can be injected at bootstrap
// and faked in tests. The signature mirrors tmmongo.Manager.GetDatabaseForTenant.
type feesDBResolver interface {
	GetDatabaseForTenant(ctx context.Context, tenantID string) (*mongo.Database, error)
}

// FeeApplier drives the in-process fee engine over a transaction's validated
// send/distribute structure. It is the narrow port the transaction handler
// depends on so the fee use case can be injected at bootstrap and faked in
// tests. The signature mirrors fees services.UseCase.CalculateFee: the engine
// mutates cf.Transaction.Send.* in place (legs are appended to Source.From /
// Distribute.To, and Send.Value moves on deductible fees) and returns a
// business error when a package rule rejects the transaction.
type FeeApplier interface {
	CalculateFee(ctx context.Context, cf *model.FeeCalculate, organizationID uuid.UUID) error
}

// applyFees drives the fee engine on the validated transaction and folds the
// resulting fee legs back into transactionInput. It mirrors the shape of
// enrichOverdraftOperations: a single seam that loads packages, runs the
// engine, and mutates the transaction so every downstream consumer of the
// re-run validate sees the fee-inclusive state.
//
// The fee engine owns its own package lookup + send/distribute mutation; this
// method only adapts the ledger transaction into the engine's FeeCalculate
// envelope and copies the mutated Send back out. The caller MUST re-run
// ValidateSendSourceAndDistribute after this returns nil so the fee legs reach
// the persistence path (BuildOperations / ProcessBalanceOperations /
// WriteTransaction) through a single reassigned validate pointer.
//
// On isRevert=true this is a no-op: the reverse transaction already carries the
// reversed fee legs reconstructed by TransactionRevert from the persisted
// parent operations, so re-charging here would double the fees.
//
// On isAnnotation=true (NOTED transactions) this is also a no-op: an annotation
// is one-sided and records no real balance movement, so charging it a fee would
// emit fee legs that have no funding side and break its invariants.
//
// A nil applier is a defensive/test no-op: bootstrap ALWAYS injects FeeApplier
// (there is no FeesEnabled flag), so the nil branch never fires in production —
// it only keeps the seam inert for tests that construct a handler without a fee
// use case.
func (handler *TransactionHandler) applyFees(
	ctx context.Context,
	transactionInput *mtransaction.Transaction,
	organizationID, ledgerID uuid.UUID,
	isRevert, isAnnotation bool,
) error {
	if isRevert || isAnnotation || handler.FeeApplier == nil {
		return nil
	}

	// Resolve the tenant's fee Mongo DB onto a derived ctx only now that we know
	// fees actually apply — the short-circuit above means reverts, annotations,
	// and the nil-applier test seam never trigger (or fail on) a resolution they
	// don't need.
	feesCtx, err := handler.resolveFeesTenantContext(ctx)
	if err != nil {
		return err
	}

	cf := &model.FeeCalculate{
		LedgerID:    ledgerID,
		Transaction: *transactionInput,
	}

	// The error is logged once by the seam caller (executeCreateTransaction);
	// recording it here too would double-log the same failure (T8).
	if err := handler.FeeApplier.CalculateFee(feesCtx, cf, organizationID); err != nil {
		return err
	}

	// The engine mutated cf.Transaction.Send in place (fee legs + moved value
	// for deductible fees). Fold the mutated send back into the caller's
	// transaction so the second validate runs over the fee-inclusive shape.
	*transactionInput = cf.Transaction

	return nil
}

// resolveFeesTenantContext returns a ctx carrying the CURRENT tenant's fee Mongo
// database on the GENERIC tmcore MB key, for use ONLY at the fee seam. The fee
// repos read GetMBContext(ctx) on the generic key, but the route-scoped
// feesTenantMiddleware that writes it is mounted on FEE routes only — never on
// the transaction route — so without this the fee lookup on an MT transaction
// would fall through to the static single-tenant fee DB shared across all
// tenants (a client-data-isolation breach). The resolution mirrors that
// middleware's single-manager path: GetDatabaseForTenant(tenantID) +
// ContextWithMB(ctx, db) with NO module.
//
// The returned ctx is DERIVED and must be passed only into applyFees, never
// back onto the request ctx — writing the generic key globally would bleed the
// fee DB onto the module-keyed onboarding/transaction injection the rest of the
// request relies on (the exact cross-route leak route-scoping prevents).
//
// In single-tenant mode (or when no manager is wired) the static fee connection
// is correct, so this is a no-op returning ctx unchanged.
func (handler *TransactionHandler) resolveFeesTenantContext(ctx context.Context) (context.Context, error) {
	if !handler.MultiTenantEnabled || handler.FeesMongoManager == nil {
		return ctx, nil
	}

	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		// MT enabled but no tenant on the ctx: fail cleanly rather than fall
		// through to the shared single-tenant fee DB.
		return nil, fmt.Errorf("fee seam: %w", tmcore.ErrTenantNotFound)
	}

	feesDB, err := handler.FeesMongoManager.GetDatabaseForTenant(ctx, tenantID)
	if err != nil {
		return nil, mapTenantError(err, tenantID)
	}

	return tmcore.ContextWithMB(ctx, feesDB), nil
}
