// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// account_blocked is a projection of account.blocked onto the balance read
// model, and the account row is the only source of truth for it. Two things
// have to hold at every site that inserts a balance:
//
//  1. Inheritance — the new row is born with the block state the owning account
//     holds at creation time. A balance that starts unblocked under a blocked
//     account is an open door on a closed account, because the transactional hot
//     path reads the balance, never the account.
//
//  2. Re-verification — the account is read again AFTER the INSERT and the
//     projection realigned when the two disagree. This is what closes the
//     creation x block race: BlockAccount writes the account row first and only
//     then issues its balance-wide UPDATE, so an insert that commits after that
//     UPDATE would keep the stale value forever. Reading the account after the
//     insert cannot miss a block that was committed before it, in any
//     interleaving.
//
// The helpers below are the single implementation of both, shared by
// CreateAccount, CreateAsset, CreateAdditionalBalance and the overdraft
// companion path.

// resolveAccountBlockedState reads the owning account and reports whether it is
// blocked. A nil Blocked column (a legacy row that was never written) is not
// blocked. HolderOffV1 is deliberate: nothing here reads the holder columns, and
// a /v1-shaped read cannot fail on a schema that predates them.
//
// found is false when the account does not exist. Callers decide what that
// means: on the inheritance leg it is a real error, on the re-verification leg
// there is simply nothing left to converge.
func (uc *UseCase) resolveAccountBlockedState(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (blocked, found bool, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.resolve_account_blocked_state")
	defer span.End()

	acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1)
	if err != nil {
		// A missing account is not an infrastructure failure: Find maps
		// sql.ErrNoRows to pkg.ValidateBusinessError(ErrEntityNotFound,
		// EntityAccount), an EntityNotFoundError. Report it as not-found so the
		// re-verification leg can no-op on a concurrently deleted account
		// instead of failing the creation after durable writes. Detection
		// mirrors the rest of this package (see update_balance_overdraft.go).
		var notFound pkg.EntityNotFoundError
		if errors.As(err, &notFound) {
			return false, false, nil
		}

		libOpentelemetry.HandleSpanError(span, "Failed to read account block state", err)
		logger.Log(ctx, libLog.LevelError, "Failed to read account block state",
			libLog.String("account_id", accountID.String()),
			libLog.Err(err))

		return false, false, err
	}

	if acc == nil {
		return false, false, nil
	}

	return acc.Blocked != nil && *acc.Blocked, true, nil
}

// inheritAccountBlockedState resolves the block state a balance about to be
// created must be born with. A read failure is propagated: guessing false here
// would mint an unblocked balance under a possibly blocked account, which is the
// exact failure mode this task removes.
//
// An account that cannot be found yields false. The caller is mid-creation on
// that very account, so a missing row is a concurrent delete, and the balance it
// is about to write is already orphaned — inventing a block state for it adds
// nothing.
func (uc *UseCase) inheritAccountBlockedState(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (bool, error) {
	blocked, _, err := uc.resolveAccountBlockedState(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		return false, err
	}

	return blocked, nil
}

// reconcileBalanceAccountBlocked is the post-INSERT half of the guarantee. It
// re-reads the account and, when the state it now holds differs from the value
// just written, realigns the projection with the same balance-wide UPDATE
// BlockAccount uses.
//
// The realign is scoped to the account, not to the single row that was just
// inserted: one UPDATE converges every balance of the account, which is strictly
// more than needed and cannot leave a sibling behind. It is idempotent, so a
// concurrent block/unblock racing this write converges on whichever value lands
// last — and that writer runs its own propagation afterwards.
//
// Failures are returned, never swallowed. A creation whose projection cannot be
// proven consistent must not be confirmed to the caller.
//
// accountID == uuid.Nil is the minimal-fixture case the overdraft companion
// documents (a balance seeded without an account id); there is no account to
// read, so there is nothing to reconcile.
func (uc *UseCase) reconcileBalanceAccountBlocked(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, written bool) error {
	if accountID == uuid.Nil {
		return nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.reconcile_balance_account_blocked")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.account_id", accountID.String()),
		attribute.Bool("app.balance.account_blocked_written", written),
	)

	current, found, err := uc.resolveAccountBlockedState(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		return err
	}

	if !found {
		logger.Log(ctx, libLog.LevelWarn, "Account disappeared before the balance projection could be re-verified",
			libLog.String("account_id", accountID.String()))

		return nil
	}

	if current == written {
		return nil
	}

	span.SetAttributes(attribute.Bool("app.balance.account_blocked_realigned", true))

	logger.Log(ctx, libLog.LevelWarn, "Account block state changed during balance creation, realigning projection",
		libLog.String("account_id", accountID.String()),
		libLog.Bool("written", written),
		libLog.Bool("current", current))

	if err := uc.BalanceRepo.UpdateAccountBlockedByAccountID(ctx, organizationID, ledgerID, accountID, current); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to realign balance account_blocked projection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to realign balance account_blocked projection",
			libLog.String("account_id", accountID.String()),
			libLog.Bool("current", current),
			libLog.Err(err))

		return err
	}

	return nil
}
