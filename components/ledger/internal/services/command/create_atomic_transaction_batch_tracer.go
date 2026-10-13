// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
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
// order. A rejection at item k releases the handles acquired for items before
// k, also in request order, plus item k's own when its reserve went unanswered,
// and returns the singular Tracer business error correlated to the rejected
// item.
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
			run.itemLedgerSettings(item).Tracer,
			item.transactionID,
			item.input.Send.Value,
			item.input.Send.Asset,
			firstSourceAccount(item.validate.Sources, item.prepared.pool.ExplicitBalances),
			item.input.Metadata,
			item.transactionDate,
			reservationTTLForStatus(item.status),
			reservationPurposeForAction(item.action),
			item.honoredTracerSkip,
			item.input.Scheme,
		)
		item.tracerReservation = reservation.Handle

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
	}

	return nil
}

// settleAtomicTransactionBatchReservations applies only a proven terminal
// direction. Confirm and release reuse the singular non-blocking transport and
// its bounded retry path. Unknown outcomes retain every reservation unchanged.
// A PENDING item's reservations stay held on success: its commit or cancel
// settles them by transaction, while a confirmed abort still releases them.
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
		item := &run.items[index]

		switch settlement {
		case atomicTransactionBatchReservationConfirmedAbort:
			uc.releaseReservations(ctx, span, logger, item.tracerReservation)
		case atomicTransactionBatchReservationKnownSuccess:
			if item.status == constant.PENDING {
				continue
			}

			uc.confirmReservations(ctx, span, logger, item.tracerReservation)
		}
	}
}

// handoffReservedAtomicTransactionBatchExecution hands idempotencyRun's
// execution to the engine phase. A failed hand-off returns before the engine
// runs, and a hand-off record never runs the engine later, so no movement can
// follow: the reservations held for reserved's items go back. A nil reserved
// holds none.
func (uc *UseCase) handoffReservedAtomicTransactionBatchExecution(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	idempotencyRun *atomicTransactionBatchRun,
	reserved *atomicTransactionBatchRun,
) error {
	if err := uc.handoffAtomicTransactionBatchExecution(ctx, idempotencyRun); err != nil {
		if reserved != nil {
			uc.settleAtomicTransactionBatchReservations(
				ctx,
				span,
				logger,
				reserved,
				atomicTransactionBatchReservationConfirmedAbort,
			)
		}

		return err
	}

	return nil
}
