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
// performs no transition: the reservations are left to the Tracer TTL.
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

		organizationID, ledgerID := run.itemScope(item)

		reservation := uc.reservePreparedTransaction(ctx, span, logger, ContextTracerInput{
			Key:      ContextTracerKey{OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: item.transactionID},
			Settings: run.itemLedgerSettings(item).Tracer,
			Amount:   item.input.Send.Value, AssetCode: item.input.Send.Asset,
			Timestamp: item.transactionDate, HonoredSkip: item.honoredTracerSkip,
			LongLived: item.status == constant.PENDING,
		}, item.input, item.validate, item.prepared.pool.ExplicitBalances)
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
		if atomicTransactionBatchHoldsReservations(run) {
			logger.Log(ctx, libLog.LevelWarn, "Atomic transaction batch outcome unknown, reservations left to tracer TTL",
				libLog.String("execution_id", run.executionID.String()))
		}

		return
	}

	for index := range run.items {
		handle := run.items[index].tracerReservation

		switch settlement {
		case atomicTransactionBatchReservationConfirmedAbort:
			uc.releaseReservations(ctx, span, logger, handle)
		case atomicTransactionBatchReservationKnownSuccess:
			if run.items[index].status != constant.PENDING {
				uc.confirmReservations(ctx, span, logger, handle)
			}
		}
	}
}

// atomicTransactionBatchFailureSettlement classifies an engine failure that
// is not a confirmed precommit refusal. An engine that was never called moved
// no money; any other failure may have published it.
func atomicTransactionBatchFailureSettlement(outcome EngineExecutionOutcome) atomicTransactionBatchReservationSettlement {
	if !outcome.Executed {
		return atomicTransactionBatchReservationConfirmedAbort
	}

	return atomicTransactionBatchReservationUnknown
}

func atomicTransactionBatchHoldsReservations(run *atomicTransactionBatchRun) bool {
	for index := range run.items {
		handle := run.items[index].tracerReservation
		if handle.ContextAttempt != nil && handle.ContextAttempt.Dispatched {
			return true
		}
	}

	return false
}
