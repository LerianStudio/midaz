// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/model"
)

// --------------------------------------------------------------------------
// Mock-based Repository interface contract tests
// --------------------------------------------------------------------------

func TestMockRepository_Query(t *testing.T) {
	t.Parallel()

	schema := []TableSchema{
		{
			SchemaName: "public",
			TableName:  "transactions",
			Columns: []ColumnInformation{
				{Name: "id", DataType: "uuid", IsPrimaryKey: true},
				{Name: "amount", DataType: "numeric"},
				{Name: "status", DataType: "varchar"},
				{Name: "created_at", DataType: "timestamp"},
			},
		},
	}

	tests := []struct {
		name       string
		schemaName string
		table      string
		fields     []string
		filter     map[string][]any
		mockSetup  func(m *MockRepository)
		wantResult []map[string]any
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "success_with_results",
			schemaName: "public",
			table:      "transactions",
			fields:     []string{"id", "amount", "status"},
			filter:     map[string][]any{"status": {"active"}},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					Query(gomock.Any(), gomock.Any(), "public", "transactions", []string{"id", "amount", "status"}, map[string][]any{"status": {"active"}}).
					Return([]map[string]any{
						{"id": "uuid-1", "amount": 100.50, "status": "active"},
						{"id": "uuid-2", "amount": 200.75, "status": "active"},
					}, nil)
			},
			wantResult: []map[string]any{
				{"id": "uuid-1", "amount": 100.50, "status": "active"},
				{"id": "uuid-2", "amount": 200.75, "status": "active"},
			},
		},
		{
			name:       "success_empty_results",
			schemaName: "public",
			table:      "transactions",
			fields:     []string{"id", "amount"},
			filter:     map[string][]any{"status": {"nonexistent"}},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					Query(gomock.Any(), gomock.Any(), "public", "transactions", []string{"id", "amount"}, map[string][]any{"status": {"nonexistent"}}).
					Return([]map[string]any{}, nil)
			},
			wantResult: []map[string]any{},
		},
		{
			name:       "success_nil_filter",
			schemaName: "public",
			table:      "transactions",
			fields:     []string{"id"},
			filter:     nil,
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					Query(gomock.Any(), gomock.Any(), "public", "transactions", []string{"id"}, gomock.Nil()).
					Return([]map[string]any{{"id": "uuid-1"}}, nil)
			},
			wantResult: []map[string]any{{"id": "uuid-1"}},
		},
		{
			name:       "success_no_schema_name",
			schemaName: "",
			table:      "transactions",
			fields:     []string{"id", "amount"},
			filter:     map[string][]any{},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					Query(gomock.Any(), gomock.Any(), "", "transactions", []string{"id", "amount"}, map[string][]any{}).
					Return([]map[string]any{{"id": "uuid-1", "amount": 50.0}}, nil)
			},
			wantResult: []map[string]any{{"id": "uuid-1", "amount": 50.0}},
		},
		{
			name:       "error_database_failure",
			schemaName: "public",
			table:      "transactions",
			fields:     []string{"id"},
			filter:     map[string][]any{},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					Query(gomock.Any(), gomock.Any(), "public", "transactions", []string{"id"}, map[string][]any{}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: true,
			errMsg:  "connection refused",
		},
		{
			name:       "error_invalid_table",
			schemaName: "public",
			table:      "nonexistent_table",
			fields:     []string{"id"},
			filter:     map[string][]any{},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					Query(gomock.Any(), gomock.Any(), "public", "nonexistent_table", []string{"id"}, map[string][]any{}).
					Return(nil, errors.New("table 'nonexistent_table' does not exist in the database"))
			},
			wantErr: true,
			errMsg:  "does not exist",
		},
		{
			name:       "success_multiple_filter_values",
			schemaName: "public",
			table:      "transactions",
			fields:     []string{"id", "status"},
			filter:     map[string][]any{"status": {"active", "pending", "completed"}},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					Query(gomock.Any(), gomock.Any(), "public", "transactions", []string{"id", "status"}, map[string][]any{"status": {"active", "pending", "completed"}}).
					Return([]map[string]any{
						{"id": "uuid-1", "status": "active"},
						{"id": "uuid-2", "status": "pending"},
					}, nil)
			},
			wantResult: []map[string]any{
				{"id": "uuid-1", "status": "active"},
				{"id": "uuid-2", "status": "pending"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockRepo := NewMockRepository(ctrl)
			tt.mockSetup(mockRepo)

			result, err := mockRepo.Query(context.Background(), schema, tt.schemaName, tt.table, tt.fields, tt.filter)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, result)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func TestMockRepository_QueryWithAdvancedFilters(t *testing.T) {
	t.Parallel()

	schema := []TableSchema{
		{
			SchemaName: "payment",
			TableName:  "transfers",
			Columns: []ColumnInformation{
				{Name: "id", DataType: "uuid", IsPrimaryKey: true},
				{Name: "amount", DataType: "numeric"},
				{Name: "status", DataType: "varchar"},
				{Name: "created_at", DataType: "timestamp"},
			},
		},
	}

	tests := []struct {
		name       string
		schemaName string
		table      string
		fields     []string
		filter     map[string]model.FilterCondition
		mockSetup  func(m *MockRepository)
		wantResult []map[string]any
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "success_equals_filter",
			schemaName: "payment",
			table:      "transfers",
			fields:     []string{"id", "amount", "status"},
			filter: map[string]model.FilterCondition{
				"status": {Equals: []any{"completed"}},
			},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					QueryWithAdvancedFilters(gomock.Any(), gomock.Any(), "payment", "transfers", []string{"id", "amount", "status"}, gomock.Any()).
					Return([]map[string]any{
						{"id": "uuid-1", "amount": 500.0, "status": "completed"},
					}, nil)
			},
			wantResult: []map[string]any{
				{"id": "uuid-1", "amount": 500.0, "status": "completed"},
			},
		},
		{
			name:       "success_greater_than_filter",
			schemaName: "payment",
			table:      "transfers",
			fields:     []string{"id", "amount"},
			filter: map[string]model.FilterCondition{
				"amount": {GreaterThan: []any{100}},
			},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					QueryWithAdvancedFilters(gomock.Any(), gomock.Any(), "payment", "transfers", []string{"id", "amount"}, gomock.Any()).
					Return([]map[string]any{
						{"id": "uuid-1", "amount": 200.0},
						{"id": "uuid-2", "amount": 350.0},
					}, nil)
			},
			wantResult: []map[string]any{
				{"id": "uuid-1", "amount": 200.0},
				{"id": "uuid-2", "amount": 350.0},
			},
		},
		{
			name:       "success_between_filter",
			schemaName: "payment",
			table:      "transfers",
			fields:     []string{"id", "created_at"},
			filter: map[string]model.FilterCondition{
				"created_at": {Between: []any{"2025-01-01", "2025-12-31"}},
			},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					QueryWithAdvancedFilters(gomock.Any(), gomock.Any(), "payment", "transfers", []string{"id", "created_at"}, gomock.Any()).
					Return([]map[string]any{
						{"id": "uuid-1", "created_at": "2025-06-15"},
					}, nil)
			},
			wantResult: []map[string]any{
				{"id": "uuid-1", "created_at": "2025-06-15"},
			},
		},
		{
			name:       "success_in_filter",
			schemaName: "payment",
			table:      "transfers",
			fields:     []string{"id", "status"},
			filter: map[string]model.FilterCondition{
				"status": {In: []any{"active", "pending", "completed"}},
			},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					QueryWithAdvancedFilters(gomock.Any(), gomock.Any(), "payment", "transfers", []string{"id", "status"}, gomock.Any()).
					Return([]map[string]any{
						{"id": "uuid-1", "status": "active"},
						{"id": "uuid-2", "status": "completed"},
					}, nil)
			},
			wantResult: []map[string]any{
				{"id": "uuid-1", "status": "active"},
				{"id": "uuid-2", "status": "completed"},
			},
		},
		{
			name:       "success_not_in_filter",
			schemaName: "payment",
			table:      "transfers",
			fields:     []string{"id", "status"},
			filter: map[string]model.FilterCondition{
				"status": {NotIn: []any{"deleted", "archived"}},
			},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					QueryWithAdvancedFilters(gomock.Any(), gomock.Any(), "payment", "transfers", []string{"id", "status"}, gomock.Any()).
					Return([]map[string]any{
						{"id": "uuid-1", "status": "active"},
					}, nil)
			},
			wantResult: []map[string]any{
				{"id": "uuid-1", "status": "active"},
			},
		},
		{
			name:       "success_combined_filters",
			schemaName: "payment",
			table:      "transfers",
			fields:     []string{"id", "amount", "status"},
			filter: map[string]model.FilterCondition{
				"amount": {GreaterOrEqual: []any{100}, LessOrEqual: []any{1000}},
				"status": {Equals: []any{"completed"}},
			},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					QueryWithAdvancedFilters(gomock.Any(), gomock.Any(), "payment", "transfers", []string{"id", "amount", "status"}, gomock.Any()).
					Return([]map[string]any{
						{"id": "uuid-1", "amount": 500.0, "status": "completed"},
					}, nil)
			},
			wantResult: []map[string]any{
				{"id": "uuid-1", "amount": 500.0, "status": "completed"},
			},
		},
		{
			name:       "success_empty_results",
			schemaName: "payment",
			table:      "transfers",
			fields:     []string{"id"},
			filter: map[string]model.FilterCondition{
				"status": {Equals: []any{"nonexistent"}},
			},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					QueryWithAdvancedFilters(gomock.Any(), gomock.Any(), "payment", "transfers", []string{"id"}, gomock.Any()).
					Return([]map[string]any{}, nil)
			},
			wantResult: []map[string]any{},
		},
		{
			name:       "error_database_failure",
			schemaName: "payment",
			table:      "transfers",
			fields:     []string{"id"},
			filter: map[string]model.FilterCondition{
				"status": {Equals: []any{"active"}},
			},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					QueryWithAdvancedFilters(gomock.Any(), gomock.Any(), "payment", "transfers", []string{"id"}, gomock.Any()).
					Return(nil, errors.New("query execution timeout"))
			},
			wantErr: true,
			errMsg:  "query execution timeout",
		},
		{
			name:       "error_invalid_filter",
			schemaName: "payment",
			table:      "transfers",
			fields:     []string{"id"},
			filter: map[string]model.FilterCondition{
				"amount": {Between: []any{100}},
			},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					QueryWithAdvancedFilters(gomock.Any(), gomock.Any(), "payment", "transfers", []string{"id"}, gomock.Any()).
					Return(nil, errors.New("between operator for field 'amount' must have exactly 2 values"))
			},
			wantErr: true,
			errMsg:  "between operator",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockRepo := NewMockRepository(ctrl)
			tt.mockSetup(mockRepo)

			result, err := mockRepo.QueryWithAdvancedFilters(context.Background(), schema, tt.schemaName, tt.table, tt.fields, tt.filter)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, result)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func TestMockRepository_GetDatabaseSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		schemas    []string
		mockSetup  func(m *MockRepository)
		wantResult []TableSchema
		wantErr    bool
		errMsg     string
	}{
		{
			name:    "success_single_schema",
			schemas: []string{"public"},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					GetDatabaseSchema(gomock.Any(), []string{"public"}).
					Return([]TableSchema{
						{
							SchemaName: "public",
							TableName:  "users",
							Columns: []ColumnInformation{
								{Name: "id", DataType: "uuid", IsPrimaryKey: true},
								{Name: "name", DataType: "varchar", IsNullable: false},
								{Name: "email", DataType: "varchar", IsNullable: true},
							},
						},
						{
							SchemaName: "public",
							TableName:  "orders",
							Columns: []ColumnInformation{
								{Name: "id", DataType: "uuid", IsPrimaryKey: true},
								{Name: "total", DataType: "numeric"},
							},
						},
					}, nil)
			},
			wantResult: []TableSchema{
				{
					SchemaName: "public",
					TableName:  "users",
					Columns: []ColumnInformation{
						{Name: "id", DataType: "uuid", IsPrimaryKey: true},
						{Name: "name", DataType: "varchar", IsNullable: false},
						{Name: "email", DataType: "varchar", IsNullable: true},
					},
				},
				{
					SchemaName: "public",
					TableName:  "orders",
					Columns: []ColumnInformation{
						{Name: "id", DataType: "uuid", IsPrimaryKey: true},
						{Name: "total", DataType: "numeric"},
					},
				},
			},
		},
		{
			name:    "success_multiple_schemas",
			schemas: []string{"public", "payment", "transfer"},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					GetDatabaseSchema(gomock.Any(), []string{"public", "payment", "transfer"}).
					Return([]TableSchema{
						{SchemaName: "public", TableName: "users", Columns: []ColumnInformation{{Name: "id", DataType: "uuid"}}},
						{SchemaName: "payment", TableName: "invoices", Columns: []ColumnInformation{{Name: "id", DataType: "uuid"}}},
						{SchemaName: "transfer", TableName: "transfers", Columns: []ColumnInformation{{Name: "id", DataType: "uuid"}}},
					}, nil)
			},
			wantResult: []TableSchema{
				{SchemaName: "public", TableName: "users", Columns: []ColumnInformation{{Name: "id", DataType: "uuid"}}},
				{SchemaName: "payment", TableName: "invoices", Columns: []ColumnInformation{{Name: "id", DataType: "uuid"}}},
				{SchemaName: "transfer", TableName: "transfers", Columns: []ColumnInformation{{Name: "id", DataType: "uuid"}}},
			},
		},
		{
			name:    "success_empty_schema",
			schemas: []string{"empty_schema"},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					GetDatabaseSchema(gomock.Any(), []string{"empty_schema"}).
					Return([]TableSchema{}, nil)
			},
			wantResult: []TableSchema{},
		},
		{
			name:    "success_schema_with_primary_keys",
			schemas: []string{"public"},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					GetDatabaseSchema(gomock.Any(), []string{"public"}).
					Return([]TableSchema{
						{
							SchemaName: "public",
							TableName:  "accounts",
							Columns: []ColumnInformation{
								{Name: "id", DataType: "uuid", IsPrimaryKey: true},
								{Name: "organization_id", DataType: "uuid", IsPrimaryKey: false},
								{Name: "balance", DataType: "numeric", IsNullable: false},
								{Name: "deleted_at", DataType: "timestamp", IsNullable: true},
							},
						},
					}, nil)
			},
			wantResult: []TableSchema{
				{
					SchemaName: "public",
					TableName:  "accounts",
					Columns: []ColumnInformation{
						{Name: "id", DataType: "uuid", IsPrimaryKey: true},
						{Name: "organization_id", DataType: "uuid", IsPrimaryKey: false},
						{Name: "balance", DataType: "numeric", IsNullable: false},
						{Name: "deleted_at", DataType: "timestamp", IsNullable: true},
					},
				},
			},
		},
		{
			name:    "error_connection_failure",
			schemas: []string{"public"},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					GetDatabaseSchema(gomock.Any(), []string{"public"}).
					Return(nil, errors.New("error querying tables: connection refused"))
			},
			wantErr: true,
			errMsg:  "connection refused",
		},
		{
			name:    "error_timeout",
			schemas: []string{"public"},
			mockSetup: func(m *MockRepository) {
				m.EXPECT().
					GetDatabaseSchema(gomock.Any(), []string{"public"}).
					Return(nil, errors.New("schema discovery timeout"))
			},
			wantErr: true,
			errMsg:  "schema discovery timeout",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockRepo := NewMockRepository(ctrl)
			tt.mockSetup(mockRepo)

			result, err := mockRepo.GetDatabaseSchema(context.Background(), tt.schemas)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, result)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func TestMockRepository_CloseConnection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mockSetup func(m *MockRepository)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "success",
			mockSetup: func(m *MockRepository) {
				m.EXPECT().CloseConnection().Return(nil)
			},
		},
		{
			name: "error_closing_connection",
			mockSetup: func(m *MockRepository) {
				m.EXPECT().CloseConnection().Return(errors.New("error closing PostgreSQL connection"))
			},
			wantErr: true,
			errMsg:  "error closing PostgreSQL connection",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockRepo := NewMockRepository(ctrl)
			tt.mockSetup(mockRepo)

			err := mockRepo.CloseConnection()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)

				return
			}

			require.NoError(t, err)
		})
	}
}

// --------------------------------------------------------------------------
// Pure function tests: buildDynamicFilters
// --------------------------------------------------------------------------

func TestBuildDynamicFilters(t *testing.T) {
	t.Parallel()

	schema := []TableSchema{
		{
			SchemaName: "public",
			TableName:  "transactions",
			Columns: []ColumnInformation{
				{Name: "id", DataType: "uuid"},
				{Name: "status", DataType: "varchar"},
				{Name: "amount", DataType: "numeric"},
			},
		},
	}

	tests := []struct {
		name         string
		schemaName   string
		table        string
		filter       map[string][]any
		wantContains string
		wantArgs     int
	}{
		{
			name:         "single_valid_filter",
			schemaName:   "public",
			table:        "transactions",
			filter:       map[string][]any{"status": {"active"}},
			wantContains: "status IN",
			wantArgs:     1,
		},
		{
			name:         "multiple_valid_filters",
			schemaName:   "public",
			table:        "transactions",
			filter:       map[string][]any{"status": {"active"}, "amount": {100}},
			wantContains: "IN",
			wantArgs:     2,
		},
		{
			name:         "filter_with_invalid_column_ignored",
			schemaName:   "public",
			table:        "transactions",
			filter:       map[string][]any{"nonexistent_column": {"value"}},
			wantContains: "",
			wantArgs:     0,
		},
		{
			name:         "empty_filter_map",
			schemaName:   "public",
			table:        "transactions",
			filter:       map[string][]any{},
			wantContains: "",
			wantArgs:     0,
		},
		{
			name:         "filter_with_empty_values_ignored",
			schemaName:   "public",
			table:        "transactions",
			filter:       map[string][]any{"status": {}},
			wantContains: "",
			wantArgs:     0,
		},
		{
			name:         "filter_for_nonexistent_table",
			schemaName:   "public",
			table:        "nonexistent",
			filter:       map[string][]any{"status": {"active"}},
			wantContains: "",
			wantArgs:     0,
		},
		{
			name:         "multi_value_filter",
			schemaName:   "public",
			table:        "transactions",
			filter:       map[string][]any{"status": {"active", "pending", "completed"}},
			wantContains: "status IN",
			wantArgs:     3,
		},
		{
			name:       "schema_qualified_resolution_uses_matching_schema",
			schemaName: "internal",
			table:      "transactions",
			filter:     map[string][]any{"internal_only": {"enabled"}},
			wantArgs:   1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
			builder := psql.Select("*").From(tt.table)

			schemaForTest := append([]TableSchema{}, schema...)
			if tt.name == "schema_qualified_resolution_uses_matching_schema" {
				schemaForTest = append(schemaForTest, TableSchema{
					SchemaName: "internal",
					TableName:  "transactions",
					Columns: []ColumnInformation{
						{Name: "internal_only", DataType: "varchar"},
					},
				})
			}

			result := buildDynamicFilters(builder, schemaForTest, tt.schemaName, tt.table, tt.filter)

			query, args, err := result.ToSql()
			require.NoError(t, err)

			if tt.wantContains != "" {
				assert.Contains(t, query, tt.wantContains)
			}

			assert.Len(t, args, tt.wantArgs)
		})
	}
}

// --------------------------------------------------------------------------
// Pure function tests: applyFilter
// --------------------------------------------------------------------------

func TestApplyFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fieldName    string
		values       []any
		wantContains string
		wantArgs     int
	}{
		{
			name:         "single_value",
			fieldName:    "status",
			values:       []any{"active"},
			wantContains: "status IN ($1)",
			wantArgs:     1,
		},
		{
			name:         "multiple_values",
			fieldName:    "status",
			values:       []any{"active", "pending", "completed"},
			wantContains: "status IN ($1,$2,$3)",
			wantArgs:     3,
		},
		{
			name:         "empty_values_returns_unmodified",
			fieldName:    "status",
			values:       []any{},
			wantContains: "",
			wantArgs:     0,
		},
		{
			name:         "numeric_values",
			fieldName:    "amount",
			values:       []any{100, 200, 300},
			wantContains: "amount IN ($1,$2,$3)",
			wantArgs:     3,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
			builder := psql.Select("*").From("test_table")

			result := applyFilter(builder, tt.fieldName, tt.values)

			query, args, err := result.ToSql()
			require.NoError(t, err)

			if tt.wantContains != "" {
				assert.Contains(t, query, tt.wantContains)
			}

			assert.Len(t, args, tt.wantArgs)
		})
	}
}

// --------------------------------------------------------------------------
// Pure function tests: isFilterConditionEmpty
// --------------------------------------------------------------------------

func TestIsFilterConditionEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		condition model.FilterCondition
		want      bool
	}{
		{
			name:      "empty_condition",
			condition: model.FilterCondition{},
			want:      true,
		},
		{
			name:      "with_equals",
			condition: model.FilterCondition{Equals: []any{"active"}},
			want:      false,
		},
		{
			name:      "with_greater_than",
			condition: model.FilterCondition{GreaterThan: []any{100}},
			want:      false,
		},
		{
			name:      "with_greater_or_equal",
			condition: model.FilterCondition{GreaterOrEqual: []any{100}},
			want:      false,
		},
		{
			name:      "with_less_than",
			condition: model.FilterCondition{LessThan: []any{1000}},
			want:      false,
		},
		{
			name:      "with_less_or_equal",
			condition: model.FilterCondition{LessOrEqual: []any{1000}},
			want:      false,
		},
		{
			name:      "with_between",
			condition: model.FilterCondition{Between: []any{100, 1000}},
			want:      false,
		},
		{
			name:      "with_in",
			condition: model.FilterCondition{In: []any{"a", "b"}},
			want:      false,
		},
		{
			name:      "with_not_in",
			condition: model.FilterCondition{NotIn: []any{"deleted"}},
			want:      false,
		},
		{
			name: "all_operators_set",
			condition: model.FilterCondition{
				Equals:         []any{"x"},
				GreaterThan:    []any{1},
				GreaterOrEqual: []any{2},
				LessThan:       []any{3},
				LessOrEqual:    []any{4},
				Between:        []any{5, 6},
				In:             []any{"y"},
				NotIn:          []any{"z"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isFilterConditionEmpty(tt.condition)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// Pure function tests: validateFilterCondition
// --------------------------------------------------------------------------

func TestValidateFilterCondition(t *testing.T) {
	t.Parallel()

	validUUID := "550e8400-e29b-41d4-a716-446655440000"

	tests := []struct {
		name      string
		fieldName string
		condition model.FilterCondition
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid_equals",
			fieldName: "status",
			condition: model.FilterCondition{Equals: []any{"active"}},
		},
		{
			name:      "valid_between_two_values",
			fieldName: "amount",
			condition: model.FilterCondition{Between: []any{100, 1000}},
		},
		{
			name:      "invalid_between_one_value",
			fieldName: "amount",
			condition: model.FilterCondition{Between: []any{100}},
			wantErr:   true,
			errMsg:    "between operator for field 'amount' must have exactly 2 values, got 1",
		},
		{
			name:      "invalid_between_three_values",
			fieldName: "amount",
			condition: model.FilterCondition{Between: []any{100, 200, 300}},
			wantErr:   true,
			errMsg:    "between operator for field 'amount' must have exactly 2 values, got 3",
		},
		{
			name:      "valid_single_gt",
			fieldName: "amount",
			condition: model.FilterCondition{GreaterThan: []any{100}},
		},
		{
			name:      "invalid_gt_multiple_values",
			fieldName: "amount",
			condition: model.FilterCondition{GreaterThan: []any{100, 200}},
			wantErr:   true,
			errMsg:    "gt operator for field 'amount' must have exactly 1 value, got 2",
		},
		{
			name:      "invalid_gte_multiple_values",
			fieldName: "amount",
			condition: model.FilterCondition{GreaterOrEqual: []any{100, 200}},
			wantErr:   true,
			errMsg:    "gte operator for field 'amount' must have exactly 1 value, got 2",
		},
		{
			name:      "invalid_lt_multiple_values",
			fieldName: "amount",
			condition: model.FilterCondition{LessThan: []any{100, 200}},
			wantErr:   true,
			errMsg:    "lt operator for field 'amount' must have exactly 1 value, got 2",
		},
		{
			name:      "invalid_lte_multiple_values",
			fieldName: "amount",
			condition: model.FilterCondition{LessOrEqual: []any{100, 200}},
			wantErr:   true,
			errMsg:    "lte operator for field 'amount' must have exactly 1 value, got 2",
		},
		{
			name:      "valid_uuid_field_with_uuid_value",
			fieldName: "user_id",
			condition: model.FilterCondition{Equals: []any{validUUID}},
		},
		{
			name:      "invalid_uuid_field_with_non_uuid_value",
			fieldName: "user_id",
			condition: model.FilterCondition{Equals: []any{"not-a-uuid"}},
			wantErr:   true,
			errMsg:    "appears to be a UUID field but received non-UUID value",
		},
		{
			name:      "empty_condition_is_valid",
			fieldName: "status",
			condition: model.FilterCondition{},
		},
		{
			name:      "non_uuid_field_accepts_any_value",
			fieldName: "status",
			condition: model.FilterCondition{Equals: []any{"any-string-value"}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateFilterCondition(tt.fieldName, tt.condition)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)

				return
			}

			require.NoError(t, err)
		})
	}
}

// --------------------------------------------------------------------------
// Pure function tests: isLikelyUUIDField
// --------------------------------------------------------------------------

func TestIsLikelyUUIDField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fieldName string
		want      bool
	}{
		{name: "id_field", fieldName: "id", want: true},
		{name: "user_id", fieldName: "user_id", want: true},
		{name: "account_id", fieldName: "account_id", want: true},
		{name: "template_id", fieldName: "template_id", want: true},
		{name: "organization_id", fieldName: "organization_id", want: true},
		{name: "uuid_field", fieldName: "uuid", want: true},
		{name: "some_uuid_col", fieldName: "some_uuid_col", want: false}, // suffix match only: _id or _uuid
		{name: "status", fieldName: "status", want: false},
		{name: "amount", fieldName: "amount", want: false},
		{name: "name", fieldName: "name", want: false},
		{name: "created_at", fieldName: "created_at", want: false},
		{name: "description", fieldName: "description", want: false},
		{name: "ID_uppercase", fieldName: "ID", want: true},
		{name: "User_ID_mixed_case", fieldName: "User_ID", want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isLikelyUUIDField(tt.fieldName)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// Pure function tests: isValidUUIDFormat
// --------------------------------------------------------------------------

func TestIsValidUUIDFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "valid_uuid_v4", input: "550e8400-e29b-41d4-a716-446655440000", want: true},
		{name: "valid_uuid_nil", input: "00000000-0000-0000-0000-000000000000", want: true},
		{name: "invalid_short_string", input: "not-a-uuid", want: false},
		{name: "invalid_empty", input: "", want: false},
		{name: "invalid_partial", input: "550e8400-e29b-41d4", want: false},
		{name: "invalid_extra_chars", input: "550e8400-e29b-41d4-a716-446655440000-extra", want: false},
		{name: "invalid_date_string", input: "2025-01-01", want: false},
		{name: "invalid_numeric_string", input: "12345", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isValidUUIDFormat(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// Pure function tests: validateUUIDFieldValues
// --------------------------------------------------------------------------

func TestValidateUUIDFieldValues(t *testing.T) {
	t.Parallel()

	validUUID1 := "550e8400-e29b-41d4-a716-446655440000"
	validUUID2 := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"

	tests := []struct {
		name      string
		fieldName string
		condition model.FilterCondition
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "all_valid_uuids_in_equals",
			fieldName: "user_id",
			condition: model.FilterCondition{Equals: []any{validUUID1, validUUID2}},
		},
		{
			name:      "invalid_uuid_in_equals",
			fieldName: "user_id",
			condition: model.FilterCondition{Equals: []any{"bad-value"}},
			wantErr:   true,
			errMsg:    "appears to be a UUID field",
		},
		{
			name:      "invalid_uuid_in_in_operator",
			fieldName: "account_id",
			condition: model.FilterCondition{In: []any{validUUID1, "not-valid"}},
			wantErr:   true,
			errMsg:    "appears to be a UUID field",
		},
		{
			name:      "invalid_uuid_in_not_in_operator",
			fieldName: "template_id",
			condition: model.FilterCondition{NotIn: []any{"wrong"}},
			wantErr:   true,
			errMsg:    "appears to be a UUID field",
		},
		{
			name:      "non_string_values_are_rejected",
			fieldName: "some_id",
			condition: model.FilterCondition{Equals: []any{123, true}},
			wantErr:   true,
			errMsg:    "non-string operand",
		},
		{
			name:      "empty_condition_is_valid",
			fieldName: "user_id",
			condition: model.FilterCondition{},
		},
		{
			name:      "valid_uuids_in_between",
			fieldName: "record_id",
			condition: model.FilterCondition{Between: []any{validUUID1, validUUID2}},
		},
		{
			name:      "invalid_uuid_in_greater_than",
			fieldName: "parent_id",
			condition: model.FilterCondition{GreaterThan: []any{"not-uuid"}},
			wantErr:   true,
			errMsg:    "appears to be a UUID field",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateUUIDFieldValues(tt.fieldName, tt.condition)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)

				return
			}

			require.NoError(t, err)
		})
	}
}

// --------------------------------------------------------------------------
// Pure function tests: isDateField
// --------------------------------------------------------------------------

func TestIsDateField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fieldName string
		want      bool
	}{
		{name: "created_at", fieldName: "created_at", want: true},
		{name: "updated_at", fieldName: "updated_at", want: true},
		{name: "deleted_at", fieldName: "deleted_at", want: true},
		{name: "completed_at", fieldName: "completed_at", want: true},
		{name: "order_date", fieldName: "order_date", want: true},
		{name: "start_time", fieldName: "start_time", want: true},
		{name: "birth_date", fieldName: "birth_date", want: true},
		{name: "end_date", fieldName: "end_date", want: true},
		{name: "processed_at", fieldName: "processed_at", want: true},
		{name: "status_is_not_date", fieldName: "status", want: false},
		{name: "amount_is_not_date", fieldName: "amount", want: false},
		{name: "name_is_not_date", fieldName: "name", want: false},
		{name: "id_is_not_date", fieldName: "id", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isDateField(tt.fieldName)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// Pure function tests: isDateString
// --------------------------------------------------------------------------

func TestIsDateString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "yyyy_mm_dd", value: "2025-01-15", want: true},
		{name: "iso_8601_with_time", value: "2025-01-15T10:30:00Z", want: true},
		{name: "iso_8601_with_milliseconds", value: "2025-01-15T10:30:00.000Z", want: true},
		{name: "short_string_not_date", value: "2025", want: false},
		{name: "no_dash_separator", value: "20250115", want: false},
		{name: "empty_string", value: "", want: false},
		{name: "non_string_int", value: 20250115, want: false},
		{name: "non_string_nil", value: nil, want: false},
		{name: "non_string_bool", value: true, want: false},
		{name: "nine_char_with_dash", value: "2025-01-1", want: false},
		{name: "text_with_dashes", value: "not-a-date", want: false}, // time.Parse rejects non-date strings
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isDateString(tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// Pure function tests: parseJSONBField
// --------------------------------------------------------------------------

func TestParseJSONBField(t *testing.T) {
	t.Parallel()

	logger := &log.NopLogger{}

	tests := []struct {
		name      string
		value     any
		wantType  string
		wantValue any
	}{
		{
			name:      "nil_value",
			value:     nil,
			wantType:  "nil",
			wantValue: nil,
		},
		{
			name:      "json_object_as_bytes",
			value:     []uint8(`{"key":"value","nested":{"a":1}}`),
			wantType:  "map",
			wantValue: map[string]any{"key": "value", "nested": map[string]any{"a": float64(1)}},
		},
		{
			name:      "json_array_as_bytes",
			value:     []uint8(`[1,2,3]`),
			wantType:  "array",
			wantValue: []any{float64(1), float64(2), float64(3)},
		},
		{
			name:      "json_string_as_bytes",
			value:     []uint8(`"hello world"`),
			wantType:  "string",
			wantValue: "hello world",
		},
		{
			name:      "non_json_bytes_returns_original",
			value:     []uint8(`not json at all {{{`),
			wantType:  "bytes",
			wantValue: []uint8(`not json at all {{{`),
		},
		{
			name:      "regular_string_passthrough",
			value:     "plain string",
			wantType:  "passthrough",
			wantValue: "plain string",
		},
		{
			name:      "integer_passthrough",
			value:     42,
			wantType:  "passthrough",
			wantValue: 42,
		},
		{
			name:      "float_passthrough",
			value:     3.14,
			wantType:  "passthrough",
			wantValue: 3.14,
		},
		{
			name:      "bool_passthrough",
			value:     true,
			wantType:  "passthrough",
			wantValue: true,
		},
		{
			name:      "empty_json_object",
			value:     []uint8(`{}`),
			wantType:  "map",
			wantValue: map[string]any{},
		},
		{
			name:      "empty_json_array",
			value:     []uint8(`[]`),
			wantType:  "array",
			wantValue: []any{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseJSONBField(context.Background(), tt.value, logger)

			switch tt.wantType {
			case "nil":
				assert.Nil(t, got)
			case "map":
				gotMap, ok := got.(map[string]any)
				require.True(t, ok, "expected map[string]any, got %T", got)
				assert.Equal(t, tt.wantValue, gotMap)
			case "array":
				gotArr, ok := got.([]any)
				require.True(t, ok, "expected []any, got %T", got)
				assert.Equal(t, tt.wantValue, gotArr)
			case "string":
				gotStr, ok := got.(string)
				require.True(t, ok, "expected string, got %T", got)
				assert.Equal(t, tt.wantValue, gotStr)
			case "bytes":
				gotBytes, ok := got.([]uint8)
				require.True(t, ok, "expected []uint8, got %T", got)
				assert.Equal(t, tt.wantValue, gotBytes)
			default:
				assert.Equal(t, tt.wantValue, got)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Pure function tests: createRowMap
// --------------------------------------------------------------------------

func TestCreateRowMap(t *testing.T) {
	t.Parallel()

	logger := &log.NopLogger{}

	tests := []struct {
		name    string
		columns []string
		values  []any
		want    map[string]any
	}{
		{
			name:    "simple_values",
			columns: []string{"id", "name", "age"},
			values:  []any{"uuid-1", "Alice", 30},
			want:    map[string]any{"id": "uuid-1", "name": "Alice", "age": 30},
		},
		{
			name:    "with_nil_value",
			columns: []string{"id", "deleted_at"},
			values:  []any{"uuid-1", nil},
			want:    map[string]any{"id": "uuid-1", "deleted_at": nil},
		},
		{
			name:    "with_jsonb_bytes",
			columns: []string{"id", "metadata"},
			values:  []any{"uuid-1", []uint8(`{"key":"value"}`)},
			want:    map[string]any{"id": "uuid-1", "metadata": map[string]any{"key": "value"}},
		},
		{
			name:    "empty_columns_and_values",
			columns: []string{},
			values:  []any{},
			want:    map[string]any{},
		},
		{
			name:    "single_column",
			columns: []string{"count"},
			values:  []any{int64(42)},
			want:    map[string]any{"count": int64(42)},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := createRowMap(context.Background(), tt.columns, tt.values, logger)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// Interface compliance test
// --------------------------------------------------------------------------

func TestMockRepository_ImplementsInterface(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mock := NewMockRepository(ctrl)

	// Verify MockRepository satisfies the Repository interface at compile time.
	var _ Repository = mock
}

// --------------------------------------------------------------------------
// Idempotency test: multiple calls to the same mock method
// --------------------------------------------------------------------------

func TestMockRepository_MultipleSequentialCalls(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := NewMockRepository(ctrl)

	schema := []TableSchema{
		{
			SchemaName: "public",
			TableName:  "users",
			Columns:    []ColumnInformation{{Name: "id", DataType: "uuid"}},
		},
	}

	// First call returns data, second call returns empty, third returns error.
	gomock.InOrder(
		mockRepo.EXPECT().
			Query(gomock.Any(), gomock.Any(), "public", "users", []string{"id"}, gomock.Any()).
			Return([]map[string]any{{"id": "uuid-1"}}, nil),
		mockRepo.EXPECT().
			Query(gomock.Any(), gomock.Any(), "public", "users", []string{"id"}, gomock.Any()).
			Return([]map[string]any{}, nil),
		mockRepo.EXPECT().
			Query(gomock.Any(), gomock.Any(), "public", "users", []string{"id"}, gomock.Any()).
			Return(nil, errors.New("transient error")),
	)

	ctx := context.Background()
	filter := map[string][]any{}

	result1, err1 := mockRepo.Query(ctx, schema, "public", "users", []string{"id"}, filter)
	require.NoError(t, err1)
	assert.Len(t, result1, 1)

	result2, err2 := mockRepo.Query(ctx, schema, "public", "users", []string{"id"}, filter)
	require.NoError(t, err2)
	assert.Empty(t, result2)

	result3, err3 := mockRepo.Query(ctx, schema, "public", "users", []string{"id"}, filter)
	require.Error(t, err3)
	assert.Nil(t, result3)
}
