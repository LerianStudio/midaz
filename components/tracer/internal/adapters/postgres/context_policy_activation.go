// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// GetRevision loads a complete, published revision from the tenant primary.
// Compilation can happen outside a transaction because published content is
// immutable. This lookup never selects or creates an active binding.
func (r *ContextPolicyRepository) GetRevision(ctx context.Context, ref model.PolicyRevision) (_ *model.ContextPolicy, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.get_policy_revision")
	defer span.End()
	defer func() { recordPolicyRepositoryError(span, retErr) }()

	logger = logging.WithTrace(ctx, logger)

	if ref.ID == uuid.Nil || ref.Revision <= 0 {
		return nil, constant.ErrInvalidRequestBody
	}

	if r.maxRules <= 0 {
		return nil, constant.ErrContextPolicyUnavailable
	}

	db, err := r.policyDatabase(ctx)
	if err != nil {
		return nil, err
	}

	statement, args, err := sq.Select("p.policy_id", "p.policy_revision", "p.default_decision", "p.rule_count",
		"r.rule_id", "r.rule_revision", "r.expression", "r.action").
		From("evaluation_policy_revisions p").
		LeftJoin("evaluation_policy_rules m ON m.policy_id = p.policy_id AND m.policy_revision = p.policy_revision").
		LeftJoin("evaluation_rule_revisions r ON r.rule_id = m.rule_id AND r.rule_revision = m.rule_revision").
		Where(sq.Eq{"p.policy_id": ref.ID, "p.policy_revision": ref.Revision}).
		OrderBy("r.rule_id").Limit(uint64(r.maxRules) + 1).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build revision query: %w", err)
	}

	rows, err := db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("read policy revision: %w", err)
	}
	defer rows.Close()

	snapshot, err := scanContextPolicy(rows, r.maxRules, nil)
	if err != nil {
		return nil, err
	}

	logger.With(libLog.Int("rules.count", len(snapshot.Rules))).Log(ctx, libLog.LevelDebug, "Loaded published policy revision")

	return snapshot, nil
}

// GetBindingWithTx locks an existing binding before capturing its audited prior
// state. Absence returns nil: concurrent creates are resolved by BindWithTx's
// unique insert. Lock order is binding row first, then the audit hash-chain lock.
func (r *ContextPolicyRepository) GetBindingWithTx(ctx context.Context, tx pgdb.Tx, key model.PolicyBindingKey) (_ *model.PolicyBindingState, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.lock_policy_binding")
	defer span.End()
	defer func() { recordPolicyRepositoryError(span, retErr) }()

	logger = logging.WithTrace(ctx, logger)

	if tx == nil {
		return nil, pgdb.ErrNilConnection
	}

	if err := key.Validate(); err != nil {
		return nil, err
	}

	statement, args, err := sq.Select("policy_id", "policy_revision", "binding_version").
		From("evaluation_policy_bindings").Where(sq.Eq{"integration_id": key.IntegrationID, "context_id": key.ContextID}).
		Suffix("FOR UPDATE").PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build binding lookup: %w", err)
	}

	var state model.PolicyBindingState
	if err := tx.QueryRowContext(ctx, statement, args...).Scan(&state.Policy.ID, &state.Policy.Revision, &state.Version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("lock policy binding: %w", err)
	}

	logger.Log(ctx, libLog.LevelDebug, "Loaded prior policy binding")

	return &state, nil
}

func (r *ContextPolicyRepository) policyDatabase(ctx context.Context) (pgdb.DB, error) {
	db, err := r.conn.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve policy database: %w", err)
	}

	if primary, ok := db.(interface{ ReadWrite() *sql.DB }); ok {
		primaryDB := primary.ReadWrite()
		if primaryDB == nil {
			return nil, pgdb.ErrNilConnection
		}

		db = primaryDB
	}

	if db == nil {
		return nil, pgdb.ErrNilConnection
	}

	return db, nil
}
