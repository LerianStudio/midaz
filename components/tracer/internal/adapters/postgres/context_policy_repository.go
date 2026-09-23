// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ContextPolicyRepository stores immutable revisions and exact bindings in the
// tenant database selected by Connection. It never accepts a tenant from a body.
// Mutations require the caller's transaction so publication, binding and the
// authorized administration audit can be committed or rolled back together.
type ContextPolicyRepository struct {
	conn     pgdb.Connection
	maxRules int
}

func NewContextPolicyRepository(conn pgdb.Connection, maxRules int) (*ContextPolicyRepository, error) {
	if conn == nil {
		return nil, pgdb.ErrNilConnection
	}

	if maxRules <= 0 {
		return nil, constant.ErrContextPolicyUnavailable
	}

	return &ContextPolicyRepository{conn: conn, maxRules: maxRules}, nil
}

// GetActive reads the binding and all its rules in one database snapshot. There
// is no wildcard, hierarchy or permissive fallback. Reads use the primary to
// avoid evaluating an obsolete binding due to replication lag.
func (r *ContextPolicyRepository) GetActive(ctx context.Context, key model.PolicyBindingKey) (_ *model.BoundContextPolicy, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.get_context_policy")
	defer span.End()
	defer func() { recordPolicyRepositoryError(span, retErr) }()

	logger = logging.WithTrace(ctx, logger)

	if err := key.Validate(); err != nil {
		return nil, err
	}

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

	maxRules := r.maxRules
	if maxRules <= 0 {
		return nil, constant.ErrContextPolicyUnavailable
	}

	query, args, err := sq.Select("b.binding_version", "p.policy_id", "p.policy_revision", "p.default_decision", "p.rule_count",
		"r.rule_id", "r.rule_revision", "r.expression", "r.action").
		From("evaluation_policy_bindings b").
		Join("evaluation_policy_revisions p ON p.policy_id = b.policy_id AND p.policy_revision = b.policy_revision").
		LeftJoin("evaluation_policy_rules m ON m.policy_id = p.policy_id AND m.policy_revision = p.policy_revision").
		LeftJoin("evaluation_rule_revisions r ON r.rule_id = m.rule_id AND r.rule_revision = m.rule_revision").
		Where(sq.Eq{"b.integration_id": key.IntegrationID, "b.context_id": key.ContextID}).
		OrderBy("r.rule_id").Limit(uint64(maxRules) + 1).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build policy query: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read policy: %w", err)
	}
	defer rows.Close()

	policy, err := scanBoundContextPolicy(rows, maxRules)
	if err != nil {
		return nil, err
	}

	logger.With(libLog.Int("rules.count", len(policy.Rules))).Log(ctx, libLog.LevelDebug, "Selected policy revision")

	return policy, nil
}

func scanBoundContextPolicy(rows *sql.Rows, maxRules int) (*model.BoundContextPolicy, error) {
	policy := &model.BoundContextPolicy{ContextPolicy: model.ContextPolicy{Rules: []model.ContextPolicyRule{}}}
	found := false

	var expected int

	for rows.Next() {
		var (
			id                 uuid.NullUUID
			revision           sql.NullInt64
			expression, action sql.NullString
		)
		if err := rows.Scan(&policy.BindingVersion, &policy.ID, &policy.Revision, &policy.DefaultDecision, &expected, &id, &revision, &expression, &action); err != nil {
			return nil, fmt.Errorf("decode policy: %w", err)
		}

		found = true

		if expected > maxRules || expected < 0 {
			return nil, constant.ErrContextPolicyUnavailable
		}

		if id.Valid {
			if !revision.Valid || !expression.Valid || !action.Valid {
				return nil, constant.ErrContextPolicyUnavailable
			}

			policy.Rules = append(policy.Rules, model.ContextPolicyRule{ID: id.UUID, Revision: revision.Int64, Expression: expression.String, Action: model.Decision(action.String)})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate policy: %w", err)
	}

	if !found || len(policy.Rules) != expected {
		return nil, constant.ErrContextPolicyUnavailable
	}

	if err := policy.Validate(maxRules); err != nil {
		return nil, fmt.Errorf("invalid stored policy: %w", constant.ErrContextPolicyUnavailable)
	}

	return policy, nil
}

// PublishWithTx inserts a complete immutable snapshot. The command must compile
// it before publication and must roll back the transaction on any error. A rule
// revision can be shared by policies only when its expression/action agree.
func (r *ContextPolicyRepository) PublishWithTx(ctx context.Context, tx pgdb.Tx, policy model.ContextPolicy, actor string, at time.Time) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.publish_context_policy")
	defer span.End()
	defer func() { recordPolicyRepositoryError(span, retErr) }()

	logger = logging.WithTrace(ctx, logger)

	if tx == nil {
		return pgdb.ErrNilConnection
	}

	if actor == "" || at.IsZero() {
		return constant.ErrInvalidRequestBody
	}

	if err := policy.Validate(r.maxRules); err != nil {
		return err
	}

	statement, args, err := sq.Insert("evaluation_policy_revisions").
		Columns("policy_id", "policy_revision", "default_decision", "rule_count", "published_by", "published_at").
		Values(policy.ID, policy.Revision, policy.DefaultDecision, len(policy.Rules), actor, at.UTC()).
		Suffix("ON CONFLICT DO NOTHING").PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build policy publication: %w", err)
	}

	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return fmt.Errorf("publish policy revision: %w", err)
	}

	if err := requirePolicyMutation(result); err != nil {
		return err
	}

	rules := slices.Clone(policy.Rules)
	slices.SortFunc(rules, func(a, b model.ContextPolicyRule) int { return bytes.Compare(a.ID[:], b.ID[:]) })

	for _, rule := range rules {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := publishContextRule(ctx, tx, policy, rule); err != nil {
			return err
		}
	}

	logger.With(libLog.Int("rules.count", len(rules))).Log(ctx, libLog.LevelDebug, "Published policy revision in transaction")

	return nil
}

func publishContextRule(ctx context.Context, tx pgdb.Tx, policy model.ContextPolicy, rule model.ContextPolicyRule) error {
	statement, args, err := sq.Insert("evaluation_rule_revisions").Columns("rule_id", "rule_revision", "expression", "action").
		Values(rule.ID, rule.Revision, rule.Expression, rule.Action).Suffix("ON CONFLICT DO NOTHING").PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build rule revision: %w", err)
	}

	if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
		return fmt.Errorf("publish rule revision: %w", err)
	}
	// The insert may wait for another publisher. A separate READ COMMITTED
	// statement sees its committed immutable row after ON CONFLICT returns.
	var identical bool
	if err := tx.QueryRowContext(ctx, "SELECT expression = $3 AND action = $4 FROM evaluation_rule_revisions WHERE rule_id = $1 AND rule_revision = $2", rule.ID, rule.Revision, rule.Expression, rule.Action).Scan(&identical); err != nil {
		return fmt.Errorf("check existing rule revision: %w", err)
	}

	if !identical {
		return constant.ErrContextPolicyConflict
	}

	statement, args, err = sq.Insert("evaluation_policy_rules").Columns("policy_id", "policy_revision", "rule_id", "rule_revision").
		Values(policy.ID, policy.Revision, rule.ID, rule.Revision).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build policy membership: %w", err)
	}

	if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
		return fmt.Errorf("publish policy membership: %w", err)
	}

	return nil
}

// BindWithTx creates a binding when expected is nil, otherwise changes it only
// when the binding version still matches expected. Stale writers never
// overwrite a concurrent activation. No implicit upsert is permitted.
func (r *ContextPolicyRepository) BindWithTx(ctx context.Context, tx pgdb.Tx, key model.PolicyBindingKey, target model.PolicyRevision, expected *int64, actor string, at time.Time) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.bind_context_policy")
	defer span.End()
	defer func() { recordPolicyRepositoryError(span, retErr) }()

	logger = logging.WithTrace(ctx, logger)

	if tx == nil {
		return pgdb.ErrNilConnection
	}

	if err := key.Validate(); err != nil {
		return err
	}

	if target.ID == uuid.Nil || target.Revision <= 0 || actor == "" || at.IsZero() {
		return constant.ErrInvalidRequestBody
	}

	if expected != nil && *expected <= 0 {
		return constant.ErrInvalidRequestBody
	}

	var (
		statement string
		args      []any
		err       error
	)
	if expected == nil {
		statement, args, err = sq.Insert("evaluation_policy_bindings").
			Columns("integration_id", "context_id", "policy_id", "policy_revision", "updated_by", "updated_at").
			Values(key.IntegrationID, key.ContextID, target.ID, target.Revision, actor, at.UTC()).
			Suffix("ON CONFLICT DO NOTHING").PlaceholderFormat(sq.Dollar).ToSql()
	} else {
		statement, args, err = sq.Update("evaluation_policy_bindings").Set("policy_id", target.ID).Set("policy_revision", target.Revision).
			Set("binding_version", sq.Expr("binding_version + 1")).Set("updated_by", actor).Set("updated_at", at.UTC()).Where(sq.Eq{"integration_id": key.IntegrationID, "context_id": key.ContextID, "binding_version": *expected}).PlaceholderFormat(sq.Dollar).ToSql()
	}

	if err != nil {
		return fmt.Errorf("build policy binding: %w", err)
	}

	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return fmt.Errorf("write policy binding: %w", err)
	}

	if err := requirePolicyMutation(result); err != nil {
		return err
	}

	logger.Log(ctx, libLog.LevelDebug, "Policy binding updated in transaction")

	return nil
}

func requirePolicyMutation(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read policy mutation count: %w", err)
	}

	if count != 1 {
		return constant.ErrContextPolicyConflict
	}

	return nil
}

func recordPolicyRepositoryError(span trace.Span, err error) {
	if err == nil {
		return
	}

	classified := pkg.ValidateBusinessError(err, constant.EntityRule)
	if errors.Is(err, constant.ErrInvalidRequestBody) {
		classified = pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), Err: err}
	}

	if pkg.IsBusinessError(classified) {
		libOtel.HandleSpanBusinessErrorEvent(span, "invalid policy publication", err)
		return
	}

	libOtel.HandleSpanError(span, "policy repository failed", err)
}
