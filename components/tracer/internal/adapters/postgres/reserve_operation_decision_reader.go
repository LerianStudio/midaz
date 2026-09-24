// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	sq "github.com/Masterminds/squirrel"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// GetByOperationWithTx resolves only the verified operation's immutable decision.
// Completion does not require the original request ID. The caller must acquire
// the operation lock first; a missing decision is nil and never an implicit ALLOW.
func (r *ReserveDecisionRepository) GetByOperationWithTx(ctx context.Context, tx pgdb.Tx, identity model.ReserveOperationIdentity) (_ *model.ReserveDecision, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.get_operation_decision")
	defer span.End()
	defer func() { recordReserveDecisionRepositoryError(span, retErr) }()

	if tx == nil {
		return nil, pgdb.ErrNilConnection
	}

	if err := identity.Validate(); err != nil {
		return nil, err
	}

	statement, args, err := sq.Select("integration_id", "transaction_id", "request_id", "context_id", "request_fingerprint", "validation_mode", "policy_snapshot", "response", "created_at").
		From("reserve_decisions").Where(sq.Eq{"integration_id": identity.IntegrationID, "transaction_id": identity.TransactionID}).
		Limit(2).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build operation decision lookup: %w", err)
	}

	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("read operation decision: %w", err)
	}
	defer rows.Close()

	var found *model.ReserveDecision

	for rows.Next() {
		d, err := r.scan(rows)
		if err != nil {
			return nil, err
		}

		if found != nil || d.Key.Identity() != identity {
			return nil, constant.ErrInternalServer
		}

		found = d
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operation decisions: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Operation decision read in transaction")

	return found, nil
}
