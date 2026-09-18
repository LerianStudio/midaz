// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"go.opentelemetry.io/otel/trace"
)

// atomicTransactionBatchReservationSettlement names only outcomes that are
// safe to project into Tracer. An indeterminate accounting result deliberately
// performs no transition: recovery owns reconciliation once engine evidence
// establishes whether money was published.
type atomicTransactionBatchReservationSettlement uint8

const (
	atomicTransactionBatchReservationUnknown atomicTransactionBatchReservationSettlement = iota
	atomicTransactionBatchReservationConfirmedAbort
	atomicTransactionBatchReservationKnownSuccess
)

// reserveAtomicTransactionBatch reserves fee-inclusive capacity in request
// order. A rejection at item k releases only handles acquired for items before
// k, also in request order, and returns the singular Tracer business error
// correlated to the rejected item.
func (uc *UseCase) reserveAtomicTransactionBatch(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	run *atomicTransactionBatchRun,
) error {
	for index := range run.items {
		item := &run.items[index]

		reservation := uc.reserveTransaction(
			ctx,
			span,
			logger,
			run.ledgerSettings.Tracer,
			item.transactionID,
			item.input.Send.Value,
			item.input.Send.Asset,
			firstSourceAccountID(item.validate.Sources, item.prepared.pool.ExplicitBalances),
			item.transactionDate,
			reservationTTLForStatus(item.status),
			item.honoredTracerSkip,
		)
		if reservation.Kind == reservationReject {
			uc.settleAtomicTransactionBatchReservations(
				ctx,
				span,
				logger,
				run,
				atomicTransactionBatchReservationConfirmedAbort,
			)

			return withAtomicTransactionBatchItemError(
				reservation.Err,
				index,
				"tracer reservation rejected",
			)
		}

		item.tracerReservation = reservation.Handle
	}

	return nil
}

// settleAtomicTransactionBatchReservations applies only a proven terminal
// direction. Confirm and release reuse the singular non-blocking transport and
// its bounded retry path. Unknown outcomes retain every reservation unchanged.
func (uc *UseCase) settleAtomicTransactionBatchReservations(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	run *atomicTransactionBatchRun,
	settlement atomicTransactionBatchReservationSettlement,
) {
	if settlement == atomicTransactionBatchReservationUnknown {
		return
	}

	for index := range run.items {
		handle := run.items[index].tracerReservation

		switch settlement {
		case atomicTransactionBatchReservationConfirmedAbort:
			uc.releaseReservations(ctx, span, logger, handle)
		case atomicTransactionBatchReservationKnownSuccess:
			uc.confirmReservations(ctx, span, logger, handle)
		}
	}
}
