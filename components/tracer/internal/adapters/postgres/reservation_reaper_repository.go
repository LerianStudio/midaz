// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"

	pgdb "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// ReservationReaperRepository adapts the two-phase reservation repository to the
// TTL-reaper's narrow surface: find the outstanding RESERVED rows past their TTL
// and release each one as EXPIRED in its own transaction. It composes the existing
// UsageReservationRepository (whose ReleaseWithTx keeps the counter bucket move and
// the row flip atomic per row) with a TxBeginner so the reaper does NOT manage the
// transaction lifecycle itself, and a Connection for the read-only sweep query.
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

// FindExpiredReservations returns the ids of RESERVED reservations whose
// reservation_expires_at is strictly before now, scanning the reaper partial
// index. Tenant resolution is carried on ctx (tmcore.ContextWithPG in MT mode),
// so the read lands on the correct database.
func (r *ReservationReaperRepository) FindExpiredReservations(ctx context.Context, now time.Time) ([]uuid.UUID, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.reservation_reaper.find_expired")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to resolve database connection", err)
		return nil, fmt.Errorf("failed to resolve database connection: %w", err)
	}

	// status = 'RESERVED' matches the partial index predicate exactly so the
	// planner uses idx_usage_reservations_reaper. Hand-written const query: a
	// fixed two-predicate scan with no dynamic columns, kept verbatim.
	const findExpiredSQL = `
		SELECT id
		FROM usage_reservations
		WHERE status = 'RESERVED' AND reservation_expires_at < $1
	`

	rows, err := db.QueryContext(ctx, findExpiredSQL, now.UTC())
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to query expired reservations", err)
		return nil, fmt.Errorf("failed to query expired reservations: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID

	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			libOtel.HandleSpanError(span, "Failed to scan expired reservation id", err)
			return nil, fmt.Errorf("failed to scan expired reservation id: %w", err)
		}

		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		libOtel.HandleSpanError(span, "Failed to iterate expired reservations", err)
		return nil, fmt.Errorf("failed to iterate expired reservations: %w", err)
	}

	return ids, nil
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
