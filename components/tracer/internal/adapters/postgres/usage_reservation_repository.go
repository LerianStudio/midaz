// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// usageReservationsTable is the PostgreSQL table name for usage reservations.
// Using a constant prevents SQL injection via table name interpolation.
const usageReservationsTable = "usage_reservations"

// UsageReservationRepository implements the two-phase reservation lifecycle over
// usage_reservations, keeping each transition atomic with the matching
// usage_counters bucket move. Every method takes the caller's db handle (a *sql.Tx
// via the pgdb.Tx adapter), so the reservation-row mutation, the counter bucket
// move, AND the caller's audit write all commit in ONE transaction owned by the
// service (mirroring the RuleRepository/LimitRepository *WithTx pattern).
//
//   - ReserveWithTx: seeds usage_counters.reserved_usage via the reserve CTE
//     (guarded on current_usage + reserved_usage + amount <= maxAmount) AND inserts
//     the reservation row (idempotent on the 4-tuple).
//   - ConfirmWithTx: moves the amount reserved_usage -> current_usage AND flips the
//     row to CONFIRMED, guarded WHERE status='RESERVED'.
//   - ReleaseWithTx: returns the amount from reserved_usage AND flips the row to
//     RELEASED/EXPIRED, same guard.
//
// A partial apply is exactly the divergence the TTL reaper would otherwise have to
// reconcile, so the counter move and the row flip MUST share the transaction.
//
// counterRepo owns the reserve CTE (the critical over-limit guard); confirm/release
// run direct counter UPDATEs on the supplied handle. Tenant resolution is handled
// by the connection the caller used to open the transaction (M1).
type UsageReservationRepository struct {
	counterRepo *UsageCounterRepository
}

// NewUsageReservationRepositoryWithConnection creates a usage reservation
// repository. counterRepo supplies the reserve CTE so the reserve guard and the row
// insert run on the same transaction handle.
func NewUsageReservationRepositoryWithConnection(counterRepo *UsageCounterRepository) *UsageReservationRepository {
	return &UsageReservationRepository{counterRepo: counterRepo}
}

// ReserveWithTx seeds the counter's reserved_usage via the reserve CTE and inserts
// the reservation row idempotently on the (transaction_id, limit_id, scope_key,
// period_key) tuple, both on the supplied transaction handle.
//
// maxAmount is the limit ceiling the reserve CTE guards against; it is supplied by
// the caller (the limit it resolved) and is NOT stored on the reservation row.
//
// Returns constant.ErrUsageCounterExceedsLimit when the combined committed +
// outstanding usage would exceed the limit (the guard denied the reservation). The
// caller is responsible for rolling the transaction back on any error so a denied
// reserve leaves no RESERVED row whose capacity was never held. A retried reserve
// for the same 4-tuple collapses onto the existing row (ON CONFLICT DO NOTHING).
func (r *UsageReservationRepository) ReserveWithTx(ctx context.Context, db pgdb.DB, reservation *model.Reservation, maxAmount int64) error {
	if db == nil {
		return pgdb.ErrNilConnection
	}

	if reservation == nil {
		return errors.New("reservation cannot be nil")
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.usage_reservation.reserve")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if err := reservation.Validate(); err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid reservation", err)
		return err
	}

	// Reserve capacity on the counter (the over-limit guard lives in the CTE). On
	// guard failure this returns ErrUsageCounterExceedsLimit; the caller rolls back
	// so the row insert below never persists.
	if _, err := r.counterRepo.UpsertAndReserveAtomic(
		ctx,
		db,
		reservation.LimitID,
		reservation.ScopeKey,
		reservation.PeriodKey,
		decimal.NewFromInt(reservation.Amount),
		decimal.NewFromInt(maxAmount),
		&reservation.ReservationExpiresAt,
	); err != nil {
		return err
	}

	insertSQL := `
		INSERT INTO usage_reservations (
			id, limit_id, scope_key, period_key, amount, status,
			transaction_id, reservation_expires_at, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (transaction_id, limit_id, scope_key, period_key) DO NOTHING
	`

	if _, err := db.ExecContext(
		ctx,
		insertSQL,
		reservation.ID,
		reservation.LimitID,
		reservation.ScopeKey,
		reservation.PeriodKey,
		reservation.Amount,
		string(reservation.Status),
		reservation.TransactionID,
		reservation.ReservationExpiresAt,
		reservation.CreatedAt,
	); err != nil {
		libOtel.HandleSpanError(span, "Failed to insert reservation row", err)
		return fmt.Errorf("failed to insert reservation row: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.usage_reservation.reserve"),
		libLog.String("reservation_id", reservation.ID.String()),
		libLog.String("limit_id", reservation.LimitID.String()),
	).Log(ctx, libLog.LevelDebug, "Reserved usage")

	return nil
}

// ConfirmWithTx moves a RESERVED reservation's amount from reserved_usage into
// current_usage on the counter and flips the row to CONFIRMED, on the supplied
// handle, guarded WHERE status='RESERVED'. A retried confirm against an
// already-terminal row is a no-op: the row read sees a terminal status and the
// counter move is NEVER issued, so the method returns ErrReservationAlreadyTerminal
// without a double-move. A missing reservation maps to ErrReservationNotFound.
func (r *UsageReservationRepository) ConfirmWithTx(ctx context.Context, db pgdb.DB, reservationID uuid.UUID) error {
	if db == nil {
		return pgdb.ErrNilConnection
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.usage_reservation.confirm")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	res, err := r.lockReservation(ctx, db, reservationID)
	if err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Reservation lookup failed", err)
		return err
	}

	if res.Status != model.StatusReserved {
		return constant.ErrReservationAlreadyTerminal
	}

	now := time.Now().UTC()

	counterUpdate := sq.Update(usageCountersTable).
		Set("current_usage", sq.Expr("current_usage + ?", res.Amount)).
		Set("reserved_usage", sq.Expr("reserved_usage - ?", res.Amount)).
		Set("last_updated_at", now).
		Where(sq.Eq{
			"limit_id":   res.LimitID,
			"scope_key":  res.ScopeKey,
			"period_key": res.PeriodKey,
		}).
		PlaceholderFormat(sq.Dollar)

	if err := r.execCounterMove(ctx, span, db, counterUpdate); err != nil {
		return err
	}

	rowUpdate := sq.Update(usageReservationsTable).
		Set("status", string(model.StatusConfirmed)).
		Set("confirmed_at", now).
		Where(sq.Eq{"id": reservationID, "status": string(model.StatusReserved)}).
		PlaceholderFormat(sq.Dollar)

	affected, err := r.execRowFlip(ctx, span, db, rowUpdate)
	if err != nil {
		return err
	}

	if affected == 0 {
		return constant.ErrReservationAlreadyTerminal
	}

	logger.With(
		libLog.String("operation", "repository.usage_reservation.confirm"),
		libLog.String("reservation_id", reservationID.String()),
	).Log(ctx, libLog.LevelDebug, "Confirmed reservation")

	return nil
}

// ReleaseWithTx returns a RESERVED reservation's amount from reserved_usage on the
// counter (without crediting current_usage) and flips the row to the given terminal
// status, on the supplied handle, guarded WHERE status='RESERVED'. status MUST be
// StatusReleased (explicit abort) or StatusExpired (reaper sweep). Idempotency
// mirrors ConfirmWithTx.
func (r *UsageReservationRepository) ReleaseWithTx(ctx context.Context, db pgdb.DB, reservationID uuid.UUID, status model.ReservationStatus) error {
	if db == nil {
		return pgdb.ErrNilConnection
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.usage_reservation.release")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if status != model.StatusReleased && status != model.StatusExpired {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid release status", constant.ErrReservationInvalidStatus)
		return constant.ErrReservationInvalidStatus
	}

	res, err := r.lockReservation(ctx, db, reservationID)
	if err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Reservation lookup failed", err)
		return err
	}

	if res.Status != model.StatusReserved {
		return constant.ErrReservationAlreadyTerminal
	}

	now := time.Now().UTC()

	counterUpdate := sq.Update(usageCountersTable).
		Set("reserved_usage", sq.Expr("reserved_usage - ?", res.Amount)).
		Set("last_updated_at", now).
		Where(sq.Eq{
			"limit_id":   res.LimitID,
			"scope_key":  res.ScopeKey,
			"period_key": res.PeriodKey,
		}).
		PlaceholderFormat(sq.Dollar)

	if err := r.execCounterMove(ctx, span, db, counterUpdate); err != nil {
		return err
	}

	rowUpdate := sq.Update(usageReservationsTable).
		Set("status", string(status)).
		Set("released_at", now).
		Where(sq.Eq{"id": reservationID, "status": string(model.StatusReserved)}).
		PlaceholderFormat(sq.Dollar)

	affected, err := r.execRowFlip(ctx, span, db, rowUpdate)
	if err != nil {
		return err
	}

	if affected == 0 {
		return constant.ErrReservationAlreadyTerminal
	}

	logger.With(
		libLog.String("operation", "repository.usage_reservation.release"),
		libLog.String("reservation_id", reservationID.String()),
		libLog.String("status", string(status)),
	).Log(ctx, libLog.LevelDebug, "Released reservation")

	return nil
}

// ConfirmByTransactionWithTx confirms EVERY RESERVED reservation row that carries
// the given transaction_id, on the supplied handle, in one transaction owned by
// the caller. For each row it applies the same counter move (reserved_usage ->
// current_usage) and row flip (-> CONFIRMED) the by-id ConfirmWithTx performs. The
// 4-tuple unique index leads with transaction_id, so the lookup is index-efficient.
//
// Returns the flipped reservations (their ids and resolved limit coordinates) so
// the caller can record one audit row per flip in the SAME transaction. The flipped
// count is len(result). Zero rows is an idempotent no-op success: either the
// transaction never reserved (tracer disabled / no counter-backed limit applied) or
// every reservation already reached a terminal state (a retried confirm). The
// caller maps an empty result to a success either way — this is the by-transaction
// confirm the ledger /commit drives with only the transaction id.
func (r *UsageReservationRepository) ConfirmByTransactionWithTx(ctx context.Context, db pgdb.DB, transactionID uuid.UUID) ([]*model.Reservation, error) {
	if db == nil {
		return nil, pgdb.ErrNilConnection
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.usage_reservation.confirm_by_transaction")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	reservations, err := r.lockReservedByTransaction(ctx, db, transactionID)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to load reserved rows for transaction", err)
		return nil, err
	}

	for i := range reservations {
		if err := r.applyConfirm(ctx, span, db, reservations[i]); err != nil {
			return nil, err
		}
	}

	span.SetAttributes(attribute.Int("db.rows_flipped", len(reservations)))

	logger.With(
		libLog.String("operation", "repository.usage_reservation.confirm_by_transaction"),
		libLog.String("transaction_id", transactionID.String()),
		libLog.Int("flipped", len(reservations)),
	).Log(ctx, libLog.LevelDebug, "Confirmed reservations by transaction")

	return reservations, nil
}

// ReleaseByTransactionWithTx releases EVERY RESERVED reservation row that carries
// the given transaction_id, on the supplied handle, in one transaction owned by the
// caller. For each row it returns the held amount to capacity (reserved_usage
// decremented, current_usage untouched) and flips the row to the given terminal
// status, mirroring the by-id ReleaseWithTx. status MUST be StatusReleased (an
// explicit abort) or StatusExpired (a reaper sweep).
//
// Returns the flipped reservations so the caller can record one audit row per flip
// in the same transaction; the flipped count is len(result). Zero rows is an
// idempotent no-op success, as in ConfirmByTransactionWithTx. This is the
// by-transaction release the ledger /cancel drives with only the transaction id.
func (r *UsageReservationRepository) ReleaseByTransactionWithTx(ctx context.Context, db pgdb.DB, transactionID uuid.UUID, status model.ReservationStatus) ([]*model.Reservation, error) {
	if db == nil {
		return nil, pgdb.ErrNilConnection
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.usage_reservation.release_by_transaction")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if status != model.StatusReleased && status != model.StatusExpired {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid release status", constant.ErrReservationInvalidStatus)
		return nil, constant.ErrReservationInvalidStatus
	}

	reservations, err := r.lockReservedByTransaction(ctx, db, transactionID)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to load reserved rows for transaction", err)
		return nil, err
	}

	for i := range reservations {
		if err := r.applyRelease(ctx, span, db, reservations[i], status); err != nil {
			return nil, err
		}
	}

	span.SetAttributes(attribute.Int("db.rows_flipped", len(reservations)))

	logger.With(
		libLog.String("operation", "repository.usage_reservation.release_by_transaction"),
		libLog.String("transaction_id", transactionID.String()),
		libLog.String("status", string(status)),
		libLog.Int("flipped", len(reservations)),
	).Log(ctx, libLog.LevelDebug, "Released reservations by transaction")

	return reservations, nil
}

// applyConfirm moves a single RESERVED reservation's amount from reserved_usage to
// current_usage and flips the row to CONFIRMED, on the supplied handle. It is the
// shared per-row body of ConfirmByTransactionWithTx; the row is already locked and
// known RESERVED by the by-transaction selector, so the WHERE status='RESERVED'
// guard on the flip stays as a belt-and-braces against a concurrent transition.
func (r *UsageReservationRepository) applyConfirm(ctx context.Context, span trace.Span, db pgdb.DB, res *model.Reservation) error {
	now := time.Now().UTC()

	counterUpdate := sq.Update(usageCountersTable).
		Set("current_usage", sq.Expr("current_usage + ?", res.Amount)).
		Set("reserved_usage", sq.Expr("reserved_usage - ?", res.Amount)).
		Set("last_updated_at", now).
		Where(sq.Eq{
			"limit_id":   res.LimitID,
			"scope_key":  res.ScopeKey,
			"period_key": res.PeriodKey,
		}).
		PlaceholderFormat(sq.Dollar)

	if err := r.execCounterMove(ctx, span, db, counterUpdate); err != nil {
		return err
	}

	rowUpdate := sq.Update(usageReservationsTable).
		Set("status", string(model.StatusConfirmed)).
		Set("confirmed_at", now).
		Where(sq.Eq{"id": res.ID, "status": string(model.StatusReserved)}).
		PlaceholderFormat(sq.Dollar)

	if _, err := r.execRowFlip(ctx, span, db, rowUpdate); err != nil {
		return err
	}

	return nil
}

// applyRelease returns a single RESERVED reservation's amount from reserved_usage
// (current_usage untouched) and flips the row to the given terminal status, on the
// supplied handle. It is the shared per-row body of ReleaseByTransactionWithTx.
func (r *UsageReservationRepository) applyRelease(ctx context.Context, span trace.Span, db pgdb.DB, res *model.Reservation, status model.ReservationStatus) error {
	now := time.Now().UTC()

	counterUpdate := sq.Update(usageCountersTable).
		Set("reserved_usage", sq.Expr("reserved_usage - ?", res.Amount)).
		Set("last_updated_at", now).
		Where(sq.Eq{
			"limit_id":   res.LimitID,
			"scope_key":  res.ScopeKey,
			"period_key": res.PeriodKey,
		}).
		PlaceholderFormat(sq.Dollar)

	if err := r.execCounterMove(ctx, span, db, counterUpdate); err != nil {
		return err
	}

	rowUpdate := sq.Update(usageReservationsTable).
		Set("status", string(status)).
		Set("released_at", now).
		Where(sq.Eq{"id": res.ID, "status": string(model.StatusReserved)}).
		PlaceholderFormat(sq.Dollar)

	if _, err := r.execRowFlip(ctx, span, db, rowUpdate); err != nil {
		return err
	}

	return nil
}

// lockReservedByTransaction reads every RESERVED reservation row for a transaction
// FOR UPDATE so the per-row counter moves and flips see a stable status under a
// concurrent by-id confirm/release or the reaper. The lookup rides the 4-tuple
// unique index (transaction_id leads). A transaction with no RESERVED rows returns
// an empty slice, NOT an error — the by-transaction confirm/release is idempotent
// over "nothing to do".
func (r *UsageReservationRepository) lockReservedByTransaction(ctx context.Context, db pgdb.DB, transactionID uuid.UUID) ([]*model.Reservation, error) {
	const selectSQL = `
		SELECT id, limit_id, scope_key, period_key, amount, status,
		       transaction_id, reservation_expires_at, created_at, confirmed_at, released_at
		FROM usage_reservations
		WHERE transaction_id = $1 AND status = 'RESERVED'
		FOR UPDATE
	`

	rows, err := db.QueryContext(ctx, selectSQL, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load reserved rows for transaction: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var reservations []*model.Reservation

	for rows.Next() {
		var (
			res         model.Reservation
			status      string
			confirmedAt sql.NullTime
			releasedAt  sql.NullTime
		)

		if err := rows.Scan(
			&res.ID,
			&res.LimitID,
			&res.ScopeKey,
			&res.PeriodKey,
			&res.Amount,
			&status,
			&res.TransactionID,
			&res.ReservationExpiresAt,
			&res.CreatedAt,
			&confirmedAt,
			&releasedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan reserved row: %w", err)
		}

		res.Status = model.ReservationStatus(status)

		if confirmedAt.Valid {
			t := confirmedAt.Time
			res.ConfirmedAt = &t
		}

		if releasedAt.Valid {
			t := releasedAt.Time
			res.ReleasedAt = &t
		}

		reservations = append(reservations, &res)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate reserved rows: %w", err)
	}

	return reservations, nil
}

// lockReservation reads the reservation row FOR UPDATE so the counter move and the
// row flip see a stable status under concurrent confirm/release. Maps a missing row
// to ErrReservationNotFound.
func (r *UsageReservationRepository) lockReservation(ctx context.Context, db pgdb.DB, reservationID uuid.UUID) (*model.Reservation, error) {
	selectSQL := `
		SELECT id, limit_id, scope_key, period_key, amount, status,
		       transaction_id, reservation_expires_at, created_at, confirmed_at, released_at
		FROM usage_reservations
		WHERE id = $1
		FOR UPDATE
	`

	var (
		res         model.Reservation
		status      string
		confirmedAt sql.NullTime
		releasedAt  sql.NullTime
	)

	err := db.QueryRowContext(ctx, selectSQL, reservationID).Scan(
		&res.ID,
		&res.LimitID,
		&res.ScopeKey,
		&res.PeriodKey,
		&res.Amount,
		&status,
		&res.TransactionID,
		&res.ReservationExpiresAt,
		&res.CreatedAt,
		&confirmedAt,
		&releasedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, constant.ErrReservationNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load reservation: %w", err)
	}

	res.Status = model.ReservationStatus(status)

	if confirmedAt.Valid {
		t := confirmedAt.Time
		res.ConfirmedAt = &t
	}

	if releasedAt.Valid {
		t := releasedAt.Time
		res.ReleasedAt = &t
	}

	return &res, nil
}

// execCounterMove runs the counter UPDATE and maps zero rows affected to the
// usage-counter-not-found sentinel: the reservation row exists but its counter
// bucket does not, which is a data-integrity fault rather than an idempotent retry.
func (r *UsageReservationRepository) execCounterMove(ctx context.Context, span trace.Span, db pgdb.DB, qb sq.UpdateBuilder) error {
	sqlStr, args, err := qb.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build counter update", err)
		return fmt.Errorf("failed to build counter update: %w", err)
	}

	result, err := db.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to move counter", err)
		return fmt.Errorf("failed to move counter: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to read counter rows affected", err)
		return fmt.Errorf("failed to read counter rows affected: %w", err)
	}

	if affected == 0 {
		libOtel.HandleSpanBusinessErrorEvent(span, "Counter bucket not found for reservation", constant.ErrUsageCounterNotFound)
		return constant.ErrUsageCounterNotFound
	}

	span.SetAttributes(attribute.Int64("db.rows_affected", affected))

	return nil
}

// execRowFlip runs the reservation-row UPDATE and returns RowsAffected so the
// caller can distinguish a successful flip (1) from a lost guard race / terminal
// row (0).
func (r *UsageReservationRepository) execRowFlip(ctx context.Context, span trace.Span, db pgdb.DB, qb sq.UpdateBuilder) (int64, error) {
	sqlStr, args, err := qb.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build reservation update", err)
		return 0, fmt.Errorf("failed to build reservation update: %w", err)
	}

	result, err := db.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to flip reservation status", err)
		return 0, fmt.Errorf("failed to flip reservation status: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to read reservation rows affected", err)
		return 0, fmt.Errorf("failed to read reservation rows affected: %w", err)
	}

	return affected, nil
}
