// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Confirm commits a reservation: the held amount moves reserved_usage ->
// current_usage and the row flips to CONFIRMED, with the audit row, in one
// transaction. The outcome names what the confirm found: a settled row counts
// as Confirmed; a row already CONFIRMED is an idempotent no-op with an empty
// outcome; a row already RELEASED is reported as AlreadyReleased, because its
// spend will never be counted and the caller decides what that means. None of
// the already-terminal cases is an error, and none moves the counter again.
//
// Confirm does NOT re-resolve limits (R38): the reservation row already carries
// limit_id / scope_key / period_key / amount.
func (s *ReservationService) Confirm(ctx context.Context, reservationID uuid.UUID) (ConfirmOutcome, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.reservation.confirm")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if reservationID == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing reservation id", constant.ErrReservationNotFound)
		return ConfirmOutcome{}, constant.ErrReservationNotFound
	}

	var (
		alreadyTerminal bool
		priorStatus     model.ReservationStatus
	)

	txErr := s.inTx(ctx, span, func(db pgdb.DB) error {
		locked, repoErr := s.repo.ConfirmWithTx(ctx, db, reservationID)
		if repoErr != nil {
			// Already terminal: the original transition moved the counter. Commit
			// nothing further; the prior status decides the outcome.
			alreadyTerminal = errors.Is(repoErr, constant.ErrReservationAlreadyTerminal)

			if locked != nil {
				priorStatus = locked.Status
			}

			return repoErr
		}

		if err := s.auditWriter.RecordReservationEventWithTx(
			ctx,
			db,
			model.AuditEventReservationConfirmed,
			model.AuditActionConfirm,
			reservationID,
			settledAuditContext(locked, model.StatusConfirmed),
		); err != nil {
			return fmt.Errorf("failed to record confirm audit event: %w", err)
		}

		return nil
	})
	if txErr != nil {
		if alreadyTerminal {
			return s.confirmAlreadyTerminal(ctx, span, logger, reservationID, priorStatus), nil
		}

		handleSettleSpanError(span, "Failed to confirm reservation", txErr)

		return ConfirmOutcome{}, txErr
	}

	return ConfirmOutcome{Confirmed: 1}, nil
}

// Release returns a reservation's held capacity on an aborted ledger transaction:
// reserved_usage is decremented (current_usage untouched) and the row flips to
// RELEASED, with the audit row, in one transaction. An already-terminal
// reservation is an idempotent success.
//
// Release does NOT re-resolve limits (R38).
func (s *ReservationService) Release(ctx context.Context, reservationID uuid.UUID) error {
	const operation = "service.reservation.release"

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, operation)
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if reservationID == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing reservation id", constant.ErrReservationNotFound)
		return constant.ErrReservationNotFound
	}

	alreadyTerminal := false

	txErr := s.inTx(ctx, span, func(db pgdb.DB) error {
		locked, repoErr := s.repo.ReleaseWithTx(ctx, db, reservationID, model.StatusReleased)
		if repoErr != nil {
			// Already terminal: the original transition moved the counter, so the
			// retry commits nothing further and succeeds.
			alreadyTerminal = errors.Is(repoErr, constant.ErrReservationAlreadyTerminal)

			return repoErr
		}

		if err := s.auditWriter.RecordReservationEventWithTx(
			ctx,
			db,
			model.AuditEventReservationReleased,
			model.AuditActionRelease,
			reservationID,
			settledAuditContext(locked, model.StatusReleased),
		); err != nil {
			return fmt.Errorf("failed to record release audit event: %w", err)
		}

		return nil
	})
	if txErr != nil {
		if alreadyTerminal {
			logger.With(
				libLog.String("operation", operation),
				libLog.String("reservation_id", reservationID.String()),
			).Log(ctx, libLog.LevelDebug, "Reservation already terminal — idempotent no-op")

			return nil
		}

		handleSettleSpanError(span, "Failed to release reservation", txErr)

		return txErr
	}

	return nil
}

// ConfirmByTransaction commits EVERY settleable reservation a transaction holds,
// addressing them by the ledger transaction id alone. This is the /commit-driven
// confirm: at /commit the ledger has only the transaction id (the reserve handle
// from create-pending does not survive the separate commit request), so the tracer
// resolves every RESERVED or EXPIRED row for the transaction and confirms each —
// the counter move, the row flip, and one audit row per flip all commit in ONE
// transaction.
//
// A transaction with nothing to settle is an idempotent no-op success: it never
// reserved, or every reservation already reached a terminal state. The outcome
// also counts the transaction's rows found in RELEASED, read under the same row
// lock as the rows it settles, so the caller learns about spend that will never be
// counted. ConfirmByTransaction does NOT re-resolve limits (R38) — each
// reservation row already carries its limit coordinates.
func (s *ReservationService) ConfirmByTransaction(ctx context.Context, transactionID uuid.UUID) (ConfirmOutcome, error) {
	const operation = "service.reservation.confirm_by_transaction"

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, operation)
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if transactionID == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing transaction id", ErrNilReservationTransationID)
		return ConfirmOutcome{}, ErrNilReservationTransationID
	}

	var outcome ConfirmOutcome

	txErr := s.inTx(ctx, span, func(db pgdb.DB) error {
		outcome = ConfirmOutcome{}

		reservations, alreadyReleased, repoErr := s.repo.ConfirmByTransactionWithTx(ctx, db, transactionID)
		if repoErr != nil {
			return repoErr
		}

		if err := s.recordByTransactionAudit(
			ctx, db, transactionID, reservations,
			model.StatusConfirmed,
			model.AuditEventReservationConfirmed,
			model.AuditActionConfirm,
		); err != nil {
			return err
		}

		outcome = ConfirmOutcome{Confirmed: len(reservations), AlreadyReleased: alreadyReleased}

		return nil
	})
	if txErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to confirm reservations by transaction", txErr)
		return ConfirmOutcome{}, txErr
	}

	span.SetAttributes(attribute.Int("app.reservation.already_released", outcome.AlreadyReleased))

	if outcome.AlreadyReleased > 0 {
		s.noteAlreadyReleased(ctx, span, logger, operation, libLog.String("transaction_id", transactionID.String()), outcome.AlreadyReleased)
	}

	return outcome, nil
}

// ReleaseByTransaction returns the held capacity for EVERY RESERVED reservation a
// transaction holds, addressing them by the ledger transaction id alone. This is
// the /cancel-driven release: reserved_usage is decremented (current_usage
// untouched) and each row flips to RELEASED, with one audit row per flip, in ONE
// transaction. Returns the flipped count; a transaction with no RESERVED rows is
// an idempotent no-op success like ConfirmByTransaction. Does NOT re-resolve
// limits (R38).
func (s *ReservationService) ReleaseByTransaction(ctx context.Context, transactionID uuid.UUID) (int, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.reservation.release_by_transaction")
	defer span.End()

	if transactionID == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing transaction id", ErrNilReservationTransationID)
		return 0, ErrNilReservationTransationID
	}

	released := 0

	txErr := s.inTx(ctx, span, func(db pgdb.DB) error {
		reservations, repoErr := s.repo.ReleaseByTransactionWithTx(ctx, db, transactionID, model.StatusReleased)
		if repoErr != nil {
			return repoErr
		}

		if err := s.recordByTransactionAudit(
			ctx, db, transactionID, reservations,
			model.StatusReleased,
			model.AuditEventReservationReleased,
			model.AuditActionRelease,
		); err != nil {
			return err
		}

		released = len(reservations)

		return nil
	})
	if txErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to release reservations by transaction", txErr)
		return 0, txErr
	}

	return released, nil
}

// recordByTransactionAudit records one audit row per reservation a by-transaction
// confirm or release flipped, on the handle that owns the flips, carrying each
// row's limit coordinates and the status it now holds.
func (s *ReservationService) recordByTransactionAudit(
	ctx context.Context,
	db pgdb.DB,
	transactionID uuid.UUID,
	reservations []*model.Reservation,
	status model.ReservationStatus,
	eventType model.AuditEventType,
	action model.AuditAction,
) error {
	for _, res := range reservations {
		if err := s.auditWriter.RecordReservationEventWithTx(
			ctx,
			db,
			eventType,
			action,
			res.ID,
			command.ReservationAuditContext{
				TransactionID: transactionID,
				LimitID:       res.LimitID,
				ScopeKey:      res.ScopeKey,
				PeriodKey:     res.PeriodKey,
				Amount:        res.Amount,
				Status:        string(status),
			},
		); err != nil {
			return fmt.Errorf("failed to record %s audit event: %w", string(action), err)
		}
	}

	return nil
}

// settledAuditContext builds the audit context of a by-id confirm or release from
// the row the repository locked, carrying the same correlation a by-transaction
// settle records and the status the row now holds. A nil row yields the status
// alone.
func settledAuditContext(res *model.Reservation, status model.ReservationStatus) command.ReservationAuditContext {
	if res == nil {
		return command.ReservationAuditContext{Status: string(status)}
	}

	return command.ReservationAuditContext{
		TransactionID: res.TransactionID,
		LimitID:       res.LimitID,
		ScopeKey:      res.ScopeKey,
		PeriodKey:     res.PeriodKey,
		Amount:        res.Amount,
		Status:        string(status),
	}
}

// confirmAlreadyTerminal resolves the outcome of a confirm that found its row
// past RESERVED. A RELEASED row is a divergence worth an operator's attention;
// a CONFIRMED row is the idempotent retry the lifecycle expects.
func (s *ReservationService) confirmAlreadyTerminal(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	reservationID uuid.UUID,
	priorStatus model.ReservationStatus,
) ConfirmOutcome {
	const operation = "service.reservation.confirm"

	if priorStatus == model.StatusReleased {
		outcome := ConfirmOutcome{AlreadyReleased: 1}

		s.noteAlreadyReleased(ctx, span, logger, operation, libLog.String("reservation_id", reservationID.String()), outcome.AlreadyReleased)

		return outcome
	}

	logger.With(
		libLog.String("operation", operation),
		libLog.String("reservation_id", reservationID.String()),
	).Log(ctx, libLog.LevelDebug, "Reservation already confirmed — idempotent no-op")

	return ConfirmOutcome{}
}

// noteAlreadyReleased records a confirm that found rows already RELEASED: a Warn
// naming the resource and the count, and a span event. It is a business
// observation, so the span stays green.
func (s *ReservationService) noteAlreadyReleased(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	operation string,
	resource libLog.Field,
	alreadyReleased int,
) {
	span.AddEvent("reservation.confirm.already_released", trace.WithAttributes(
		attribute.Int("app.reservation.already_released", alreadyReleased),
	))

	logger.With(
		libLog.String("operation", operation),
		resource,
		libLog.Int("already_released", alreadyReleased),
	).Log(ctx, libLog.LevelWarn, "Confirm found reservations already released")
}

// handleSettleSpanError records a failed by-id settle on the span by error
// class: an unknown reservation is a business outcome the transport maps to
// NotFound, so the span stays green; anything else is technical.
func handleSettleSpanError(span trace.Span, msg string, err error) {
	if errors.Is(err, constant.ErrReservationNotFound) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, msg, err)
		return
	}

	libOpentelemetry.HandleSpanError(span, msg, err)
}
