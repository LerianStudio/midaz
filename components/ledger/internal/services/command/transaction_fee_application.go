// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

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
// Only the /v2 create pipeline calls it. The /v1 transaction contract does not
// include fees, so a /v1 create posts exactly as authored and never reaches the
// package lookup or the tenant fee-DB resolution; a revert does not call it either,
// because the reverse transaction already carries the reversed fee legs
// reconstructed by TransactionRevert from the persisted parent operations, so
// re-charging would double the fees.
//
// On isAnnotation=true (NOTED transactions) this is a no-op: an annotation
// is one-sided and records no real balance movement, so charging it a fee would
// emit fee legs that have no funding side and break its invariants.
//
// A nil applier is a defensive/test no-op: bootstrap ALWAYS injects FeeApplier
// (there is no FeesEnabled flag), so the nil branch never fires in production —
// it only keeps the seam inert for tests that construct a handler without a fee
// use case.
//
// An honored per-call fee skip (honoredFeeSkip=true, the two keys having already
// agreed at the resolution point upstream) bypasses the entire engine: no package
// lookup, no tenant resolution, no send mutation. The transaction posts as
// authored.
func (uc *UseCase) applyFees(
	ctx context.Context,
	transactionInput *mtransaction.Transaction,
	organizationID, ledgerID uuid.UUID,
	isAnnotation, honoredFeeSkip bool,
) error {
	if honoredFeeSkip {
		return nil
	}

	if isAnnotation || uc.FeeApplier == nil {
		return nil
	}

	// Resolve the tenant's fee Mongo DB onto a derived ctx only now that we know
	// fees actually apply — the short-circuit above means reverts, annotations,
	// and the nil-applier test seam never trigger (or fail on) a resolution they
	// don't need.
	feesCtx, err := uc.resolveFeesTenantContext(ctx)
	if err != nil {
		return err
	}

	cf := &model.FeeCalculate{
		LedgerID:    ledgerID,
		Transaction: *transactionInput,
	}

	// The error is logged once by the seam caller (CreateTransactionV2);
	// recording it here too would double-log the same failure (T8).
	if err := uc.FeeApplier.CalculateFee(feesCtx, cf, organizationID); err != nil {
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
func (uc *UseCase) resolveFeesTenantContext(ctx context.Context) (context.Context, error) {
	if !uc.MultiTenantEnabled || uc.FeesMongoManager == nil {
		return ctx, nil
	}

	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		// MT enabled but no tenant on the ctx: fail cleanly rather than fall
		// through to the shared single-tenant fee DB.
		return nil, fmt.Errorf("fee seam: %w", tmcore.ErrTenantNotFound)
	}

	feesDB, err := uc.FeesMongoManager.GetDatabaseForTenant(ctx, tenantID)
	if err != nil {
		return nil, MapTenantError(ctx, err, tenantID)
	}

	return tmcore.ContextWithMB(ctx, feesDB), nil
}
