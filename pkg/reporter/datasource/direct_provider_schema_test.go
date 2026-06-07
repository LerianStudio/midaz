// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"errors"
	"fmt"
	"testing"

	pkgErr "github.com/LerianStudio/midaz/v4/pkg"
	constant "github.com/LerianStudio/midaz/v4/pkg/constant"
	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	pg "github.com/LerianStudio/midaz/v4/pkg/reporter/postgres"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDirectProvider_GetDataSourceSchema_PostgreSQL(t *testing.T) {
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

	schema, err := provider.GetDataSourceSchema(context.Background(), "pg_main")

	require.NoError(t, err)
	require.NotNil(t, schema)
	assert.Equal(t, "pg_main", schema.DataSourceID)
	assert.Len(t, schema.Tables, 1)
	assert.Equal(t, "public.users", schema.Tables[0].Name)

	// M3: Verify actual field names and types
	require.Len(t, schema.Tables[0].Fields, 2)
	assert.Equal(t, "id", schema.Tables[0].Fields[0].Name)
	assert.Equal(t, "uuid", schema.Tables[0].Fields[0].Type)
	assert.Equal(t, "name", schema.Tables[0].Fields[1].Name)
	assert.Equal(t, "varchar", schema.Tables[0].Fields[1].Type)
}

func TestDirectProvider_GetDataSourceSchema_MongoDB(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockMongoRepo := mongodb.NewMockRepository(ctrl)
	mockMongoRepo.EXPECT().
		GetDatabaseSchema(gomock.Any()).
		Return([]mongodb.CollectionSchema{
			{
				CollectionName: "orders",
				Fields: []mongodb.FieldInformation{
					{Name: "_id", DataType: "objectId"},
					{Name: "total", DataType: "double"},
				},
			},
		}, nil)

	dsMap := map[string]pkg.DataSource{
		"mongo_orders": {
			DatabaseType:      pkg.MongoDBType,
			Initialized:       true,
			Status:            libConstants.DataSourceStatusAvailable,
			MongoDBRepository: mockMongoRepo,
			MongoDBName:       "orders_db",
		},
	}

	sds := newTestSafeDataSources(t, dsMap)
	provider := NewDirectProvider(sds, nil, nil)

	schema, err := provider.GetDataSourceSchema(context.Background(), "mongo_orders")

	require.NoError(t, err)
	require.NotNil(t, schema)
	assert.Equal(t, "mongo_orders", schema.DataSourceID)
	assert.Len(t, schema.Tables, 1)
	assert.Equal(t, "orders", schema.Tables[0].Name)

	// M3: Verify actual field names and types
	require.Len(t, schema.Tables[0].Fields, 2)
	assert.Equal(t, "_id", schema.Tables[0].Fields[0].Name)
	assert.Equal(t, "objectId", schema.Tables[0].Fields[0].Type)
	assert.Equal(t, "total", schema.Tables[0].Fields[1].Name)
	assert.Equal(t, "double", schema.Tables[0].Fields[1].Type)
}

func TestDirectProvider_GetDataSourceSchema_NotFound(t *testing.T) {
	sds := newTestSafeDataSources(t, map[string]pkg.DataSource{})
	provider := NewDirectProvider(sds, nil, nil)

	schema, err := provider.GetDataSourceSchema(context.Background(), "nonexistent")

	require.Error(t, err)
	assert.Nil(t, schema)
	assert.True(t, errors.Is(err, ErrDataSourceNotFound))
}

func TestDirectProvider_GetDataSourceSchema_EmptyID(t *testing.T) {
	sds := newTestSafeDataSources(t, map[string]pkg.DataSource{})
	provider := NewDirectProvider(sds, nil, nil)

	schema, err := provider.GetDataSourceSchema(context.Background(), "")

	require.Error(t, err)
	assert.Nil(t, schema)
}

// H1: Test unsupported database type returns error.
func TestDirectProvider_GetDataSourceSchema_UnsupportedType(t *testing.T) {
	dsMap := map[string]pkg.DataSource{
		"unknown_ds": {
			DatabaseType: "unsupported",
			Initialized:  true,
			Status:       libConstants.DataSourceStatusAvailable,
		},
	}

	sds := newTestSafeDataSources(t, dsMap)
	provider := NewDirectProvider(sds, nil, nil)

	schema, err := provider.GetDataSourceSchema(context.Background(), "unknown_ds")

	require.Error(t, err)
	assert.Nil(t, schema)
	assert.Contains(t, err.Error(), "unsupported database type")
	assert.Contains(t, err.Error(), "unknown_ds")
}

// H2: Test nil PostgresRepository returns error.
func TestDirectProvider_GetDataSourceSchema_NilPostgresRepository(t *testing.T) {
	dsMap := map[string]pkg.DataSource{
		"pg_nil": {
			DatabaseType:       pkg.PostgreSQLType,
			Initialized:        true,
			Status:             libConstants.DataSourceStatusAvailable,
			PostgresRepository: nil,
		},
	}

	sds := newTestSafeDataSources(t, dsMap)
	provider := NewDirectProvider(sds, nil, nil)

	schema, err := provider.GetDataSourceSchema(context.Background(), "pg_nil")

	require.Error(t, err)
	assert.Nil(t, schema)
	// nil PostgresRepository is caught by the unavailability check before reaching
	// getPostgresSchema, so the error is ErrDataSourceUnavailable (0285).
	var unavailableErr pkgErr.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailableErr)
	assert.Equal(t, constant.ErrDataSourceUnavailable.Error(), unavailableErr.Code)
}

// H3: Test nil MongoDBRepository returns error.
func TestDirectProvider_GetDataSourceSchema_NilMongoDBRepository(t *testing.T) {
	dsMap := map[string]pkg.DataSource{
		"mongo_nil": {
			DatabaseType:      pkg.MongoDBType,
			Initialized:       true,
			Status:            libConstants.DataSourceStatusAvailable,
			MongoDBRepository: nil,
		},
	}

	sds := newTestSafeDataSources(t, dsMap)
	provider := NewDirectProvider(sds, nil, nil)

	schema, err := provider.GetDataSourceSchema(context.Background(), "mongo_nil")

	require.Error(t, err)
	assert.Nil(t, schema)
	// nil MongoDBRepository is caught by the unavailability check before reaching
	// getMongoDBSchema, so the error is ErrDataSourceUnavailable (0285).
	var unavailableErr pkgErr.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailableErr)
	assert.Equal(t, constant.ErrDataSourceUnavailable.Error(), unavailableErr.Code)
}

// H4: Test repository error propagates correctly.
func TestDirectProvider_GetDataSourceSchema_RepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)

	repoErr := fmt.Errorf("connection refused")
	mockPGRepo := pg.NewMockRepository(ctrl)
	mockPGRepo.EXPECT().
		GetDatabaseSchema(gomock.Any(), []string{"public"}).
		Return(nil, repoErr)

	dsMap := map[string]pkg.DataSource{
		"pg_err": {
			DatabaseType:       pkg.PostgreSQLType,
			Initialized:        true,
			Status:             libConstants.DataSourceStatusAvailable,
			PostgresRepository: mockPGRepo,
			Schemas:            []string{"public"},
		},
	}

	sds := newTestSafeDataSources(t, dsMap)
	provider := NewDirectProvider(sds, nil, nil)

	schema, err := provider.GetDataSourceSchema(context.Background(), "pg_err")

	require.Error(t, err)
	assert.Nil(t, schema)
	assert.ErrorIs(t, err, repoErr)
}
