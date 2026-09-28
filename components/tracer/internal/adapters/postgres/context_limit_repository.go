// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// ContextLimitRepositoryConfig bounds the exhaustive candidate snapshot. Scope
// bytes are bounded in SQL before JSON decoding; excess is never truncated.
type ContextLimitRepositoryConfig struct {
	MaxAccounts   int
	MaxLimits     int
	MaxScopes     int
	MaxScopeBytes int
}

// ContextLimitRepository has no fallback pool: each operation requires the
// caller's tenant-primary transaction. It neither authorizes producers/admins,
// proves official account ownership, writes audit nor commits the transaction.
type ContextLimitRepository struct {
	config     ContextLimitRepositoryConfig
	fetchLimit uint64
}

func NewContextLimitRepository(config ContextLimitRepositoryConfig) (*ContextLimitRepository, error) {
	maximum := config.MaxLimits
	if maximum <= 0 || config.MaxAccounts <= 0 || config.MaxScopes <= 0 || config.MaxScopeBytes <= 0 {
		return nil, constant.ErrInvalidRequestBody
	}

	return &ContextLimitRepository{config: config, fetchLimit: uint64(maximum) + 1}, nil
}

// ListCandidatesWithTx reads every potentially applicable ACTIVE limit. Assets
// must be the distinct debit codes that satisfy the asset code rule (other
// codes can never name a limit) and account IDs the distinct internal debit
// accounts. A limit applies by exact asset code equality; broad or
// unsupported scopes are retained for validation, not filtered by current time.
// Limits of other asset codes and limits naming only other accounts are excluded.
//
// Rows are locked FOR SHARE OF limits in UUID order. The admission caller must
// acquire its operation/account locks before calling, then counter/audit locks;
// this method does not lock accounts or prevent a later limit insertion. The
// statement's snapshot defines the selected set. Every error requires rollback.
func (r *ContextLimitRepository) ListCandidatesWithTx(ctx context.Context, tx pgdb.Tx, assets []string, accountIDs []uuid.UUID) (_ []model.ContextAccountLimit, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_context_limits")
	defer span.End()
	defer func() { recordContextLimitRepositoryError(span, retErr) }()

	if tx == nil {
		return nil, pgdb.ErrNilConnection
	}

	accounts, err := r.validateLookup(assets, accountIDs)
	if err != nil {
		return nil, err
	}

	if len(accounts) == 0 || len(assets) == 0 {
		return []model.ContextAccountLimit{}, nil
	}

	statement, args, err := r.candidateQuery(assets, accounts).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build context limit snapshot: %w", err)
	}

	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("read context limit snapshot: %w", err)
	}
	defer rows.Close()

	limits := make([]model.ContextAccountLimit, 0)

	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if len(limits) >= r.config.MaxLimits {
			return nil, constant.ErrContextLimitsUnavailable
		}

		limit, err := r.scanCandidate(rows)
		if err != nil {
			return nil, err
		}

		limits = append(limits, limit)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate context limits: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logging.WithTrace(ctx, logger).With(libLog.Int("limits.count", len(limits))).Log(ctx, libLog.LevelDebug, "Complete context limit snapshot read")

	return limits, nil
}

// validateLookup accepts accounts without assets: debits whose codes cannot
// name a limit leave nothing to look up. Assets without accounts are invalid.
func (r *ContextLimitRepository) validateLookup(assets []string, accountIDs []uuid.UUID) ([]string, error) {
	if len(accountIDs) > r.config.MaxAccounts || len(assets) > len(accountIDs) {
		return nil, constant.ErrInvalidRequestBody
	}

	codes := make(map[string]struct{}, len(assets))
	for _, asset := range assets {
		if _, duplicate := codes[asset]; duplicate || utils.ValidateAssetCode(asset) != nil {
			return nil, constant.ErrInvalidRequestBody
		}

		codes[asset] = struct{}{}
	}

	seen := make(map[uuid.UUID]struct{}, len(accountIDs))

	accounts := make([]string, 0, len(accountIDs))
	for _, id := range accountIDs {
		if _, duplicate := seen[id]; duplicate || id == uuid.Nil {
			return nil, constant.ErrInvalidRequestBody
		}

		seen[id] = struct{}{}
		accounts = append(accounts, id.String())
	}

	return accounts, nil
}

func (r *ContextLimitRepository) limitSnapshotQuery() sq.SelectBuilder {
	return sq.Select("l.id", "l.name", "l.description", "l.limit_type", "l.max_amount", "l.asset").
		Column(sq.Expr(`CASE WHEN jsonb_typeof(l.scopes)='array' THEN
   CASE WHEN jsonb_array_length(l.scopes)<=? AND octet_length(l.scopes::text)<=? THEN l.scopes END ELSE NULL END`, r.config.MaxScopes, r.config.MaxScopeBytes)).
		Columns("l.status", "l.reset_at", "l.active_time_start", "l.active_time_end", "l.custom_start_date", "l.custom_end_date", "l.created_at", "l.updated_at", "l.deleted_at").
		From("limits l")
}

func (r *ContextLimitRepository) candidateQuery(assets, accounts []string) sq.SelectBuilder {
	return r.limitSnapshotQuery().
		Where(sq.Eq{"l.status": model.LimitStatusActive, "l.deleted_at": nil, "l.asset": assets}).
		Where(sq.Expr(`CASE WHEN jsonb_typeof(l.scopes)='array' THEN
   jsonb_array_length(l.scopes)=0 OR EXISTS (
    SELECT 1 FROM jsonb_array_elements(l.scopes) AS scope
    WHERE scope->>'accountId' IS NULL OR CASE
     WHEN pg_input_is_valid(scope->>'accountId','uuid') THEN (scope->>'accountId')::uuid=ANY(?::uuid[])
     ELSE true END
   ) ELSE true END`, accounts)).
		OrderBy("l.id").Limit(r.fetchLimit).Suffix("FOR SHARE OF l").PlaceholderFormat(sq.Dollar)
}

func (r *ContextLimitRepository) scanCandidate(rows *sql.Rows) (model.ContextAccountLimit, error) {
	var (
		stored LimitPostgreSQLModel
		scopes []byte
	)
	if err := rows.Scan(&stored.ID, &stored.Name, &stored.Description, &stored.LimitType, &stored.MaxAmount, &stored.Asset,
		&scopes, &stored.Status, &stored.ResetAt, &stored.ActiveTimeStart, &stored.ActiveTimeEnd, &stored.CustomStartDate, &stored.CustomEndDate,
		&stored.CreatedAt, &stored.UpdatedAt, &stored.DeletedAt); err != nil {
		return model.ContextAccountLimit{}, fmt.Errorf("scan context limit: %w", err)
	}

	if len(scopes) == 0 {
		return model.ContextAccountLimit{}, constant.ErrContextLimitsUnavailable
	}

	var decoded []model.Scope

	decoder := json.NewDecoder(bytes.NewReader(scopes))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&decoded); err != nil {
		return model.ContextAccountLimit{}, constant.ErrContextLimitsUnavailable
	}

	if len(decoded) > r.config.MaxScopes {
		return model.ContextAccountLimit{}, constant.ErrContextLimitsUnavailable
	}

	stored.Scopes = string(scopes)

	definition, err := stored.ToEntity()
	if err != nil {
		return model.ContextAccountLimit{}, constant.ErrContextLimitsUnavailable
	}

	if utils.ValidateAssetCode(definition.Asset) != nil {
		return model.ContextAccountLimit{}, constant.ErrContextLimitsUnavailable
	}

	return model.ContextAccountLimit{Definition: *definition}, nil
}

func recordContextLimitRepositoryError(span trace.Span, err error) {
	if err == nil {
		return
	}

	if errors.Is(err, constant.ErrInvalidRequestBody) {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid context limit lookup", err)
		return
	}

	libOtel.HandleSpanError(span, "Context limit repository failed", err)
}
