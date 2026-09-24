// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

// GetForAssetBindingWithTx locks the current definition before checking producer
// facts. Unlike admission, administration may prepare a DRAFT or INACTIVE limit.
// Existing references are returned only to the authorized command, which must
// reject replacement without exposing the previous producer's reference.
func (r *ContextLimitRepository) GetForAssetBindingWithTx(ctx context.Context, tx pgdb.Tx, limitID uuid.UUID) (_ *model.ContextAccountLimit, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.get_limit_for_asset_binding")
	defer span.End()
	defer func() { recordContextLimitRepositoryError(span, retErr) }()

	if tx == nil {
		return nil, pgdb.ErrNilConnection
	}

	if limitID == uuid.Nil {
		return nil, constant.ErrInvalidRequestBody
	}

	statement, args, err := r.limitSnapshotQuery().Where(sq.Eq{"l.id": limitID, "l.deleted_at": nil}).Limit(1).Suffix("FOR UPDATE OF l").PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build limit binding snapshot: %w", err)
	}

	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("read limit binding snapshot: %w", err)
	}
	defer rows.Close()

	var result *model.ContextAccountLimit

	if rows.Next() {
		limit, err := r.scanCandidate(rows)
		if err != nil {
			return nil, err
		}

		result = &limit
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate limit binding snapshot: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if result == nil {
		return nil, constant.ErrLimitNotFound
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Limit definition locked for asset association")

	return result, nil
}
