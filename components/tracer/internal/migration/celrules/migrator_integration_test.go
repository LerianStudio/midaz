// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package celrules

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// sqlTxBeginner adapts a raw *sql.DB to pgdb.TxBeginner. The integration suite
// connects with a raw *sql.DB (testutil.SetupIntegrationDB), whose BeginTx
// returns *sql.Tx — which satisfies pgdb.Tx structurally but not the TxBeginner
// return type, so this thin adapter bridges the concrete return.
type sqlTxBeginner struct{ db *sql.DB }

func (b sqlTxBeginner) BeginTx(ctx context.Context, opts *sql.TxOptions) (pgdb.Tx, error) {
	return b.db.BeginTx(ctx, opts)
}

// celCompiler adapts *cel.Environment.Compile to the ExpressionCompiler port,
// discarding the AST because the recompile-all gate only cares whether the
// rewritten expression compiles against the NEW (asset) environment.
type celCompiler struct{ env *cel.Environment }

func (c celCompiler) Compile(expression string) error {
	_, err := c.env.Compile(expression)
	return err
}

func newTestMigrator(t *testing.T, db *sql.DB) *Migrator {
	t.Helper()

	env, err := cel.NewEnvironment()
	require.NoError(t, err)

	repo := postgres.NewRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})

	m, err := NewMigrator(sqlTxBeginner{db: db}, repo, celCompiler{env: env}, cel.RewriteCurrencyToAsset, false)
	require.NoError(t, err)

	return m
}

func cleanRules(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `DELETE FROM rules`)
	require.NoError(t, err)
}

func seedRule(t *testing.T, db *sql.DB, name, expression string) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO rules (id, name, expression, action, scopes, status)
		 VALUES ($1, $2, $3, 'ALLOW', '[]', 'ACTIVE')`,
		id, name, expression)
	require.NoError(t, err)

	return id
}

func readExpression(t *testing.T, db *sql.DB, id uuid.UUID) string {
	t.Helper()

	var expr string
	err := db.QueryRowContext(context.Background(),
		`SELECT expression FROM rules WHERE id = $1`, id).Scan(&expr)
	require.NoError(t, err)

	return expr
}

func mustRewrite(t *testing.T, expression string) string {
	t.Helper()
	out, err := cel.RewriteCurrencyToAsset(expression)
	require.NoError(t, err)
	return out
}

func TestIntegration_CELRuleMigration_Up_RewritesGlobalPreservesOthers(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	cleanRules(t, db)
	t.Cleanup(func() { cleanRules(t, db) })

	const (
		globalExpr = `currency == "BRL"`
		metaExpr   = `metadata.currency == "USD"`
		shadowExpr = `["BRL"].exists(currency, currency == "BRL")`
	)

	globalID := seedRule(t, db, "global-currency", globalExpr)
	metaID := seedRule(t, db, "metadata-currency", metaExpr)
	shadowID := seedRule(t, db, "shadowing-currency", shadowExpr)

	m := newTestMigrator(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	res, err := m.Up(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, res.Scanned)

	// The global currency reference is renamed to asset and recompiles/evaluates
	// against the new environment.
	gotGlobal := readExpression(t, db, globalID)
	assert.Equal(t, mustRewrite(t, globalExpr), gotGlobal)
	assert.Contains(t, gotGlobal, "asset")
	assert.NotContains(t, tokens(gotGlobal), "currency",
		"global rule must no longer reference the currency global")

	// A metadata.currency field selection is preserved (currency is a field name,
	// not the global variable).
	gotMeta := readExpression(t, db, metaID)
	assert.Equal(t, mustRewrite(t, metaExpr), gotMeta)
	assert.Contains(t, gotMeta, "metadata.currency")

	// A comprehension binding named currency shadows the global inside the macro
	// body and is preserved.
	gotShadow := readExpression(t, db, shadowID)
	assert.Equal(t, mustRewrite(t, shadowExpr), gotShadow)
	assert.Contains(t, gotShadow, "currency")
	assert.NotContains(t, gotShadow, "asset")
}

func TestIntegration_CELRuleMigration_Up_RollsBackOnBadRule(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	cleanRules(t, db)
	t.Cleanup(func() { cleanRules(t, db) })

	// The bad rule rewrites cleanly (parse-only) but references an undeclared
	// variable, so it fails the recompile-all gate against the new environment.
	seeds := map[string]string{
		"global-currency":    `currency == "BRL"`,
		"metadata-currency":  `metadata.currency == "USD"`,
		"shadowing-currency": `["BRL"].exists(currency, currency == "BRL")`,
		"broken-rule":        `currency == "BRL" && undeclaredThing > 0`,
	}

	ids := make(map[uuid.UUID]string, len(seeds))
	for name, expr := range seeds {
		ids[seedRule(t, db, name, expr)] = expr
	}

	m := newTestMigrator(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	_, err := m.Up(ctx)
	require.Error(t, err, "recompile-all gate must abort on the undeclared reference")

	// All-or-nothing: not a single rule was mutated.
	for id, original := range ids {
		assert.Equal(t, original, readExpression(t, db, id),
			"rollback must leave every rule byte-identical to its seeded expression")
	}
}

func TestIntegration_CELRuleMigration_Down_FailsLoudAndMutatesNothing(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	cleanRules(t, db)
	t.Cleanup(func() { cleanRules(t, db) })

	const globalExpr = `currency == "BRL"`
	globalID := seedRule(t, db, "global-currency", globalExpr)

	m := newTestMigrator(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	err := m.Down(ctx)
	require.Error(t, err, "down must fail loud")
	assert.ErrorIs(t, err, ErrDownIrreversible)

	assert.Equal(t, globalExpr, readExpression(t, db, globalID),
		"down must not mutate any rule")
}

// tokens returns the expression with string literals removed so an assertion can
// distinguish the currency IDENTIFIER from the substring "currency" appearing
// inside a quoted literal.
func tokens(expression string) string {
	var b strings.Builder

	inString := false
	for _, r := range expression {
		if r == '"' {
			inString = !inString
			continue
		}

		if !inString {
			b.WriteRune(r)
		}
	}

	return b.String()
}
