// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"

	pgdb "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// ruleSyncColumns defines the column list for rule sync queries.
// Shared between GetAllActiveRules and GetRulesUpdatedSince to keep in sync with scanRulesFromRows.
var ruleSyncColumns = []string{
	"id", "name", "description", "expression", "action", "scopes",
	"status", "created_at", "updated_at", "activated_at", "deactivated_at", "deleted_at",
}

// RuleSyncRepository provides database queries for the cache sync system.
// Tenant resolution is handled by the underlying pgdb.Connection (M1).
type RuleSyncRepository struct {
	conn pgdb.Connection
}

// NewRuleSyncRepositoryWithConnection creates a new RuleSyncRepository with a custom pgdb.Connection.
// Intended for testing with mock or integration database connections.
func NewRuleSyncRepositoryWithConnection(conn pgdb.Connection) *RuleSyncRepository {
	return &RuleSyncRepository{
		conn: conn,
	}
}

// GetAllActiveRules retrieves all rules with status=ACTIVE.
func (r *RuleSyncRepository) GetAllActiveRules(ctx context.Context) ([]*model.Rule, error) {
	db, err := r.conn.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	query := sq.Select(ruleSyncColumns...).
		From(tableName).
		Where(sq.Eq{"status": model.RuleStatusActive}).
		Where(sq.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query active rules: %w", err)
	}
	defer rows.Close()

	return r.scanRulesFromRows(ctx, rows)
}

// GetRulesUpdatedSince retrieves all rules updated at or after the given timestamp.
// Returns ALL statuses to detect deactivations/deletions.
func (r *RuleSyncRepository) GetRulesUpdatedSince(ctx context.Context, since time.Time) ([]*model.Rule, error) {
	db, err := r.conn.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	query := sq.Select(ruleSyncColumns...).
		From(tableName).
		Where(sq.GtOrEq{"updated_at": since}).
		OrderBy("updated_at ASC").
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query updated rules: %w", err)
	}
	defer rows.Close()

	return r.scanRulesFromRows(ctx, rows)
}

// scanRulesFromRows scans all rows into model.Rule using the same pattern
// as Repository.scanRuleFromRows (12-column scan + RulePostgreSQLModel + ToEntity).
func (r *RuleSyncRepository) scanRulesFromRows(ctx context.Context, rows *sql.Rows) ([]*model.Rule, error) {
	rules := make([]*model.Rule, 0, 64)

	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("context cancelled during scan: %w", err)
		}

		var (
			dbModel    RulePostgreSQLModel
			scopesJSON []byte
		)

		err := rows.Scan(
			&dbModel.ID, &dbModel.Name, &dbModel.Description,
			&dbModel.Expression, &dbModel.Action, &scopesJSON,
			&dbModel.Status, &dbModel.CreatedAt, &dbModel.UpdatedAt,
			&dbModel.ActivatedAt, &dbModel.DeactivatedAt, &dbModel.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan rule row: %w", err)
		}

		dbModel.Scopes = string(scopesJSON)

		rule, err := dbModel.ToEntity()
		if err != nil {
			return nil, fmt.Errorf("failed to convert rule model to entity: %w", err)
		}

		if rule == nil {
			continue // defense-in-depth: skip nil entity from ToEntity
		}

		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return rules, nil
}
