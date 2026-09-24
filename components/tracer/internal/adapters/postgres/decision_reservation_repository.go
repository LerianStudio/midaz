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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ReserveForDecisionWithTx holds capacity for an immutable decision. The caller
// must lock the operation, recheck replay, acquire sorted account locks and call
// this method in counter-coordinate order. Insertions are not replay: duplicate
// ownership conflicts; replay returns the previously stored decision instead.
//
// The decision FK is deferred so capacity can be rolled back to a savepoint on
// DENY/REVIEW before persisting the final result. The caller must write that
// decision and mandatory audit before commit, and roll back on any error.
// counterExpiresAt is the resolved limit period's cleanup time, NOT the request's
// reservation TTL. The latter cannot expire a decision-owned reservation.
func (r *UsageReservationRepository) ReserveForDecisionWithTx(ctx context.Context, tx pgdb.Tx, decisionID uuid.UUID, reservation *model.Reservation, maxAmount decimal.Decimal, counterExpiresAt time.Time) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.reserve_decision_capacity")
	defer span.End()
	defer func() {
		if errors.Is(retErr, constant.ErrUsageCounterExceedsLimit) {
			libOtel.HandleSpanBusinessErrorEvent(span, "capacity exceeds limit", retErr)
		} else {
			recordReserveDecisionRepositoryError(span, retErr)
		}
	}()

	if tx == nil || r.counterRepo == nil {
		return pgdb.ErrNilConnection
	}

	if decisionID == uuid.Nil || counterExpiresAt.IsZero() || maxAmount.IsNegative() {
		return constant.ErrInvalidRequestBody
	}

	if err := validateDecisionReservation(reservation); err != nil {
		return err
	}

	if reservation.Amount.GreaterThan(maxAmount) {
		return constant.ErrUsageCounterExceedsLimit
	}

	statement, args, err := sq.Insert(usageReservationsTable).
		Columns("id", "limit_id", "scope_key", "period_key", "amount", "status", "transaction_id", "reservation_expires_at", "created_at", "decision_id").
		Values(reservation.ID, reservation.LimitID, reservation.ScopeKey, reservation.PeriodKey, reservation.Amount,
			string(reservation.Status), reservation.TransactionID, reservation.ReservationExpiresAt.UTC(), reservation.CreatedAt.UTC(), decisionID).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build decision reservation insert: %w", err)
	}

	if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return constant.ErrReserveDecisionConflict
		}

		return fmt.Errorf("insert decision reservation: %w", err)
	}

	expiresAt := counterExpiresAt.UTC()
	if _, err := r.counterRepo.UpsertAndReserveAtomic(ctx, tx, reservation.LimitID, reservation.ScopeKey, reservation.PeriodKey, reservation.Amount, maxAmount, &expiresAt); err != nil {
		return err
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Decision capacity held in transaction")

	return nil
}

// SettleDecisionWithTx moves only the capacity owned by the resolved decision.
// The caller must resolve authenticated ownership and lock/complete the operation
// first, then write mandatory audit in this SAME transaction. This method does
// not authenticate, infer a missing decision's outcome, commit or retry.
// Returned rows are the pre-transition snapshots for audit; an identical repeat
// returns an empty list. Opposite terminal states conflict rather than remap money.
func (r *UsageReservationRepository) SettleDecisionWithTx(ctx context.Context, tx pgdb.Tx, decisionID uuid.UUID, status model.ReservationStatus) (_ []*model.Reservation, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.settle_decision_capacity")
	defer span.End()
	defer func() { recordReserveDecisionRepositoryError(span, retErr) }()

	if tx == nil {
		return nil, pgdb.ErrNilConnection
	}

	if decisionID == uuid.Nil || (status != model.StatusConfirmed && status != model.StatusReleased) {
		return nil, constant.ErrInvalidRequestBody
	}

	reservations, err := lockDecisionReservations(ctx, tx, decisionID)
	if err != nil {
		return nil, err
	}

	for _, res := range reservations {
		if res.Status != model.StatusReserved && res.Status != status {
			return nil, constant.ErrReserveOperationConflict
		}
	}

	changed := make([]*model.Reservation, 0, len(reservations))
	for _, res := range reservations {
		if res.Status == status {
			continue
		}

		if status == model.StatusConfirmed {
			err = r.applyConfirm(ctx, span, tx, res)
		} else {
			err = r.applyRelease(ctx, span, tx, res, status)
		}

		if err != nil {
			return nil, err
		}

		changed = append(changed, res)
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Decision capacity settled in transaction")

	return changed, nil
}

func lockDecisionReservations(ctx context.Context, tx pgdb.Tx, decisionID uuid.UUID) ([]*model.Reservation, error) {
	// Order counter coordinates before touching counters. UUID identity is only
	// the tie-breaker; sorting reservation IDs alone can invert shared counters.
	statement, args, err := sq.Select("id", "limit_id", "scope_key", "period_key", "amount", "status", "transaction_id", "reservation_expires_at", "created_at", "confirmed_at", "released_at").
		From(usageReservationsTable).Where(sq.Eq{"decision_id": decisionID}).
		OrderBy("limit_id", "scope_key", "period_key", "id").Suffix("FOR UPDATE").PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build decision capacity lookup: %w", err)
	}

	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("lock decision capacity: %w", err)
	}
	defer rows.Close()

	reservations := make([]*model.Reservation, 0)

	for rows.Next() {
		var (
			res                     model.Reservation
			confirmedAt, releasedAt sql.NullTime
		)
		if err := rows.Scan(&res.ID, &res.LimitID, &res.ScopeKey, &res.PeriodKey, &res.Amount, &res.Status,
			&res.TransactionID, &res.ReservationExpiresAt, &res.CreatedAt, &confirmedAt, &releasedAt); err != nil {
			return nil, fmt.Errorf("scan decision capacity: %w", err)
		}

		if confirmedAt.Valid {
			res.ConfirmedAt = &confirmedAt.Time
		}

		if releasedAt.Valid {
			res.ReleasedAt = &releasedAt.Time
		}

		reservations = append(reservations, &res)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read decision capacity: %w", err)
	}

	return reservations, nil
}

func validateDecisionReservation(reservation *model.Reservation) error {
	if reservation == nil || reservation.ID == uuid.Nil ||
		reservation.Status != model.StatusReserved || !reservation.Amount.IsPositive() ||
		reservation.CreatedAt.IsZero() || reservation.ConfirmedAt != nil || reservation.ReleasedAt != nil {
		return constant.ErrInvalidRequestBody
	}

	if err := reservation.Validate(); err != nil {
		return constant.ErrInvalidRequestBody
	}

	return nil
}
