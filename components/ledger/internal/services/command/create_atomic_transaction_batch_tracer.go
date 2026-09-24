// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
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
	var deadline time.Time

	if uc.ContextTracer != nil {
		participants := 0
		var budget time.Duration

		for index := range run.items {
			item := &run.items[index]
			settings := run.itemLedgerSettings(item).Tracer
			if item.honoredTracerSkip || settings.Mode == "" || settings.Mode == mmodel.TracerModeOff {
				continue
			}

			participants++
			budget += time.Duration(settings.TimeoutMs) * time.Millisecond
		}

		if participants == 0 {
			return nil
		}

		if participants > uc.ContextTracer.recovery.config.MaxBatch {
			return constant.ErrInvalidRequestBody
		}
		// Earlier items must remain prepared while later items use their own
		// Reserve budgets. All members share this immutable dispatch deadline.
		deadline = uc.ContextTracer.recovery.now().UTC().Add(budget).Add(uc.ContextTracer.recovery.config.AttemptTimeout)
	}
	for index := range run.items {
		item := &run.items[index]

		organizationID, ledgerID := run.itemScope(item)
		reservation := uc.reservePreparedTransaction(ctx, span, logger, ContextTracerInput{
			Key:         tracerreservation.Key{OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: item.transactionID},
			ExecutionID: run.executionID, Settings: run.itemLedgerSettings(item).Tracer,
			Amount: item.input.Send.Value, AssetCode: item.input.Send.Asset,
			Timestamp: item.transactionDate, HonoredSkip: item.honoredTracerSkip,
			LongLived: item.status == constant.PENDING, DispatchDeadline: deadline,
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

func (uc *UseCase) beginAtomicContextReservations(ctx context.Context, run *atomicTransactionBatchRun) error {
	if uc.ContextTracer == nil {
		return nil
	}

	attempts := make([]ContextTracerAttempt, 0, len(run.items))
	for _, item := range run.items {
		if item.tracerReservation.ContextAttempt != nil {
			attempts = append(attempts, *item.tracerReservation.ContextAttempt)
		}
	}

	return uc.ContextTracer.BeginBatchExecution(ctx, attempts)
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
