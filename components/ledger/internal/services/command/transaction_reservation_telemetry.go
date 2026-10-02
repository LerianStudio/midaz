// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
)

// Bounded operation vocabulary of a seam call: the call and, for a confirm or a
// release, the address it was sent with.
const (
	reservationOperationReserve              = "reserve"
	reservationConfirmOperationByID          = "confirm"
	reservationConfirmOperationByTransaction = "confirm_by_transaction"
	reservationReleaseOperationByID          = "release"
	reservationReleaseOperationByTransaction = "release_by_transaction"
)

// reservationCredentialRejectedTransitionMsg is the Error line a confirm or a
// release writes when the tracer rejects the ledger's seam credential.
const reservationCredentialRejectedTransitionMsg = "Tracer rejected the ledger's seam credential on a reservation transition; retrying off the request path"

// reservationCredentialUnavailableTransitionMsg is the Error line a confirm or a
// release writes when the ledger could not obtain its seam credential, so the
// call was never sent.
const reservationCredentialUnavailableTransitionMsg = "Tracer seam credential unavailable on a reservation transition; call not sent, retrying off the request path"

// tracerReservationConfirmAlreadyReleased counts confirms that found at least one
// reservation the tracer had already released. Each one is money that moved
// while the limit never counted it, so a non-zero rate is an under-enforcement
// signal. It carries only the operation attribute: no transaction, tenant or
// amount.
var tracerReservationConfirmAlreadyReleased = metrics.Metric{
	Name:        "tracer_reservation_confirm_already_released_total",
	Unit:        "1",
	Description: "Tracer reservation confirms that found already-released reservations, by operation.",
}

// Bounded reason vocabulary of a credential failure: the tracer rejected the
// ledger's credential, or the ledger could not obtain one and sent nothing.
const (
	reservationCredentialReasonRejected = "rejected"
	reservationCredentialReasonNotSent  = "not_sent"
)

// tracerReservationCredentialRejected counts seam calls that failed on the
// ledger's credential: the tracer rejected it, or the ledger could not obtain one
// and never sent the call. Neither heals without an operator (a missing or
// revoked credential, an unreachable Access Manager), so any non-zero rate needs
// one. It carries only the operation and reason attributes: no transaction,
// tenant or credential.
var tracerReservationCredentialRejected = metrics.Metric{
	Name:        "tracer_reservation_credential_rejected_total",
	Unit:        "1",
	Description: "Tracer reservation seam calls that failed on the ledger credential, by operation and reason (rejected, not_sent).",
}

// reservationCredentialFailureReason classifies err as a credential failure and
// reports whether it is one.
func reservationCredentialFailureReason(err error) (string, bool) {
	switch {
	case errors.Is(err, tracer.ErrTracerUnauthorized):
		return reservationCredentialReasonRejected, true
	case errors.Is(err, tracer.ErrTracerCredentialUnavailable):
		return reservationCredentialReasonNotSent, true
	default:
		return "", false
	}
}

// recordReservationCredentialRejected increments the credential failure counter
// for operation and reason. A nil factory records nothing; an emit failure is
// logged at Debug and never fails the caller.
func recordReservationCredentialRejected(ctx context.Context, factory *metrics.MetricsFactory, logger libLog.Logger, operation, reason string) {
	if factory == nil {
		return
	}

	counter, err := factory.Counter(tracerReservationCredentialRejected)
	if err == nil {
		err = counter.WithLabels(map[string]string{"operation": operation, "reason": reason}).Add(ctx, 1)
	}

	if err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit a tracer reservation credential metric", libLog.Err(err))
	}
}

// transitionOperation names a confirm or release transition in the bounded
// operation vocabulary.
func transitionOperation(transition reservationTransition) string {
	switch {
	case transition.Action == reservationActionConfirm && transition.byTransaction():
		return reservationConfirmOperationByTransaction
	case transition.Action == reservationActionConfirm:
		return reservationConfirmOperationByID
	case transition.byTransaction():
		return reservationReleaseOperationByTransaction
	default:
		return reservationReleaseOperationByID
	}
}

// recordReservationConfirmOutcome flags a confirm whose outcome reports released
// reservations: a Warn, a span event and one counter increment. It never fails
// the caller and never schedules a retry, because the money has already moved
// and a repeated confirm cannot count a spend against a released reservation.
// The span stays green: the tracer answered as designed. A nil factory skips
// only the counter.
func recordReservationConfirmOutcome(
	ctx context.Context,
	span trace.Span,
	factory *metrics.MetricsFactory,
	logger libLog.Logger,
	transition reservationTransition,
	outcome tracer.ConfirmOutcome,
) {
	if outcome.AlreadyReleased <= 0 {
		return
	}

	operation := reservationConfirmOperationByID
	if transition.byTransaction() {
		operation = reservationConfirmOperationByTransaction
	}

	logger.Log(ctx, libLog.LevelWarn,
		"Tracer reservation confirm found reservations already released; the spend will not be counted against the limit",
		libLog.String("transaction_id", transition.TransactionID.String()),
		libLog.Int("already_released", outcome.AlreadyReleased),
		libLog.String("operation", operation))

	span.AddEvent("tracer.reservation.confirm_already_released", trace.WithAttributes(
		attribute.String("app.reservation.operation", operation),
		attribute.Int("app.reservation.already_released", outcome.AlreadyReleased),
	))

	if factory == nil {
		return
	}

	counter, err := factory.Counter(tracerReservationConfirmAlreadyReleased)
	if err == nil {
		err = counter.WithLabels(map[string]string{"operation": operation}).Add(ctx, 1)
	}

	if err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit a tracer reservation confirm metric", libLog.Err(err))
	}
}
