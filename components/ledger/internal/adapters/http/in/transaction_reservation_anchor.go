// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// reservationOutcomeKind enumerates the three branches the reserve anchor can
// take before the balance commit.
type reservationOutcomeKind int

const (
	// reservationProceed: the create path continues to ProcessBalanceOperations.
	// Handle holds the reservation ids to confirm/release post-commit (it is
	// empty when the tracer was skipped — off/advisory/nil/fail-open).
	reservationProceed reservationOutcomeKind = iota

	// reservationReject: the transaction MUST be rejected before any balance
	// move (a DENIED limit decision, or a fail-closed unavailable tracer). Err
	// carries the business error for the HTTP response; the caller releases the
	// idempotency key and removes the Redis-queue entry, mirroring the
	// post-fee re-validation rejection mechanics.
	reservationReject
)

// reservationOutcome is the decision the reserve anchor returns to the create
// seam. It is deliberately a value type with no balance data — the reserve seam
// observes amounts and gates execution; it never alters Send.Value or balance
// math (third rail).
type reservationOutcome struct {
	Kind   reservationOutcomeKind
	Handle reservationHandle
	Err    error
}

// reservationHandle carries the reservation ids produced by a successful
// reserve so the post-commit transport (confirm on success, release on abort)
// can address them. An empty handle means there is nothing to confirm or
// release (tracer skipped or no capacity-backed limit applied).
type reservationHandle struct {
	ReservationIDs []uuid.UUID
}

// reservationTTLPolicy selects the reservation lifetime hint passed to the
// tracer. Direct transactions get the tracer's default (short, reaper-swept)
// TTL; PENDING transactions get a long-lived hint so a reservation does not
// expire under a still-valid pending that has no existing sweep (R18).
type reservationTTLPolicy bool

const (
	reservationTTLDefault   reservationTTLPolicy = false
	reservationTTLLongLived reservationTTLPolicy = true
)

// reserveTransaction is the reserve anchor (F3-T13). It is called immediately
// before ProcessBalanceOperations on FEE-INCLUSIVE amounts and gates execution
// on the per-ledger tracer settings:
//
//   - mode=off (or nil reserver): skipped — returns proceed with an empty handle.
//   - mode=advisory: the reserve is called but never blocks — a DENIED decision
//     or an unavailable tracer still returns proceed (advisory observes, the
//     real gate is enforce).
//   - mode=enforce: a DENIED decision rejects before the balance commit; an
//     unavailable tracer branches on failPosture (open → proceed + SKIPPED
//     audit, closed → reject).
//
// It NEVER mutates Send.Value or any balance state; amount/asset are read-only
// inputs observed for the reservation request.
func (handler *TransactionHandler) reserveTransaction(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	settings mmodel.TracerSettings,
	transactionID uuid.UUID,
	amount decimal.Decimal,
	asset string,
	ttl reservationTTLPolicy,
) reservationOutcome {
	// off, unconfigured, or no client injected: the create path is unchanged.
	if handler.TracerReserver == nil || settings.Mode == mmodel.TracerModeOff || settings.Mode == "" {
		return reservationOutcome{Kind: reservationProceed}
	}

	advisory := settings.Mode == mmodel.TracerModeAdvisory

	req := tracer.ReserveRequest{
		TransactionID: transactionID,
		Amount:        amount.String(),
		Currency:      asset,
	}
	if ttl == reservationTTLLongLived {
		req.TransactionType = reservationLongLivedHint
	}

	result, err := handler.TracerReserver.Reserve(ctx, req)
	if err != nil {
		return handler.handleReserveError(ctx, span, logger, settings, transactionID, advisory, err)
	}

	if result.Denied {
		// Advisory observes the denial but never blocks; enforce rejects.
		if advisory {
			logger.Log(ctx, libLog.LevelWarn, "Tracer reservation denied in advisory mode; proceeding without gating",
				libLog.String("transaction_id", transactionID.String()))

			return reservationOutcome{Kind: reservationProceed}
		}

		rejectErr := pkg.ValidateBusinessError(constant.ErrTransactionReservationDenied, constant.EntityTransaction)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Tracer reservation denied", rejectErr)
		logger.Log(ctx, libLog.LevelWarn, "Tracer reservation denied; rejecting before balance commit",
			libLog.String("transaction_id", transactionID.String()))

		return reservationOutcome{Kind: reservationReject, Err: rejectErr}
	}

	return reservationOutcome{
		Kind:   reservationProceed,
		Handle: reservationHandle{ReservationIDs: result.ReservationIDs},
	}
}

// handleReserveError maps a reserve transport failure to an outcome. An
// availability failure (tracer.ErrTracerUnavailable) is gated by failPosture;
// advisory never blocks regardless. A non-availability error (e.g. a bad
// request the tracer rejects) is treated like an availability failure for
// gating purposes so a tracer defect cannot silently let an enforce ledger
// commit unchecked under fail-closed, while fail-open still proceeds.
func (handler *TransactionHandler) handleReserveError(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	settings mmodel.TracerSettings,
	transactionID uuid.UUID,
	advisory bool,
	err error,
) reservationOutcome {
	libOpentelemetry.HandleSpanError(span, "Tracer reservation call failed", err)

	if advisory {
		logger.Log(ctx, libLog.LevelWarn, "Tracer reservation failed in advisory mode; proceeding",
			libLog.String("transaction_id", transactionID.String()),
			libLog.Err(err))

		return reservationOutcome{Kind: reservationProceed}
	}

	if settings.FailPosture == mmodel.TracerFailPostureClosed {
		rejectErr := pkg.ValidateBusinessError(constant.ErrTransactionReservationUnavailable, constant.EntityTransaction)

		logger.Log(ctx, libLog.LevelWarn, "Tracer unavailable and failPosture=closed; rejecting transaction",
			libLog.String("transaction_id", transactionID.String()),
			libLog.Err(err))

		return reservationOutcome{Kind: reservationReject, Err: rejectErr}
	}

	// failPosture=open (the default): record a SKIPPED audit and proceed so a
	// degraded tracer cannot block all transactions (R20). The SKIPPED audit is
	// the tracer's own record — best-effort via Release on no ids is a no-op, so
	// the audit is emitted by the tracer reserve attempt itself; here we mark
	// the span and continue.
	span.SetAttributes(attribute.Bool("app.tracer.reservation_skipped", true))
	logger.Log(ctx, libLog.LevelWarn, "Tracer unavailable and failPosture=open; skipping reservation and proceeding",
		libLog.String("transaction_id", transactionID.String()),
		libLog.Err(err))

	return reservationOutcome{Kind: reservationProceed}
}

// reservationLongLivedHint is the transactionType marker the ledger sends so the
// tracer assigns a long-lived reservation_expires_at to a PENDING-transaction
// reservation (F3-T15). It is distinct from the sub-minute direct-transaction
// TTL and from the 300s pending Redis mutual-exclusion lock (do not conflate).
const reservationLongLivedHint = "pending-long-lived"

// confirmReservations commits held reservations after a successful balance
// commit (F3-T14, the success phase). Transport is best-effort: a failure is
// logged at Warn, span-recorded, and never propagated — the TTL reaper is the
// durability backstop (design call G). A nil reserver or empty handle is a
// no-op.
func (handler *TransactionHandler) confirmReservations(ctx context.Context, span trace.Span, logger libLog.Logger, handle reservationHandle) {
	if handler.TracerReserver == nil {
		return
	}

	for _, id := range handle.ReservationIDs {
		if err := handler.TracerReserver.Confirm(ctx, id); err != nil {
			handler.recordReservationTransportFailure(ctx, span, logger, "confirm", id, err)
		}
	}
}

// releaseReservations returns held reservations on an aborted transaction
// (F3-T14, the abort phase). Same best-effort posture as confirmReservations.
func (handler *TransactionHandler) releaseReservations(ctx context.Context, span trace.Span, logger libLog.Logger, handle reservationHandle) {
	if handler.TracerReserver == nil {
		return
	}

	for _, id := range handle.ReservationIDs {
		if err := handler.TracerReserver.Release(ctx, id); err != nil {
			handler.recordReservationTransportFailure(ctx, span, logger, "release", id, err)
		}
	}
}

// recordReservationTransportFailure logs and span-records a confirm/release
// transport failure without propagating it. Both an availability failure
// (tracer.ErrTracerUnavailable) and any other transport error are the
// lost-transport case the reaper backstops at TTL, so both are Warn-logged and
// swallowed.
func (handler *TransactionHandler) recordReservationTransportFailure(ctx context.Context, span trace.Span, logger libLog.Logger, action string, id uuid.UUID, err error) {
	libOpentelemetry.HandleSpanError(span, "Tracer reservation "+action+" transport failed", err)

	logger.Log(ctx, libLog.LevelWarn, "Tracer reservation transport failed; reaper will reconcile at TTL",
		libLog.String("reservation_action", action),
		libLog.String("reservation_id", id.String()),
		libLog.Err(err))
}

// confirmReservationsByTransaction commits a transaction's held reservations at
// /commit (F3-T15, PENDING success phase). At /commit the ledger holds only the
// transaction id — the reserve handle from create-pending does not survive the
// separate commit request — so the tracer flips every RESERVED reservation the
// transaction holds, addressed by transaction id. Gated on the per-ledger tracer
// settings (off / nil reserver → no call); same best-effort, non-blocking posture
// as the by-id transport: a failure is logged at Warn, span-recorded, and never
// propagated, with the TTL reaper as the durability backstop.
func (handler *TransactionHandler) confirmReservationsByTransaction(ctx context.Context, span trace.Span, logger libLog.Logger, settings mmodel.TracerSettings, transactionID uuid.UUID) {
	if !handler.tracerReservationEnabled(settings) {
		return
	}

	if err := handler.TracerReserver.ConfirmByTransaction(ctx, transactionID); err != nil {
		handler.recordReservationByTransactionFailure(ctx, span, logger, "confirm", transactionID, err)
	}
}

// releaseReservationsByTransaction returns a transaction's held reservations at
// /cancel (F3-T15, PENDING abort phase). Same transaction-id addressing, gating,
// and non-blocking posture as confirmReservationsByTransaction.
func (handler *TransactionHandler) releaseReservationsByTransaction(ctx context.Context, span trace.Span, logger libLog.Logger, settings mmodel.TracerSettings, transactionID uuid.UUID) {
	if !handler.tracerReservationEnabled(settings) {
		return
	}

	if err := handler.TracerReserver.ReleaseByTransaction(ctx, transactionID); err != nil {
		handler.recordReservationByTransactionFailure(ctx, span, logger, "release", transactionID, err)
	}
}

// tracerReservationEnabled reports whether the by-transaction confirm/release
// transport should fire: a reserver must be injected and the per-ledger mode must
// not be off/unset, mirroring the gate the reserve anchor applies at create time.
// Advisory and enforce both confirm/release — advisory observes the lifecycle, it
// only declines to BLOCK the request, and a confirm/release here never blocks.
func (handler *TransactionHandler) tracerReservationEnabled(settings mmodel.TracerSettings) bool {
	return handler.TracerReserver != nil && settings.Mode != mmodel.TracerModeOff && settings.Mode != ""
}

// recordReservationByTransactionFailure logs and span-records a by-transaction
// confirm/release transport failure without propagating it — the reaper reconciles
// any lost transition at TTL.
func (handler *TransactionHandler) recordReservationByTransactionFailure(ctx context.Context, span trace.Span, logger libLog.Logger, action string, transactionID uuid.UUID, err error) {
	libOpentelemetry.HandleSpanError(span, "Tracer reservation "+action+" by transaction transport failed", err)

	logger.Log(ctx, libLog.LevelWarn, "Tracer reservation by-transaction transport failed; reaper will reconcile at TTL",
		libLog.String("reservation_action", action),
		libLog.String("transaction_id", transactionID.String()),
		libLog.Err(err))
}

// reservationTTLForStatus selects the TTL policy from the transaction status:
// PENDING transactions get the long-lived hint, everything else gets the
// default reaper-swept TTL.
func reservationTTLForStatus(transactionStatus string) reservationTTLPolicy {
	if transactionStatus == constant.PENDING {
		return reservationTTLLongLived
	}

	return reservationTTLDefault
}
