// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// --------------------------------------------------------------------------
// Mock-based Repository interface contract tests
// --------------------------------------------------------------------------

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
// Interface compliance test
// --------------------------------------------------------------------------

func TestMockRepository_ImplementsInterface(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mock := NewMockRepository(ctrl)

	// Verify MockRepository satisfies the Repository interface at compile time.
	var _ Repository = mock
}
