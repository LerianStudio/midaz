// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ReserveDecisionRepository persists the original decision independently of
// reservation rows. Storage bounds must cover every still-replayable record;
// they are not the current policy's evaluation limits.
type ReserveDecisionRepository struct {
	conn            pgdb.Connection
	maxRules        int
	maxReservations int
}

func NewReserveDecisionRepository(conn pgdb.Connection, maxRules, maxReservations int) (*ReserveDecisionRepository, error) {
	if conn == nil {
		return nil, pgdb.ErrNilConnection
	}

	if maxRules <= 0 || maxReservations <= 0 {
		return nil, constant.ErrInvalidRequestBody
	}

	return &ReserveDecisionRepository{conn: conn, maxRules: maxRules, maxReservations: maxReservations}, nil
}

// Get uses the primary in the resolved tenant database. A miss is nil, nil.
// Request-ID reuse for another transaction is a conflict, never a cache miss.
func (r *ReserveDecisionRepository) Get(ctx context.Context, key model.ReserveOperationKey) (_ *model.ReserveDecision, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.get_reserve_decision")
	defer span.End()
	defer func() { recordReserveDecisionRepositoryError(span, retErr) }()

	if err := key.Validate(); err != nil {
		return nil, err
	}

	db, err := r.primaryDatabase(ctx)
	if err != nil {
		return nil, err
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Looking up reserve decision on primary")

	return r.read(ctx, db, key)
}

// GetWithTx repeats the replay lookup inside the caller's transaction, after
// acquiring the operation lock. This method itself does not acquire that lock.
func (r *ReserveDecisionRepository) GetWithTx(ctx context.Context, tx pgdb.Tx, key model.ReserveOperationKey) (_ *model.ReserveDecision, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.get_reserve_decision_with_tx")
	defer span.End()
	defer func() { recordReserveDecisionRepositoryError(span, retErr) }()

	if tx == nil {
		return nil, pgdb.ErrNilConnection
	}

	if err := key.Validate(); err != nil {
		return nil, err
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Looking up reserve decision in transaction")

	return r.read(ctx, tx, key)
}

func (r *ReserveDecisionRepository) read(ctx context.Context, db pgdb.DB, key model.ReserveOperationKey) (*model.ReserveDecision, error) {
	statement, args, err := sq.Select("integration_id", "transaction_id", "request_id", "context_id", "request_fingerprint", "validation_mode", "policy_snapshot", "response", "created_at").
		From("reserve_decisions").Where(sq.Eq{"integration_id": key.IntegrationID}).
		Where(sq.Or{sq.Eq{"transaction_id": key.TransactionID}, sq.Eq{"request_id": key.RequestID}}).Limit(2).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build decision lookup: %w", err)
	}

	rows, err := db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("read reserve decision: %w", err)
	}
	defer rows.Close()

	var found *model.ReserveDecision

	for rows.Next() {
		decision, err := r.scan(rows)
		if err != nil {
			return nil, err
		}

		if found != nil || decision.Key != key {
			return nil, constant.ErrReserveDecisionConflict
		}

		found = decision
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reserve decisions: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return found, nil
}

func (r *ReserveDecisionRepository) scan(rows *sql.Rows) (*model.ReserveDecision, error) {
	var (
		d                             model.ReserveDecision
		fingerprint, policy, response []byte
	)
	if err := rows.Scan(&d.Key.IntegrationID, &d.Key.TransactionID, &d.Key.RequestID, &d.ContextID, &fingerprint, &d.ValidationMode, &policy, &response, &d.CreatedAt); err != nil {
		return nil, fmt.Errorf("decode reserve decision: %w", err)
	}

	if len(fingerprint) != len(d.Fingerprint) {
		return nil, constant.ErrInternalServer
	}

	copy(d.Fingerprint[:], fingerprint)

	if len(policy) > 0 {
		if err := json.Unmarshal(policy, &d.Policy); err != nil {
			return nil, constant.ErrInternalServer
		}
	}

	if err := json.Unmarshal(response, &d.Result); err != nil {
		return nil, constant.ErrInternalServer
	}

	d.CreatedAt = d.CreatedAt.UTC()
	if err := d.Validate(r.maxRules, r.maxReservations); err != nil {
		return nil, constant.ErrInternalServer
	}

	return &d, nil
}

// CreateWithTx only inserts a complete immutable decision. The caller owns the
// tenant transaction and MUST roll it back on any error, and commit the decision
// together with capacity changes and mandatory audit. This repository does not
// evaluate controls, emit audit, commit, or retry an unknown commit outcome.
// A duplicate is a conflict: replay must read and compare the original digest.
func (r *ReserveDecisionRepository) CreateWithTx(ctx context.Context, tx pgdb.Tx, d model.ReserveDecision) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_reserve_decision")
	defer span.End()
	defer func() { recordReserveDecisionRepositoryError(span, retErr) }()

	if tx == nil {
		return pgdb.ErrNilConnection
	}

	if err := d.Validate(r.maxRules, r.maxReservations); err != nil {
		return err
	}

	response, err := json.Marshal(d.Result)
	if err != nil {
		return fmt.Errorf("encode reserve response: %w", err)
	}

	var policyID, policyRevision, policySnapshot any

	if d.Policy != nil {
		encoded, err := json.Marshal(d.Policy)
		if err != nil {
			return fmt.Errorf("encode decision policy: %w", err)
		}

		policyID, policyRevision, policySnapshot = d.Policy.ID, d.Policy.Revision, string(encoded)
	}

	statement, args, err := sq.Insert("reserve_decisions").Columns("evaluation_id", "integration_id", "transaction_id", "request_id", "context_id", "contract_revision", "request_fingerprint", "validation_mode", "policy_id", "policy_revision", "policy_snapshot", "response", "created_at").
		Values(d.Result.EvaluationID, d.Key.IntegrationID, d.Key.TransactionID, d.Key.RequestID, d.ContextID, d.Result.ContractRevision, d.Fingerprint[:], d.ValidationMode, policyID, policyRevision, policySnapshot, string(response), d.CreatedAt.UTC()).Suffix("ON CONFLICT DO NOTHING").PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build decision insert: %w", err)
	}

	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23514" && pgErr.ConstraintName == "reserve_operation_closed" {
			return constant.ErrReserveOperationConflict
		}

		return fmt.Errorf("insert reserve decision: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read decision insert count: %w", err)
	}

	if count != 1 {
		return constant.ErrReserveDecisionConflict
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Reserve decision inserted in transaction")

	return nil
}

func recordReserveDecisionRepositoryError(span trace.Span, err error) {
	if err == nil {
		return
	}

	if errors.Is(err, constant.ErrInvalidRequestBody) || errors.Is(err, constant.ErrReserveDecisionConflict) || errors.Is(err, constant.ErrReserveOperationConflict) {
		libOtel.HandleSpanBusinessErrorEvent(span, "invalid reserve decision operation", err)
		return
	}

	libOtel.HandleSpanError(span, "reserve decision repository failed", err)
}
