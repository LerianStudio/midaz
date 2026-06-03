// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/datasource"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/model"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/redis"
)

// TestUseCase_GetDataSourceInformation_WithProvider verifies that when a
// DataSourceProvider is set on the UseCase, GetDataSourceInformation delegates
// to provider.ListDataSources() instead of iterating ExternalDataSources directly.
func TestUseCase_GetDataSourceInformation_WithProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setupMock    func(ctrl *gomock.Controller) datasource.DataSourceProvider
		expectResult []*model.DataSourceInformation
	}{
		{
			name: "Success - Provider returns multiple datasources",
			setupMock: func(ctrl *gomock.Controller) datasource.DataSourceProvider {
				mock := datasource.NewMockDataSourceProvider(ctrl)
				mock.EXPECT().ListDataSources(gomock.Any()).Return([]datasource.DataSourceInfo{
					{ID: "pg_ds", Name: "pg_db", Type: "postgresql", Status: "available"},
					{ID: "mongo_ds", Name: "mongo_db", Type: "mongodb", Status: "available"},
				}, nil)
				return mock
			},
			expectResult: []*model.DataSourceInformation{
				{Id: "pg_ds", ExternalName: "pg_db", Type: "postgresql"},
				{Id: "mongo_ds", ExternalName: "mongo_db", Type: "mongodb"},
			},
		},
		{
			name: "Success - Provider returns empty list",
			setupMock: func(ctrl *gomock.Controller) datasource.DataSourceProvider {
				mock := datasource.NewMockDataSourceProvider(ctrl)
				mock.EXPECT().ListDataSources(gomock.Any()).Return([]datasource.DataSourceInfo{}, nil)
				return mock
			},
			expectResult: []*model.DataSourceInformation{},
		},
		{
			name: "Error - Provider returns error, falls back to empty",
			setupMock: func(ctrl *gomock.Controller) datasource.DataSourceProvider {
				mock := datasource.NewMockDataSourceProvider(ctrl)
				mock.EXPECT().ListDataSources(gomock.Any()).Return(nil, errors.New("provider error"))
				return mock
			},
			expectResult: []*model.DataSourceInformation{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			provider := tt.setupMock(ctrl)

			svc := &UseCase{
				Logger:              log.NewNop(),
				Tracer:              noop.NewTracerProvider().Tracer("test"),
				DataSourceProvider:  provider,
				ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
			}

			ctx := context.Background()
			result := svc.GetDataSourceInformation(ctx)
			assert.ElementsMatch(t, tt.expectResult, result)
		})
	}
}

// TestUseCase_GetDataSourceDetailsByID_WithProvider verifies that when a
// DataSourceProvider is set, GetDataSourceDetailsByID delegates to
// provider.GetDataSourceSchema() instead of direct repository access.
func TestUseCase_GetDataSourceDetailsByID_WithProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		dataSourceID string
		setupMock    func(ctrl *gomock.Controller) (datasource.DataSourceProvider, *redis.MockRedisRepository)
		expectErr    bool
		errContains  string
		expectResult *model.DataSourceDetails
	}{
		{
			name:         "Success - Provider returns schema for PostgreSQL datasource",
			dataSourceID: "pg_ds",
			setupMock: func(ctrl *gomock.Controller) (datasource.DataSourceProvider, *redis.MockRedisRepository) {
				mockProvider := datasource.NewMockDataSourceProvider(ctrl)
				mockRedis := redis.NewMockRedisRepository(ctrl)

				// Cache miss
				mockRedis.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil)

				mockProvider.EXPECT().GetDataSourceSchema(gomock.Any(), "pg_ds").Return(&datasource.DataSourceSchema{
					DataSourceID: "pg_ds",
					Tables: []datasource.SchemaTable{
						{
							Name:   "public.accounts",
							Schema: "public",
							Fields: []datasource.SchemaField{
								{Name: "id", Type: "uuid"},
								{Name: "name", Type: "varchar"},
							},
						},
					},
				}, nil)

				// Cache set
				mockRedis.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

				return mockProvider, mockRedis
			},
			expectErr: false,
			expectResult: &model.DataSourceDetails{
				Id:   "pg_ds",
				Type: "postgresql",
				Tables: []model.TableDetails{
					{
						Name:   "public.accounts",
						Fields: []string{"id", "name"},
					},
				},
			},
		},
		{
			name:         "Success - Provider returns schema for MongoDB datasource",
			dataSourceID: "mongo_ds",
			setupMock: func(ctrl *gomock.Controller) (datasource.DataSourceProvider, *redis.MockRedisRepository) {
				mockProvider := datasource.NewMockDataSourceProvider(ctrl)
				mockRedis := redis.NewMockRedisRepository(ctrl)

				// Cache miss
				mockRedis.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil)

				mockProvider.EXPECT().GetDataSourceSchema(gomock.Any(), "mongo_ds").Return(&datasource.DataSourceSchema{
					DataSourceID: "mongo_ds",
					Tables: []datasource.SchemaTable{
						{
							Name: "users",
							Fields: []datasource.SchemaField{
								{Name: "email", Type: "string"},
								{Name: "name", Type: "string"},
							},
						},
					},
				}, nil)

				// Cache set
				mockRedis.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

				return mockProvider, mockRedis
			},
			expectErr: false,
			expectResult: &model.DataSourceDetails{
				Id:   "mongo_ds",
				Type: "mongodb",
				Tables: []model.TableDetails{
					{
						Name:   "users",
						Fields: []string{"email", "name"},
					},
				},
			},
		},
		{
			name:         "Error - Provider returns not found",
			dataSourceID: "missing_ds",
			setupMock: func(ctrl *gomock.Controller) (datasource.DataSourceProvider, *redis.MockRedisRepository) {
				mockProvider := datasource.NewMockDataSourceProvider(ctrl)
				mockRedis := redis.NewMockRedisRepository(ctrl)

				// Cache miss
				mockRedis.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil)

				mockProvider.EXPECT().GetDataSourceSchema(gomock.Any(), "missing_ds").
					Return(nil, datasource.ErrDataSourceNotFound)

				return mockProvider, mockRedis
			},
			expectErr:    true,
			errContains:  datasource.ErrDataSourceNotFound.Error(),
			expectResult: nil,
		},
		{
			name:         "Error - Provider returns generic error",
			dataSourceID: "broken_ds",
			setupMock: func(ctrl *gomock.Controller) (datasource.DataSourceProvider, *redis.MockRedisRepository) {
				mockProvider := datasource.NewMockDataSourceProvider(ctrl)
				mockRedis := redis.NewMockRedisRepository(ctrl)

				// Cache miss
				mockRedis.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil)

				mockProvider.EXPECT().GetDataSourceSchema(gomock.Any(), "broken_ds").
					Return(nil, errors.New("connection refused"))

				return mockProvider, mockRedis
			},
			expectErr:    true,
			errContains:  "connection refused",
			expectResult: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			provider, mockRedis := tt.setupMock(ctrl)

			svc := &UseCase{
				Logger:              log.NewNop(),
				Tracer:              noop.NewTracerProvider().Tracer("test"),
				DataSourceProvider:  provider,
				ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
				RedisRepo:           mockRedis,
			}

			ctx := context.Background()
			result, err := svc.GetDataSourceDetailsByID(ctx, tt.dataSourceID)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectResult, result)
			}
		})
	}
}
