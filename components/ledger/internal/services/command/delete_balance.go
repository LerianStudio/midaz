// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

func (uc *UseCase) DeleteBalance(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.delete_balance")
	defer span.End()

	balance, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balance on repo by id", err)
		logger.Log(ctx, libLog.LevelError, "Error getting balance", libLog.Err(err))

		return err
	}

	// Scope protection: internal-scope balances are managed exclusively by
	// the system (e.g. overdraft reserves) and cannot be deleted via the
	// public API. This guard runs BEFORE the funds-check so the caller
	// receives the specific 0169 code instead of a generic validation.
	if balance != nil && balance.Settings != nil && balance.Settings.BalanceScope == mmodel.BalanceScopeInternal {
		err = pkg.ValidateBusinessError(constant.ErrDeletionOfInternalBalance, constant.EntityBalance)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Internal-scope balance cannot be deleted", err)
		logger.Log(ctx, libLog.LevelWarn, "Rejected deletion of internal-scope balance", libLog.Err(err))

		return err
	}

	if balance != nil && (!balance.Available.IsZero() || !balance.OnHold.IsZero()) {
		err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, constant.EntityBalance)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance cannot be deleted because it still has funds in it.", err)
		logger.Log(ctx, libLog.LevelWarn, "Error deleting balance", libLog.Err(err))

		return err
	}

	err = uc.BalanceRepo.Delete(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete balance on repo", err)
		logger.Log(ctx, libLog.LevelError, "Error delete balance", libLog.Err(err))

		return err
	}

	uc.emitBalanceDeletedEvent(ctx, span, logger, balance, time.Now())

	return nil
}

// emitBalanceDeletedEvent publishes the balance.deleted event for a
// successfully soft-deleted balance. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// Durability of the event is owned by PG and (follow-up task) the outbox
// subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked immediately after BalanceRepo.Delete succeeds on the
// explicit DELETE .../balances/:balance_id endpoint. The repository's
// SQL is `UPDATE balance SET deleted_at = NOW()`, so the wall-clock
// captured by the caller and the persisted timestamp differ only by
// clock skew. The cascade delete-all-by-account-id path is NOT covered:
// account.deleted carries that fan-out signal.
//
// The pre-delete Find call (already required by the scope + funds
// guards) supplies the balance's accountId for the payload — the repo's
// Delete signature carries only the balanceID. If Find returned nil
// (unexpected: the guards run after Find), the emission is skipped
// rather than producing a payload with empty identity.
//
// Wire-format mapping lives in pkg/streaming/events/balance_deleted.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitBalanceDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, b *mmodel.Balance, deletedAt time.Time) {
	if b == nil {
		return
	}

	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.BalanceDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewBalanceDeleted(b.ID, b.OrganizationID, b.LedgerID, b.AccountID, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
