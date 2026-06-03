// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"tracer/internal/adapters/postgres/db/mocks"
	"tracer/internal/testutil"
	"tracer/pkg/constant"
	"tracer/pkg/model"
	pkgHTTP "tracer/pkg/net/http"
)

// setupMockDB creates a gomock controller, mock DBConnection, and sqlmock for testing.
// Returns the repository, sqlmock for query expectations, and a cleanup function.
func setupMockDB(t *testing.T) (*Repository, sqlmock.Sqlmock, func()) {
	t.Helper()

	ctrl := gomock.NewController(t)
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(db, nil).AnyTimes()

	repo := NewRepositoryWithConnection(mockConn)

	cleanup := func() {
		db.Close()
		ctrl.Finish()
	}

	return repo, sqlMock, cleanup
}

// testRule creates a test rule with default values.
func testRule() *model.Rule {
	return &model.Rule{
		ID:          uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
		Name:        "test rule",
		Description: testutil.StringPtr("Test description"),
		Expression:  "amount > 1000",
		Action:      model.DecisionDeny,
		Scopes:      []model.Scope{},
		Status:      model.RuleStatusDraft,
		CreatedAt:   time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		DeletedAt:   nil,
	}
}

// ruleColumns returns the column names for rule queries.
func ruleColumns() []string {
	return []string{"id", "name", "description", "expression", "action", "scopes", "status", "created_at", "updated_at", "activated_at", "deactivated_at", "deleted_at"}
}

// ruleRow creates a sqlmock row from a rule.
func ruleRow(t *testing.T, rule *model.Rule) *sqlmock.Rows {
	t.Helper()

	scopesJSON, err := json.Marshal(rule.Scopes)
	require.NoError(t, err, "failed to marshal scopes")

	var deletedAt interface{}
	if rule.DeletedAt != nil {
		deletedAt = *rule.DeletedAt
	}

	return sqlmock.NewRows(ruleColumns()).
		AddRow(
			rule.ID,
			rule.Name,
			rule.Description,
			rule.Expression,
			rule.Action,
			scopesJSON,
			rule.Status,
			rule.CreatedAt,
			rule.UpdatedAt,
			rule.ActivatedAt,
			rule.DeactivatedAt,
			deletedAt,
		)
}

// emptyScopesJSON returns JSON for an empty scopes slice.
// Helper to reduce duplication in test mockSetup closures.
func emptyScopesJSON(t *testing.T) []byte {
	t.Helper()

	scopesJSON, err := json.Marshal([]model.Scope{})
	require.NoError(t, err, "failed to marshal empty scopes")

	return scopesJSON
}

// mustEncodeCursor encodes a cursor and panics on error.
// Used in test table definitions where t is not available.
func mustEncodeCursor(cursor pkgHTTP.Cursor) string {
	encoded, err := pkgHTTP.EncodeCursor(cursor)
	if err != nil {
		panic("failed to encode cursor: " + err.Error())
	}

	return encoded
}

func TestRepository_GetByID(t *testing.T) {
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		ruleID    uuid.UUID
		mockSetup func(t *testing.T, mock sqlmock.Sqlmock, id uuid.UUID)
		wantErr   bool
		errIs     error
	}{
		{
			name:   "Success - returns rule",
			ruleID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock, id uuid.UUID) {
				rule := testRule()
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WithArgs(id).
					WillReturnRows(ruleRow(t, rule))
			},
			wantErr: false,
		},
		{
			name:   "Error - rule not found",
			ruleID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440099"),
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock, id uuid.UUID) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WithArgs(id).
					WillReturnError(sql.ErrNoRows)
			},
			wantErr: true,
			errIs:   constant.ErrRuleNotFound,
		},
		{
			name:   "Error - database query fails",
			ruleID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock, id uuid.UUID) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WithArgs(id).
					WillReturnError(errors.New("connection timeout"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupMockDB(t)
			defer cleanup()

			tt.mockSetup(t, sqlMock, tt.ruleID)

			ctx := context.Background()
			result, err := repo.GetByID(ctx, tt.ruleID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.True(t, errors.Is(err, tt.errIs))
				}
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}

			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestRepository_GetByName(t *testing.T) {
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		ruleName  string
		mockSetup func(t *testing.T, mock sqlmock.Sqlmock, name string)
		wantErr   bool
		errIs     error
	}{
		{
			name:     "Success - returns rule",
			ruleName: "test rule",
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock, name string) {
				rule := testRule()
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WithArgs(name).
					WillReturnRows(ruleRow(t, rule))
			},
			wantErr: false,
		},
		{
			name:     "Error - rule not found",
			ruleName: "nonexistent rule",
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock, name string) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WithArgs(name).
					WillReturnError(sql.ErrNoRows)
			},
			wantErr: true,
			errIs:   constant.ErrRuleNotFound,
		},
		{
			name:     "Error - database query fails",
			ruleName: "test rule",
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock, name string) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WithArgs(name).
					WillReturnError(errors.New("database error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupMockDB(t)
			defer cleanup()

			tt.mockSetup(t, sqlMock, tt.ruleName)

			ctx := context.Background()
			result, err := repo.GetByName(ctx, tt.ruleName)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.True(t, errors.Is(err, tt.errIs))
				}
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.ruleName, result.Name)
			}

			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestRepository_ListByStatus(t *testing.T) {
	testutil.SetupTestTracing(t)

	activeStatus := model.RuleStatusActive

	tests := []struct {
		name      string
		status    *model.RuleStatus
		mockSetup func(t *testing.T, mock sqlmock.Sqlmock)
		wantCount int
		wantErr   bool
	}{
		{
			name:   "Success - returns all rules when status is nil",
			status: nil,
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock) {
				rule1 := testRule()
				rule2 := testRule()
				rule2.ID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
				rule2.Name = "test rule 2"

				scopesJSON := emptyScopesJSON(t)
				rows := sqlmock.NewRows(ruleColumns()).
					AddRow(rule1.ID, rule1.Name, rule1.Description, rule1.Expression, rule1.Action, scopesJSON, rule1.Status, rule1.CreatedAt, rule1.UpdatedAt, nil, nil, nil).
					AddRow(rule2.ID, rule2.Name, rule2.Description, rule2.Expression, rule2.Action, scopesJSON, rule2.Status, rule2.CreatedAt, rule2.UpdatedAt, nil, nil, nil)

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(rows)
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:   "Success - returns rules filtered by status",
			status: &activeStatus,
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock) {
				rule := testRule()
				rule.Status = model.RuleStatusActive

				scopesJSON := emptyScopesJSON(t)
				rows := sqlmock.NewRows(ruleColumns()).
					AddRow(rule.ID, rule.Name, rule.Description, rule.Expression, rule.Action, scopesJSON, rule.Status, rule.CreatedAt, rule.UpdatedAt, nil, nil, nil)

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WithArgs(activeStatus).
					WillReturnRows(rows)
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:   "Success - returns empty list",
			status: nil,
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows(ruleColumns())
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(rows)
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:   "Error - database query fails",
			status: nil,
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnError(errors.New("connection lost"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupMockDB(t)
			defer cleanup()

			tt.mockSetup(t, sqlMock)

			ctx := context.Background()
			result, err := repo.ListByStatus(ctx, tt.status)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Len(t, result, tt.wantCount)
			}

			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestRepository_UpdateStatus(t *testing.T) {
	testutil.SetupTestTracing(t)

	tests := []struct {
		name          string
		ruleID        uuid.UUID
		status        model.RuleStatus
		activatedAt   *time.Time
		deactivatedAt *time.Time
		mockSetup     func(mock sqlmock.Sqlmock, id uuid.UUID, status model.RuleStatus)
		wantErr       bool
		errIs         error
	}{
		{
			name:   "Success - updates status to active",
			ruleID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			status: model.RuleStatusActive,
			mockSetup: func(mock sqlmock.Sqlmock, id uuid.UUID, status model.RuleStatus) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE rules`)).
					WithArgs(
						status,
						sqlmock.AnyArg(), // updated_at
						id,
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantErr: false,
		},
		{
			name:   "Success - updates status to inactive",
			ruleID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			status: model.RuleStatusInactive,
			mockSetup: func(mock sqlmock.Sqlmock, id uuid.UUID, status model.RuleStatus) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE rules`)).
					WithArgs(
						status,
						sqlmock.AnyArg(),
						id,
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantErr: false,
		},
		{
			name:          "Success - updates status to active with activated_at timestamp",
			ruleID:        uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			status:        model.RuleStatusActive,
			activatedAt:   func() *time.Time { ts := testutil.FixedTime().UTC(); return &ts }(),
			deactivatedAt: nil,
			mockSetup: func(mock sqlmock.Sqlmock, id uuid.UUID, status model.RuleStatus) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE rules`)).
					WithArgs(
						status,
						sqlmock.AnyArg(), // updated_at
						sqlmock.AnyArg(), // activated_at
						id,
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantErr: false,
		},
		{
			name:          "Success - updates status to inactive with deactivated_at timestamp",
			ruleID:        uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			status:        model.RuleStatusInactive,
			activatedAt:   nil,
			deactivatedAt: func() *time.Time { ts := testutil.FixedTime().UTC(); return &ts }(),
			mockSetup: func(mock sqlmock.Sqlmock, id uuid.UUID, status model.RuleStatus) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE rules`)).
					WithArgs(
						status,
						sqlmock.AnyArg(), // updated_at
						sqlmock.AnyArg(), // deactivated_at
						id,
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantErr: false,
		},
		{
			name:          "Success - updates status with both activated_at and deactivated_at timestamps",
			ruleID:        uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			status:        model.RuleStatusInactive,
			activatedAt:   func() *time.Time { ts := testutil.FixedTime().UTC().Add(-24 * time.Hour); return &ts }(),
			deactivatedAt: func() *time.Time { ts := testutil.FixedTime().UTC(); return &ts }(),
			mockSetup: func(mock sqlmock.Sqlmock, id uuid.UUID, status model.RuleStatus) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE rules`)).
					WithArgs(
						status,
						sqlmock.AnyArg(), // updated_at
						sqlmock.AnyArg(), // activated_at
						sqlmock.AnyArg(), // deactivated_at
						id,
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantErr: false,
		},
		{
			name:   "Error - rule not found",
			ruleID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440099"),
			status: model.RuleStatusActive,
			mockSetup: func(mock sqlmock.Sqlmock, id uuid.UUID, status model.RuleStatus) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE rules`)).
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantErr: true,
			errIs:   constant.ErrRuleNotFound,
		},
		{
			name:   "Error - database update fails",
			ruleID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			status: model.RuleStatusActive,
			mockSetup: func(mock sqlmock.Sqlmock, id uuid.UUID, status model.RuleStatus) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE rules`)).
					WillReturnError(errors.New("deadlock detected"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupMockDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock, tt.ruleID, tt.status)

			ctx := context.Background()
			err := repo.UpdateStatus(ctx, tt.ruleID, tt.status, testutil.FixedTime(), tt.activatedAt, tt.deactivatedAt)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.True(t, errors.Is(err, tt.errIs))
				}
			} else {
				require.NoError(t, err)
			}

			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestRepository_List(t *testing.T) {
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		filter    *model.ListRulesFilter
		mockSetup func(t *testing.T, mock sqlmock.Sqlmock)
		wantCount int
		wantMore  bool
		wantErr   bool
	}{
		{
			name: "Success - returns paginated rules",
			filter: &model.ListRulesFilter{
				Limit:     10,
				SortOrder: "DESC",
			},
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock) {
				rule1 := testRule()
				rule2 := testRule()
				rule2.ID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")

				scopesJSON := emptyScopesJSON(t)
				rows := sqlmock.NewRows(ruleColumns()).
					AddRow(rule1.ID, rule1.Name, rule1.Description, rule1.Expression, rule1.Action, scopesJSON, rule1.Status, rule1.CreatedAt, rule1.UpdatedAt, nil, nil, nil).
					AddRow(rule2.ID, rule2.Name, rule2.Description, rule2.Expression, rule2.Action, scopesJSON, rule2.Status, rule2.CreatedAt, rule2.UpdatedAt, nil, nil, nil)

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(rows)
			},
			wantCount: 2,
			wantMore:  false,
			wantErr:   false,
		},
		{
			name: "Success - returns with hasMore=true",
			filter: &model.ListRulesFilter{
				Limit:     1,
				SortOrder: "DESC",
			},
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock) {
				rule1 := testRule()
				rule2 := testRule()
				rule2.ID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")

				scopesJSON := emptyScopesJSON(t)
				rows := sqlmock.NewRows(ruleColumns()).
					AddRow(rule1.ID, rule1.Name, rule1.Description, rule1.Expression, rule1.Action, scopesJSON, rule1.Status, rule1.CreatedAt, rule1.UpdatedAt, nil, nil, nil).
					AddRow(rule2.ID, rule2.Name, rule2.Description, rule2.Expression, rule2.Action, scopesJSON, rule2.Status, rule2.CreatedAt, rule2.UpdatedAt, nil, nil, nil)

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(rows)
			},
			wantCount: 1,
			wantMore:  true,
			wantErr:   false,
		},
		{
			name: "Success - filter by status",
			filter: &model.ListRulesFilter{
				Status:    testutil.Ptr(model.RuleStatusActive),
				Limit:     10,
				SortOrder: "DESC",
			},
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock) {
				rule := testRule()
				rule.Status = model.RuleStatusActive

				scopesJSON := emptyScopesJSON(t)
				rows := sqlmock.NewRows(ruleColumns()).
					AddRow(rule.ID, rule.Name, rule.Description, rule.Expression, rule.Action, scopesJSON, rule.Status, rule.CreatedAt, rule.UpdatedAt, nil, nil, nil)

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(rows)
			},
			wantCount: 1,
			wantMore:  false,
			wantErr:   false,
		},
		{
			name: "Success - empty result",
			filter: &model.ListRulesFilter{
				Limit:     10,
				SortOrder: "DESC",
			},
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows(ruleColumns())
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(rows)
			},
			wantCount: 0,
			wantMore:  false,
			wantErr:   false,
		},
		{
			name: "Success - with cursor for pagination",
			filter: &model.ListRulesFilter{
				Limit: 10,
				Cursor: mustEncodeCursor(pkgHTTP.Cursor{
					ID:         "550e8400-e29b-41d4-a716-446655440001",
					SortValue:  "2024-01-15T10:00:00Z",
					SortBy:     "created_at",
					SortOrder:  "DESC",
					PointsNext: true,
				}),
				SortBy:    "created_at",
				SortOrder: "DESC",
			},
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock) {
				rule := testRule()
				rule.ID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")

				scopesJSON := emptyScopesJSON(t)
				rows := sqlmock.NewRows(ruleColumns()).
					AddRow(rule.ID, rule.Name, rule.Description, rule.Expression, rule.Action, scopesJSON, rule.Status, rule.CreatedAt, rule.UpdatedAt, nil, nil, nil)

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(rows)
			},
			wantCount: 1,
			wantMore:  false,
			wantErr:   false,
		},
		{
			name: "Error - invalid sort column",
			filter: &model.ListRulesFilter{
				Limit:     10,
				SortBy:    "invalid_column",
				SortOrder: "DESC",
			},
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock) {
				// No query expected - validation fails before query
			},
			wantErr: true,
		},
		{
			name: "Error - database query fails",
			filter: &model.ListRulesFilter{
				Limit:     10,
				SortOrder: "DESC",
			},
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnError(errors.New("query timeout"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupMockDB(t)
			defer cleanup()

			tt.mockSetup(t, sqlMock)

			ctx := context.Background()
			result, err := repo.List(ctx, tt.filter)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Len(t, result.Rules, tt.wantCount)
				assert.Equal(t, tt.wantMore, result.HasMore)
			}

			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestRepository_ListActiveByScopes(t *testing.T) {
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		scopes    []model.Scope
		mockSetup func(t *testing.T, mock sqlmock.Sqlmock)
		wantCount int
		wantErr   bool
	}{
		{
			name:   "Success - returns active rules matching scopes",
			scopes: []model.Scope{{TransactionType: testutil.Ptr(model.TransactionTypeCard)}},
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock) {
				rule := testRule()
				rule.Status = model.RuleStatusActive

				scopesJSON := emptyScopesJSON(t)
				rows := sqlmock.NewRows(ruleColumns()).
					AddRow(rule.ID, rule.Name, rule.Description, rule.Expression, rule.Action, scopesJSON, rule.Status, rule.CreatedAt, rule.UpdatedAt, nil, nil, nil)

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(rows)
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:   "Success - returns empty when no matching scopes",
			scopes: []model.Scope{{TransactionType: testutil.Ptr(model.TransactionTypePix)}},
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows(ruleColumns())
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(rows)
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:   "Error - database query fails",
			scopes: []model.Scope{},
			mockSetup: func(t *testing.T, mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnError(errors.New("connection error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupMockDB(t)
			defer cleanup()

			tt.mockSetup(t, sqlMock)

			ctx := context.Background()
			result, err := repo.ListActiveByScopes(ctx, tt.scopes)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Len(t, result, tt.wantCount)
			}

			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestGetSortValueFromRule(t *testing.T) {
	rule := testRule()

	tests := []struct {
		name     string
		sortBy   string
		expected string
	}{
		{
			name:     "sort by name",
			sortBy:   "name",
			expected: rule.Name,
		},
		{
			name:     "sort by status",
			sortBy:   "status",
			expected: string(rule.Status),
		},
		{
			name:     "sort by created_at",
			sortBy:   "created_at",
			expected: rule.CreatedAt.Format(time.RFC3339Nano),
		},
		{
			name:     "sort by updated_at",
			sortBy:   "updated_at",
			expected: rule.UpdatedAt.Format(time.RFC3339Nano),
		},
		{
			name:     "sort by unknown defaults to created_at",
			sortBy:   "unknown",
			expected: rule.CreatedAt.Format(time.RFC3339Nano),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSortValueFromRule(rule, tt.sortBy)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapRuleSortFieldToColumn(t *testing.T) {
	tests := []struct {
		name     string
		sortBy   string
		expected string
	}{
		// snake_case should work
		{name: "created_at maps correctly", sortBy: "created_at", expected: "created_at"},
		{name: "updated_at maps correctly", sortBy: "updated_at", expected: "updated_at"},
		{name: "name maps correctly", sortBy: "name", expected: "name"},
		{name: "status maps correctly", sortBy: "status", expected: "status"},
		// camelCase should NOT work anymore
		{name: "createdAt is rejected", sortBy: "createdAt", expected: ""},
		{name: "updatedAt is rejected", sortBy: "updatedAt", expected: ""},
		// invalid field
		{name: "invalid field returns empty", sortBy: "invalid", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapRuleSortFieldToColumn(tt.sortBy)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEscapeLikePattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no special characters",
			input:    "simple text",
			expected: "simple text",
		},
		{
			name:     "percent sign",
			input:    "test%value",
			expected: `test\%value`,
		},
		{
			name:     "underscore",
			input:    "test_value",
			expected: `test\_value`,
		},
		{
			name:     "backslash",
			input:    `test\value`,
			expected: `test\\value`,
		},
		{
			name:     "multiple special characters",
			input:    `100%_test\pattern`,
			expected: `100\%\_test\\pattern`,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only special characters",
			input:    `%_\`,
			expected: `\%\_\\`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeLikePattern(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRuleRepository_UsesConnGetDB verifies that the repository now delegates
// tenant resolution entirely to its pgdb.Connection (M1). The per-repo getDB
// helper was removed: tenant-pool short-circuit lives on the adapter. This
// test proves an actual repository operation (GetByID) calls conn.GetDB
// exactly once and does not reimplement the selector. Exercising a real
// repository method (instead of poking repo.conn directly) ensures the
// integration point under test is the production code path, not a manual
// stand-in.
func TestRuleRepository_UsesConnGetDB(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockDB, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mockDB.Close() })

	// Expect a SELECT against the rules table — the precise SQL is repo's
	// internal contract, so we only match on the leading verb. A no-row
	// response forces GetByID to return ErrNotFound, which is fine for the
	// purpose of this test (we are checking GetDB delegation, not query
	// shape).
	sqlMock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows(nil))

	mockConn := mocks.NewMockConnection(ctrl)
	// Exactly once: the repository must not double-resolve the DB inside a
	// single query.
	mockConn.EXPECT().GetDB(gomock.Any()).Return(mockDB, nil).Times(1)

	repo := NewRepositoryWithConnection(mockConn)
	require.NotNil(t, repo)

	// Exercise a real repository operation so GetDB is invoked through the
	// production code path. The result is irrelevant — the assertion is on
	// the gomock expectation (Times(1)).
	_, _ = repo.GetByID(context.Background(), uuid.New())

	require.NoError(t, sqlMock.ExpectationsWereMet())
}
