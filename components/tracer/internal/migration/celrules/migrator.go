// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package celrules holds the one-shot migration that rewrites the GLOBAL
// currency reference to asset in every stored CEL rule expression.
//
// It runs OUTSIDE the numbered golang-migrate .sql sequence: the rewrite is
// applied to rule text (not schema), so it needs the CEL rewriter and the new
// (asset) environment, neither of which is available to a SQL migration. The
// job is the logical "000023" release step and runs in the same coordinated,
// stop-the-world / drained release as the schema rename — there is no
// currency->asset alias, so the CEL env variable and the stored rules must both
// be asset before any instance serves traffic.
//
// The up path is atomic: a single transaction takes a write lock on the rules
// table, rewrites and RECOMPILES every non-deleted rule against the new
// environment, and commits only if all recompile. Any failure rolls the whole
// transaction back, so no partial rewrite is ever persisted.
//
// The job is SINGLE-TENANT only for now. It resolves the rules through the
// static connection and has no per-tenant fan-out, so it refuses to run under
// MULTI_TENANT_ENABLED=true (ErrMultiTenantUnsupported) rather than leave a
// multi-tenant deployment half-migrated. A per-tenant execution context is a
// documented follow-up; the rollout stays stop-the-world / drained.
//
// The down path is IRREVERSIBLE and fails loud. The rules table stores no
// provenance of the rewrite, so a rule authored or edited with asset is
// indistinguishable from a migrated one; a blind asset->currency rewrite would
// corrupt legitimately-authored rules. The job therefore refuses to run down.
package celrules

//go:generate mockgen -source=migrator.go -destination=migrator_mock.go -package=celrules

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOtel "github.com/LerianStudio/lib-observability/v2/tracing"
	"go.opentelemetry.io/otel/attribute"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// lockRulesStmt takes a table-level write lock for the duration of the
// migration transaction. SHARE ROW EXCLUSIVE conflicts with every write lock
// (ROW EXCLUSIVE and stronger) so concurrent rule writes block until commit or
// rollback, while plain reads (ACCESS SHARE) are still allowed.
const lockRulesStmt = `LOCK TABLE rules IN SHARE ROW EXCLUSIVE MODE`

// ErrDownIrreversible is returned by Down. It is a job-control signal, not an
// API/domain error code: the caller (cmd entrypoint) maps it to a non-zero exit.
var ErrDownIrreversible = errors.New(
	"irreversible: no rule-rewrite provenance is stored, so a migrated asset rule cannot be distinguished from an authored one; refusing to rewrite asset back to currency",
)

// ErrMultiTenantUnsupported aborts the job when MULTI_TENANT_ENABLED=true. It
// is a job-control signal, not an API/domain error code: the job has no
// per-tenant fan-out, so running it in multi-tenant mode would leave tenants
// half-migrated. Per-tenant execution is a documented follow-up.
var ErrMultiTenantUnsupported = errors.New(
	"multi-tenant CEL rule migration is not supported yet; per-tenant execution is a follow-up. Refusing to run under MULTI_TENANT_ENABLED=true to avoid a half-migrated multi-tenant deployment",
)

// RuleStore is the subset of the rule repository the migration needs: read all
// rules, and update a rule inside a caller-provided transaction so the rewrite
// commits atomically with the table lock. Satisfied by *postgres.Repository.
type RuleStore interface {
	ListByStatus(ctx context.Context, status *model.RuleStatus) ([]*model.Rule, error)
	UpdateWithTx(ctx context.Context, db pgdb.DB, rule *model.Rule) error
}

// ExpressionCompiler compiles a rewritten expression against the NEW (asset)
// CEL environment. A non-nil error means the rewrite does not compile there
// (e.g. an undeclared reference) and MUST abort the migration.
type ExpressionCompiler interface {
	Compile(expression string) error
}

// Rewriter renames the global currency reference to asset in a single stored
// expression, preserving field selections, string literals, and shadowing
// comprehension bindings. Satisfied by cel.RewriteCurrencyToAsset.
type Rewriter func(expression string) (string, error)

// Result reports what the up migration did. It carries only counts — never rule
// expressions, which may encode business logic.
type Result struct {
	Scanned   int
	Rewritten int
	Unchanged int
}

// Migrator applies the currency->asset rewrite to all stored rules atomically.
type Migrator struct {
	txBeginner         pgdb.TxBeginner
	store              RuleStore
	compiler           ExpressionCompiler
	rewrite            Rewriter
	multiTenantEnabled bool
}

// NewMigrator wires the migration collaborators. All collaborators are
// required. multiTenantEnabled reflects MULTI_TENANT_ENABLED: when true, Up
// refuses (ErrMultiTenantUnsupported) before touching the database.
func NewMigrator(txBeginner pgdb.TxBeginner, store RuleStore, compiler ExpressionCompiler, rewrite Rewriter, multiTenantEnabled bool) (*Migrator, error) {
	if txBeginner == nil {
		return nil, errors.New("txBeginner is required")
	}

	if store == nil {
		return nil, errors.New("rule store is required")
	}

	if compiler == nil {
		return nil, errors.New("expression compiler is required")
	}

	if rewrite == nil {
		return nil, errors.New("rewriter is required")
	}

	return &Migrator{
		txBeginner:         txBeginner,
		store:              store,
		compiler:           compiler,
		rewrite:            rewrite,
		multiTenantEnabled: multiTenantEnabled,
	}, nil
}

// Up rewrites every non-deleted rule's currency reference to asset in one
// atomic transaction: lock, rewrite-all, recompile-all gate, persist-all,
// commit. Any error rolls back the whole transaction, so a failed recompile
// leaves the rule table untouched.
//
// In multi-tenant mode Up refuses with ErrMultiTenantUnsupported before opening
// any transaction, so no database work happens.
func (m *Migrator) Up(ctx context.Context) (Result, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "migration.celrules.up")
	defer span.End()

	var res Result

	if m.multiTenantEnabled {
		libOtel.HandleSpanError(span, "Refusing multi-tenant CEL rule migration", ErrMultiTenantUnsupported)
		logger.Log(ctx, libLog.LevelError, "Refusing multi-tenant CEL rule migration", libLog.Err(ErrMultiTenantUnsupported))

		return res, ErrMultiTenantUnsupported
	}

	tx, err := m.txBeginner.BeginTx(ctx, nil)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to begin migration transaction", err)
		return res, fmt.Errorf("begin transaction: %w", err)
	}

	if tx == nil {
		err := errors.New("BeginTx returned nil transaction without error")
		libOtel.HandleSpanError(span, "Nil migration transaction", err)

		return res, err
	}

	committed := false

	defer func() {
		if committed {
			return
		}

		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			logger.Log(ctx, libLog.LevelError, "Failed to roll back migration transaction", libLog.Err(rbErr))
		}
	}()

	if _, err := tx.ExecContext(ctx, lockRulesStmt); err != nil {
		libOtel.HandleSpanError(span, "Failed to lock rules table", err)
		return res, fmt.Errorf("lock rules table: %w", err)
	}

	rules, err := m.store.ListByStatus(ctx, nil)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to load rules", err)
		return res, fmt.Errorf("load rules: %w", err)
	}

	res.Scanned = len(rules)

	// Rewrite and recompile EVERY rule before persisting any. A single failed
	// recompile aborts here, before the first write, so the deferred rollback
	// leaves the table untouched.
	toUpdate := make([]*model.Rule, 0, len(rules))

	for _, rule := range rules {
		// An empty or whitespace-only expression has no currency reference to
		// rewrite; treat it as unchanged rather than letting the rewriter reject
		// it (ErrExpressionSyntax) and abort the whole migration.
		if strings.TrimSpace(rule.Expression) == "" {
			res.Unchanged++
			continue
		}

		rewritten, err := m.rewrite(rule.Expression)
		if err != nil {
			// The rewrite error wraps cel-go output that can embed the rule
			// expression; keep that detail at Debug and put only the sentinel
			// and rule ID on the operator-facing span and log.
			boundedErr := fmt.Errorf("%w for rule %s", constant.ErrExpressionSyntax, rule.ID)
			libOtel.HandleSpanError(span, "Failed to rewrite rule expression", boundedErr)
			logger.Log(ctx, libLog.LevelError, "Failed to rewrite rule expression", libLog.String("rule_id", rule.ID.String()), libLog.Err(constant.ErrExpressionSyntax))
			logger.Log(ctx, libLog.LevelDebug, "Rule expression rewrite error detail", libLog.String("rule_id", rule.ID.String()), libLog.Err(err))

			return res, fmt.Errorf("rewrite rule %s: %w", rule.ID, err)
		}

		if err := m.compiler.Compile(rewritten); err != nil {
			// The compiler error can echo the rewritten expression; keep it at
			// Debug and put only a bounded classification plus rule ID on the
			// operator-facing span and log.
			boundedErr := fmt.Errorf("rewritten expression failed the recompile-all gate for rule %s", rule.ID)
			libOtel.HandleSpanError(span, "Rewritten expression failed the recompile-all gate", boundedErr)
			logger.Log(ctx, libLog.LevelError, "Rewritten expression failed the recompile-all gate", libLog.String("rule_id", rule.ID.String()))
			logger.Log(ctx, libLog.LevelDebug, "Recompile-all gate error detail", libLog.String("rule_id", rule.ID.String()), libLog.Err(err))

			return res, fmt.Errorf("recompile gate failed for rule %s: %w", rule.ID, err)
		}

		if rewritten == rule.Expression {
			res.Unchanged++
			continue
		}

		rule.Expression = rewritten
		toUpdate = append(toUpdate, rule)
	}

	for _, rule := range toUpdate {
		if err := m.store.UpdateWithTx(ctx, tx, rule); err != nil {
			libOtel.HandleSpanError(span, "Failed to persist rewritten rule", err)
			return res, fmt.Errorf("persist rule %s: %w", rule.ID, err)
		}
	}

	res.Rewritten = len(toUpdate)

	if err := tx.Commit(); err != nil {
		libOtel.HandleSpanError(span, "Failed to commit migration", err)
		return res, fmt.Errorf("commit migration: %w", err)
	}

	committed = true

	span.SetAttributes(
		attribute.Int("app.migration.rules_scanned", res.Scanned),
		attribute.Int("app.migration.rules_rewritten", res.Rewritten),
		attribute.Int("app.migration.rules_unchanged", res.Unchanged),
	)

	logger.Log(
		ctx, libLog.LevelInfo, "CEL stored-rule migration committed",
		libLog.Int("rules_scanned", res.Scanned),
		libLog.Int("rules_rewritten", res.Rewritten),
		libLog.Int("rules_unchanged", res.Unchanged),
	)

	return res, nil
}

// Down refuses to run: the rewrite is irreversible without stored provenance.
// It performs no I/O and always returns ErrDownIrreversible so the caller exits
// non-zero instead of silently corrupting authored asset rules.
func (m *Migrator) Down(ctx context.Context) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "migration.celrules.down")
	defer span.End()

	libOtel.HandleSpanError(span, "Refusing irreversible down migration", ErrDownIrreversible)
	logger.Log(ctx, libLog.LevelError, "Refusing to run irreversible down migration", libLog.Err(ErrDownIrreversible))

	return ErrDownIrreversible
}
