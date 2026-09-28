// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// reservationOutcomeKind enumerates the branches the reserve anchor can take
// before the balance commit.
type reservationOutcomeKind int

const (
	// reservationProceed: the create path continues to accounting-engine execution.
	// Handle carries the admission to complete post-commit (it is skipped when
	// the tracer was not consulted — off/nil/honored skip).
	reservationProceed reservationOutcomeKind = iota

	// reservationReject: the transaction MUST be rejected before any balance
	// move (a DENY or REVIEW decision under enforce, a deterministic contract
	// failure, or a fail-closed unavailable tracer). Err carries the business
	// error for the HTTP response; the caller releases the idempotency key and
	// removes the Redis-queue entry, mirroring the post-fee re-validation
	// rejection mechanics.
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

// reservationHandle carries the admission a post-commit completion (confirm on
// success, release on abort) addresses by transaction. A handle without an
// admission means there is nothing to confirm or release.
//
// It also carries the amount that was held. It is not needed to address the
// completion, but it is needed to REPORT one that could not be delivered: an
// operator reading a lost confirm has to know how much spending went uncounted.
type reservationHandle struct {
	ContextAttempt *ContextTracerAttempt
	TransactionID  uuid.UUID
	Amount         decimal.Decimal
	Asset          string
}

// confirmReservations commits held reservations after a successful balance
// commit. Transport never blocks the request: a failure is logged at Warn,
// span-recorded, and never propagated, because the money has already moved and
// the response is owed now. The failure is NOT dropped, though — it is handed
// to the retrier, which keeps trying off the request path until the tracer
// accepts it or the budget runs out. A handle without an admission is a no-op.
func (uc *UseCase) confirmReservations(ctx context.Context, span trace.Span, logger libLog.Logger, handle reservationHandle) {
	if handle.ContextAttempt == nil {
		return
	}

	uc.confirmReservationsByTransaction(ctx, span, logger, handle.ContextAttempt.Settings, handle, handle.ContextAttempt.Skipped)
}

// releaseReservations returns held reservations on an aborted transaction. Same
// non-blocking, retried posture as confirmReservations: a failure never fails
// the request and is redelivered off the request path. The direction of the
// loss is the opposite one — a release that never lands leaves capacity held
// against a transaction that moved no money — which is why the report
// distinguishes them.
func (uc *UseCase) releaseReservations(ctx context.Context, span trace.Span, logger libLog.Logger, handle reservationHandle) {
	if handle.ContextAttempt == nil {
		return
	}

	uc.releaseReservationsByTransaction(ctx, span, logger, handle.ContextAttempt.Settings, handle, handle.ContextAttempt.Skipped)
}

// confirmReservationsByTransaction commits a transaction's held reservations,
// addressed by transaction id. At /commit the ledger holds only the transaction
// id — the admission from create-pending does not survive the separate commit
// request — so the tracer flips every reservation the transaction holds. It
// belongs to the /v2 contract only; the /v1 contract carries no tracer. It is
// gated on the per-ledger tracer settings (off / no coordinator → no call) and
// on an honored per-call tracer skip, so a skip honored at create removes the
// cost here rather than relocating it to commit.
//
// Living only on the /v2 pipeline carries an accepted cost. A by-transaction
// call cannot tell whether the transaction holds reservations, so a PENDING
// created on /v2 and committed through /v1 never gets its confirm: the
// reservation is expired by the Tracer TTL, and the committed amount is never
// counted against the usage limit. Mixing mounts across one transaction
// lifecycle is therefore unsupported — see docs/api/SCOPING.md.
func (uc *UseCase) confirmReservationsByTransaction(ctx context.Context, span trace.Span, logger libLog.Logger, settings mmodel.TracerSettings, identity reservationHandle, honoredTracerSkip bool) {
	if honoredTracerSkip || !uc.tracerReservationEnabled(settings) {
		return
	}

	uc.completeContextReservation(ctx, span, logger, settings, identity, reservationActionConfirm)
}

// releaseReservationsByTransaction returns a transaction's held reservations at
// /cancel. Same transaction-id addressing, gating, and non-blocking posture as
// confirmReservationsByTransaction.
func (uc *UseCase) releaseReservationsByTransaction(ctx context.Context, span trace.Span, logger libLog.Logger, settings mmodel.TracerSettings, identity reservationHandle, honoredTracerSkip bool) {
	if honoredTracerSkip || !uc.tracerReservationEnabled(settings) {
		return
	}

	uc.completeContextReservation(ctx, span, logger, settings, identity, reservationActionRelease)
}

// tracerReservationEnabled reports whether the by-transaction confirm/release
// should fire: the context coordinator must be injected and the per-ledger mode
// must not be off/unset, mirroring the gate admission applies at create time.
// Advisory and enforce both confirm/release — advisory observes the lifecycle,
// it only declines to BLOCK the request, and a confirm/release here never
// blocks.
func (uc *UseCase) tracerReservationEnabled(settings mmodel.TracerSettings) bool {
	return uc.ContextTracer != nil && settings.Mode != mmodel.TracerModeOff && settings.Mode != ""
}

// reservationRequestIDNamespace is the UUIDv5 namespace used to derive a
// reserve requestId from a transactionID. A fixed namespace makes the requestId
// deterministic per transaction, so a retried reserve carries the same requestId
// and dedups against the prior attempt rather than minting a fresh request.
var reservationRequestIDNamespace = uuid.MustParse("6f3c2d1e-4b5a-4c6d-8e7f-0a1b2c3d4e5f")

// reservationRequestID derives the deterministic reserve requestId for a
// transaction. The tracer reserve contract requires a non-nil requestId; the
// ledger has no separate request handle at the anchor, so it derives one from
// the transactionID. Determinism is the contract: identical transactionID →
// identical requestId, so retries do not present as distinct requests.
func reservationRequestID(transactionID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(reservationRequestIDNamespace, transactionID[:])
}

func logReservationByTransactionFailure(ctx context.Context, span trace.Span, logger libLog.Logger, transition reservationTransition, err error) {
	libOpentelemetry.HandleSpanError(span, "Tracer reservation "+transition.Action+" by transaction transport failed", err)
	recordTracerFailureCause(span, err)

	logger.Log(ctx, libLog.LevelWarn, "Tracer reservation by-transaction transport failed on the first attempt; retrying off the request path",
		append(transition.logFields(), libLog.Err(err)))
}
