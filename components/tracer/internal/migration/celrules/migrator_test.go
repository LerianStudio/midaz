// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package celrules

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// --- transaction stub ----------------------------------------------------
//
// fakeTx is kept as a small state-recording stub (not a GoMock) on purpose:
// the tests assert the commit/rollback LIFECYCLE (which of Commit/Rollback ran,
// and that Rollback runs on any error), which reads more clearly from recorded
// booleans than from ordered GoMock expectations. The store, compiler, and
// transaction beginner are GoMock mocks.
type fakeTx struct {
	execErr    error
	commitErr  error
	committed  bool
	rolledBack bool
}

func (f *fakeTx) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, f.execErr
}

func (f *fakeTx) QueryContext(context.Context, string, ...any) (*sql.Rows, error) { return nil, nil }
func (f *fakeTx) QueryRowContext(context.Context, string, ...any) *sql.Row        { return nil }

func (f *fakeTx) Commit() error {
	f.committed = true
	return f.commitErr
}

func (f *fakeTx) Rollback() error {
	f.rolledBack = true
	return nil
}

// identityRewrite returns the input unchanged (marks the rule "unchanged").
func identityRewrite(expression string) (string, error) { return expression, nil }

// toAssetRewrite is a trivial rename used only to make the changed-vs-unchanged
// branch observable in unit tests; the real rename lives in the cel package.
func toAssetRewrite(expression string) (string, error) {
	if expression == "currency" {
		return "asset", nil
	}

	return expression, nil
}

func newRule(t *testing.T, expression string) *model.Rule {
	t.Helper()
	return &model.Rule{ID: uuid.New(), Expression: expression}
}

// --- tests ---------------------------------------------------------------

func TestNewMigrator_RequiredArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		nilArg  string // which collaborator to pass as nil; "" = all present
		wantErr string
	}{
		{"nil txBeginner", "txBeginner", "txBeginner is required"},
		{"nil store", "store", "rule store is required"},
		{"nil compiler", "compiler", "expression compiler is required"},
		{"nil rewrite", "rewrite", "rewriter is required"},
		{"all present", "", ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			var beginner pgdb.TxBeginner = dbmocks.NewMockTxBeginner(ctrl)

			var store RuleStore = NewMockRuleStore(ctrl)

			var compiler ExpressionCompiler = NewMockExpressionCompiler(ctrl)

			rewrite := Rewriter(identityRewrite)

			switch tt.nilArg {
			case "txBeginner":
				beginner = nil
			case "store":
				store = nil
			case "compiler":
				compiler = nil
			case "rewrite":
				rewrite = nil
			}

			m, err := NewMigrator(beginner, store, compiler, rewrite, false)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, m)

				return
			}

			require.NoError(t, err)
			assert.NotNil(t, m)
		})
	}
}

func TestMigrator_Up_ErrorPaths(t *testing.T) {
	t.Parallel()

	beginErr := errors.New("begin boom")
	lockErr := errors.New("lock boom")
	listErr := errors.New("list boom")
	rewriteErr := errors.New("rewrite boom")
	compileErr := errors.New("undeclared reference")
	updateErr := errors.New("update boom")
	commitErr := errors.New("commit boom")

	tests := []struct {
		name string
		// setup configures the mocks for the case and returns the fakeTx that
		// BeginTx will hand back (nil when no transaction is started), so the
		// subtest can assert rollback.
		setup      func(b *dbmocks.MockTxBeginner, s *MockRuleStore, c *MockExpressionCompiler) *fakeTx
		rewrite    Rewriter
		wantErr    string
		wantRolled bool // true when a tx was started and must be rolled back
	}{
		{
			name: "begin transaction error",
			setup: func(b *dbmocks.MockTxBeginner, _ *MockRuleStore, _ *MockExpressionCompiler) *fakeTx {
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(nil, beginErr)
				return nil
			},
			rewrite: identityRewrite,
			wantErr: "begin transaction",
		},
		{
			name: "nil transaction without error",
			setup: func(b *dbmocks.MockTxBeginner, _ *MockRuleStore, _ *MockExpressionCompiler) *fakeTx {
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(nil, nil)
				return nil
			},
			rewrite: identityRewrite,
			wantErr: "nil transaction",
		},
		{
			name: "lock table error",
			setup: func(b *dbmocks.MockTxBeginner, _ *MockRuleStore, _ *MockExpressionCompiler) *fakeTx {
				tx := &fakeTx{execErr: lockErr}
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
				return tx
			},
			rewrite:    identityRewrite,
			wantErr:    "lock rules table",
			wantRolled: true,
		},
		{
			name: "list rules error",
			setup: func(b *dbmocks.MockTxBeginner, s *MockRuleStore, _ *MockExpressionCompiler) *fakeTx {
				tx := &fakeTx{}
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
				s.EXPECT().ListByStatus(gomock.Any(), gomock.Any()).Return(nil, listErr)
				return tx
			},
			rewrite:    identityRewrite,
			wantErr:    "load rules",
			wantRolled: true,
		},
		{
			name: "rewrite error",
			setup: func(b *dbmocks.MockTxBeginner, s *MockRuleStore, _ *MockExpressionCompiler) *fakeTx {
				tx := &fakeTx{}
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
				s.EXPECT().ListByStatus(gomock.Any(), gomock.Any()).Return([]*model.Rule{newRule(t, "currency")}, nil)
				return tx
			},
			rewrite:    func(string) (string, error) { return "", rewriteErr },
			wantErr:    "rewrite rule",
			wantRolled: true,
		},
		{
			name: "recompile gate error rolls back",
			setup: func(b *dbmocks.MockTxBeginner, s *MockRuleStore, c *MockExpressionCompiler) *fakeTx {
				tx := &fakeTx{}
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
				s.EXPECT().ListByStatus(gomock.Any(), gomock.Any()).Return([]*model.Rule{newRule(t, "currency")}, nil)
				c.EXPECT().Compile("asset").Return(compileErr)
				return tx
			},
			rewrite:    toAssetRewrite,
			wantErr:    "recompile gate failed",
			wantRolled: true,
		},
		{
			name: "update error rolls back",
			setup: func(b *dbmocks.MockTxBeginner, s *MockRuleStore, c *MockExpressionCompiler) *fakeTx {
				tx := &fakeTx{}
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
				s.EXPECT().ListByStatus(gomock.Any(), gomock.Any()).Return([]*model.Rule{newRule(t, "currency")}, nil)
				c.EXPECT().Compile("asset").Return(nil)
				s.EXPECT().UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(updateErr)
				return tx
			},
			rewrite:    toAssetRewrite,
			wantErr:    "persist rule",
			wantRolled: true,
		},
		{
			name: "commit error",
			setup: func(b *dbmocks.MockTxBeginner, s *MockRuleStore, c *MockExpressionCompiler) *fakeTx {
				tx := &fakeTx{commitErr: commitErr}
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
				s.EXPECT().ListByStatus(gomock.Any(), gomock.Any()).Return([]*model.Rule{newRule(t, "currency")}, nil)
				c.EXPECT().Compile("asset").Return(nil)
				s.EXPECT().UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				return tx
			},
			rewrite:    toAssetRewrite,
			wantErr:    "commit migration",
			wantRolled: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			beginner := dbmocks.NewMockTxBeginner(ctrl)
			store := NewMockRuleStore(ctrl)
			compiler := NewMockExpressionCompiler(ctrl)

			tx := tt.setup(beginner, store, compiler)

			m, err := NewMigrator(beginner, store, compiler, tt.rewrite, false)
			require.NoError(t, err)

			_, err = m.Up(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)

			if tt.wantRolled {
				require.NotNil(t, tx)
				assert.True(t, tx.rolledBack, "a started transaction must be rolled back on any error")
			}
		})
	}
}

func TestMigrator_Up_Success(t *testing.T) {
	t.Parallel()

	tx := &fakeTx{}
	changed := newRule(t, "currency")   // rewrites to "asset" -> updated
	unchanged := newRule(t, "constant") // rewrite is identity -> skipped

	ctrl := gomock.NewController(t)
	beginner := dbmocks.NewMockTxBeginner(ctrl)
	store := NewMockRuleStore(ctrl)
	compiler := NewMockExpressionCompiler(ctrl)

	beginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
	store.EXPECT().ListByStatus(gomock.Any(), gomock.Any()).Return([]*model.Rule{changed, unchanged}, nil)
	// The recompile-all gate runs for every scanned rule before any persist.
	compiler.EXPECT().Compile("asset").Return(nil)
	compiler.EXPECT().Compile("constant").Return(nil)

	var persisted []*model.Rule

	store.EXPECT().UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ pgdb.DB, rule *model.Rule) error {
			persisted = append(persisted, rule)
			return nil
		},
	)

	m, err := NewMigrator(beginner, store, compiler, toAssetRewrite, false)
	require.NoError(t, err)

	res, err := m.Up(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 2, res.Scanned)
	assert.Equal(t, 1, res.Rewritten)
	assert.Equal(t, 1, res.Unchanged)

	assert.True(t, tx.committed)
	assert.False(t, tx.rolledBack)

	require.Len(t, persisted, 1)
	assert.Equal(t, changed.ID, persisted[0].ID)
	assert.Equal(t, "asset", persisted[0].Expression)
}

// TestMigrator_Up_EmptyExpression_SkippedAsUnchanged proves an empty or
// whitespace-only expression is counted as unchanged and never reaches the
// rewriter (which would reject it) or the recompile gate.
func TestMigrator_Up_EmptyExpression_SkippedAsUnchanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		expression string
	}{
		{"empty string", ""},
		{"whitespace only", "   \t\n"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tx := &fakeTx{}

			ctrl := gomock.NewController(t)
			beginner := dbmocks.NewMockTxBeginner(ctrl)
			store := NewMockRuleStore(ctrl)
			compiler := NewMockExpressionCompiler(ctrl)

			beginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
			store.EXPECT().ListByStatus(gomock.Any(), gomock.Any()).Return([]*model.Rule{newRule(t, tt.expression)}, nil)
			// No compiler.Compile and no store.UpdateWithTx expectations: an empty
			// expression must be skipped before both. GoMock fails on any such call.

			// The rewriter must never be invoked for an empty expression.
			failRewrite := func(string) (string, error) {
				t.Errorf("rewriter must not be called for an empty expression")
				return "", errors.New("rewriter should not run")
			}

			m, err := NewMigrator(beginner, store, compiler, failRewrite, false)
			require.NoError(t, err)

			res, err := m.Up(context.Background())
			require.NoError(t, err)

			assert.Equal(t, 1, res.Scanned)
			assert.Equal(t, 0, res.Rewritten)
			assert.Equal(t, 1, res.Unchanged)

			assert.True(t, tx.committed)
			assert.False(t, tx.rolledBack)
		})
	}
}

// TestMigrator_Up_MultiTenant_RefusesBeforeAnyDBWork proves the multi-tenant
// guard: Up aborts with ErrMultiTenantUnsupported before opening a transaction.
// The transaction beginner carries NO BeginTx expectation, so a single call
// would fail the test — proving no database work happens.
func TestMigrator_Up_MultiTenant_RefusesBeforeAnyDBWork(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	beginner := dbmocks.NewMockTxBeginner(ctrl) // no BeginTx expected
	store := NewMockRuleStore(ctrl)             // no calls expected
	compiler := NewMockExpressionCompiler(ctrl) // no calls expected

	m, err := NewMigrator(beginner, store, compiler, identityRewrite, true)
	require.NoError(t, err)

	res, err := m.Up(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMultiTenantUnsupported)
	assert.Equal(t, Result{}, res, "no counts should be recorded when the guard trips")
}

func TestMigrator_Down_FailsLoud(t *testing.T) {
	t.Parallel()

	tx := &fakeTx{}

	ctrl := gomock.NewController(t)
	beginner := dbmocks.NewMockTxBeginner(ctrl) // no calls expected
	store := NewMockRuleStore(ctrl)             // no calls expected
	compiler := NewMockExpressionCompiler(ctrl)

	m, err := NewMigrator(beginner, store, compiler, identityRewrite, false)
	require.NoError(t, err)

	err = m.Down(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDownIrreversible)

	// Down performs no I/O.
	assert.False(t, tx.committed)
	assert.False(t, tx.rolledBack)
}
