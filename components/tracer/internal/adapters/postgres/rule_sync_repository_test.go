// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// setupRuleSyncRepo wires a RuleSyncRepository over a sqlmock DB via a mock
// Connection, mirroring setupMockDB used for *Repository.
func setupRuleSyncRepo(t *testing.T) (*RuleSyncRepository, sqlmock.Sqlmock, func()) {
	t.Helper()

	ctrl := gomock.NewController(t)
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(db, nil).AnyTimes()

	repo := NewRuleSyncRepositoryWithConnection(mockConn)

	cleanup := func() {
		db.Close()
		ctrl.Finish()
	}

	return repo, sqlMock, cleanup
}

// setupRuleSyncRepoNoDB wires a RuleSyncRepository whose Connection.GetDB always
// fails, exercising the connection-resolution error branch.
func setupRuleSyncRepoNoDB(t *testing.T) (*RuleSyncRepository, func()) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(nil, errors.New("connection refused")).AnyTimes()

	repo := NewRuleSyncRepositoryWithConnection(mockConn)

	return repo, ctrl.Finish
}

func TestRuleSyncRepository_GetAllActiveRules(t *testing.T) {
	testutil.SetupTestTracing(t)

	t.Run("Success - returns active rules ordered created_at DESC", func(t *testing.T) {
		repo, mock, cleanup := setupRuleSyncRepo(t)
		defer cleanup()

		ruleA := testRule()
		ruleA.Status = model.RuleStatusActive

		ruleB := testRule()
		ruleB.ID = testutil.MustDeterministicUUID(9001)
		ruleB.Name = "second active rule"
		ruleB.Status = model.RuleStatusActive

		mock.ExpectQuery(`SELECT id, name`).
			WithArgs(model.RuleStatusActive).
			WillReturnRows(
				sqlmock.NewRows(ruleColumns()).
					AddRow(ruleA.ID, ruleA.Name, ruleA.Description, ruleA.Expression,
						ruleA.Action, emptyScopesJSON(t), ruleA.Status,
						ruleA.CreatedAt, ruleA.UpdatedAt, nil, nil, nil).
					AddRow(ruleB.ID, ruleB.Name, ruleB.Description, ruleB.Expression,
						ruleB.Action, emptyScopesJSON(t), ruleB.Status,
						ruleB.CreatedAt, ruleB.UpdatedAt, nil, nil, nil),
			)

		rules, err := repo.GetAllActiveRules(context.Background())
		require.NoError(t, err)
		require.Len(t, rules, 2)
		assert.Equal(t, ruleA.ID, rules[0].ID)
		assert.Equal(t, ruleB.ID, rules[1].ID)
		assert.Equal(t, model.RuleStatusActive, rules[0].Status)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - empty result set returns empty slice", func(t *testing.T) {
		repo, mock, cleanup := setupRuleSyncRepo(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT id, name`).
			WithArgs(model.RuleStatusActive).
			WillReturnRows(sqlmock.NewRows(ruleColumns()))

		rules, err := repo.GetAllActiveRules(context.Background())
		require.NoError(t, err)
		assert.Empty(t, rules)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - query fails", func(t *testing.T) {
		repo, mock, cleanup := setupRuleSyncRepo(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT id, name`).
			WillReturnError(errors.New("connection lost"))

		rules, err := repo.GetAllActiveRules(context.Background())
		require.Error(t, err)
		assert.Nil(t, rules)
		assert.Contains(t, err.Error(), "failed to query active rules")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - scan fails on malformed row", func(t *testing.T) {
		repo, mock, cleanup := setupRuleSyncRepo(t)
		defer cleanup()

		// Two-column row against a 12-column scan forces Scan to error.
		mock.ExpectQuery(`SELECT id, name`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow("bad", "row"))

		rules, err := repo.GetAllActiveRules(context.Background())
		require.Error(t, err)
		assert.Nil(t, rules)
		assert.Contains(t, err.Error(), "failed to scan rule row")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - GetDB resolution fails", func(t *testing.T) {
		repo, finish := setupRuleSyncRepoNoDB(t)
		defer finish()

		rules, err := repo.GetAllActiveRules(context.Background())
		require.Error(t, err)
		assert.Nil(t, rules)
		assert.Contains(t, err.Error(), "failed to get database connection")
	})
}

func TestRuleSyncRepository_GetRulesUpdatedSince(t *testing.T) {
	testutil.SetupTestTracing(t)

	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("Success - returns all-status rules updated since timestamp", func(t *testing.T) {
		repo, mock, cleanup := setupRuleSyncRepo(t)
		defer cleanup()

		active := testRule()
		active.Status = model.RuleStatusActive

		deleted := testRule()
		deleted.ID = testutil.MustDeterministicUUID(9002)
		deleted.Name = "deactivated rule"
		deleted.Status = model.RuleStatusDeleted

		mock.ExpectQuery(`SELECT id, name`).
			WithArgs(since).
			WillReturnRows(
				sqlmock.NewRows(ruleColumns()).
					AddRow(active.ID, active.Name, active.Description, active.Expression,
						active.Action, emptyScopesJSON(t), active.Status,
						active.CreatedAt, active.UpdatedAt, nil, nil, nil).
					AddRow(deleted.ID, deleted.Name, deleted.Description, deleted.Expression,
						deleted.Action, emptyScopesJSON(t), deleted.Status,
						deleted.CreatedAt, deleted.UpdatedAt, nil, nil, deleted.CreatedAt),
			)

		rules, err := repo.GetRulesUpdatedSince(context.Background(), since)
		require.NoError(t, err)
		require.Len(t, rules, 2)
		// Deactivations/deletions must be surfaced so the cache can prune them.
		assert.Equal(t, model.RuleStatusDeleted, rules[1].Status)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - query fails", func(t *testing.T) {
		repo, mock, cleanup := setupRuleSyncRepo(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT id, name`).
			WillReturnError(errors.New("statement timeout"))

		rules, err := repo.GetRulesUpdatedSince(context.Background(), since)
		require.Error(t, err)
		assert.Nil(t, rules)
		assert.Contains(t, err.Error(), "failed to query updated rules")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - GetDB resolution fails", func(t *testing.T) {
		repo, finish := setupRuleSyncRepoNoDB(t)
		defer finish()

		rules, err := repo.GetRulesUpdatedSince(context.Background(), since)
		require.Error(t, err)
		assert.Nil(t, rules)
		assert.Contains(t, err.Error(), "failed to get database connection")
	})
}

// TestRuleSyncRepository_scanRulesFromRows_ToEntityError verifies that a row
// whose scopes column holds invalid JSON fails ToEntity conversion and is
// surfaced as a wrapped error rather than swallowed.
func TestRuleSyncRepository_scanRulesFromRows_ToEntityError(t *testing.T) {
	testutil.SetupTestTracing(t)

	repo, mock, cleanup := setupRuleSyncRepo(t)
	defer cleanup()

	rule := testRule()
	rule.Status = model.RuleStatusActive

	mock.ExpectQuery(`SELECT id, name`).
		WithArgs(model.RuleStatusActive).
		WillReturnRows(
			sqlmock.NewRows(ruleColumns()).
				AddRow(rule.ID, rule.Name, rule.Description, rule.Expression,
					rule.Action, []byte("{not-valid-json"), rule.Status,
					rule.CreatedAt, rule.UpdatedAt, nil, nil, nil),
		)

	rules, err := repo.GetAllActiveRules(context.Background())
	require.Error(t, err)
	assert.Nil(t, rules)
	assert.Contains(t, err.Error(), "failed to convert rule model to entity")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRuleSyncRepository_scanRulesFromRows_RowErr verifies a row-iteration error
// (rows.Err()) is wrapped and surfaced.
func TestRuleSyncRepository_scanRulesFromRows_RowErr(t *testing.T) {
	testutil.SetupTestTracing(t)

	repo, mock, cleanup := setupRuleSyncRepo(t)
	defer cleanup()

	rule := testRule()
	rule.Status = model.RuleStatusActive

	rows := sqlmock.NewRows(ruleColumns()).
		AddRow(rule.ID, rule.Name, rule.Description, rule.Expression,
			rule.Action, emptyScopesJSON(t), rule.Status,
			rule.CreatedAt, rule.UpdatedAt, nil, nil, nil).
		RowError(0, errors.New("network read failure"))

	mock.ExpectQuery(`SELECT id, name`).
		WithArgs(model.RuleStatusActive).
		WillReturnRows(rows)

	rules, err := repo.GetAllActiveRules(context.Background())
	require.Error(t, err)
	assert.Nil(t, rules)
	assert.Contains(t, err.Error(), "rows iteration error")
	require.NoError(t, mock.ExpectationsWereMet())
}
