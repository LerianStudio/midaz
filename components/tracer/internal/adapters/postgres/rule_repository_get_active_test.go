// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// TestRepository_GetActiveRules exercises the GetActiveRules dispatcher: a nil or
// empty scope must route to the global ListByStatus(ACTIVE) path, while a
// populated scope must route to the scope-filtered ListActiveByScopes path. The
// distinguishing observable is the WHERE clause: the global path filters by
// status only, the scoped path adds a JSONB scope predicate.
func TestRepository_GetActiveRules(t *testing.T) {
	testutil.SetupTestTracing(t)

	activeRule := func() *model.Rule {
		r := testRule()
		r.Status = model.RuleStatusActive
		return r
	}

	t.Run("nil scope routes to global ListByStatus", func(t *testing.T) {
		repo, mock, cleanup := setupMockDB(t)
		defer cleanup()

		rule := activeRule()
		// Global path: status filter, NO JSONB scope predicate.
		mock.ExpectQuery(regexp.QuoteMeta(`WHERE deleted_at IS NULL AND status = $1`)).
			WithArgs(model.RuleStatusActive).
			WillReturnRows(sqlmock.NewRows(ruleColumns()).
				AddRow(rule.ID, rule.Name, rule.Description, rule.Expression,
					rule.Action, emptyScopesJSON(t), rule.Status,
					rule.CreatedAt, rule.UpdatedAt, nil, nil, nil))

		rules, err := repo.GetActiveRules(context.Background(), nil)
		require.NoError(t, err)
		require.Len(t, rules, 1)
		assert.Equal(t, rule.ID, rules[0].ID)
		assert.Equal(t, model.RuleStatusActive, rules[0].Status)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty scope routes to global ListByStatus", func(t *testing.T) {
		repo, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// An all-nil-fields scope is IsEmpty()==true and must NOT take the
		// scope-filtered path; it falls through to the global query.
		emptyScope := &model.Scope{}
		require.True(t, emptyScope.IsEmpty(), "guard: this scope must be empty")

		mock.ExpectQuery(regexp.QuoteMeta(`WHERE deleted_at IS NULL AND status = $1`)).
			WithArgs(model.RuleStatusActive).
			WillReturnRows(sqlmock.NewRows(ruleColumns()))

		rules, err := repo.GetActiveRules(context.Background(), emptyScope)
		require.NoError(t, err)
		assert.Empty(t, rules)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("populated scope routes to ListActiveByScopes", func(t *testing.T) {
		repo, mock, cleanup := setupMockDB(t)
		defer cleanup()

		scope := &model.Scope{TransactionType: testutil.Ptr(model.TransactionTypeCard)}
		require.False(t, scope.IsEmpty(), "guard: this scope must be non-empty")

		rule := activeRule()
		// Scope-filtered path: the query carries a JSONB scope predicate
		// (jsonb_array_elements), which the global path never emits.
		mock.ExpectQuery(`jsonb_array_elements`).
			WillReturnRows(sqlmock.NewRows(ruleColumns()).
				AddRow(rule.ID, rule.Name, rule.Description, rule.Expression,
					rule.Action, emptyScopesJSON(t), rule.Status,
					rule.CreatedAt, rule.UpdatedAt, nil, nil, nil))

		rules, err := repo.GetActiveRules(context.Background(), scope)
		require.NoError(t, err)
		require.Len(t, rules, 1)
		assert.Equal(t, rule.ID, rules[0].ID)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
