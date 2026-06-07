// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

//go:generate mockgen -source=reservation_service.go -destination=mocks/reservation_service_mock.go -package=mocks

import (
	"context"
	"errors"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// reservationTTL is the lifetime of a DIRECT-transaction RESERVED row before the
// reaper may expire it. It bounds how long an abandoned reservation can hold
// capacity when the ledger neither confirms nor releases (crash between reserve
// and commit). Direct transactions resolve in seconds, so a short TTL keeps the
// reaper converging quickly.
const reservationTTL = 5 * time.Minute

// defaultLongLivedReservationTTL is the lifetime granted to a PENDING-transaction
// reservation (long-lived hint). PENDING transactions persist indefinitely with no
// existing sweep (R18), so a 5-minute direct TTL would expire a reservation backing
// a still-valid pending and the reaper would wrongly return its hold. 30 days is the
// pragmatic ceiling: long enough that real pending lifetimes never hit it, short
// enough that the reaper still converges a genuinely abandoned pending instead of
// holding capacity forever. Operators tune it via RESERVATION_LONG_LIVED_TTL_HOURS.
const defaultLongLivedReservationTTL = 720 * time.Hour // 30 days

// Sentinel errors for ReservationService constructor validation.
var (
	ErrNilReservationConn         = errors.New("reservation: database connection cannot be nil")
	ErrNilLimitResolver           = errors.New("reservation: limit resolver cannot be nil")
	ErrNilReservationRepo         = errors.New("reservation: reservation repository cannot be nil")
	ErrNilReservationAuditWriter  = errors.New("reservation: audit writer cannot be nil")
	ErrNilReservationRequest      = errors.New("reservation: request cannot be nil")
	ErrNilReservationTransationID = errors.New("reservation: transaction id is required")
)

// LimitResolver resolves the applicable limits for a transaction ONCE and computes
// the per-limit reservation parameters. Implemented by query.LimitCheckerService.
type LimitResolver interface {
	// ResolveReservations returns one ReservationSpec per counter-backed applicable
	// limit, or denied=true when a limit's ceiling is exceeded (the reserve must be
	// rejected before any capacity is held).
	ResolveReservations(ctx context.Context, input *model.CheckLimitsInput) ([]query.ReservationSpec, bool, error)
}

// ReservationRepository persists the two-phase reservation lifecycle. Every method
// takes the caller's transaction handle so the reservation-row mutation, the
// counter bucket move, and the audit write commit together. Implemented by
// postgres.UsageReservationRepository.
type ReservationRepository interface {
	ReserveWithTx(ctx context.Context, db pgdb.DB, reservation *model.Reservation, maxAmount int64) error
	ConfirmWithTx(ctx context.Context, db pgdb.DB, reservationID uuid.UUID) error
	ReleaseWithTx(ctx context.Context, db pgdb.DB, reservationID uuid.UUID, status model.ReservationStatus) error
	ConfirmByTransactionWithTx(ctx context.Context, db pgdb.DB, transactionID uuid.UUID) ([]*model.Reservation, error)
	ReleaseByTransactionWithTx(ctx context.Context, db pgdb.DB, transactionID uuid.UUID, status model.ReservationStatus) ([]*model.Reservation, error)
}

// ReservationAuditWriter records reservation lifecycle audit events inside the
// transaction that owns the counter move. Implemented by
// command.RecordAuditEventCommand.
type ReservationAuditWriter interface {
	RecordReservationEventWithTx(
		ctx context.Context,
		db pgdb.DB,
		eventType model.AuditEventType,
		action model.AuditAction,
		reservationID uuid.UUID,
		auditCtx command.ReservationAuditContext,
	) error
}

// ReserveResult is the handle returned to the caller after a reserve attempt.
// Denied is the limit-exceeded decision (the same shape the synchronous Validate
// produces on a limit breach): when true, no reservation was held and
// ReservationIDs is empty. Otherwise ReservationIDs holds one id per counter-backed
// limit that was reserved — the ledger confirms or releases each in phase two.
type ReserveResult struct {
	Denied         bool
	ReservationIDs []uuid.UUID
}

// ReservationService owns the two-phase reservation lifecycle: it resolves limits
// once and reserves capacity (phase one), then confirms or releases (phase two).
// Each method runs in its own transaction so the counter move, the reservation-row
// flip, and the audit row commit atomically.
type ReservationService struct {
	conn         pgdb.TxBeginner
	resolver     LimitResolver
	repo         ReservationRepository
	auditWriter  ReservationAuditWriter
	clock        clock.Clock
	longLivedTTL time.Duration
}

// NewReservationService constructs a ReservationService with dependency
// validation. clk may be nil — a RealClock is used. The long-lived TTL defaults
// to defaultLongLivedReservationTTL (30 days); use
// NewReservationServiceWithLongLivedTTL to override it from configuration.
func NewReservationService(
	conn pgdb.TxBeginner,
	resolver LimitResolver,
	repo ReservationRepository,
	auditWriter ReservationAuditWriter,
	clk clock.Clock,
) (*ReservationService, error) {
	return NewReservationServiceWithLongLivedTTL(conn, resolver, repo, auditWriter, clk, 0)
}

// NewReservationServiceWithLongLivedTTL is the full constructor. longLivedTTL is
// the lifetime granted to PENDING-transaction reservations (the longLived reserve
// hint, R18); a non-positive value falls back to defaultLongLivedReservationTTL.
// Direct-transaction reservations always use the short reservationTTL.
func NewReservationServiceWithLongLivedTTL(
	conn pgdb.TxBeginner,
	resolver LimitResolver,
	repo ReservationRepository,
	auditWriter ReservationAuditWriter,
	clk clock.Clock,
	longLivedTTL time.Duration,
) (*ReservationService, error) {
	if conn == nil {
		return nil, ErrNilReservationConn
	}

	if resolver == nil {
		return nil, ErrNilLimitResolver
	}

	if repo == nil {
		return nil, ErrNilReservationRepo
	}

	if auditWriter == nil {
		return nil, ErrNilReservationAuditWriter
	}

	if clk == nil {
		clk = clock.RealClock{}
	}

	if longLivedTTL <= 0 {
		longLivedTTL = defaultLongLivedReservationTTL
	}

	return &ReservationService{
		conn:         conn,
		resolver:     resolver,
		repo:         repo,
		auditWriter:  auditWriter,
		clock:        clk,
		longLivedTTL: longLivedTTL,
	}, nil
}

// Reserve resolves the applicable limits ONCE, holds capacity for each
// counter-backed limit, and returns a handle the ledger uses to confirm or release.
// This is the ALLOW-path persistence of the two-phase model.
//
// Resolution and reservation share ONE transaction so the per-limit reserves are
// all-or-nothing: if any limit's guard denies, the whole transaction rolls back and
// the result is the limit-exceeded decision (Denied=true) with no capacity held —
// exactly the decision shape the synchronous Validate path produces today.
//
// The resolved limit set (LimitID/ScopeKey/PeriodKey/Amount) is carried on each
// reservation row, so confirm/release never re-resolve limits (R38). The ledger
// transactionID is the 4-tuple idempotency key: a retried reserve collapses onto
// the existing rows rather than double-reserving (R11/R35).
//
// longLived selects the reservation lifetime: false (direct transaction) uses the
// short reservationTTL so the reaper converges quickly; true (PENDING transaction,
// R18) uses the configured long-lived TTL so a reservation backing a still-valid
// pending does not expire before the pending commits or cancels.
func (s *ReservationService) Reserve(ctx context.Context, transactionID uuid.UUID, input *model.CheckLimitsInput, longLived bool) (*ReserveResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.reservation.reserve")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if transactionID == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing transaction id", ErrNilReservationTransationID)
		return nil, ErrNilReservationTransationID
	}

	if input == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Nil reserve input", ErrNilReservationRequest)
		return nil, ErrNilReservationRequest
	}

	specs, denied, err := s.resolver.ResolveReservations(ctx, input)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve reservations", err)
		return nil, fmt.Errorf("failed to resolve reservations: %w", err)
	}

	// Denied by a PER_TRANSACTION cap or the amount-alone pre-check: no capacity
	// to hold, return the limit-exceeded decision without opening a transaction.
	if denied {
		return &ReserveResult{Denied: true}, nil
	}

	// No applicable counter-backed limits: nothing to reserve, allow.
	if len(specs) == 0 {
		return &ReserveResult{}, nil
	}

	ttl := reservationTTL
	if longLived {
		ttl = s.longLivedTTL
	}

	expiresAt := s.clock.Now().UTC().Add(ttl)
	reservationIDs := make([]uuid.UUID, 0, len(specs))

	guardDenied := false

	txErr := s.inTx(ctx, span, func(db pgdb.DB) error {
		for i := range specs {
			spec := specs[i]

			reservation, err := model.NewReservation(
				spec.LimitID,
				transactionID,
				spec.ScopeKey,
				spec.PeriodKey,
				spec.Amount,
				expiresAt,
				s.clock.Now().UTC(),
			)
			if err != nil {
				return err
			}

			if err := s.repo.ReserveWithTx(ctx, db, reservation, spec.MaxAmount); err != nil {
				// The reserve guard denied this limit: roll back the whole tx so no
				// partial capacity is held, and surface the limit-exceeded decision.
				if errors.Is(err, constant.ErrUsageCounterExceedsLimit) {
					guardDenied = true
					return err
				}

				return err
			}

			if err := s.auditWriter.RecordReservationEventWithTx(
				ctx,
				db,
				model.AuditEventReservationReserved,
				model.AuditActionReserve,
				reservation.ID,
				command.ReservationAuditContext{
					TransactionID: transactionID,
					LimitID:       spec.LimitID,
					ScopeKey:      spec.ScopeKey,
					PeriodKey:     spec.PeriodKey,
					Amount:        spec.Amount,
					Status:        string(model.StatusReserved),
				},
			); err != nil {
				return fmt.Errorf("failed to record reserve audit event: %w", err)
			}

			reservationIDs = append(reservationIDs, reservation.ID)
		}

		return nil
	})
	if txErr != nil {
		if guardDenied {
			// Limit-exceeded is a business decision, not a service failure: the
			// rollback already released any partial holds.
			return &ReserveResult{Denied: true}, nil
		}

		libOpentelemetry.HandleSpanError(span, "Failed to reserve capacity", txErr)

		return nil, txErr
	}

	logger.With(
		libLog.String("operation", "service.reservation.reserve"),
		libLog.String("transaction_id", transactionID.String()),
		libLog.Int("reservations", len(reservationIDs)),
	).Log(ctx, libLog.LevelInfo, "Reserved capacity")

	return &ReserveResult{ReservationIDs: reservationIDs}, nil
}

// Confirm commits a reservation: the held amount moves reserved_usage ->
// current_usage and the row flips to CONFIRMED, with the audit row, in one
// transaction. A confirm against an already-terminal row is an idempotent success
// (the repo's WHERE status='RESERVED' guard returns no rows; the service maps
// ErrReservationAlreadyTerminal to nil so a retried confirm does not error).
//
// Confirm does NOT re-resolve limits (R38): the reservation row already carries
// limit_id / scope_key / period_key / amount.
func (s *ReservationService) Confirm(ctx context.Context, reservationID uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.reservation.confirm")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	return s.terminate(ctx, span, logger, reservationID,
		model.StatusConfirmed,
		model.AuditEventReservationConfirmed,
		model.AuditActionConfirm,
		"service.reservation.confirm",
	)
}

// Release returns a reservation's held capacity on an aborted ledger transaction:
// reserved_usage is decremented (current_usage untouched) and the row flips to
// RELEASED, with the audit row, in one transaction. Idempotent like Confirm.
//
// Release does NOT re-resolve limits (R38).
func (s *ReservationService) Release(ctx context.Context, reservationID uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.reservation.release")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	return s.terminate(ctx, span, logger, reservationID,
		model.StatusReleased,
		model.AuditEventReservationReleased,
		model.AuditActionRelease,
		"service.reservation.release",
	)
}

// ConfirmByTransaction commits EVERY RESERVED reservation a transaction holds,
// addressing them by the ledger transaction id alone. This is the /commit-driven
// confirm: at /commit the ledger has only the transaction id (the reserve handle
// from create-pending does not survive the separate commit request), so the tracer
// resolves every RESERVED row for the transaction and confirms each — the counter
// move, the row flip, and one audit row per flip all commit in ONE transaction.
//
// A transaction with no RESERVED rows is an idempotent no-op success: it never
// reserved, or every reservation already reached a terminal state. ConfirmByTransaction
// does NOT re-resolve limits (R38) — each reservation row already carries its limit
// coordinates.
func (s *ReservationService) ConfirmByTransaction(ctx context.Context, transactionID uuid.UUID) (int, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.reservation.confirm_by_transaction")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	return s.terminateByTransaction(ctx, span, logger, transactionID,
		model.StatusConfirmed,
		model.AuditEventReservationConfirmed,
		model.AuditActionConfirm,
		"service.reservation.confirm_by_transaction",
	)
}

// ReleaseByTransaction returns the held capacity for EVERY RESERVED reservation a
// transaction holds, addressing them by the ledger transaction id alone. This is
// the /cancel-driven release: reserved_usage is decremented (current_usage
// untouched) and each row flips to RELEASED, with one audit row per flip, in ONE
// transaction. Idempotent over "nothing to do" like ConfirmByTransaction; does NOT
// re-resolve limits (R38).
func (s *ReservationService) ReleaseByTransaction(ctx context.Context, transactionID uuid.UUID) (int, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.reservation.release_by_transaction")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	return s.terminateByTransaction(ctx, span, logger, transactionID,
		model.StatusReleased,
		model.AuditEventReservationReleased,
		model.AuditActionRelease,
		"service.reservation.release_by_transaction",
	)
}

// terminateByTransaction is the shared confirm/release-by-transaction body: open a
// tx, flip every RESERVED row the transaction holds via the repo, record one audit
// row per flipped reservation in the same tx, then commit. Returns the flipped
// count; zero rows commits cleanly and reports a no-op success (the by-transaction
// transitions are idempotent over an absent or already-terminal transaction).
func (s *ReservationService) terminateByTransaction(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	transactionID uuid.UUID,
	terminalStatus model.ReservationStatus,
	eventType model.AuditEventType,
	action model.AuditAction,
	operation string,
) (int, error) {
	if transactionID == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing transaction id", ErrNilReservationTransationID)
		return 0, ErrNilReservationTransationID
	}

	flipped := 0

	txErr := s.inTx(ctx, span, func(db pgdb.DB) error {
		var (
			reservations []*model.Reservation
			repoErr      error
		)

		if terminalStatus == model.StatusConfirmed {
			reservations, repoErr = s.repo.ConfirmByTransactionWithTx(ctx, db, transactionID)
		} else {
			reservations, repoErr = s.repo.ReleaseByTransactionWithTx(ctx, db, transactionID, terminalStatus)
		}

		if repoErr != nil {
			return repoErr
		}

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
					Status:        string(terminalStatus),
				},
			); err != nil {
				return fmt.Errorf("failed to record %s audit event: %w", string(action), err)
			}
		}

		flipped = len(reservations)

		return nil
	})
	if txErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to terminate reservations by transaction", txErr)
		return 0, txErr
	}

	logger.With(
		libLog.String("operation", operation),
		libLog.String("transaction_id", transactionID.String()),
		libLog.String("status", string(terminalStatus)),
		libLog.Int("flipped", flipped),
	).Log(ctx, libLog.LevelInfo, "Reservations terminated by transaction")

	return flipped, nil
}

// terminate is the shared confirm/release transaction body: open a tx, apply the
// counter move + row flip via the repo, record the audit row in the same tx, then
// commit. An already-terminal reservation is mapped to success (idempotent retry).
func (s *ReservationService) terminate(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	reservationID uuid.UUID,
	terminalStatus model.ReservationStatus,
	eventType model.AuditEventType,
	action model.AuditAction,
	operation string,
) error {
	if reservationID == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing reservation id", constant.ErrReservationNotFound)
		return constant.ErrReservationNotFound
	}

	terminal := false

	txErr := s.inTx(ctx, span, func(db pgdb.DB) error {
		var repoErr error

		if terminalStatus == model.StatusConfirmed {
			repoErr = s.repo.ConfirmWithTx(ctx, db, reservationID)
		} else {
			repoErr = s.repo.ReleaseWithTx(ctx, db, reservationID, terminalStatus)
		}

		if repoErr != nil {
			// Already terminal: idempotent retry. Commit nothing further and treat
			// as success — the original transition already moved the counter.
			if errors.Is(repoErr, constant.ErrReservationAlreadyTerminal) {
				terminal = true
				return repoErr
			}

			return repoErr
		}

		if err := s.auditWriter.RecordReservationEventWithTx(
			ctx,
			db,
			eventType,
			action,
			reservationID,
			command.ReservationAuditContext{
				Status: string(terminalStatus),
			},
		); err != nil {
			return fmt.Errorf("failed to record %s audit event: %w", string(action), err)
		}

		return nil
	})
	if txErr != nil {
		if terminal {
			logger.With(
				libLog.String("operation", operation),
				libLog.String("reservation_id", reservationID.String()),
			).Log(ctx, libLog.LevelInfo, "Reservation already terminal — idempotent no-op")

			return nil
		}

		libOpentelemetry.HandleSpanError(span, "Failed to terminate reservation", txErr)

		return txErr
	}

	logger.With(
		libLog.String("operation", operation),
		libLog.String("reservation_id", reservationID.String()),
		libLog.String("status", string(terminalStatus)),
	).Log(ctx, libLog.LevelInfo, "Reservation terminated")

	return nil
}

// inTx runs fn inside a transaction owned by the service. Commits on success,
// rolls back on error or panic. Mirrors ValidationService's transaction handling
// and the command package's executeInTx so the reservation lifecycle keeps the
// same atomicity and rollback-logging discipline.
func (s *ReservationService) inTx(ctx context.Context, span trace.Span, fn func(pgdb.DB) error) (err error) {
	tx, beginErr := s.conn.BeginTx(ctx, nil)
	if beginErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to begin transaction", beginErr)
		return fmt.Errorf("failed to begin reservation transaction: %w", beginErr)
	}

	if tx == nil {
		return errors.New("reservation_service: BeginTx returned nil transaction without error")
	}

	committed := false

	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback()

			if recoveredErr, ok := recovered.(error); ok {
				err = fmt.Errorf("reservation transaction callback panicked: %w", recoveredErr)
			} else {
				err = fmt.Errorf("reservation transaction callback panicked: %v", recovered)
			}

			return
		}

		if committed {
			return
		}

		if rbErr := tx.Rollback(); rbErr != nil {
			logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
			logger = logging.WithTrace(ctx, logger)
			logger.With(
				libLog.String("operation", "service.reservation.rollback"),
				libLog.String("error.message", rbErr.Error()),
			).Log(ctx, libLog.LevelWarn, "Failed to rollback reservation transaction")
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}

	if commitErr := tx.Commit(); commitErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to commit transaction", commitErr)
		return fmt.Errorf("failed to commit reservation transaction: %w", commitErr)
	}

	committed = true

	return nil
}
