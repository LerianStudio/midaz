// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// ActivateAccount transitions a PENDING_CRM_LINK + blocked=true account to
// ACTIVE + blocked=false and re-enables its default balance for sending and
// receiving.
//
// The account-side mutation (status + blocked) is atomic: it runs inside a
// single PG transaction in the onboarding DB using SELECT ... FOR UPDATE so
// concurrent writers cannot race the state change. When the account is not
// in PENDING_CRM_LINK + blocked=true, the call returns
// constant.ErrInvalidAccountActivationState and no row is modified.
//
// The balance-side mutation (allow_sending + allow_receiving) runs as a
// separate best-effort step against the transaction DB, because in multi-
// instance deployments the onboarding and transaction databases may be
// different PG instances — a cross-DB transaction is not available. This is
// acceptable because transaction eligibility also gates on the account's
// Status.Code and Blocked, so the balance flags are defense-in-depth, not the
// primary safety gate. A balance-update failure is logged as WARN and the
// caller observes success on the account transition.
//
// This is the Ledger-owned saga's activation step (see
// docs/plans/plan-mode-crm-ledger-abstraction-layer-*.md). It is not wired
// into any public HTTP route.
func (uc *UseCase) ActivateAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.activate_account")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Activating pending account",
		libLog.String("organization_id", organizationID.String()),
		libLog.String("ledger_id", ledgerID.String()),
		libLog.String("account_id", accountID.String()))

	if err := uc.AccountRepo.ActivatePendingAccount(ctx, organizationID, ledgerID, accountID); err != nil {
		// ActivatePendingAccount returns a business error
		// (ErrInvalidAccountActivationState) when the precondition fails and
		// a technical error otherwise; both are already span-annotated at the
		// repo layer.
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to activate pending account", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to activate pending account", libLog.Err(err))

		return err
	}

	// Best-effort unblock of the default balance. Failures do not roll back
	// the account transition: transaction eligibility gates on account.Status
	// and account.Blocked, so the account-side update is the authoritative
	// safety gate.
	allow := true
	balanceUpdate := mmodel.UpdateBalance{
		AllowSending:   &allow,
		AllowReceiving: &allow,
	}

	if err := uc.BalanceRepo.UpdateAllByAccountID(ctx, organizationID, ledgerID, accountID, balanceUpdate); err != nil {
		// Degraded-but-recoverable: account is live, balance flags may still
		// be false until a later reconciliation. Log as WARN, not ERROR.
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to unblock default balance after activation", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to unblock default balance after activation",
			libLog.String("account_id", accountID.String()),
			libLog.Err(err))
	}

	return nil
}
