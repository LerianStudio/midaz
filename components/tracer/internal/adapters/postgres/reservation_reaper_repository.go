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

// ReservationReaperRepository adapts the two-phase reservation repository to the
// TTL-reaper's narrow surface: find the outstanding RESERVED rows past their TTL
// and release each legacy one as EXPIRED in its own transaction. It composes the
// existing UsageReservationRepository (whose ReleaseWithTx keeps the counter bucket
// move and the row flip atomic per row) with a TxBeginner so the reaper does NOT
// manage the transaction lifecycle itself, and a Connection for the read-only
// sweep query. Decision-owned rows are only located here; they expire through
// their operation.
//
// It writes NO per-row audit rows — the reaper batches the audit side into one
// summary event per sweep (Q11). The find query rides the
// idx_usage_reservations_reaper partial index.
type ReservationReaperRepository struct {
	conn       pgdb.Connection
	txBeginner pgdb.TxBeginner
	resRepo    *UsageReservationRepository
}

// NewReservationReaperRepository builds a reaper repository. conn supplies the
// read handle for the sweep, txBeginner opens the per-row release transaction, and
// resRepo runs the atomic EXPIRED transition on that transaction.
func NewReservationReaperRepository(
	conn pgdb.Connection,
	txBeginner pgdb.TxBeginner,
	resRepo *UsageReservationRepository,
) *ReservationReaperRepository {
	return &ReservationReaperRepository{conn: conn, txBeginner: txBeginner, resRepo: resRepo}
}

// FindExpiredReservations returns at most limit RESERVED reservations whose
// reservation_expires_at is strictly before now, ordered by (expiry, id) and
// strictly after the after position when one is given, with the owning
// operation of each decision-owned row. The status is a literal rather
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

	query := sq.Select("r.id", "r.reservation_expires_at", "r.decision_id", "d.integration_id", "d.transaction_id").
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
			decisionID    uuid.NullUUID
			integrationID sql.NullString
			transactionID uuid.NullUUID
		)

		if err := rows.Scan(&item.ID, &item.ExpiresAt, &decisionID, &integrationID, &transactionID); err != nil {
			libOtel.HandleSpanError(span, "Failed to scan expired reservation", err)
			return nil, fmt.Errorf("failed to scan expired reservation: %w", err)
		}

		item.ExpiresAt = item.ExpiresAt.UTC()

		if decisionID.Valid {
			// The deferred ownership FK guarantees the decision; a missing one
			// must never demote the row to a lone legacy expiry.
			if !integrationID.Valid || !transactionID.Valid {
				libOtel.HandleSpanError(span, "Expired reservation lost its decision", constant.ErrInternalServer)
				return nil, fmt.Errorf("expired reservation %s has no owning decision: %w", item.ID, constant.ErrInternalServer)
			}

			item.Operation = &model.ReserveOperationIdentity{IntegrationID: integrationID.String, TransactionID: transactionID.UUID}
		}

		expired = append(expired, item)
	}

	if err := rows.Err(); err != nil {
		libOtel.HandleSpanError(span, "Failed to iterate expired reservations", err)
		return nil, fmt.Errorf("failed to iterate expired reservations: %w", err)
	}

	logger.With(libLog.Int("count", len(expired))).Log(ctx, libLog.LevelDebug, "Found expired reservations")

	return expired, nil
}

// ReleaseExpired flips a RESERVED reservation to EXPIRED and returns its held
// amount to the counter, atomically in one transaction. A reservation that has
// already reached a terminal state (a confirm/release raced the sweep) is an
// idempotent no-op: ReleaseWithTx returns ErrReservationAlreadyTerminal, which is
// mapped to success here so the reaper never reports an expected race as an error.
func (r *ReservationReaperRepository) ReleaseExpired(ctx context.Context, reservationID uuid.UUID) (err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.reservation_reaper.release_expired")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	tx, beginErr := r.txBeginner.BeginTx(ctx, nil)
	if beginErr != nil {
		libOtel.HandleSpanError(span, "Failed to begin transaction", beginErr)
		return fmt.Errorf("failed to begin reaper transaction: %w", beginErr)
	}

	if tx == nil {
		return errors.New("reservation_reaper: BeginTx returned nil transaction without error")
	}

	committed := false

	defer func() {
		if committed {
			return
		}

		if rbErr := tx.Rollback(); rbErr != nil {
			logger.With(
				libLog.String("operation", "repository.reservation_reaper.rollback"),
				libLog.String("error.message", rbErr.Error()),
			).Log(ctx, libLog.LevelWarn, "Failed to rollback reaper transaction")
		}
	}()

	if relErr := r.resRepo.ReleaseWithTx(ctx, tx, reservationID, model.StatusExpired); relErr != nil {
		// Already terminal: a confirm/release committed between the find and this
		// release. Commit nothing — the row is already in a terminal state and its
		// counter move already happened. Treat as an idempotent no-op success.
		if errors.Is(relErr, constant.ErrReservationAlreadyTerminal) {
			return nil
		}

		return relErr
	}

	if commitErr := tx.Commit(); commitErr != nil {
		libOtel.HandleSpanError(span, "Failed to commit transaction", commitErr)
		return fmt.Errorf("failed to commit reaper transaction: %w", commitErr)
	}

	committed = true

	return nil
}
