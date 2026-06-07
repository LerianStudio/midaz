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
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// DefaultDeleteBatchSize is the number of rows to delete per iteration when cleaning up expired counters.
// This prevents long-running locks on large tables by breaking the delete into smaller batches.
const DefaultDeleteBatchSize = 1000

// usageCountersTable is the PostgreSQL table name for usage counters.
// Using a constant prevents SQL injection via table name interpolation.
const usageCountersTable = "usage_counters"

// upsertAndIncrementCTEQuery is the CTE query for atomic upsert+increment operations.
// This query always returns (current_usage, succeeded) in a single round-trip.
//
// Strategy:
// 1. CTE 'attempt' tries INSERT ... ON CONFLICT DO UPDATE with WHERE guard
// 2. If succeeds: returns (new current_usage, true)
// 3. If WHERE guard fails: CTE returns 0 rows
// 4. Outer query uses COALESCE: if CTE empty, fallback to SELECT + false flag
//
// Parameters: $1=counterID, $2=limitID, $3=scopeKey, $4=periodKey, $5=amount (INSERT),
//
//	$6=now (INSERT last_updated_at), $7=amount (UPDATE), $8=now (UPDATE last_updated_at),
//	$9=amount (WHERE check), $10=maxAmount, $11=expiresAt
const upsertAndIncrementCTEQuery = `
	WITH attempt AS (
		INSERT INTO usage_counters (id, limit_id, scope_key, period_key, current_usage, last_updated_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $11)
		ON CONFLICT (limit_id, scope_key, period_key) 
		DO UPDATE SET 
			current_usage = usage_counters.current_usage + $7,
			last_updated_at = $8,
			expires_at = $11
		WHERE usage_counters.current_usage + $9 <= $10
		RETURNING current_usage, true as succeeded
	)
	SELECT 
		COALESCE(
			(SELECT current_usage FROM attempt),
			(SELECT current_usage FROM usage_counters 
			 WHERE limit_id = $2 AND scope_key = $3 AND period_key = $4),
			$5
		) as current_usage,
		COALESCE(
			(SELECT succeeded FROM attempt),
			false
		) as succeeded
`

// upsertAndReserveCTEQuery is the CTE query for the two-phase reserve path.
// It mirrors upsertAndIncrementCTEQuery but moves the amount into the
// reserved_usage bucket (not current_usage) and guards against the COMBINED
// committed + outstanding usage so concurrent reservations cannot over-commit.
//
// Strategy (identical control flow to the increment CTE):
// 1. CTE 'attempt' tries INSERT ... ON CONFLICT DO UPDATE with the WHERE guard
// 2. If succeeds: returns (new reserved_usage, true)
// 3. If WHERE guard fails: CTE returns 0 rows
// 4. Outer query uses COALESCE: if CTE empty, fallback to SELECT + false flag
//
// THE critical correctness line is the WHERE guard:
//
//	current_usage + reserved_usage + $9 <= $10
//
// Accounting for reserved_usage is what prevents the TOCTOU over-limit bug: two
// reservers each see capacity individually but their reservations sum past the
// limit. The INSERT branch seeds reserved_usage = amount and current_usage = 0.
//
// Parameters: $1=counterID, $2=limitID, $3=scopeKey, $4=periodKey, $5=amount (INSERT reserved_usage),
//
//	$6=now (INSERT last_updated_at), $7=amount (UPDATE reserved_usage increment), $8=now (UPDATE last_updated_at),
//	$9=amount (WHERE check), $10=maxAmount, $11=expiresAt
const upsertAndReserveCTEQuery = `
	WITH attempt AS (
		INSERT INTO usage_counters (id, limit_id, scope_key, period_key, current_usage, reserved_usage, last_updated_at, expires_at)
		VALUES ($1, $2, $3, $4, 0, $5, $6, $11)
		ON CONFLICT (limit_id, scope_key, period_key)
		DO UPDATE SET
			reserved_usage = usage_counters.reserved_usage + $7,
			last_updated_at = $8,
			expires_at = $11
		WHERE usage_counters.current_usage + usage_counters.reserved_usage + $9 <= $10
		RETURNING reserved_usage, true as succeeded
	)
	SELECT
		COALESCE(
			(SELECT reserved_usage FROM attempt),
			(SELECT reserved_usage FROM usage_counters
			 WHERE limit_id = $2 AND scope_key = $3 AND period_key = $4),
			$5
		) as reserved_usage,
		COALESCE(
			(SELECT succeeded FROM attempt),
			false
		) as succeeded
`

// UsageCounterRepository implements query.UsageCounterRepository using PostgreSQL.
// Provides atomic usage counter operations with row-level locking (SELECT FOR UPDATE).
// Tenant resolution is handled by the underlying pgdb.Connection (M1).
type UsageCounterRepository struct {
	conn            pgdb.Connection
	deleteBatchSize int
}

// NewUsageCounterRepositoryWithConnection creates a new PostgreSQL usage counter repository with a custom pgdb.Connection.
// This is primarily used for testing with mock connections.
func NewUsageCounterRepositoryWithConnection(conn pgdb.Connection) *UsageCounterRepository {
	return &UsageCounterRepository{
		conn:            conn,
		deleteBatchSize: DefaultDeleteBatchSize,
	}
}

// GetOrCreateForUpdate retrieves or creates a usage counter with row-level lock.
// Uses SELECT FOR UPDATE to prevent race conditions during concurrent increments.
// If the counter doesn't exist, it creates one with currentUsage=0.
func (r *UsageCounterRepository) GetOrCreateForUpdate(ctx context.Context, limitID uuid.UUID, scopeKey, periodKey string) (*model.UsageCounter, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.usage_counter.get_or_create_for_update")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get database connection", err)
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.usage_counter.get_or_create_for_update"),
		libLog.String("limit_id", limitID.String()),
		libLog.String("scope_key", scopeKey),
		libLog.String("period_key", periodKey),
	).Log(ctx, libLog.LevelInfo, "Getting or creating usage counter with lock")

	// Try to get existing counter with FOR UPDATE lock
	selectQuery := sq.Select("id", "limit_id", "scope_key", "period_key", "current_usage", "last_updated_at").
		From(usageCountersTable).
		Where(sq.Eq{
			"limit_id":   limitID,
			"scope_key":  scopeKey,
			"period_key": periodKey,
		}).
		Suffix("FOR UPDATE").
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := selectQuery.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build select query", err)
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	counter, err := r.scanCounter(ctx, db.QueryRowContext(ctx, sqlStr, args...))
	if err == nil {
		logger.With(
			libLog.String("operation", "repository.usage_counter.get_or_create_for_update"),
			libLog.String("counter_id", counter.ID.String()),
			libLog.Any("current_usage", counter.CurrentUsage),
		).Log(ctx, libLog.LevelInfo, "Found existing usage counter")

		return counter, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		libOtel.HandleSpanError(span, "Failed to get usage counter", err)
		return nil, fmt.Errorf("failed to get usage counter: %w", err)
	}

	// Counter doesn't exist, create new one
	newCounter, err := model.NewUsageCounter(limitID, scopeKey, periodKey, time.Now())
	if err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Failed to create usage counter model", err)
		return nil, err
	}

	// Convert entity to database model using ToEntity/FromEntity pattern
	var dbModel UsageCounterPostgreSQLModel
	if err := dbModel.FromEntity(newCounter); err != nil {
		return nil, fmt.Errorf("failed to convert entity to database model: %w", err)
	}

	insertQuery := sq.Insert(usageCountersTable).
		Columns("id", "limit_id", "scope_key", "period_key", "current_usage", "last_updated_at").
		Values(dbModel.ID, dbModel.LimitID, dbModel.ScopeKey, dbModel.PeriodKey, dbModel.CurrentUsage, dbModel.LastUpdatedAt).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err = insertQuery.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build insert query", err)
		return nil, fmt.Errorf("failed to build insert query: %w", err)
	}

	_, err = db.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		// Only retry on unique constraint violation (SQLSTATE 23505)
		// This handles the race condition where another transaction inserted the counter
		if !IsUniqueViolation(err) {
			// Not a unique constraint violation - return the original error
			libOtel.HandleSpanError(span, "Failed to insert usage counter", err)
			return nil, fmt.Errorf("failed to insert usage counter: %w", err)
		}

		// Handle concurrent insert race condition
		// Another transaction inserted the counter, try to select it again
		selectQuery = sq.Select("id", "limit_id", "scope_key", "period_key", "current_usage", "last_updated_at").
			From(usageCountersTable).
			Where(sq.Eq{
				"limit_id":   limitID,
				"scope_key":  scopeKey,
				"period_key": periodKey,
			}).
			Suffix("FOR UPDATE").
			PlaceholderFormat(sq.Dollar)

		sqlStr, args, err = selectQuery.ToSql()
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to build retry select query", err)
			return nil, fmt.Errorf("failed to build retry select query: %w", err)
		}

		var retryErr error

		counter, retryErr = r.scanCounter(ctx, db.QueryRowContext(ctx, sqlStr, args...))
		if retryErr != nil {
			libOtel.HandleSpanError(span, "Failed to insert or get usage counter", retryErr)
			return nil, fmt.Errorf("failed to insert or get usage counter: %w", retryErr)
		}

		logger.With(
			libLog.String("operation", "repository.usage_counter.get_or_create_for_update"),
			libLog.String("counter_id", counter.ID.String()),
			libLog.String("note", "found after concurrent insert"),
		).Log(ctx, libLog.LevelInfo, "Found usage counter after retry")

		return counter, nil
	}

	// Re-select the inserted row with FOR UPDATE to acquire the row-level lock
	// This ensures the returned counter has the lock, matching the existing row path
	selectInserted := sq.Select("id", "limit_id", "scope_key", "period_key", "current_usage", "last_updated_at").
		From(usageCountersTable).
		Where(sq.Eq{"id": newCounter.ID}).
		Suffix("FOR UPDATE").
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err = selectInserted.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build post-insert select query", err)
		return nil, fmt.Errorf("failed to build post-insert select query: %w", err)
	}

	counter, err = r.scanCounter(ctx, db.QueryRowContext(ctx, sqlStr, args...))
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to select inserted usage counter", err)
		return nil, fmt.Errorf("failed to select inserted usage counter: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.usage_counter.get_or_create_for_update"),
		libLog.String("counter_id", counter.ID.String()),
	).Log(ctx, libLog.LevelInfo, "Created new usage counter")

	return counter, nil
}

// IncrementAtomic atomically increments the usage counter.
// Uses a single UPDATE statement for atomic increment.
func (r *UsageCounterRepository) IncrementAtomic(ctx context.Context, counterID uuid.UUID, amount decimal.Decimal) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.usage_counter.increment_atomic")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if amount.IsNegative() {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid increment amount", constant.ErrUsageCounterIncrementNonNegative)
		return constant.ErrUsageCounterIncrementNonNegative
	}

	if amount.IsZero() {
		return nil
	}

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get database connection", err)
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	updateQuery := sq.Update(usageCountersTable).
		Set("current_usage", sq.Expr("current_usage + ?", amount)).
		Set("last_updated_at", time.Now().UTC()).
		Where(sq.Eq{"id": counterID}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := updateQuery.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build update query", err)
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := db.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to increment counter", err)
		return fmt.Errorf("failed to increment counter: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get rows affected", err)
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		libOtel.HandleSpanBusinessErrorEvent(span, "Usage counter not found", constant.ErrUsageCounterNotFound)
		return constant.ErrUsageCounterNotFound
	}

	logger.With(
		libLog.String("operation", "repository.usage_counter.increment_atomic"),
		libLog.String("counter_id", counterID.String()),
		libLog.String("amount", amount.String()),
	).Log(ctx, libLog.LevelInfo, "Incremented usage counter")

	return nil
}

// UpsertAndIncrementAtomic atomically creates or increments a usage counter using the provided database connection.
// This allows callers to pass either a regular DB connection or a transaction (*sql.Tx),
// enabling atomic operations with other database changes.
//
// IMPORTANT: The WHERE guard only applies to the DO UPDATE (conflict) path.
// The INSERT path creates a new counter with current_usage = amount without a WHERE guard.
// Therefore, the caller MUST pre-check amount > maxAmount before calling this method.
//
// The expiresAt parameter specifies when the counter should be eligible for cleanup.
// If expiresAt is nil, the counter will never be automatically deleted (fail-safe behavior).
func (r *UsageCounterRepository) UpsertAndIncrementAtomic(
	ctx context.Context,
	db pgdb.DB,
	limitID uuid.UUID,
	scopeKey string,
	periodKey string,
	amount decimal.Decimal,
	maxAmount decimal.Decimal,
	expiresAt *time.Time,
) (decimal.Decimal, error) {
	if db == nil {
		return decimal.Zero, pgdb.ErrNilConnection
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.usage_counter.upsert_and_increment_atomic")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	return r.upsertAndIncrementAtomicInternal(ctx, db, limitID, scopeKey, periodKey, amount, maxAmount, expiresAt, logger, span, "repository.usage_counter.upsert_and_increment_atomic")
}

// upsertAndIncrementAtomicInternal contains the shared upsert logic.
// It performs validation, query building, and execution using the provided database connection.
func (r *UsageCounterRepository) upsertAndIncrementAtomicInternal(
	ctx context.Context,
	db pgdb.DB,
	limitID uuid.UUID,
	scopeKey string,
	periodKey string,
	amount decimal.Decimal,
	maxAmount decimal.Decimal,
	expiresAt *time.Time,
	logger libLog.Logger,
	span trace.Span,
	operationName string,
) (decimal.Decimal, error) {
	// Early return for zero amount: no-op, avoids unnecessary DB round-trip.
	if amount.IsZero() {
		return decimal.Zero, nil
	}

	// Pre-check: reject negative amounts before any SQL.
	if amount.IsNegative() {
		return decimal.Zero, constant.ErrUsageCounterIncrementNonNegative
	}

	// Pre-check: amount > maxAmount means even a brand-new counter would exceed the limit.
	// This is MANDATORY because the INSERT path has no WHERE guard.
	if amount.GreaterThan(maxAmount) {
		logger.With(
			libLog.String("operation", operationName),
			libLog.String("amount", amount.String()),
			libLog.String("max_amount", maxAmount.String()),
			libLog.String("limit_id", limitID.String()),
		).Log(ctx, libLog.LevelInfo, "Amount exceeds maxAmount (pre-check)")
		libOtel.HandleSpanBusinessErrorEvent(span, "Amount exceeds limit (pre-check)", constant.ErrUsageCounterExceedsLimit)

		return decimal.Zero, constant.ErrUsageCounterExceedsLimit
	}

	now := time.Now().UTC()
	counterID := uuid.New()

	// Use pre-defined CTE query for atomic upsert+increment
	query := upsertAndIncrementCTEQuery

	args := []any{
		counterID.String(), // $1
		limitID.String(),   // $2
		scopeKey,           // $3
		periodKey,          // $4
		amount,             // $5 (INSERT initial value)
		now,                // $6 (INSERT last_updated_at)
		amount,             // $7 (UPDATE increment)
		now,                // $8 (UPDATE last_updated_at)
		amount,             // $9 (WHERE guard check)
		maxAmount,          // $10 (WHERE guard limit)
		expiresAt,          // $11 (expires_at for cleanup)
	}

	var (
		currentUsage decimal.Decimal
		succeeded    bool
	)

	err := db.QueryRowContext(ctx, query, args...).Scan(&currentUsage, &succeeded)
	if err != nil {
		libOtel.HandleSpanError(span, "Database error in CTE upsert", err)

		return decimal.Zero, fmt.Errorf("failed to scan CTE result for limit %s scope %s period %s: %w",
			limitID, scopeKey, periodKey, err)
	}

	// Check the succeeded flag to determine if the operation was successful
	if !succeeded {
		// WHERE guard failed: current_usage + amount > maxAmount
		// The CTE attempt returned no rows, so COALESCE returned the old current_usage
		logger.With(
			libLog.String("operation", operationName),
			libLog.String("limit_id", limitID.String()),
			libLog.String("scope_key", scopeKey),
			libLog.String("period_key", periodKey),
			libLog.String("current_usage", currentUsage.String()),
			libLog.String("amount", amount.String()),
			libLog.String("max_amount", maxAmount.String()),
		).Log(ctx, libLog.LevelInfo, "Limit exceeded (WHERE guard)")
		libOtel.HandleSpanBusinessErrorEvent(span, "Limit exceeded", constant.ErrUsageCounterExceedsLimit)

		return currentUsage, constant.ErrUsageCounterExceedsLimit
	}

	// Success: counter was incremented
	logger.With(
		libLog.String("operation", operationName),
		libLog.String("limit_id", limitID.String()),
		libLog.String("scope_key", scopeKey),
		libLog.String("period_key", periodKey),
		libLog.String("new_usage", currentUsage.String()),
	).Log(ctx, libLog.LevelInfo, "Upsert and increment completed")

	return currentUsage, nil
}

// UpsertAndReserveAtomic atomically creates or reserves capacity on a usage counter
// using the provided database connection. It is the reserve-path twin of
// UpsertAndIncrementAtomic: instead of committing into current_usage, it moves the
// amount into reserved_usage and guards against the COMBINED committed + outstanding
// usage so concurrent reservers cannot over-commit a limit.
//
// IMPORTANT: The WHERE guard only applies to the DO UPDATE (conflict) path. The
// INSERT path seeds reserved_usage = amount and current_usage = 0 WITHOUT a WHERE
// guard, so the caller MUST pre-check amount > maxAmount before calling this method
// (a fresh counter would otherwise reserve past the limit on the first request).
//
// Returns the resulting reserved_usage and a success flag wrapped as an error: on a
// failed guard it returns the current reserved_usage with constant.ErrUsageCounterExceedsLimit.
//
// The expiresAt parameter feeds usage_counters.expires_at for counter cleanup; it is
// distinct from the per-reservation TTL tracked on usage_reservations.
func (r *UsageCounterRepository) UpsertAndReserveAtomic(
	ctx context.Context,
	db pgdb.DB,
	limitID uuid.UUID,
	scopeKey string,
	periodKey string,
	amount decimal.Decimal,
	maxAmount decimal.Decimal,
	expiresAt *time.Time,
) (decimal.Decimal, error) {
	if db == nil {
		return decimal.Zero, pgdb.ErrNilConnection
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.usage_counter.upsert_and_reserve_atomic")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	const operationName = "repository.usage_counter.upsert_and_reserve_atomic"

	// Early return for zero amount: no-op, avoids an unnecessary DB round-trip.
	if amount.IsZero() {
		return decimal.Zero, nil
	}

	// Pre-check: reject negative amounts before any SQL.
	if amount.IsNegative() {
		return decimal.Zero, constant.ErrUsageCounterIncrementNonNegative
	}

	// Pre-check: amount > maxAmount means even a brand-new counter would exceed the
	// limit. MANDATORY because the INSERT path has no WHERE guard.
	if amount.GreaterThan(maxAmount) {
		logger.With(
			libLog.String("operation", operationName),
			libLog.String("amount", amount.String()),
			libLog.String("max_amount", maxAmount.String()),
			libLog.String("limit_id", limitID.String()),
		).Log(ctx, libLog.LevelInfo, "Reserve amount exceeds maxAmount (pre-check)")
		libOtel.HandleSpanBusinessErrorEvent(span, "Reserve amount exceeds limit (pre-check)", constant.ErrUsageCounterExceedsLimit)

		return decimal.Zero, constant.ErrUsageCounterExceedsLimit
	}

	now := time.Now().UTC()
	counterID := uuid.New()

	args := []any{
		counterID.String(), // $1
		limitID.String(),   // $2
		scopeKey,           // $3
		periodKey,          // $4
		amount,             // $5 (INSERT initial reserved_usage)
		now,                // $6 (INSERT last_updated_at)
		amount,             // $7 (UPDATE reserved_usage increment)
		now,                // $8 (UPDATE last_updated_at)
		amount,             // $9 (WHERE guard check)
		maxAmount,          // $10 (WHERE guard limit)
		expiresAt,          // $11 (expires_at for cleanup)
	}

	var (
		reservedUsage decimal.Decimal
		succeeded     bool
	)

	err := db.QueryRowContext(ctx, upsertAndReserveCTEQuery, args...).Scan(&reservedUsage, &succeeded)
	if err != nil {
		libOtel.HandleSpanError(span, "Database error in CTE reserve", err)

		return decimal.Zero, fmt.Errorf("failed to scan reserve CTE result for limit %s scope %s period %s: %w",
			limitID, scopeKey, periodKey, err)
	}

	if !succeeded {
		// WHERE guard failed: current_usage + reserved_usage + amount > maxAmount.
		logger.With(
			libLog.String("operation", operationName),
			libLog.String("limit_id", limitID.String()),
			libLog.String("scope_key", scopeKey),
			libLog.String("period_key", periodKey),
			libLog.String("reserved_usage", reservedUsage.String()),
			libLog.String("amount", amount.String()),
			libLog.String("max_amount", maxAmount.String()),
		).Log(ctx, libLog.LevelInfo, "Limit exceeded (reserve WHERE guard)")
		libOtel.HandleSpanBusinessErrorEvent(span, "Limit exceeded", constant.ErrUsageCounterExceedsLimit)

		return reservedUsage, constant.ErrUsageCounterExceedsLimit
	}

	logger.With(
		libLog.String("operation", operationName),
		libLog.String("limit_id", limitID.String()),
		libLog.String("scope_key", scopeKey),
		libLog.String("period_key", periodKey),
		libLog.String("new_reserved_usage", reservedUsage.String()),
	).Log(ctx, libLog.LevelInfo, "Upsert and reserve completed")

	return reservedUsage, nil
}

// GetByLimitID retrieves all usage counters for a specific limit.
func (r *UsageCounterRepository) GetByLimitID(ctx context.Context, limitID uuid.UUID) ([]model.UsageCounter, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.usage_counter.get_by_limit_id")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get database connection", err)
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	query := sq.Select("id", "limit_id", "scope_key", "period_key", "current_usage", "last_updated_at").
		From(usageCountersTable).
		Where(sq.Eq{"limit_id": limitID}).
		OrderBy("period_key DESC", "scope_key ASC").
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := query.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build query", err)
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.usage_counter.get_by_limit_id"),
		libLog.String("limit_id", limitID.String()),
	).Log(ctx, libLog.LevelInfo, "Getting usage counters by limit ID")

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get usage counters", err)
		return nil, fmt.Errorf("failed to get usage counters: %w", err)
	}
	defer rows.Close()

	var counters []model.UsageCounter

	for rows.Next() {
		counter, err := r.scanCounterFromRows(ctx, rows)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to scan usage counter", err)
			return nil, fmt.Errorf("failed to scan usage counter: %w", err)
		}

		counters = append(counters, *counter)
	}

	if err := rows.Err(); err != nil {
		libOtel.HandleSpanError(span, "Error iterating usage counters", err)
		return nil, fmt.Errorf("error iterating usage counters: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.usage_counter.get_by_limit_id"),
		libLog.String("limit_id", limitID.String()),
		libLog.Int("count", len(counters)),
	).Log(ctx, libLog.LevelInfo, "Retrieved usage counters")

	return counters, nil
}

// GetUsageForLimits retrieves current usage for multiple limits using the provided database connection.
// This allows callers to pass either a regular DB connection or a transaction (*sql.Tx),
// enabling atomic operations with other database changes.
func (r *UsageCounterRepository) GetUsageForLimits(ctx context.Context, db pgdb.DB, limitIDs []uuid.UUID, scopeKey, periodKey string) (map[uuid.UUID]decimal.Decimal, error) {
	if db == nil {
		return nil, pgdb.ErrNilConnection
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.usage_counter.get_usage_for_limits")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if len(limitIDs) == 0 {
		return make(map[uuid.UUID]decimal.Decimal), nil
	}

	return r.getUsageForLimitsInternal(ctx, db, limitIDs, scopeKey, periodKey, logger, span, "repository.usage_counter.get_usage_for_limits")
}

// getUsageForLimitsInternal contains the shared query logic for GetUsageForLimits.
// It performs query building and execution using the provided database connection.
func (r *UsageCounterRepository) getUsageForLimitsInternal(
	ctx context.Context,
	db pgdb.DB,
	limitIDs []uuid.UUID,
	scopeKey, periodKey string,
	logger libLog.Logger,
	span trace.Span,
	operationName string,
) (map[uuid.UUID]decimal.Decimal, error) {
	query := sq.Select("limit_id", "current_usage").
		From(usageCountersTable).
		Where(sq.Eq{
			"limit_id":   limitIDs,
			"scope_key":  scopeKey,
			"period_key": periodKey,
		}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := query.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build query", err)
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	logger.With(
		libLog.String("operation", operationName),
		libLog.Int("limit_ids_count", len(limitIDs)),
		libLog.String("scope_key", scopeKey),
		libLog.String("period_key", periodKey),
	).Log(ctx, libLog.LevelInfo, "Getting usage for limits")

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get usage", err)
		return nil, fmt.Errorf("failed to get usage: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]decimal.Decimal)

	for rows.Next() {
		var limitID uuid.UUID

		var currentUsage decimal.Decimal

		if err := rows.Scan(&limitID, &currentUsage); err != nil {
			libOtel.HandleSpanError(span, "Failed to scan usage", err)
			return nil, fmt.Errorf("failed to scan usage: %w", err)
		}

		result[limitID] = currentUsage
	}

	if err := rows.Err(); err != nil {
		libOtel.HandleSpanError(span, "Error iterating usage", err)
		return nil, fmt.Errorf("error iterating usage: %w", err)
	}

	logger.With(
		libLog.String("operation", operationName),
		libLog.Int("found_count", len(result)),
	).Log(ctx, libLog.LevelInfo, "Retrieved usage for limits")

	return result, nil
}

// scanCounter scans a single row into a UsageCounter model using the ToEntity/FromEntity pattern.
func (r *UsageCounterRepository) scanCounter(ctx context.Context, row *sql.Row) (*model.UsageCounter, error) {
	var dbModel UsageCounterPostgreSQLModel

	// Check for context cancellation before processing
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	err := row.Scan(
		&dbModel.ID,
		&dbModel.LimitID,
		&dbModel.ScopeKey,
		&dbModel.PeriodKey,
		&dbModel.CurrentUsage,
		&dbModel.LastUpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Convert database model to domain entity
	counter, err := dbModel.ToEntity()
	if err != nil {
		return nil, fmt.Errorf("failed to convert to entity: %w", err)
	}

	return counter, nil
}

// scanCounterFromRows scans a row from Rows into a UsageCounter model using the ToEntity/FromEntity pattern.
func (r *UsageCounterRepository) scanCounterFromRows(ctx context.Context, rows *sql.Rows) (*model.UsageCounter, error) {
	var dbModel UsageCounterPostgreSQLModel

	// Check for context cancellation before processing
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	err := rows.Scan(
		&dbModel.ID,
		&dbModel.LimitID,
		&dbModel.ScopeKey,
		&dbModel.PeriodKey,
		&dbModel.CurrentUsage,
		&dbModel.LastUpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Convert database model to domain entity
	counter, err := dbModel.ToEntity()
	if err != nil {
		return nil, fmt.Errorf("failed to convert to entity: %w", err)
	}

	return counter, nil
}

// DeleteExpiredCounters removes usage counters whose expires_at is before now.
// Counters with NULL expires_at are preserved (never deleted).
// Deletes are performed in batches to prevent long-running locks on large tables.
// Returns the total number of deleted counters.
func (r *UsageCounterRepository) DeleteExpiredCounters(ctx context.Context, now time.Time) (int64, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.usage_counter.delete_expired_counters_by_expires_at")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	logger.With(
		libLog.String("operation", "repository.usage_counter.delete_expired_counters_by_expires_at"),
		libLog.String("now", now.Format(time.RFC3339)),
		libLog.Int("batch_size", r.deleteBatchSize),
	).Log(ctx, libLog.LevelInfo, "Deleting expired usage counters by expires_at in batches")

	var totalDeleted int64

	for {
		// Check for context cancellation before each batch to allow graceful shutdown
		if err := ctx.Err(); err != nil {
			logger.With(
				libLog.String("operation", "repository.usage_counter.delete_expired_counters_by_expires_at"),
				libLog.Any("total_deleted", totalDeleted),
				libLog.String("reason", err.Error()),
			).Log(ctx, libLog.LevelInfo, "Stopping batch deletion due to context cancellation")
			libOtel.HandleSpanError(span, "Context cancelled during batch deletion", err)

			return totalDeleted, fmt.Errorf("context cancelled during batch deletion: %w", err)
		}

		// Get a fresh connection for each batch to avoid holding transactions across iterations
		db, err := r.conn.GetDB(ctx)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to get database connection", err)
			return totalDeleted, fmt.Errorf("failed to get database connection: %w", err)
		}

		// Build batched delete query using subquery:
		// DELETE FROM usage_counters WHERE id IN (SELECT id FROM usage_counters WHERE expires_at IS NOT NULL AND expires_at < $1 LIMIT $2)
		// PostgreSQL doesn't support LIMIT directly on DELETE, so we use a subquery approach.
		// Counters with NULL expires_at are preserved (never deleted automatically).
		deleteQuery := fmt.Sprintf(
			"DELETE FROM %s WHERE id IN (SELECT id FROM %s WHERE expires_at IS NOT NULL AND expires_at < $1 LIMIT $2)",
			usageCountersTable, usageCountersTable,
		)

		result, err := db.ExecContext(ctx, deleteQuery, now, r.deleteBatchSize)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to delete expired counters batch", err)
			return totalDeleted, fmt.Errorf("failed to delete expired counters by expires_at: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to get rows affected", err)
			return totalDeleted, fmt.Errorf("failed to get rows affected: %w", err)
		}

		totalDeleted += rowsAffected

		logger.With(
			libLog.String("operation", "repository.usage_counter.delete_expired_counters_by_expires_at"),
			libLog.Any("batch_deleted", rowsAffected),
			libLog.Any("total_deleted", totalDeleted),
		).Log(ctx, libLog.LevelDebug, "Deleted batch of expired usage counters")

		// Stop when no more rows to delete
		if rowsAffected == 0 {
			break
		}
	}

	logger.With(
		libLog.String("operation", "repository.usage_counter.delete_expired_counters_by_expires_at"),
		libLog.Any("deleted_count", totalDeleted),
	).Log(ctx, libLog.LevelInfo, "Deleted expired usage counters by expires_at")

	return totalDeleted, nil
}
