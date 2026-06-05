// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"go.opentelemetry.io/otel/trace/noop"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/postgres"

	"github.com/stretchr/testify/assert"
)

func TestUseCase_GetDataSourceInformation(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because ResetRegisteredDataSourceIDsForTesting mutates global state
	ctx := context.Background()

	// Register datasource IDs for testing
	pkg.ResetRegisteredDataSourceIDsForTesting()
	pkg.RegisterDataSourceIDsForTesting([]string{"mongo_ds", "pg_ds"})

	pgConfig := &postgres.Connection{DBName: "pg_db"}

	tests := []struct {
		name         string
		setupSvc     func() *UseCase
		expectResult []*model.DataSourceInformation
	}{
		{
			name: "Success - Both MongoDB and PostgreSQL present",
			setupSvc: func() *UseCase {
				return &UseCase{
					Logger: log.NewNop(),
					Tracer: noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"mongo_ds": {
							DatabaseType:      pkg.MongoDBType,
							MongoDBName:       "mongo_db",
							MongoDBRepository: mongodb.NewMockRepository(nil),
						},
						"pg_ds": {
							DatabaseType:       pkg.PostgreSQLType,
							DatabaseConfig:     pgConfig,
							PostgresRepository: postgres.NewMockRepository(nil),
						},
					}),
				}
			},
			expectResult: []*model.DataSourceInformation{
				{
					Id:           "mongo_ds",
					ExternalName: "mongo_db",
					Type:         pkg.MongoDBType,
				},
				{
					Id:           "pg_ds",
					ExternalName: "pg_db",
					Type:         pkg.PostgreSQLType,
				},
			},
		},
		{
			name: "Success - Only MongoDB present",
			setupSvc: func() *UseCase {
				return &UseCase{
					Logger: log.NewNop(),
					Tracer: noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"mongo_ds": {
							DatabaseType:      pkg.MongoDBType,
							MongoDBName:       "mongo_db",
							MongoDBRepository: mongodb.NewMockRepository(nil),
						},
					}),
				}
			},
			expectResult: []*model.DataSourceInformation{
				{
					Id:           "mongo_ds",
					ExternalName: "mongo_db",
					Type:         pkg.MongoDBType,
				},
			},
		},
		{
			name: "Success - Only PostgreSQL present",
			setupSvc: func() *UseCase {
				return &UseCase{
					Logger: log.NewNop(),
					Tracer: noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"pg_ds": {
							DatabaseType:       pkg.PostgreSQLType,
							DatabaseConfig:     pgConfig,
							PostgresRepository: postgres.NewMockRepository(nil),
						},
					}),
				}
			},
			expectResult: []*model.DataSourceInformation{
				{
					Id:           "pg_ds",
					ExternalName: "pg_db",
					Type:         pkg.PostgreSQLType,
				},
			},
		},
		{
			name: "Success - No data sources",
			setupSvc: func() *UseCase {
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"), ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{})}
			},
			expectResult: []*model.DataSourceInformation{},
		},
		{
			name: "Unknown type - should return empty slice",
			setupSvc: func() *UseCase {
				return &UseCase{
					Logger: log.NewNop(),
					Tracer: noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"unknown_ds": {
							DatabaseType: "unknown",
						},
					}),
				}
			},
			expectResult: []*model.DataSourceInformation{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := tt.setupSvc()
			result := svc.GetDataSourceInformation(ctx)
			assert.ElementsMatch(t, tt.expectResult, result)
		})
	}
}
