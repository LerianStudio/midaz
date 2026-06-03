// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"testing"

	"github.com/LerianStudio/reporter/pkg"
	pg "github.com/LerianStudio/reporter/pkg/postgres"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDirectProvider_ValidateSchema_FieldsExist(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockPGRepo := pg.NewMockRepository(ctrl)
	mockPGRepo.EXPECT().
		GetDatabaseSchema(gomock.Any(), []string{"public"}).
		Return([]pg.TableSchema{
			{
				SchemaName: "public",
				TableName:  "users",
				Columns: []pg.ColumnInformation{
					{Name: "id", DataType: "uuid"},
					{Name: "name", DataType: "varchar"},
					{Name: "email", DataType: "varchar"},
				},
			},
		}, nil)

	dsMap := map[string]pkg.DataSource{
		"pg_main": {
			DatabaseType:       pkg.PostgreSQLType,
			Initialized:        true,
			Status:             libConstants.DataSourceStatusAvailable,
			PostgresRepository: mockPGRepo,
			Schemas:            []string{"public"},
		},
	}

	sds := newTestSafeDataSources(t, dsMap)
	provider := NewDirectProvider(sds, nil, nil)

	result, err := provider.ValidateSchema(context.Background(), "pg_main", map[string][]string{"users": {"id", "name"}})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Warnings)
}

func TestDirectProvider_ValidateSchema_FieldsMissing(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockPGRepo := pg.NewMockRepository(ctrl)
	mockPGRepo.EXPECT().
		GetDatabaseSchema(gomock.Any(), []string{"public"}).
		Return([]pg.TableSchema{
			{
				SchemaName: "public",
				TableName:  "users",
				Columns: []pg.ColumnInformation{
					{Name: "id", DataType: "uuid"},
				},
			},
		}, nil)

	dsMap := map[string]pkg.DataSource{
		"pg_main": {
			DatabaseType:       pkg.PostgreSQLType,
			Initialized:        true,
			Status:             libConstants.DataSourceStatusAvailable,
			PostgresRepository: mockPGRepo,
			Schemas:            []string{"public"},
		},
	}

	sds := newTestSafeDataSources(t, dsMap)
	provider := NewDirectProvider(sds, nil, nil)

	result, err := provider.ValidateSchema(context.Background(), "pg_main", map[string][]string{"users": {"nonexistent_field"}})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Valid)
}

func TestDirectProvider_ValidateSchema_UnavailableDatasource_D7(t *testing.T) {
	dsMap := map[string]pkg.DataSource{
		"pg_down": {
			DatabaseType: pkg.PostgreSQLType,
			Initialized:  false,
			Status:       libConstants.DataSourceStatusUnavailable,
			DatabaseConfig: &pg.Connection{
				DBName: "down_db",
			},
		},
	}

	sds := newTestSafeDataSources(t, dsMap)
	provider := NewDirectProvider(sds, nil, nil)

	result, err := provider.ValidateSchema(context.Background(), "pg_down", map[string][]string{"users": {"id"}})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Valid)
	require.Len(t, result.Warnings, 1)
	assert.Equal(t, WarningCodeDataSourceUnavailable, result.Warnings[0].Code)
}

func TestDirectProvider_ValidateSchema_EmptyFields(t *testing.T) {
	dsMap := map[string]pkg.DataSource{
		"pg_main": {
			DatabaseType: pkg.PostgreSQLType,
			Initialized:  true,
			Status:       libConstants.DataSourceStatusAvailable,
		},
	}

	sds := newTestSafeDataSources(t, dsMap)
	provider := NewDirectProvider(sds, nil, nil)

	result, err := provider.ValidateSchema(context.Background(), "pg_main", map[string][]string{})

	require.Error(t, err)
	assert.Nil(t, result)
}
