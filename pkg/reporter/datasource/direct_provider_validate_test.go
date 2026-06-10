// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"testing"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	pg "github.com/LerianStudio/midaz/v4/pkg/reporter/postgres"

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

// TestDirectProvider_ValidateSchema_EmptyFieldTable_PGMongoParity locks D1: a
// table key mapped to an EMPTY field slice must be treated identically by the
// PostgreSQL and MongoDB validators. The agreed semantics is "the table exists,
// there are no fields to validate" → Valid, in BOTH backends. Before the fix the
// PG validator rejected (MissingTables) while Mongo accepted, a silent
// cross-backend divergence.
func TestDirectProvider_ValidateSchema_EmptyFieldTable_PGMongoParity(t *testing.T) {
	const table = "existing_table"

	tableFields := map[string][]string{table: {}}

	// --- PostgreSQL: existing_table is present in the schema, no fields requested.
	pgCtrl := gomock.NewController(t)
	mockPGRepo := pg.NewMockRepository(pgCtrl)
	mockPGRepo.EXPECT().
		GetDatabaseSchema(gomock.Any(), []string{"public"}).
		Return([]pg.TableSchema{
			{
				SchemaName: "public",
				TableName:  table,
				Columns: []pg.ColumnInformation{
					{Name: "id", DataType: "uuid"},
				},
			},
		}, nil)

	pgDS := map[string]pkg.DataSource{
		"pg_main": {
			DatabaseType:       pkg.PostgreSQLType,
			Initialized:        true,
			Status:             libConstants.DataSourceStatusAvailable,
			PostgresRepository: mockPGRepo,
			Schemas:            []string{"public"},
		},
	}
	pgProvider := NewDirectProvider(newTestSafeDataSources(t, pgDS), nil, nil)

	pgResult, pgErr := pgProvider.ValidateSchema(context.Background(), "pg_main", tableFields)
	require.NoError(t, pgErr)
	require.NotNil(t, pgResult)
	assert.Empty(t, pgResult.MissingTables, "empty-field table must not be reported as missing under PostgreSQL")

	// --- MongoDB: existing_table is present as a collection, no fields requested.
	mongoCtrl := gomock.NewController(t)
	mockMongoRepo := mongodb.NewMockRepository(mongoCtrl)
	mockMongoRepo.EXPECT().
		GetDatabaseSchema(gomock.Any()).
		Return([]mongodb.CollectionSchema{
			{
				CollectionName: table,
				Fields: []mongodb.FieldInformation{
					{Name: "_id", DataType: "objectId"},
				},
			},
		}, nil)

	mongoDS := map[string]pkg.DataSource{
		"mongo_main": {
			DatabaseType:      pkg.MongoDBType,
			Initialized:       true,
			Status:            libConstants.DataSourceStatusAvailable,
			MongoDBRepository: mockMongoRepo,
			MongoDBName:       "main_db",
		},
	}
	mongoProvider := NewDirectProvider(newTestSafeDataSources(t, mongoDS), nil, nil)

	mongoResult, mongoErr := mongoProvider.ValidateSchema(context.Background(), "mongo_main", tableFields)
	require.NoError(t, mongoErr)
	require.NotNil(t, mongoResult)
	assert.Empty(t, mongoResult.MissingTables, "empty-field table must not be reported as missing under MongoDB")

	// The load-bearing parity assertion: both backends reach the SAME verdict.
	assert.Equal(t, mongoResult.Valid, pgResult.Valid,
		"PostgreSQL and MongoDB validators must agree on an empty-field table; got PG=%v Mongo=%v",
		pgResult.Valid, mongoResult.Valid)
	assert.True(t, pgResult.Valid, "empty-field table semantics: table exists, nothing to validate → Valid")
}
