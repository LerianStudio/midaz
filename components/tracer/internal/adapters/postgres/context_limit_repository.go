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
	"strings"
	"unicode/utf8"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// ContextLimitRepositoryConfig bounds the exhaustive candidate snapshot. Scope
// bytes are bounded in SQL before JSON decoding; excess is never truncated.
type ContextLimitRepositoryConfig struct {
	MaxAccounts   int
	MaxLimits     int
	MaxScopes     int
	MaxScopeBytes int
	MaxTextBytes  int
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
	if maximum <= 0 || config.MaxAccounts <= 0 || config.MaxScopes <= 0 || config.MaxScopeBytes <= 0 || config.MaxTextBytes <= 0 {
		return nil, constant.ErrInvalidRequestBody
	}

	return &ContextLimitRepository{config: config, fetchLimit: uint64(maximum) + 1}, nil
}

// BindAssetWithTx associates a limit exactly once without replacing financial
// history. The caller must resolve every scoped account's official asset,
// authorize the namespace, validate eligibility and write mandatory audit in
// this same transaction. The composite FK enforces the stored code and prevents
// later code changes. Duplicate writes conflict even when values are identical;
// command-level idempotency is not inferred from this persistence operation.
func (r *ContextLimitRepository) BindAssetWithTx(ctx context.Context, tx pgdb.Tx, limitID uuid.UUID, asset tracercontract.AssetRef) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.bind_limit_asset")
	defer span.End()
	defer func() { recordContextLimitRepositoryError(span, retErr) }()

	if tx == nil {
		return pgdb.ErrNilConnection
	}

	if limitID == uuid.Nil {
		return constant.ErrInvalidRequestBody
	}

	if err := asset.Validate(asset.Namespace, r.config.MaxTextBytes); err != nil {
		return err
	}

	statement, args, err := sq.Insert("limit_asset_references").Columns("limit_id", "asset_namespace", "asset_id", "asset_code").
		Values(limitID, asset.Namespace, asset.ID, asset.Code).Suffix("ON CONFLICT (limit_id) DO NOTHING").PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build limit asset association: %w", err)
	}

	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == "limit_asset_reference_limit_fk" {
			return constant.ErrContextLimitsUnavailable
		}

		return fmt.Errorf("insert limit asset association: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count limit asset association: %w", err)
	}

	if count == 0 {
		return constant.ErrLimitAssetReferenceConflict
	}

	if count != 1 {
		return constant.ErrInternalServer
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Limit asset associated in transaction")

	return nil
}

// ListCandidatesWithTx reads every potentially applicable ACTIVE limit. Account
// IDs must be the distinct internal debit accounts; namespace must come from
// verified integration configuration. Unmapped candidates and broad/unsupported
// scopes are retained for validation, not filtered by currency or current time.
// Foreign mapped namespaces and limits naming only other accounts are excluded.
//
// Rows are locked FOR SHARE OF limits in UUID order. The admission caller must
// acquire its operation/account locks before calling, then counter/audit locks;
// this method does not lock accounts or prevent a later limit insertion. The
// statement's snapshot defines the selected set. Every error requires rollback.
func (r *ContextLimitRepository) ListCandidatesWithTx(ctx context.Context, tx pgdb.Tx, namespace string, accountIDs []uuid.UUID) (_ []model.ContextAccountLimit, retErr error) {
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

	accounts, err := r.validateLookup(namespace, accountIDs)
	if err != nil {
		return nil, err
	}

	if len(accounts) == 0 {
		return []model.ContextAccountLimit{}, nil
	}

	statement, args, err := r.candidateQuery(namespace, accounts).ToSql()
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

		limit, err := r.scanCandidate(rows, namespace)
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

func (r *ContextLimitRepository) validateLookup(namespace string, accountIDs []uuid.UUID) ([]string, error) {
	if len(namespace) == 0 || len(namespace) > r.config.MaxTextBytes || strings.TrimSpace(namespace) != namespace ||
		!utf8.ValidString(namespace) || strings.ContainsRune(namespace, '\x00') || len(accountIDs) > r.config.MaxAccounts {
		return nil, constant.ErrInvalidRequestBody
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

func (r *ContextLimitRepository) candidateQuery(namespace string, accounts []string) sq.SelectBuilder {
	return sq.Select("l.id", "l.name", "l.description", "l.limit_type", "l.max_amount", "l.asset").
		Column(sq.Expr(`CASE WHEN jsonb_typeof(l.scopes)='array' THEN
   CASE WHEN jsonb_array_length(l.scopes)<=? AND octet_length(l.scopes::text)<=? THEN l.scopes END ELSE NULL END`, r.config.MaxScopes, r.config.MaxScopeBytes)).
		Columns("l.status", "l.reset_at", "l.active_time_start", "l.active_time_end", "l.custom_start_date", "l.custom_end_date", "l.created_at", "l.updated_at", "l.deleted_at").
		Column(sq.Expr("CASE WHEN octet_length(a.asset_namespace)<=? THEN a.asset_namespace ELSE '' END", r.config.MaxTextBytes)).
		Column(sq.Expr("CASE WHEN octet_length(a.asset_id)<=? THEN a.asset_id ELSE '' END", r.config.MaxTextBytes)).
		Column(sq.Expr("CASE WHEN octet_length(a.asset_code)<=? THEN a.asset_code ELSE '' END", r.config.MaxTextBytes)).
		Columns("a.limit_id").From("limits l").LeftJoin("limit_asset_references a ON a.limit_id=l.id").
		Where(sq.Eq{"l.status": model.LimitStatusActive, "l.deleted_at": nil}).
		Where(sq.Or{sq.Eq{"a.limit_id": nil}, sq.Eq{"a.asset_namespace": namespace}}).
		Where(sq.Expr(`CASE WHEN jsonb_typeof(l.scopes)='array' THEN
   jsonb_array_length(l.scopes)=0 OR EXISTS (
    SELECT 1 FROM jsonb_array_elements(l.scopes) AS scope
    WHERE scope->>'accountId' IS NULL OR CASE
     WHEN pg_input_is_valid(scope->>'accountId','uuid') THEN (scope->>'accountId')::uuid=ANY(?::uuid[])
     ELSE true END
   ) ELSE true END`, accounts)).
		OrderBy("l.id").Limit(r.fetchLimit).Suffix("FOR SHARE OF l").PlaceholderFormat(sq.Dollar)
}

func (r *ContextLimitRepository) scanCandidate(rows *sql.Rows, namespace string) (model.ContextAccountLimit, error) {
	var (
		stored              LimitPostgreSQLModel
		scopes              []byte
		ns, id, code, owner sql.NullString
	)
	if err := rows.Scan(&stored.ID, &stored.Name, &stored.Description, &stored.LimitType, &stored.MaxAmount, &stored.Asset,
		&scopes, &stored.Status, &stored.ResetAt, &stored.ActiveTimeStart, &stored.ActiveTimeEnd, &stored.CustomStartDate, &stored.CustomEndDate,
		&stored.CreatedAt, &stored.UpdatedAt, &stored.DeletedAt, &ns, &id, &code, &owner); err != nil {
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

	limit := model.ContextAccountLimit{Definition: *definition}
	if owner.Valid {
		limit.Asset = tracercontract.AssetRef{Namespace: ns.String, ID: id.String, Code: code.String}
		if err := limit.Asset.Validate(namespace, r.config.MaxTextBytes); err != nil {
			return model.ContextAccountLimit{}, constant.ErrContextLimitsUnavailable
		}

		if limit.Asset.Code != definition.Asset {
			return model.ContextAccountLimit{}, constant.ErrContextLimitsUnavailable
		}
	}

	return limit, nil
}

func recordContextLimitRepositoryError(span trace.Span, err error) {
	if err == nil {
		return
	}

	if errors.Is(err, constant.ErrInvalidRequestBody) || errors.Is(err, constant.ErrLimitAssetReferenceConflict) {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid limit asset operation", err)
		return
	}

	libOtel.HandleSpanError(span, "Context limit repository failed", err)
}
