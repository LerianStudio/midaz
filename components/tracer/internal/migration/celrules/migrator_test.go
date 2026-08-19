// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package celrules

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// The transaction lifecycle (Commit on success, Rollback on any error) is
// asserted through GoMock expectations on dbmocks.MockTx: an EXPECT().Rollback()
// that goes uncalled fails the controller, and an unexpected Commit fails too.

// identityRewrite returns the input unchanged (marks the rule "unchanged").
func identityRewrite(expression string) (string, bool, error) { return expression, false, nil }

// toAssetRewrite is a trivial rename used only to make the changed-vs-unchanged
// branch observable in unit tests; the real rename lives in the cel package. It
// reports changed=true only when it actually renames.
func toAssetRewrite(expression string) (string, bool, error) {
	if expression == "currency" {
		return "asset", true, nil
	}

	return expression, false, nil
}

// canonicalizeNoRename mimics the real rewriter's canonical serialization of a
// rule that carries NO global currency: it returns different text (whitespace
// normalized) but reports changed=false, so the migrator must classify it as
// unchanged and never persist it.
func canonicalizeNoRename(expression string) (string, bool, error) {
	if expression == "amount>0" {
		return "amount > 0", false, nil
	}

	return expression, false, nil
}

func newRule(t *testing.T, seed int64, expression string) *model.Rule {
	t.Helper()
	return &model.Rule{ID: testutil.MustDeterministicUUID(seed), Expression: expression}
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

	// rawLeakToken is a distinctive marker embedded in the collaborator errors
	// for the rewrite and recompile branches. Up must return a bounded sentinel
	// (ErrExpressionSyntax) and MUST NOT propagate this raw collaborator text.
	const rawLeakToken = "RAW_CEL_LEAK_TOKEN"

	beginErr := errors.New("begin boom")
	lockErr := errors.New("lock boom")
	listErr := errors.New("list boom")
	rewriteErr := errors.New("rewrite boom " + rawLeakToken)
	compileErr := errors.New("undeclared reference " + rawLeakToken)
	updateErr := errors.New("update boom")
	commitErr := errors.New("commit boom")

	tests := []struct {
		name string
		// setup configures the mocks for the case. The transaction lifecycle is
		// asserted by the expectations themselves: a case that must roll back
		// sets tx.EXPECT().Rollback(), which the controller fails if uncalled.
		setup   func(b *dbmocks.MockTxBeginner, s *MockRuleStore, c *MockExpressionCompiler, tx *dbmocks.MockTx)
		rewrite Rewriter
		wantErr string
		// wantErrIs, when set, asserts the returned error wraps this sentinel
		// (errors.Is). Used for the rewrite/recompile branches, which return a
		// bounded ErrExpressionSyntax rather than the raw cel-go error.
		wantErrIs error
		// wantNoLeak, when set, asserts the returned error text does NOT contain
		// this token — proving the bounded sentinel does not propagate the
		// collaborator's raw error detail.
		wantNoLeak string
	}{
		{
			name: "begin transaction error",
			setup: func(b *dbmocks.MockTxBeginner, _ *MockRuleStore, _ *MockExpressionCompiler, _ *dbmocks.MockTx) {
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(nil, beginErr)
			},
			rewrite: identityRewrite,
			wantErr: "begin transaction",
		},
		{
			name: "nil transaction without error",
			setup: func(b *dbmocks.MockTxBeginner, _ *MockRuleStore, _ *MockExpressionCompiler, _ *dbmocks.MockTx) {
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(nil, nil)
			},
			rewrite: identityRewrite,
			wantErr: "nil transaction",
		},
		{
			name: "lock table error",
			setup: func(b *dbmocks.MockTxBeginner, _ *MockRuleStore, _ *MockExpressionCompiler, tx *dbmocks.MockTx) {
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
				tx.EXPECT().ExecContext(gomock.Any(), gomock.Any()).Return(nil, lockErr)
				tx.EXPECT().Rollback().Return(nil)
			},
			rewrite: identityRewrite,
			wantErr: "lock rules table",
		},
		{
			name: "list rules error",
			setup: func(b *dbmocks.MockTxBeginner, s *MockRuleStore, _ *MockExpressionCompiler, tx *dbmocks.MockTx) {
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
				tx.EXPECT().ExecContext(gomock.Any(), gomock.Any()).Return(nil, nil)
				s.EXPECT().ListByStatus(gomock.Any(), gomock.Any()).Return(nil, listErr)
				tx.EXPECT().Rollback().Return(nil)
			},
			rewrite: identityRewrite,
			wantErr: "load rules",
		},
		{
			name: "rewrite error",
			setup: func(b *dbmocks.MockTxBeginner, s *MockRuleStore, _ *MockExpressionCompiler, tx *dbmocks.MockTx) {
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
				tx.EXPECT().ExecContext(gomock.Any(), gomock.Any()).Return(nil, nil)
				s.EXPECT().ListByStatus(gomock.Any(), gomock.Any()).Return([]*model.Rule{newRule(t, 1, "currency")}, nil)
				tx.EXPECT().Rollback().Return(nil)
			},
			rewrite:    func(string) (string, bool, error) { return "", false, rewriteErr },
			wantErrIs:  constant.ErrExpressionSyntax,
			wantNoLeak: rawLeakToken,
		},
		{
			name: "recompile gate error rolls back",
			setup: func(b *dbmocks.MockTxBeginner, s *MockRuleStore, c *MockExpressionCompiler, tx *dbmocks.MockTx) {
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
				tx.EXPECT().ExecContext(gomock.Any(), gomock.Any()).Return(nil, nil)
				s.EXPECT().ListByStatus(gomock.Any(), gomock.Any()).Return([]*model.Rule{newRule(t, 2, "currency")}, nil)
				c.EXPECT().Compile("asset").Return(compileErr)
				tx.EXPECT().Rollback().Return(nil)
			},
			rewrite:    toAssetRewrite,
			wantErrIs:  constant.ErrExpressionSyntax,
			wantNoLeak: rawLeakToken,
		},
		{
			name: "update error rolls back",
			setup: func(b *dbmocks.MockTxBeginner, s *MockRuleStore, c *MockExpressionCompiler, tx *dbmocks.MockTx) {
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
				tx.EXPECT().ExecContext(gomock.Any(), gomock.Any()).Return(nil, nil)
				s.EXPECT().ListByStatus(gomock.Any(), gomock.Any()).Return([]*model.Rule{newRule(t, 3, "currency")}, nil)
				c.EXPECT().Compile("asset").Return(nil)
				s.EXPECT().UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(updateErr)
				tx.EXPECT().Rollback().Return(nil)
			},
			rewrite: toAssetRewrite,
			wantErr: "persist rule",
		},
		{
			name: "commit error",
			setup: func(b *dbmocks.MockTxBeginner, s *MockRuleStore, c *MockExpressionCompiler, tx *dbmocks.MockTx) {
				b.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
				tx.EXPECT().ExecContext(gomock.Any(), gomock.Any()).Return(nil, nil)
				s.EXPECT().ListByStatus(gomock.Any(), gomock.Any()).Return([]*model.Rule{newRule(t, 4, "currency")}, nil)
				c.EXPECT().Compile("asset").Return(nil)
				s.EXPECT().UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				tx.EXPECT().Commit().Return(commitErr)
				// Commit failed, so the deferred rollback still runs.
				tx.EXPECT().Rollback().Return(nil)
			},
			rewrite: toAssetRewrite,
			wantErr: "commit migration",
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
			tx := dbmocks.NewMockTx(ctrl)

			tt.setup(beginner, store, compiler, tx)

			m, err := NewMigrator(beginner, store, compiler, tt.rewrite, false)
			require.NoError(t, err)

			_, err = m.Up(context.Background())
			require.Error(t, err)

			if tt.wantErr != "" {
				assert.Contains(t, err.Error(), tt.wantErr)
			}

			if tt.wantErrIs != nil {
				assert.ErrorIs(t, err, tt.wantErrIs)
			}

			if tt.wantNoLeak != "" {
				assert.NotContains(t, err.Error(), tt.wantNoLeak,
					"the bounded error must not leak the collaborator's raw error text")
			}
		})
	}
}

func TestMigrator_Up_Success(t *testing.T) {
	t.Parallel()

	changed := newRule(t, 5, "currency")   // rewrites to "asset" -> updated
	unchanged := newRule(t, 6, "constant") // rewrite is identity -> skipped

	ctrl := gomock.NewController(t)
	beginner := dbmocks.NewMockTxBeginner(ctrl)
	store := NewMockRuleStore(ctrl)
	compiler := NewMockExpressionCompiler(ctrl)
	tx := dbmocks.NewMockTx(ctrl)

	beginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
	tx.EXPECT().ExecContext(gomock.Any(), gomock.Any()).Return(nil, nil)
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
	// Commit on success; no Rollback expectation, so the controller fails the
	// test if the migrator ever rolls back the committed path.
	tx.EXPECT().Commit().Return(nil)

	m, err := NewMigrator(beginner, store, compiler, toAssetRewrite, false)
	require.NoError(t, err)

	res, err := m.Up(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 2, res.Scanned)
	assert.Equal(t, 1, res.Rewritten)
	assert.Equal(t, 1, res.Unchanged)

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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			beginner := dbmocks.NewMockTxBeginner(ctrl)
			store := NewMockRuleStore(ctrl)
			compiler := NewMockExpressionCompiler(ctrl)
			tx := dbmocks.NewMockTx(ctrl)

			beginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
			tx.EXPECT().ExecContext(gomock.Any(), gomock.Any()).Return(nil, nil)
			store.EXPECT().ListByStatus(gomock.Any(), gomock.Any()).Return([]*model.Rule{newRule(t, 7, tt.expression)}, nil)
			// No compiler.Compile and no store.UpdateWithTx expectations: an empty
			// expression must be skipped before both. GoMock fails on any such call.
			tx.EXPECT().Commit().Return(nil)

			// The rewriter must never be invoked for an empty expression.
			failRewrite := func(string) (string, bool, error) {
				t.Errorf("rewriter must not be called for an empty expression")
				return "", false, errors.New("rewriter should not run")
			}

			m, err := NewMigrator(beginner, store, compiler, failRewrite, false)
			require.NoError(t, err)

			res, err := m.Up(context.Background())
			require.NoError(t, err)

			assert.Equal(t, 1, res.Scanned)
			assert.Equal(t, 0, res.Rewritten)
			assert.Equal(t, 1, res.Unchanged)
		})
	}
}

// TestMigrator_Up_NonCanonicalNoCurrency_SkippedAsUnchanged proves a rule with
// NO global currency is classified unchanged and never persisted, even when the
// rewriter returns different (canonicalized) text. Classification is by the
// changed flag, not by text inequality: "amount>0" reformats to "amount > 0" with
// no currency->asset rename, so it must NOT be counted or written as rewritten.
func TestMigrator_Up_NonCanonicalNoCurrency_SkippedAsUnchanged(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	beginner := dbmocks.NewMockTxBeginner(ctrl)
	store := NewMockRuleStore(ctrl)
	compiler := NewMockExpressionCompiler(ctrl)
	tx := dbmocks.NewMockTx(ctrl)

	beginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
	tx.EXPECT().ExecContext(gomock.Any(), gomock.Any()).Return(nil, nil)
	store.EXPECT().ListByStatus(gomock.Any(), gomock.Any()).Return([]*model.Rule{newRule(t, 8, "amount>0")}, nil)
	// The recompile-all gate still runs against the canonicalized text, but no
	// UpdateWithTx expectation: the rule must be skipped as unchanged. GoMock
	// fails the test on any persist call.
	compiler.EXPECT().Compile("amount > 0").Return(nil)
	tx.EXPECT().Commit().Return(nil)

	m, err := NewMigrator(beginner, store, compiler, canonicalizeNoRename, false)
	require.NoError(t, err)

	res, err := m.Up(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, res.Scanned)
	assert.Equal(t, 0, res.Rewritten)
	assert.Equal(t, 1, res.Unchanged)
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

	ctrl := gomock.NewController(t)
	// No BeginTx expectation: a single database call would fail the test, which
	// is the proof that Down performs no I/O.
	beginner := dbmocks.NewMockTxBeginner(ctrl)
	store := NewMockRuleStore(ctrl)
	compiler := NewMockExpressionCompiler(ctrl)

	m, err := NewMigrator(beginner, store, compiler, identityRewrite, false)
	require.NoError(t, err)

	err = m.Down(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDownIrreversible)
}
