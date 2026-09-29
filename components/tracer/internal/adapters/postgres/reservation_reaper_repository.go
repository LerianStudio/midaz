// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ReservationReaperRepository is the TTL reaper's read surface: it locates the
// outstanding RESERVED rows past their TTL together with the operation that owns
// each one. The rows expire through their operation, never one by one, so this
// repository writes nothing. The find query rides the
// idx_usage_reservations_reaper partial index.
type ReservationReaperRepository struct {
	conn pgdb.Connection
}

// NewReservationReaperRepository builds a reaper repository over conn, which
// supplies the read handle for the sweep.
func NewReservationReaperRepository(conn pgdb.Connection) *ReservationReaperRepository {
	return &ReservationReaperRepository{conn: conn}
}

// FindExpiredReservations returns at most limit RESERVED reservations whose
// reservation_expires_at is strictly before now, ordered by (expiry, id) and
// strictly after the after position when one is given, with the owning
// operation of each row. The status is a literal rather
// than a bind parameter so a generic plan can still prove the reaper partial
// index's predicate. Tenant resolution is carried on ctx (tmcore.ContextWithPG
// in MT mode), so the read lands on the correct database.
func (r *ReservationReaperRepository) FindExpiredReservations(
	ctx context.Context,
	now time.Time,
	after *model.ReservationExpiryPosition,
	limit int,
) ([]model.ExpiredReservation, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.reservation_reaper.find_expired")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if limit <= 0 {
		return nil, fmt.Errorf("expired reservations limit must be positive: %w", constant.ErrInvalidRequestBody)
	}

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to resolve database connection", err)
		return nil, fmt.Errorf("failed to resolve database connection: %w", err)
	}

	query := sq.Select("r.id", "r.reservation_expires_at", "d.integration_id", "d.transaction_id").
		From(usageReservationsTable + " AS r").
		LeftJoin("reserve_decisions AS d ON d.evaluation_id = r.decision_id AND d.transaction_id = r.transaction_id").
		Where("r.status = '" + string(model.StatusReserved) + "'").
		Where(sq.Lt{"r.reservation_expires_at": now.UTC()})

	if after != nil {
		query = query.Where(sq.Expr("(r.reservation_expires_at, r.id) > (?, ?)", after.ExpiresAt.UTC(), after.ID))
	}

	statement, args, err := query.
		OrderBy("r.reservation_expires_at", "r.id").
		Limit(uint64(limit)).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build expired reservations query", err)
		return nil, fmt.Errorf("failed to build expired reservations query: %w", err)
	}

	rows, err := db.QueryContext(ctx, statement, args...)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to query expired reservations", err)
		return nil, fmt.Errorf("failed to query expired reservations: %w", err)
	}
	defer rows.Close()

	var expired []model.ExpiredReservation

	for rows.Next() {
		var (
			item          model.ExpiredReservation
			integrationID sql.NullString
			transactionID uuid.NullUUID
		)

		if err := rows.Scan(&item.ID, &item.ExpiresAt, &integrationID, &transactionID); err != nil {
			libOtel.HandleSpanError(span, "Failed to scan expired reservation", err)
			return nil, fmt.Errorf("failed to scan expired reservation: %w", err)
		}

		// The decision CHECK and the deferred ownership FK guarantee the
		// decision; a row without one must fail the sweep rather than expire
		// outside an operation.
		if !integrationID.Valid || !transactionID.Valid {
			libOtel.HandleSpanError(span, "Expired reservation lost its decision", constant.ErrInternalServer)
			return nil, fmt.Errorf("expired reservation %s has no owning decision: %w", item.ID, constant.ErrInternalServer)
		}

		item.ExpiresAt = item.ExpiresAt.UTC()
		item.Operation = model.ReserveOperationIdentity{IntegrationID: integrationID.String, TransactionID: transactionID.UUID}

		expired = append(expired, item)
	}

	if err := rows.Err(); err != nil {
		libOtel.HandleSpanError(span, "Failed to iterate expired reservations", err)
		return nil, fmt.Errorf("failed to iterate expired reservations: %w", err)
	}

	logger.With(libLog.Int("count", len(expired))).Log(ctx, libLog.LevelDebug, "Found expired reservations")

	return expired, nil
}

// FindExpiredOperations returns at most limit OPEN operations whose expires_at
// is strictly before now and whose decision holds no RESERVED reservation,
// ordered by (expiry, integration, transaction) and strictly after the after
// position when one is given. An operation that still holds a reservation is
// left to FindExpiredReservations, so the two sweeps never select the same
// operation. The status is a literal so a generic plan can still prove the
// idx_reserve_operations_expiry partial index's predicate.
func (r *ReservationReaperRepository) FindExpiredOperations(
	ctx context.Context,
	now time.Time,
	after *model.OperationExpiryPosition,
	limit int,
) ([]model.ExpiredOperation, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.reservation_reaper.find_expired_operations")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if limit <= 0 {
		return nil, fmt.Errorf("expired operations limit must be positive: %w", constant.ErrInvalidRequestBody)
	}

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to resolve database connection", err)
		return nil, fmt.Errorf("failed to resolve database connection: %w", err)
	}

	query := sq.Select("o.integration_id", "o.transaction_id", "o.expires_at").
		From("reserve_operations AS o").
		Where("o.status = '" + string(model.OperationOpen) + "'").
		Where(sq.Lt{"o.expires_at": now.UTC()}).
		Where("NOT EXISTS (SELECT 1 FROM reserve_decisions AS d JOIN " + usageReservationsTable + " AS r" +
			" ON r.decision_id = d.evaluation_id AND r.transaction_id = d.transaction_id" +
			" WHERE d.integration_id = o.integration_id AND d.transaction_id = o.transaction_id" +
			" AND r.status = '" + string(model.StatusReserved) + "')")

	if after != nil {
		query = query.Where(sq.Expr("(o.expires_at, o.integration_id, o.transaction_id) > (?, ?, ?)",
			after.ExpiresAt.UTC(), after.Operation.IntegrationID, after.Operation.TransactionID))
	}

	statement, args, err := query.
		OrderBy("o.expires_at", "o.integration_id", "o.transaction_id").
		Limit(uint64(limit)).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build expired operations query", err)
		return nil, fmt.Errorf("failed to build expired operations query: %w", err)
	}

	rows, err := db.QueryContext(ctx, statement, args...)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to query expired operations", err)
		return nil, fmt.Errorf("failed to query expired operations: %w", err)
	}
	defer rows.Close()

	var expired []model.ExpiredOperation

	for rows.Next() {
		var item model.ExpiredOperation

		if err := rows.Scan(&item.Operation.IntegrationID, &item.Operation.TransactionID, &item.ExpiresAt); err != nil {
			libOtel.HandleSpanError(span, "Failed to scan expired operation", err)
			return nil, fmt.Errorf("failed to scan expired operation: %w", err)
		}

		item.ExpiresAt = item.ExpiresAt.UTC()

		expired = append(expired, item)
	}

	if err := rows.Err(); err != nil {
		libOtel.HandleSpanError(span, "Failed to iterate expired operations", err)
		return nil, fmt.Errorf("failed to iterate expired operations: %w", err)
	}

	logger.With(libLog.Int("count", len(expired))).Log(ctx, libLog.LevelDebug, "Found expired operations")

	return expired, nil
}
