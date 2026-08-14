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

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/components/tracer/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// setupLimitRepositoryMockDB creates a gomock controller, mock DBConnection, and sqlmock for testing.
// Returns the repository, sqlmock for query expectations, and a cleanup function.
func setupLimitRepositoryMockDB(t *testing.T) (*LimitRepository, sqlmock.Sqlmock, func()) {
	t.Helper()

	ctrl := gomock.NewController(t)
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(db, nil).AnyTimes()

	repo := NewLimitRepositoryWithConnection(mockConn)

	cleanup := func() {
		sqlMock.ExpectClose()
		err := db.Close()
		require.NoError(t, err)
	}

	return repo, sqlMock, cleanup
}

// testLimit creates a test limit with default values.
func testLimit() *model.Limit {
	resetAt := time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC)
	return &model.Limit{
		ID:          uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
		Name:        "Daily Transaction Limit",
		Description: testutil.StringPtr("Test description"),
		LimitType:   model.LimitTypeDaily,
		MaxAmount:   decimal.RequireFromString("1000"),
		Currency:    "USD",
		Scopes:      []model.Scope{},
		Status:      model.LimitStatusActive,
		ResetAt:     &resetAt,
		CreatedAt:   time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		DeletedAt:   nil,
	}
}

// limitColumns returns the column names for limit queries.
func limitColumns() []string {
	return []string{"id", "name", "description", "limit_type", "max_amount", "currency", "scopes", "status", "reset_at", "active_time_start", "active_time_end", "custom_start_date", "custom_end_date", "created_at", "updated_at", "deleted_at"}
}

// limitRow creates a sqlmock row from a limit.
func limitRow(t *testing.T, lmt *model.Limit) *sqlmock.Rows {
	t.Helper()

	scopesJSON, err := json.Marshal(lmt.Scopes)
	require.NoError(t, err, "failed to marshal scopes")

	var deletedAt interface{}
	if lmt.DeletedAt != nil {
		deletedAt = *lmt.DeletedAt
	}

	var resetAt interface{}
	if lmt.ResetAt != nil {
		resetAt = *lmt.ResetAt
	}

	var activeTimeStart, activeTimeEnd interface{}
	if lmt.ActiveTimeStart != nil {
		activeTimeStart = lmt.ActiveTimeStart.String()
	}
	if lmt.ActiveTimeEnd != nil {
		activeTimeEnd = lmt.ActiveTimeEnd.String()
	}

	var customStartDate, customEndDate interface{}
	if lmt.CustomStartDate != nil {
		customStartDate = *lmt.CustomStartDate
	}
	if lmt.CustomEndDate != nil {
		customEndDate = *lmt.CustomEndDate
	}

	return sqlmock.NewRows(limitColumns()).
		AddRow(
			lmt.ID,
			lmt.Name,
			lmt.Description,
			lmt.LimitType,
			lmt.MaxAmount,
			lmt.Currency,
			scopesJSON,
			lmt.Status,
			resetAt,
			activeTimeStart,
			activeTimeEnd,
			customStartDate,
			customEndDate,
			lmt.CreatedAt,
			lmt.UpdatedAt,
			deletedAt,
		)
}

func TestLimitRepository_GetByID_ConnectionError(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(nil, errors.New("connection refused"))

	repo := NewLimitRepositoryWithConnection(mockConn)

	ctx := context.Background()
	result, err := repo.GetByID(ctx, testutil.MustDeterministicUUID(999))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get database connection")
	assert.Nil(t, result)
}

func TestLimitRepository_GetByID(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		limitID   uuid.UUID
		mockSetup func(mock sqlmock.Sqlmock)
		want      *model.Limit
		wantErr   bool
		errType   error
	}{
		{
			name:    "Success - finds limit",
			limitID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WithArgs(uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")).
					WillReturnRows(limitRow(t, testLimit()))
			},
			want:    testLimit(),
			wantErr: false,
		},
		{
			name:    "Error - limit not found",
			limitID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440099"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WithArgs(uuid.MustParse("550e8400-e29b-41d4-a716-446655440099")).
					WillReturnError(sql.ErrNoRows)
			},
			wantErr: true,
			errType: constant.ErrLimitNotFound,
		},
		{
			name:    "Error - database query fails",
			limitID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WithArgs(uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")).
					WillReturnError(errors.New("database error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()
			result, err := repo.GetByID(ctx, tt.limitID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.want.ID, result.ID)
				assert.Equal(t, tt.want.Name, result.Name)
				assert.Equal(t, tt.want.LimitType, result.LimitType)
				assert.Equal(t, tt.want.MaxAmount, result.MaxAmount)
				assert.Equal(t, tt.want.Currency, result.Currency)
				assert.Equal(t, tt.want.Status, result.Status)
			}

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestLimitRepository_List_ConnectionError(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(nil, errors.New("connection refused"))

	repo := NewLimitRepositoryWithConnection(mockConn)

	ctx := context.Background()
	filters := &model.ListLimitsFilter{Limit: 10}
	result, err := repo.List(ctx, filters)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get database connection")
	assert.Nil(t, result)
}

func TestLimitRepository_List(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		filters   *model.ListLimitsFilter
		mockSetup func(mock sqlmock.Sqlmock)
		wantLen   int
		wantErr   bool
		errMsg    string
	}{
		{
			name:    "Success - finds all limits with limit+1 fetch semantics",
			filters: &model.ListLimitsFilter{Limit: 10},
			mockSetup: func(mock sqlmock.Sqlmock) {
				rows := limitRow(t, testLimit())
				// Query fetches limit+1 (11) to detect hasMore; no filter args since only deleted_at IS NULL
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, description, limit_type, max_amount, currency, scopes, status, reset_at, active_time_start, active_time_end, custom_start_date, custom_end_date, created_at, updated_at, deleted_at FROM limits WHERE deleted_at IS NULL ORDER BY created_at DESC, id DESC LIMIT 11`)).
					WillReturnRows(rows)
			},
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "Success - filters by status with correct args",
			filters: func() *model.ListLimitsFilter {
				status := model.LimitStatusActive
				return &model.ListLimitsFilter{
					Status: &status,
					Limit:  10,
				}
			}(),
			mockSetup: func(mock sqlmock.Sqlmock) {
				rows := limitRow(t, testLimit())
				// Query includes status filter arg; limit+1 (11) for hasMore detection
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, description, limit_type, max_amount, currency, scopes, status, reset_at, active_time_start, active_time_end, custom_start_date, custom_end_date, created_at, updated_at, deleted_at FROM limits WHERE deleted_at IS NULL AND status = $1 ORDER BY created_at DESC, id DESC LIMIT 11`)).
					WithArgs(string(model.LimitStatusActive)).
					WillReturnRows(rows)
			},
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "Success - filters by limit type with correct args",
			filters: func() *model.ListLimitsFilter {
				limitType := model.LimitTypeDaily
				return &model.ListLimitsFilter{
					LimitType: &limitType,
					Limit:     10,
				}
			}(),
			mockSetup: func(mock sqlmock.Sqlmock) {
				rows := limitRow(t, testLimit())
				// Query includes limit_type filter arg; limit+1 (11) for hasMore detection
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, description, limit_type, max_amount, currency, scopes, status, reset_at, active_time_start, active_time_end, custom_start_date, custom_end_date, created_at, updated_at, deleted_at FROM limits WHERE deleted_at IS NULL AND limit_type = $1 ORDER BY created_at DESC, id DESC LIMIT 11`)).
					WithArgs(string(model.LimitTypeDaily)).
					WillReturnRows(rows)
			},
			wantLen: 1,
			wantErr: false,
		},
		{
			name:    "Success - empty results with default pagination",
			filters: &model.ListLimitsFilter{Limit: 10},
			mockSetup: func(mock sqlmock.Sqlmock) {
				// Query uses limit+1 (11) even for empty results
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, description, limit_type, max_amount, currency, scopes, status, reset_at, active_time_start, active_time_end, custom_start_date, custom_end_date, created_at, updated_at, deleted_at FROM limits WHERE deleted_at IS NULL ORDER BY created_at DESC, id DESC LIMIT 11`)).
					WillReturnRows(sqlmock.NewRows(limitColumns()))
			},
			wantLen: 0,
			wantErr: false,
		},
		{
			name:    "Error - database query fails",
			filters: &model.ListLimitsFilter{Limit: 10},
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, description, limit_type, max_amount, currency, scopes, status, reset_at, active_time_start, active_time_end, custom_start_date, custom_end_date, created_at, updated_at, deleted_at FROM limits WHERE deleted_at IS NULL ORDER BY created_at DESC, id DESC LIMIT 11`)).
					WillReturnError(errors.New("database error"))
			},
			wantErr: true,
			errMsg:  "failed to list limits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()
			result, err := repo.List(ctx, tt.filters)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Len(t, result.Limits, tt.wantLen)
			}

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestLimitRepository_UpdateStatus_ConnectionError(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(nil, errors.New("connection refused"))

	repo := NewLimitRepositoryWithConnection(mockConn)

	ctx := context.Background()
	err := repo.UpdateStatus(ctx, testutil.MustDeterministicUUID(998), model.LimitStatusInactive, testutil.FixedTime())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get database connection")
}

func TestLimitRepository_UpdateStatus(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		limitID   uuid.UUID
		status    model.LimitStatus
		mockSetup func(mock sqlmock.Sqlmock)
		wantErr   bool
		errType   error
		errMsg    string
	}{
		{
			name:    "Success - updates status to inactive",
			limitID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			status:  model.LimitStatusInactive,
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE limits`)).
					WithArgs(
						model.LimitStatusInactive,
						sqlmock.AnyArg(), // updatedAt
						uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantErr: false,
		},
		{
			name:    "Success - updates status to deleted with deleted_at",
			limitID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440002"),
			status:  model.LimitStatusDeleted,
			mockSetup: func(mock sqlmock.Sqlmock) {
				// When status is DELETED, the query includes deleted_at field
				// Args order: status, updated_at, deleted_at, id
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE limits`)).
					WithArgs(
						model.LimitStatusDeleted,
						sqlmock.AnyArg(), // updatedAt
						sqlmock.AnyArg(), // deleted_at (same as updatedAt)
						uuid.MustParse("550e8400-e29b-41d4-a716-446655440002"),
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantErr: false,
		},
		{
			name:    "Error - limit not found",
			limitID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440099"),
			status:  model.LimitStatusInactive,
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE limits`)).
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantErr: true,
			errType: constant.ErrLimitNotFound,
		},
		{
			name:    "Error - database update fails",
			limitID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			status:  model.LimitStatusInactive,
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE limits`)).
					WillReturnError(errors.New("database error"))
			},
			wantErr: true,
			errMsg:  "failed to update limit status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()
			err := repo.UpdateStatus(ctx, tt.limitID, tt.status, testutil.FixedTime())

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestLimitRepository_List_InvalidCursor(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	tests := []struct {
		name    string
		cursor  string
		wantErr bool
		errType error
	}{
		{
			name:    "Error - invalid base64 cursor",
			cursor:  "not-valid-base64!!!",
			wantErr: true,
			errType: constant.ErrInvalidCursor,
		},
		{
			name:    "Error - invalid UUID in cursor",
			cursor:  "bm90LWEtdXVpZA==", // base64 of "not-a-uuid"
			wantErr: true,
			errType: constant.ErrInvalidCursor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _, cleanup := setupLimitRepositoryMockDB(t)
			defer cleanup()

			ctx := context.Background()
			filters := &model.ListLimitsFilter{
				Cursor: tt.cursor,
				Limit:  10,
			}

			result, err := repo.List(ctx, filters)

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.errType)
			assert.Nil(t, result)
		})
	}
}

func TestLimitRepository_List_NilFilters(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
	defer cleanup()

	// Expect query with default limit (10+1=11)
	sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(sqlmock.NewRows(limitColumns()))

	ctx := context.Background()
	result, err := repo.List(ctx, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Limits)
	assert.False(t, result.HasMore)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestLimitRepository_List_ZeroLimit(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
	defer cleanup()

	// Expect query - with zero limit, should use default (10+1=11)
	sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(sqlmock.NewRows(limitColumns()))

	ctx := context.Background()
	filters := &model.ListLimitsFilter{Limit: 0}
	result, err := repo.List(ctx, filters)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Limits)
	assert.False(t, result.HasMore)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestLimitRepository_List_LimitBounds(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
	defer cleanup()

	// Expect query - with limit > MaxLimit (100), should cap at MaxLimit
	sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(sqlmock.NewRows(limitColumns()))

	ctx := context.Background()
	filters := &model.ListLimitsFilter{Limit: 500}
	result, err := repo.List(ctx, filters)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Limits)
	assert.False(t, result.HasMore)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestLimitRepository_normalizeListFilters(t *testing.T) {
	t.Parallel()

	repo := &LimitRepository{}

	tests := []struct {
		name          string
		filters       *model.ListLimitsFilter
		expectedLimit int
	}{
		{
			name:          "nil filters returns default limit",
			filters:       nil,
			expectedLimit: trcConstant.DefaultPaginationLimit,
		},
		{
			name:          "zero limit uses default",
			filters:       &model.ListLimitsFilter{Limit: 0},
			expectedLimit: trcConstant.DefaultPaginationLimit,
		},
		{
			name:          "negative limit uses default",
			filters:       &model.ListLimitsFilter{Limit: -5},
			expectedLimit: trcConstant.DefaultPaginationLimit,
		},
		{
			name:          "limit exceeding max is capped",
			filters:       &model.ListLimitsFilter{Limit: 500},
			expectedLimit: trcConstant.MaxPaginationLimit,
		},
		{
			name:          "valid limit is preserved",
			filters:       &model.ListLimitsFilter{Limit: 25},
			expectedLimit: 25,
		},
		{
			name:          "limit at max boundary is preserved",
			filters:       &model.ListLimitsFilter{Limit: trcConstant.MaxPaginationLimit},
			expectedLimit: trcConstant.MaxPaginationLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.normalizeListFilters(tt.filters)

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedLimit, result.Limit)
		})
	}
}

func TestLimitRepository_normalizeListFilters_PreservesOtherFields(t *testing.T) {
	t.Parallel()

	repo := &LimitRepository{}

	status := model.LimitStatusActive
	limitType := model.LimitTypeDaily
	filters := &model.ListLimitsFilter{
		Status:    &status,
		LimitType: &limitType,
		Limit:     25,
		Cursor:    "test-cursor",
		SortBy:    "name",
		SortOrder: "ASC",
	}

	result := repo.normalizeListFilters(filters)

	require.NotNil(t, result)
	assert.Equal(t, &status, result.Status)
	assert.Equal(t, &limitType, result.LimitType)
	assert.Equal(t, 25, result.Limit)
	assert.Equal(t, "test-cursor", result.Cursor)
	assert.Equal(t, "name", result.SortBy)
	assert.Equal(t, "ASC", result.SortOrder)
}

func TestLimitRepository_validateAndNormalizeSort(t *testing.T) {
	t.Parallel()

	repo := &LimitRepository{}

	tests := []struct {
		name              string
		filters           *model.ListLimitsFilter
		expectedSortBy    string
		expectedSortOrder string
		wantErr           bool
		errType           error
	}{
		{
			name:              "empty sortBy defaults to createdAt",
			filters:           &model.ListLimitsFilter{SortBy: "", SortOrder: ""},
			expectedSortBy:    "created_at", // Repository returns snake_case column name
			expectedSortOrder: "DESC",
			wantErr:           false,
		},
		{
			name:              "empty sortOrder defaults to DESC",
			filters:           &model.ListLimitsFilter{SortBy: "name", SortOrder: ""},
			expectedSortBy:    "name",
			expectedSortOrder: "DESC",
			wantErr:           false,
		},
		{
			name:              "valid sortBy name is accepted",
			filters:           &model.ListLimitsFilter{SortBy: "name", SortOrder: "ASC"},
			expectedSortBy:    "name",
			expectedSortOrder: "ASC",
			wantErr:           false,
		},
		{
			name:              "valid sortBy created_at is accepted",
			filters:           &model.ListLimitsFilter{SortBy: "created_at", SortOrder: "DESC"},
			expectedSortBy:    "created_at",
			expectedSortOrder: "DESC",
			wantErr:           false,
		},
		{
			name:              "valid sortBy updated_at is accepted",
			filters:           &model.ListLimitsFilter{SortBy: "updated_at", SortOrder: "ASC"},
			expectedSortBy:    "updated_at",
			expectedSortOrder: "ASC",
			wantErr:           false,
		},
		{
			name:              "valid sortBy max_amount is accepted",
			filters:           &model.ListLimitsFilter{SortBy: "max_amount", SortOrder: "DESC"},
			expectedSortBy:    "max_amount",
			expectedSortOrder: "DESC",
			wantErr:           false,
		},
		{
			name:              "lowercase sortOrder is uppercased",
			filters:           &model.ListLimitsFilter{SortBy: "name", SortOrder: "asc"},
			expectedSortBy:    "name",
			expectedSortOrder: "ASC",
			wantErr:           false,
		},
		{
			name:              "invalid sortOrder defaults to DESC",
			filters:           &model.ListLimitsFilter{SortBy: "name", SortOrder: "invalid"},
			expectedSortBy:    "name",
			expectedSortOrder: "DESC",
			wantErr:           false,
		},
		{
			name:    "invalid sortBy returns error",
			filters: &model.ListLimitsFilter{SortBy: "invalid_column", SortOrder: "ASC"},
			wantErr: true,
			errType: constant.ErrInvalidSortColumn,
		},
		{
			name:    "SQL injection attempt in sortBy returns error",
			filters: &model.ListLimitsFilter{SortBy: "name; DROP TABLE limits;--", SortOrder: "ASC"},
			wantErr: true,
			errType: constant.ErrInvalidSortColumn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortBy, sortOrder, err := repo.validateAndNormalizeSort(tt.filters)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedSortBy, sortBy)
				assert.Equal(t, tt.expectedSortOrder, sortOrder)
			}
		})
	}
}

func TestLimitRepository_List_InvalidSortColumn(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo, _, cleanup := setupLimitRepositoryMockDB(t)
	defer cleanup()

	ctx := context.Background()
	filters := &model.ListLimitsFilter{
		Limit:  10,
		SortBy: "invalid_column",
	}

	result, err := repo.List(ctx, filters)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrInvalidSortColumn)
	assert.Nil(t, result)
}

func TestLimitRepository_applyCursorFilter_CursorValidation(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	tests := []struct {
		name              string
		cursor            pkgHTTP.Cursor
		requestedSortBy   string
		requestedOrderDir string
		wantSortBy        string
		wantOrderDir      string
		wantErr           bool
		errType           error
		errContains       string
	}{
		{
			name: "Success - cursor with matching sort parameters",
			cursor: pkgHTTP.Cursor{
				ID:        "550e8400-e29b-41d4-a716-446655440001",
				SortValue: "2024-01-15T10:00:00Z",
				SortBy:    "created_at",
				SortOrder: "DESC",
			},
			requestedSortBy:   "created_at",
			requestedOrderDir: "DESC",
			wantSortBy:        "created_at",
			wantOrderDir:      "DESC",
			wantErr:           false,
		},
		{
			name: "Success - cursor sortBy empty defaults to created_at",
			cursor: pkgHTTP.Cursor{
				ID:        "550e8400-e29b-41d4-a716-446655440001",
				SortValue: "2024-01-15T10:00:00Z",
				SortBy:    "",
				SortOrder: "DESC",
			},
			requestedSortBy:   "created_at",
			requestedOrderDir: "DESC",
			wantSortBy:        "created_at",
			wantOrderDir:      "DESC",
			wantErr:           false,
		},
		{
			name: "Success - cursor sortOrder invalid defaults to DESC",
			cursor: pkgHTTP.Cursor{
				ID:        "550e8400-e29b-41d4-a716-446655440001",
				SortValue: "2024-01-15T10:00:00Z",
				SortBy:    "created_at",
				SortOrder: "INVALID",
			},
			requestedSortBy:   "created_at",
			requestedOrderDir: "DESC",
			wantSortBy:        "created_at",
			wantOrderDir:      "DESC",
			wantErr:           false,
		},
		{
			name: "Success - ASC sort order",
			cursor: pkgHTTP.Cursor{
				ID:        "550e8400-e29b-41d4-a716-446655440001",
				SortValue: "Test Limit",
				SortBy:    "name",
				SortOrder: "ASC",
			},
			requestedSortBy:   "name",
			requestedOrderDir: "ASC",
			wantSortBy:        "name",
			wantOrderDir:      "ASC",
			wantErr:           false,
		},
		{
			name: "Error - cursor sortBy does not match request",
			cursor: pkgHTTP.Cursor{
				ID:        "550e8400-e29b-41d4-a716-446655440001",
				SortValue: "2024-01-15T10:00:00Z",
				SortBy:    "created_at",
				SortOrder: "DESC",
			},
			requestedSortBy:   "name",
			requestedOrderDir: "DESC",
			wantErr:           true,
			errType:           constant.ErrInvalidCursor,
			errContains:       "cursor sortBy does not match request",
		},
		{
			name: "Error - cursor sortOrder does not match request",
			cursor: pkgHTTP.Cursor{
				ID:        "550e8400-e29b-41d4-a716-446655440001",
				SortValue: "2024-01-15T10:00:00Z",
				SortBy:    "created_at",
				SortOrder: "DESC",
			},
			requestedSortBy:   "created_at",
			requestedOrderDir: "ASC",
			wantErr:           true,
			errType:           constant.ErrInvalidCursor,
			errContains:       "cursor sortOrder does not match request",
		},
		{
			name: "Error - invalid sort column in cursor",
			cursor: pkgHTTP.Cursor{
				ID:        "550e8400-e29b-41d4-a716-446655440001",
				SortValue: "2024-01-15T10:00:00Z",
				SortBy:    "invalid_column",
				SortOrder: "DESC",
			},
			requestedSortBy:   "invalid_column",
			requestedOrderDir: "DESC",
			wantErr:           true,
			errType:           constant.ErrInvalidSortColumn,
		},
		{
			name: "Success - cursor with max_amount decimal value",
			cursor: pkgHTTP.Cursor{
				ID:        "550e8400-e29b-41d4-a716-446655440001",
				SortValue: "1000.50",
				SortBy:    "max_amount",
				SortOrder: "DESC",
			},
			requestedSortBy:   "max_amount",
			requestedOrderDir: "DESC",
			wantSortBy:        "max_amount",
			wantOrderDir:      "DESC",
			wantErr:           false,
		},
		{
			name: "Success - cursor with max_amount integer value",
			cursor: pkgHTTP.Cursor{
				ID:        "550e8400-e29b-41d4-a716-446655440001",
				SortValue: "5000",
				SortBy:    "max_amount",
				SortOrder: "ASC",
			},
			requestedSortBy:   "max_amount",
			requestedOrderDir: "ASC",
			wantSortBy:        "max_amount",
			wantOrderDir:      "ASC",
			wantErr:           false,
		},
		{
			name: "Error - SQL injection attempt in cursor sortBy",
			cursor: pkgHTTP.Cursor{
				ID:        "550e8400-e29b-41d4-a716-446655440001",
				SortValue: "2024-01-15T10:00:00Z",
				SortBy:    "name; DROP TABLE limits;--",
				SortOrder: "DESC",
			},
			requestedSortBy:   "name; DROP TABLE limits;--",
			requestedOrderDir: "DESC",
			wantErr:           true,
			errType:           constant.ErrInvalidSortColumn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
			defer cleanup()

			cursorStr, err := pkgHTTP.EncodeCursor(tt.cursor)
			require.NoError(t, err)

			filters := &model.ListLimitsFilter{
				Cursor:    cursorStr,
				Limit:     10,
				SortBy:    tt.requestedSortBy,
				SortOrder: tt.requestedOrderDir,
			}

			if !tt.wantErr {
				sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(sqlmock.NewRows(limitColumns()))
			}

			ctx := context.Background()
			result, err := repo.List(ctx, filters)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestLimitRepository_applyCursorFilter_SortOrderCaseInsensitive(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	// Test that cursor sortOrder comparison is case insensitive (request is uppercased)
	tests := []struct {
		name               string
		cursorSortOrder    string
		requestedSortOrder string
		wantErr            bool
	}{
		{
			name:               "ASC matches asc (uppercased)",
			cursorSortOrder:    "ASC",
			requestedSortOrder: "asc",
			wantErr:            false,
		},
		{
			name:               "DESC matches desc (uppercased)",
			cursorSortOrder:    "DESC",
			requestedSortOrder: "desc",
			wantErr:            false,
		},
		{
			name:               "DESC does not match ASC",
			cursorSortOrder:    "DESC",
			requestedSortOrder: "ASC",
			wantErr:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
			defer cleanup()

			cursor := pkgHTTP.Cursor{
				ID:        "550e8400-e29b-41d4-a716-446655440001",
				SortValue: "2024-01-15T10:00:00Z",
				SortBy:    "created_at",
				SortOrder: tt.cursorSortOrder,
			}
			cursorStr, err := pkgHTTP.EncodeCursor(cursor)
			require.NoError(t, err)

			filters := &model.ListLimitsFilter{
				Cursor:    cursorStr,
				Limit:     10,
				SortBy:    "created_at",
				SortOrder: tt.requestedSortOrder,
			}

			if !tt.wantErr {
				sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(sqlmock.NewRows(limitColumns()))
			}

			ctx := context.Background()
			result, err := repo.List(ctx, filters)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, constant.ErrInvalidCursor)
				assert.Contains(t, err.Error(), "cursor sortOrder does not match request")
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestLimitRepository_applyCursorFilter_InvalidSortColumnInCursor(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	// This test specifically covers the case where:
	// 1. Request has a valid sortBy (passes validateAndNormalizeSort)
	// 2. Cursor has an invalid sortBy (fails validation in applyCursorFilter lines 460-463)
	// This ensures the IsValidLimitSortField check inside applyCursorFilter is exercised

	tests := []struct {
		name            string
		cursorSortBy    string
		requestedSortBy string
		errType         error
	}{
		{
			name:            "cursor has invalid column while request has valid column",
			cursorSortBy:    "invalid_column",
			requestedSortBy: "created_at",
			errType:         constant.ErrInvalidSortColumn,
		},
		{
			name:            "cursor has SQL injection attempt while request is valid",
			cursorSortBy:    "name; DROP TABLE limits;--",
			requestedSortBy: "name",
			errType:         constant.ErrInvalidSortColumn,
		},
		{
			name:            "cursor has column with spaces while request is valid",
			cursorSortBy:    "created at",
			requestedSortBy: "created_at",
			errType:         constant.ErrInvalidSortColumn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
			defer cleanup()

			cursor := pkgHTTP.Cursor{
				ID:        "550e8400-e29b-41d4-a716-446655440001",
				SortValue: "2024-01-15T10:00:00Z",
				SortBy:    tt.cursorSortBy,
				SortOrder: "DESC",
			}
			cursorStr, err := pkgHTTP.EncodeCursor(cursor)
			require.NoError(t, err)

			filters := &model.ListLimitsFilter{
				Cursor:    cursorStr,
				Limit:     10,
				SortBy:    tt.requestedSortBy,
				SortOrder: "DESC",
			}

			ctx := context.Background()
			result, err := repo.List(ctx, filters)

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.errType)
			assert.Nil(t, result)

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestLimitRepository_applyCursorFilter_EmptyCursor(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
	defer cleanup()

	// Empty cursor should not trigger any cursor validation
	filters := &model.ListLimitsFilter{
		Cursor:    "",
		Limit:     10,
		SortBy:    "created_at",
		SortOrder: "DESC",
	}

	sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(sqlmock.NewRows(limitColumns()))

	ctx := context.Background()
	result, err := repo.List(ctx, filters)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestLimitRepository_buildNextCursor(t *testing.T) {
	t.Parallel()

	repo := &LimitRepository{}

	tests := []struct {
		name           string
		limit          *model.Limit
		sortBy         string
		sortOrder      string
		wantSortValue  string
		wantSortOrder  string // Expected sortOrder in cursor (may be normalized)
		wantPointsNext bool
	}{
		{
			name:           "builds cursor with created_at sort",
			limit:          testLimit(),
			sortBy:         "created_at",
			sortOrder:      "DESC",
			wantSortValue:  testLimit().CreatedAt.Format(time.RFC3339Nano),
			wantSortOrder:  "DESC",
			wantPointsNext: true,
		},
		{
			name:           "builds cursor with updated_at sort",
			limit:          testLimit(),
			sortBy:         "updated_at",
			sortOrder:      "ASC",
			wantSortValue:  testLimit().UpdatedAt.Format(time.RFC3339Nano),
			wantSortOrder:  "ASC",
			wantPointsNext: true,
		},
		{
			name:           "builds cursor with name sort",
			limit:          testLimit(),
			sortBy:         "name",
			sortOrder:      "ASC",
			wantSortValue:  testLimit().Name,
			wantSortOrder:  "ASC",
			wantPointsNext: true,
		},
		{
			name:           "builds cursor with max_amount sort",
			limit:          testLimit(),
			sortBy:         "max_amount",
			sortOrder:      "DESC",
			wantSortValue:  "1000",
			wantSortOrder:  "DESC",
			wantPointsNext: true,
		},
		{
			name:           "normalizes lowercase sortOrder to uppercase",
			limit:          testLimit(),
			sortBy:         "created_at",
			sortOrder:      "desc",
			wantSortValue:  testLimit().CreatedAt.Format(time.RFC3339Nano),
			wantSortOrder:  "DESC",
			wantPointsNext: true,
		},
		{
			name:           "normalizes mixed case sortOrder to uppercase",
			limit:          testLimit(),
			sortBy:         "name",
			sortOrder:      "Asc",
			wantSortValue:  testLimit().Name,
			wantSortOrder:  "ASC",
			wantPointsNext: true,
		},
		{
			name:           "defaults invalid sortOrder to DESC",
			limit:          testLimit(),
			sortBy:         "created_at",
			sortOrder:      "invalid",
			wantSortValue:  testLimit().CreatedAt.Format(time.RFC3339Nano),
			wantSortOrder:  "DESC",
			wantPointsNext: true,
		},
		{
			name:           "defaults empty sortOrder to DESC",
			limit:          testLimit(),
			sortBy:         "created_at",
			sortOrder:      "",
			wantSortValue:  testLimit().CreatedAt.Format(time.RFC3339Nano),
			wantSortOrder:  "DESC",
			wantPointsNext: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cursorStr, err := repo.buildNextCursor(tt.limit, tt.sortBy, tt.sortOrder)

			require.NoError(t, err)
			require.NotEmpty(t, cursorStr)

			// Decode the cursor to verify its contents
			cursor, err := pkgHTTP.DecodeCursor(cursorStr)
			require.NoError(t, err)

			assert.Equal(t, tt.limit.ID.String(), cursor.ID)
			assert.Equal(t, tt.wantSortValue, cursor.SortValue)
			assert.Equal(t, tt.sortBy, cursor.SortBy)
			assert.Equal(t, tt.wantSortOrder, cursor.SortOrder)
			assert.Equal(t, tt.wantPointsNext, cursor.PointsNext)
		})
	}
}

func TestLimitRepository_buildNextCursor_InvalidSortBy(t *testing.T) {
	t.Parallel()

	repo := &LimitRepository{}

	tests := []struct {
		name   string
		sortBy string
	}{
		{
			name:   "rejects unknown sortBy field",
			sortBy: "unknown_field",
		},
		{
			name:   "rejects empty sortBy",
			sortBy: "",
		},
		{
			name:   "rejects SQL injection attempt",
			sortBy: "created_at; DROP TABLE limits;--",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cursorStr, err := repo.buildNextCursor(testLimit(), tt.sortBy, "DESC")

			assert.ErrorIs(t, err, constant.ErrInvalidSortColumn)
			assert.Empty(t, cursorStr)
		})
	}
}

func TestLimitRepository_buildNextCursor_RoundTrip(t *testing.T) {
	t.Parallel()

	repo := &LimitRepository{}

	// Test that a cursor can be encoded and then decoded back for use in pagination
	lmt := testLimit()
	sortBy := "created_at"
	sortOrder := "DESC"

	// Build cursor from limit
	cursorStr, err := repo.buildNextCursor(lmt, sortBy, sortOrder)
	require.NoError(t, err)

	// Decode cursor
	cursor, err := pkgHTTP.DecodeCursor(cursorStr)
	require.NoError(t, err)

	// Verify all fields are preserved
	assert.Equal(t, lmt.ID.String(), cursor.ID)
	assert.Equal(t, lmt.CreatedAt.Format(time.RFC3339Nano), cursor.SortValue)
	assert.Equal(t, sortBy, cursor.SortBy)
	assert.Equal(t, sortOrder, cursor.SortOrder)
	assert.True(t, cursor.PointsNext)

	// Re-encode to verify stability
	reEncodedStr, err := pkgHTTP.EncodeCursor(cursor)
	require.NoError(t, err)
	assert.Equal(t, cursorStr, reEncodedStr)
}

func TestGetSortValueFromLimit(t *testing.T) {
	t.Parallel()

	lmt := testLimit()

	tests := []struct {
		name      string
		sortBy    string
		wantValue string
	}{
		{
			name:      "name field",
			sortBy:    "name",
			wantValue: lmt.Name,
		},
		{
			name:      "max_amount field",
			sortBy:    "max_amount",
			wantValue: "1000",
		},
		{
			name:      "updated_at field",
			sortBy:    "updated_at",
			wantValue: lmt.UpdatedAt.Format(time.RFC3339Nano),
		},
		{
			name:      "created_at field",
			sortBy:    "created_at",
			wantValue: lmt.CreatedAt.Format(time.RFC3339Nano),
		},
		{
			name:      "unknown field defaults to created_at",
			sortBy:    "unknown",
			wantValue: lmt.CreatedAt.Format(time.RFC3339Nano),
		},
		{
			name:      "empty field defaults to created_at",
			sortBy:    "",
			wantValue: lmt.CreatedAt.Format(time.RFC3339Nano),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSortValueFromLimit(lmt, tt.sortBy)
			assert.Equal(t, tt.wantValue, result)
		})
	}
}

func TestGetSortValueFromLimit_DifferentAmounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		maxAmount decimal.Decimal
		wantValue string
	}{
		{
			name:      "zero amount",
			maxAmount: decimal.RequireFromString("0"),
			wantValue: "0",
		},
		{
			name:      "small amount",
			maxAmount: decimal.RequireFromString("1"),
			wantValue: "1",
		},
		{
			name:      "large amount",
			maxAmount: decimal.RequireFromString("99999999.99"),
			wantValue: "99999999.99",
		},
		{
			name:      "negative amount",
			maxAmount: decimal.RequireFromString("-5"),
			wantValue: "-5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lmt := testLimit()
			lmt.MaxAmount = tt.maxAmount

			result := getSortValueFromLimit(lmt, "max_amount")
			assert.Equal(t, tt.wantValue, result)
		})
	}
}

func TestLimitRepository_List_Pagination_HasMore(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
	defer cleanup()

	// Create 3 limits - request limit is 2, so we expect hasMore=true
	limit1 := testLimit()
	limit1.ID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	limit1.Name = "Limit 1"

	limit2 := testLimit()
	limit2.ID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	limit2.Name = "Limit 2"

	limit3 := testLimit()
	limit3.ID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440003")
	limit3.Name = "Limit 3"

	// Return 3 rows (limit+1) to indicate more pages exist
	rows := sqlmock.NewRows(limitColumns())
	for _, lmt := range []*model.Limit{limit1, limit2, limit3} {
		scopesJSON, err := json.Marshal(lmt.Scopes)
		require.NoError(t, err)

		var resetAt interface{}
		if lmt.ResetAt != nil {
			resetAt = *lmt.ResetAt
		}
		rows.AddRow(
			lmt.ID, lmt.Name, lmt.Description, lmt.LimitType, lmt.MaxAmount,
			lmt.Currency, scopesJSON, lmt.Status, resetAt,
			nil, nil, nil, nil,
			lmt.CreatedAt, lmt.UpdatedAt, nil,
		)
	}

	sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(rows)

	ctx := context.Background()
	filters := &model.ListLimitsFilter{
		Limit:     2,
		SortBy:    "created_at",
		SortOrder: "DESC",
	}

	result, err := repo.List(ctx, filters)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Limits, 2, "should return only requested limit count")
	assert.True(t, result.HasMore, "should indicate more pages exist")
	assert.NotEmpty(t, result.NextCursor, "should have next cursor when hasMore is true")

	// Verify the cursor contains correct data from last returned item (limit2)
	cursor, err := pkgHTTP.DecodeCursor(result.NextCursor)
	require.NoError(t, err)
	assert.Equal(t, limit2.ID.String(), cursor.ID)
	assert.Equal(t, limit2.CreatedAt.Format(time.RFC3339Nano), cursor.SortValue)
	assert.Equal(t, "created_at", cursor.SortBy)
	assert.Equal(t, "DESC", cursor.SortOrder)
	assert.True(t, cursor.PointsNext)

	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestLimitRepository_List_Pagination_NoMore(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
	defer cleanup()

	// Create 2 limits - request limit is 2, so we expect hasMore=false
	limit1 := testLimit()
	limit1.ID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	limit2 := testLimit()
	limit2.ID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")

	// Return exactly 2 rows (equal to limit) - no more pages
	rows := sqlmock.NewRows(limitColumns())
	for _, lmt := range []*model.Limit{limit1, limit2} {
		scopesJSON, err := json.Marshal(lmt.Scopes)
		require.NoError(t, err)

		var resetAt interface{}
		if lmt.ResetAt != nil {
			resetAt = *lmt.ResetAt
		}
		rows.AddRow(
			lmt.ID, lmt.Name, lmt.Description, lmt.LimitType, lmt.MaxAmount,
			lmt.Currency, scopesJSON, lmt.Status, resetAt,
			nil, nil, nil, nil,
			lmt.CreatedAt, lmt.UpdatedAt, nil,
		)
	}

	sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(rows)

	ctx := context.Background()
	filters := &model.ListLimitsFilter{
		Limit:     2,
		SortBy:    "created_at",
		SortOrder: "DESC",
	}

	result, err := repo.List(ctx, filters)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Limits, 2)
	assert.False(t, result.HasMore, "should indicate no more pages")
	assert.Empty(t, result.NextCursor, "should not have next cursor when hasMore is false")

	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestLimitRepository_List_Pagination_EmptyResult(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
	defer cleanup()

	// Return empty result
	sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(sqlmock.NewRows(limitColumns()))

	ctx := context.Background()
	filters := &model.ListLimitsFilter{
		Limit:     10,
		SortBy:    "created_at",
		SortOrder: "DESC",
	}

	result, err := repo.List(ctx, filters)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Limits)
	assert.False(t, result.HasMore)
	assert.Empty(t, result.NextCursor)

	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestLimitRepository_List_Pagination_SingleResult(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
	defer cleanup()

	// Single result with limit > 1
	lmt := testLimit()
	rows := limitRow(t, lmt)

	sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(rows)

	ctx := context.Background()
	filters := &model.ListLimitsFilter{
		Limit:     10,
		SortBy:    "created_at",
		SortOrder: "DESC",
	}

	result, err := repo.List(ctx, filters)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Limits, 1)
	assert.False(t, result.HasMore, "single result less than limit means no more pages")
	assert.Empty(t, result.NextCursor)

	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestLimitRepository_List_Pagination_ExactlyAtLimit(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	// When we get exactly (limit+1) results, hasMore is true
	// When we get exactly limit results, hasMore is false
	// This test verifies the boundary condition

	repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
	defer cleanup()

	// Request limit=1, return 2 results (limit+1) to trigger hasMore
	limit1 := testLimit()
	limit1.ID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	limit2 := testLimit()
	limit2.ID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")

	rows := sqlmock.NewRows(limitColumns())
	for _, lmt := range []*model.Limit{limit1, limit2} {
		scopesJSON, err := json.Marshal(lmt.Scopes)
		require.NoError(t, err)

		var resetAt interface{}
		if lmt.ResetAt != nil {
			resetAt = *lmt.ResetAt
		}
		rows.AddRow(
			lmt.ID, lmt.Name, lmt.Description, lmt.LimitType, lmt.MaxAmount,
			lmt.Currency, scopesJSON, lmt.Status, resetAt,
			nil, nil, nil, nil,
			lmt.CreatedAt, lmt.UpdatedAt, nil,
		)
	}

	sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(rows)

	ctx := context.Background()
	filters := &model.ListLimitsFilter{
		Limit:     1,
		SortBy:    "created_at",
		SortOrder: "DESC",
	}

	result, err := repo.List(ctx, filters)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Limits, 1, "should trim to requested limit")
	assert.True(t, result.HasMore, "receiving limit+1 results means more pages")
	assert.NotEmpty(t, result.NextCursor)

	// Cursor should point to the last returned item
	cursor, err := pkgHTTP.DecodeCursor(result.NextCursor)
	require.NoError(t, err)
	assert.Equal(t, limit1.ID.String(), cursor.ID)
	assert.Equal(t, limit1.CreatedAt.Format(time.RFC3339Nano), cursor.SortValue)
	assert.Equal(t, "created_at", cursor.SortBy)
	assert.Equal(t, "DESC", cursor.SortOrder)
	assert.True(t, cursor.PointsNext)

	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestLimitRepository_List_Pagination_CursorWithDifferentSortFields(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		sortBy    string
		sortOrder string
	}{
		{
			name:      "sort by name ASC",
			sortBy:    "name",
			sortOrder: "ASC",
		},
		{
			name:      "sort by updated_at DESC",
			sortBy:    "updated_at",
			sortOrder: "DESC",
		},
		{
			name:      "sort by max_amount ASC",
			sortBy:    "max_amount",
			sortOrder: "ASC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupLimitRepositoryMockDB(t)
			defer cleanup()

			// Return 2 results with limit=1 to trigger hasMore
			limit1 := testLimit()
			limit1.ID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

			limit2 := testLimit()
			limit2.ID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")

			rows := sqlmock.NewRows(limitColumns())
			for _, lmt := range []*model.Limit{limit1, limit2} {
				scopesJSON, err := json.Marshal(lmt.Scopes)
				require.NoError(t, err)

				var resetAt interface{}
				if lmt.ResetAt != nil {
					resetAt = *lmt.ResetAt
				}
				rows.AddRow(
					lmt.ID, lmt.Name, lmt.Description, lmt.LimitType, lmt.MaxAmount,
					lmt.Currency, scopesJSON, lmt.Status, resetAt,
					nil, nil, nil, nil,
					lmt.CreatedAt, lmt.UpdatedAt, nil,
				)
			}

			sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(rows)

			ctx := context.Background()
			filters := &model.ListLimitsFilter{
				Limit:     1,
				SortBy:    tt.sortBy,
				SortOrder: tt.sortOrder,
			}

			result, err := repo.List(ctx, filters)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.HasMore)
			assert.NotEmpty(t, result.NextCursor)

			// Verify cursor has correct sort parameters
			cursor, err := pkgHTTP.DecodeCursor(result.NextCursor)
			require.NoError(t, err)
			assert.Equal(t, limit1.ID.String(), cursor.ID)
			assert.Equal(t, tt.sortBy, cursor.SortBy)
			assert.Equal(t, tt.sortOrder, cursor.SortOrder)
			assert.True(t, cursor.PointsNext)

			// Verify SortValue matches the expected value from the last returned item
			expectedSortValue := getSortValueFromLimit(limit1, tt.sortBy)
			assert.Equal(t, expectedSortValue, cursor.SortValue)

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

// =============================================================================
// snake_case migration tests for sort field mapping, value extraction,
// and cursor validation
// =============================================================================

func TestLimitSortFieldToColumn_SnakeCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		sortBy   string
		expected string
	}{
		{name: "created_at maps correctly", sortBy: "created_at", expected: "created_at"},
		{name: "updated_at maps correctly", sortBy: "updated_at", expected: "updated_at"},
		{name: "max_amount maps correctly", sortBy: "max_amount", expected: "max_amount"},
		{name: "name maps correctly", sortBy: "name", expected: "name"},
		{name: "createdAt is rejected", sortBy: "createdAt", expected: ""},
		{name: "updatedAt is rejected", sortBy: "updatedAt", expected: ""},
		{name: "maxAmount is rejected", sortBy: "maxAmount", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapLimitSortFieldToColumn(tt.sortBy)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetSortValueFromLimit_SnakeCase(t *testing.T) {
	t.Parallel()

	lmt := testLimit()

	tests := []struct {
		name     string
		sortBy   string
		expected string
	}{
		{name: "created_at returns value", sortBy: "created_at", expected: lmt.CreatedAt.Format(time.RFC3339Nano)},
		{name: "updated_at returns value", sortBy: "updated_at", expected: lmt.UpdatedAt.Format(time.RFC3339Nano)},
		{name: "max_amount returns value", sortBy: "max_amount", expected: lmt.MaxAmount.String()},
		{name: "name returns value", sortBy: "name", expected: lmt.Name},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSortValueFromLimit(lmt, tt.sortBy)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateCursorSortValue_SnakeCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sortBy    string
		sortValue string
		wantErr   bool
	}{
		{
			name:      "created_at with valid timestamp",
			sortBy:    "created_at",
			sortValue: "2024-01-15T10:00:00Z",
			wantErr:   false,
		},
		{
			name:      "updated_at with valid timestamp",
			sortBy:    "updated_at",
			sortValue: "2024-01-15T10:00:00Z",
			wantErr:   false,
		},
		{
			name:      "max_amount with valid decimal",
			sortBy:    "max_amount",
			sortValue: "1000.50",
			wantErr:   false,
		},
		{
			name:      "name with valid string",
			sortBy:    "name",
			sortValue: "test-limit",
			wantErr:   false,
		},
		{
			name:      "created_at with invalid timestamp",
			sortBy:    "created_at",
			sortValue: "not-a-timestamp",
			wantErr:   true,
		},
		{
			name:      "max_amount with invalid decimal",
			sortBy:    "max_amount",
			sortValue: "not-a-number",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCursorSortValue(tt.sortBy, tt.sortValue)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
