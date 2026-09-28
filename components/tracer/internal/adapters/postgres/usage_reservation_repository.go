// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	sq "github.com/Masterminds/squirrel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// usageReservationsTable is the PostgreSQL table name for usage reservations.
// Using a constant prevents SQL injection via table name interpolation.
const usageReservationsTable = "usage_reservations"

// reserveScopeLockSQL takes a transaction-scoped advisory lock. pg_advisory_xact_lock
// blocks until the key is free and releases it automatically at commit or rollback,
// so no explicit unlock is needed and a crashed transaction never leaks the lock.
const reserveScopeLockSQL = `SELECT pg_advisory_xact_lock($1)`

// reserveLockTimeout bounds how long the reserve transaction may wait on the
// per-account advisory lock. Without it a reserve blocked by same-account contention
// parks its pooled connection until the lock frees, so sustained contention on one
// hot account can drain the per-tenant pool (default 25) into an indefinite queue.
// When the wait exceeds this budget PostgreSQL aborts the statement with SQLSTATE
// 55P03 (lock_not_available); the service rolls the transaction back, releasing the
// connection, and the lock timeout surfaces as an error to the producer. Nothing on
// this path retries it. 3s sits well above the
// time a legitimate same-account reserve queue drains (each reserve touches a handful
// of rows in single-digit milliseconds) yet far below the point a stuck waiter would
// starve the pool.
const reserveLockTimeout = 3 * time.Second

// reserveLockTimeoutSQL bounds the reserve transaction's lock wait. set_config with
// is_local=true is the SET LOCAL equivalent that accepts a bind parameter, so the
// timeout applies only within the current transaction and reverts at commit/rollback,
// and no value is interpolated into the statement text.
const reserveLockTimeoutSQL = `SELECT set_config('lock_timeout', $1, true)`

// UsageReservationRepository keeps decision-owned reservations atomic with their
// usage_counters bucket moves. Every method takes the caller's transaction
// handle, so the reservation-row mutation, the counter bucket move and the
// caller's audit write commit together in ONE transaction owned by the service.
// A partial apply would leave a counter that disagrees with its reservations, so
// the counter move and the row flip MUST share the transaction.
//
// counterRepo owns the reserve CTE (the critical over-limit guard); settlement
// runs direct counter UPDATEs on the supplied handle. Tenant resolution is
// handled by the connection the caller used to open the transaction.
type UsageReservationRepository struct {
	counterRepo *UsageCounterRepository
}

// NewUsageReservationRepositoryWithConnection creates a usage reservation
// repository. counterRepo supplies the reserve CTE so the reserve guard and the
// row insert run on the same transaction handle.
func NewUsageReservationRepositoryWithConnection(counterRepo *UsageCounterRepository) *UsageReservationRepository {
	return &UsageReservationRepository{counterRepo: counterRepo}
}

// AcquireReserveScopeLock sets a transaction-scoped lock_timeout FIRST, then takes
// the advisory lock, so a wait that exceeds reserveLockTimeout surfaces as SQLSTATE
// 55P03, returned as an error to the producer without a retry, rather than parking
// the pooled connection indefinitely under same-account contention.
func (r *UsageReservationRepository) AcquireReserveScopeLock(ctx context.Context, db pgdb.DB, key int64) error {
	if db == nil {
		return pgdb.ErrNilConnection
	}

	//nolint:dogsled // NewTrackingFromContext returns four values; this lock-only path needs just the tracer.
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.usage_reservation.acquire_scope_lock")
	defer span.End()

	span.SetAttributes(attribute.Int64("app.request.reserve_scope_lock_key", key))

	if _, err := db.ExecContext(ctx, reserveLockTimeoutSQL, reserveLockTimeout.String()); err != nil {
		libOtel.HandleSpanError(span, "Failed to set reserve lock_timeout", err)
		return fmt.Errorf("failed to set reserve lock_timeout: %w", err)
	}

	if _, err := db.ExecContext(ctx, reserveScopeLockSQL, key); err != nil {
		libOtel.HandleSpanError(span, "Failed to acquire reserve scope advisory lock", err)
		return fmt.Errorf("failed to acquire reserve scope advisory lock: %w", err)
	}

	return nil
}

// applyConfirm settles one RESERVED reservation onto the counter, moving its
// amount out of reserved_usage and into current_usage, and flips the row to
// CONFIRMED, on the supplied handle. The row flip is guarded on RESERVED, so a
// concurrent transition loses cleanly and surfaces as
// ErrReservationAlreadyTerminal.
func (r *UsageReservationRepository) applyConfirm(ctx context.Context, span trace.Span, db pgdb.DB, res *model.Reservation) error {
	now := time.Now().UTC()

	counterUpdate := sq.Update(usageCountersTable).
		Set("current_usage", sq.Expr("current_usage + ?", res.Amount)).
		Set("last_updated_at", now).
		Set("reserved_usage", sq.Expr("reserved_usage - ?", res.Amount)).
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

	affected, err := r.execRowFlip(ctx, span, db, rowUpdate)
	if err != nil {
		return err
	}

	if affected == 0 {
		return constant.ErrReservationAlreadyTerminal
	}

	return nil
}

// applyRelease returns a single RESERVED reservation's amount from reserved_usage
// (current_usage untouched) and flips the row to the given terminal status, on the
// supplied handle.
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
