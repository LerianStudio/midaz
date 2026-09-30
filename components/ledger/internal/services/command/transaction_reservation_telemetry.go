// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
)

// Bounded operation vocabulary of a confirm: the address it was sent with.
const (
	reservationConfirmOperationByID          = "confirm"
	reservationConfirmOperationByTransaction = "confirm_by_transaction"
)

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
