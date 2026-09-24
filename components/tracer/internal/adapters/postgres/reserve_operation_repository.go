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

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ReserveOperationRepository serializes admission and known completion through
// a durable row lock. Every method requires the caller's tenant transaction;
// it never infers accounting outcomes, moves capacity, emits audit or commits.
type ReserveOperationRepository struct{}

func NewReserveOperationRepository() *ReserveOperationRepository {
	return &ReserveOperationRepository{}
}

// LockWithTx creates an OPEN marker when absent and locks the operation until
// transaction completion. Call it BEFORE account/counter/audit locks, then
// repeat the decision lookup. An existing decision remains replayable even
// when this state is terminal; a new evaluation must be refused in that case.
// Waits inherit the caller's deadline, and errors require transaction rollback.
func (r *ReserveOperationRepository) LockWithTx(ctx context.Context, tx pgdb.Tx, identity model.ReserveOperationIdentity) (_ *model.ReserveOperationState, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.lock_reserve_operation")
	defer span.End()
	defer func() { recordReserveDecisionRepositoryError(span, retErr) }()

	if tx == nil {
		return nil, pgdb.ErrNilConnection
	}

	if err := identity.Validate(); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO reserve_operations (integration_id, transaction_id, status)
		VALUES ($1, $2, 'OPEN') ON CONFLICT (integration_id, transaction_id) DO NOTHING`, identity.IntegrationID, identity.TransactionID); err != nil {
		return nil, fmt.Errorf("ensure reserve operation: %w", err)
	}
	// READ COMMITTED's second statement sees a concurrent insertion after the
	// unique-index wait. Higher isolation may report serialization failure;
	// callers must never treat that as successful admission or completion.
	state, err := scanReserveOperation(tx.QueryRowContext(ctx, `SELECT status, completed_at FROM reserve_operations
		WHERE integration_id=$1 AND transaction_id=$2 FOR UPDATE`, identity.IntegrationID, identity.TransactionID))
	if err != nil {
		return nil, err
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Reserve operation locked in transaction")

	return state, nil
}

// CompleteWithTx records a known terminal outcome, including when Reserve has
// not arrived. The use case MUST settle any decision-owned capacity and write
// mandatory audit in this same transaction before committing. A same-outcome
// replay returns changed=false and the original completion time; the opposite
// outcome conflicts. No result here is durable until the caller's commit.
func (r *ReserveOperationRepository) CompleteWithTx(ctx context.Context, tx pgdb.Tx, identity model.ReserveOperationIdentity, status model.ReserveOperationStatus, at time.Time) (_ *model.ReserveOperationState, changed bool, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.complete_reserve_operation")
	defer span.End()
	defer func() { recordReserveDecisionRepositoryError(span, retErr) }()

	completion := model.ReserveOperationState{Status: status, CompletedAt: &at}
	if err := completion.Validate(); err != nil {
		return nil, false, err
	}

	before, err := r.LockWithTx(ctx, tx, identity)
	if err != nil {
		return nil, false, err
	}

	if before.Status == status {
		return before, false, nil
	}

	if before.Status != model.OperationOpen {
		return nil, false, constant.ErrReserveOperationConflict
	}

	state, err := scanReserveOperation(tx.QueryRowContext(ctx, `UPDATE reserve_operations SET status=$3, completed_at=$4
		WHERE integration_id=$1 AND transaction_id=$2 AND status='OPEN' RETURNING status, completed_at`, identity.IntegrationID, identity.TransactionID, status, at.UTC()))
	if err != nil {
		return nil, false, err
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Reserve operation completed in transaction")

	return state, true, nil
}

func scanReserveOperation(row *sql.Row) (*model.ReserveOperationState, error) {
	var (
		state model.ReserveOperationState
		at    sql.NullTime
	)
	if err := row.Scan(&state.Status, &at); err != nil {
		return nil, fmt.Errorf("read reserve operation state: %w", err)
	}

	if at.Valid {
		utc := at.Time.UTC()
		state.CompletedAt = &utc
	}

	if err := state.Validate(); err != nil {
		return nil, constant.ErrInternalServer
	}

	return &state, nil
}
