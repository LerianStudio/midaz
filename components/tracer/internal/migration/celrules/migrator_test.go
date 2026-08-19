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

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// --- fakes ---------------------------------------------------------------

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

type fakeBeginner struct {
	tx  pgdb.Tx
	err error
}

func (f fakeBeginner) BeginTx(context.Context, *sql.TxOptions) (pgdb.Tx, error) {
	return f.tx, f.err
}

type fakeStore struct {
	rules     []*model.Rule
	listErr   error
	updateErr error
	updated   []*model.Rule
}

func (f *fakeStore) ListByStatus(context.Context, *model.RuleStatus) ([]*model.Rule, error) {
	return f.rules, f.listErr
}

func (f *fakeStore) UpdateWithTx(_ context.Context, _ pgdb.DB, rule *model.Rule) error {
	if f.updateErr != nil {
		return f.updateErr
	}

	f.updated = append(f.updated, rule)

	return nil
}

type fakeCompiler struct {
	fn func(string) error
}

func (f fakeCompiler) Compile(expression string) error {
	if f.fn != nil {
		return f.fn(expression)
	}

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

	store := &fakeStore{}
	compiler := fakeCompiler{}
	beginner := fakeBeginner{tx: &fakeTx{}}

	tests := []struct {
		name       string
		txBeginner pgdb.TxBeginner
		store      RuleStore
		compiler   ExpressionCompiler
		rewrite    Rewriter
		wantErr    string
	}{
		{"nil txBeginner", nil, store, compiler, identityRewrite, "txBeginner is required"},
		{"nil store", beginner, nil, compiler, identityRewrite, "rule store is required"},
		{"nil compiler", beginner, store, nil, identityRewrite, "expression compiler is required"},
		{"nil rewrite", beginner, store, compiler, nil, "rewriter is required"},
		{"all present", beginner, store, compiler, identityRewrite, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, err := NewMigrator(tt.txBeginner, tt.store, tt.compiler, tt.rewrite)

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
		name       string
		beginner   func() (fakeBeginner, *fakeTx)
		store      *fakeStore
		compiler   fakeCompiler
		rewrite    Rewriter
		wantErr    string
		wantRolled bool // true when a tx was started and must be rolled back
	}{
		{
			name:     "begin transaction error",
			beginner: func() (fakeBeginner, *fakeTx) { return fakeBeginner{err: beginErr}, nil },
			store:    &fakeStore{},
			rewrite:  identityRewrite,
			wantErr:  "begin transaction",
		},
		{
			name:     "nil transaction without error",
			beginner: func() (fakeBeginner, *fakeTx) { return fakeBeginner{}, nil },
			store:    &fakeStore{},
			rewrite:  identityRewrite,
			wantErr:  "nil transaction",
		},
		{
			name: "lock table error",
			beginner: func() (fakeBeginner, *fakeTx) {
				tx := &fakeTx{execErr: lockErr}
				return fakeBeginner{tx: tx}, tx
			},
			store:      &fakeStore{},
			rewrite:    identityRewrite,
			wantErr:    "lock rules table",
			wantRolled: true,
		},
		{
			name: "list rules error",
			beginner: func() (fakeBeginner, *fakeTx) {
				tx := &fakeTx{}
				return fakeBeginner{tx: tx}, tx
			},
			store:      &fakeStore{listErr: listErr},
			rewrite:    identityRewrite,
			wantErr:    "load rules",
			wantRolled: true,
		},
		{
			name: "rewrite error",
			beginner: func() (fakeBeginner, *fakeTx) {
				tx := &fakeTx{}
				return fakeBeginner{tx: tx}, tx
			},
			store:      &fakeStore{rules: []*model.Rule{newRule(t, "currency")}},
			rewrite:    func(string) (string, error) { return "", rewriteErr },
			wantErr:    "rewrite rule",
			wantRolled: true,
		},
		{
			name: "recompile gate error rolls back",
			beginner: func() (fakeBeginner, *fakeTx) {
				tx := &fakeTx{}
				return fakeBeginner{tx: tx}, tx
			},
			store:      &fakeStore{rules: []*model.Rule{newRule(t, "currency")}},
			compiler:   fakeCompiler{fn: func(string) error { return compileErr }},
			rewrite:    toAssetRewrite,
			wantErr:    "recompile gate failed",
			wantRolled: true,
		},
		{
			name: "update error rolls back",
			beginner: func() (fakeBeginner, *fakeTx) {
				tx := &fakeTx{}
				return fakeBeginner{tx: tx}, tx
			},
			store:      &fakeStore{rules: []*model.Rule{newRule(t, "currency")}, updateErr: updateErr},
			rewrite:    toAssetRewrite,
			wantErr:    "persist rule",
			wantRolled: true,
		},
		{
			name: "commit error",
			beginner: func() (fakeBeginner, *fakeTx) {
				tx := &fakeTx{commitErr: commitErr}
				return fakeBeginner{tx: tx}, tx
			},
			store:      &fakeStore{rules: []*model.Rule{newRule(t, "currency")}},
			rewrite:    toAssetRewrite,
			wantErr:    "commit migration",
			wantRolled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			beginner, tx := tt.beginner()

			m, err := NewMigrator(beginner, tt.store, tt.compiler, tt.rewrite)
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

	store := &fakeStore{rules: []*model.Rule{changed, unchanged}}

	m, err := NewMigrator(fakeBeginner{tx: tx}, store, fakeCompiler{}, toAssetRewrite)
	require.NoError(t, err)

	res, err := m.Up(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 2, res.Scanned)
	assert.Equal(t, 1, res.Rewritten)
	assert.Equal(t, 1, res.Unchanged)

	assert.True(t, tx.committed)
	assert.False(t, tx.rolledBack)

	require.Len(t, store.updated, 1)
	assert.Equal(t, changed.ID, store.updated[0].ID)
	assert.Equal(t, "asset", store.updated[0].Expression)
}

func TestMigrator_Down_FailsLoud(t *testing.T) {
	t.Parallel()

	tx := &fakeTx{}
	store := &fakeStore{}

	m, err := NewMigrator(fakeBeginner{tx: tx}, store, fakeCompiler{}, identityRewrite)
	require.NoError(t, err)

	err = m.Down(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDownIrreversible)

	// Down performs no I/O.
	assert.False(t, tx.committed)
	assert.False(t, tx.rolledBack)
	assert.Empty(t, store.updated)
}
