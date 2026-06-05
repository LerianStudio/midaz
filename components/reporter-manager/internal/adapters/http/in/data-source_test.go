// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/LerianStudio/midaz/v4/components/reporter-manager/internal/services"
	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/redis"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func setupTestApp() *fiber.App {
	return fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
}

func TestDataSourceHandler_GetDataSourceInformation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setupService   func() *services.UseCase
		expectedStatus int
	}{
		{
			name: "Success - Returns empty list when no data sources",
			setupService: func() *services.UseCase {
				return &services.UseCase{
					Logger:              log.NewNop(),
					Tracer:              noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
				}
			},
			expectedStatus: fiber.StatusOK,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := setupTestApp()

			handler := &DataSourceHandler{
				service: tt.setupService(),
			}

			app.Get("/v1/data-sources", func(c *fiber.Ctx) error {
				c.SetUserContext(context.Background())
				return handler.GetDataSourceInformation(c)
			})

			req := httptest.NewRequest("GET", "/v1/data-sources", nil)
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestDataSourceHandler_GetDataSourceInformationByID(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because it modifies global state (immutable registry)
	pkg.ResetRegisteredDataSourceIDsForTesting()
	pkg.RegisterDataSourceIDsForTesting([]string{"midaz_onboarding"})
	t.Cleanup(func() { pkg.ResetRegisteredDataSourceIDsForTesting() })

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMongoRepo := mongodb.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mongoSchema := []mongodb.CollectionSchema{
		{
			CollectionName: "collection1",
			Fields: []mongodb.FieldInformation{
				{Name: "field1", DataType: "string"},
				{Name: "field2", DataType: "int"},
			},
		},
	}

	const mongoDSID = "midaz_onboarding"
	const notFoundDSID = "not_found_ds"

	tests := []struct {
		name           string
		dataSourceID   string
		setupService   func() *services.UseCase
		mockSetup      func()
		expectedStatus int
		expectError    bool
	}{
		{
			name:         "Success - Returns MongoDB data source details",
			dataSourceID: mongoDSID,
			setupService: func() *services.UseCase {
				return &services.UseCase{
					Logger: log.NewNop(),
					Tracer: noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						mongoDSID: {
							DatabaseType:      pkg.MongoDBType,
							MongoDBName:       "mongo_db",
							MongoDBRepository: mockMongoRepo,
							Initialized:       true,
						},
					}),
					RedisRepo: mockRedisRepo,
				}
			},
			mockSetup: func() {
				mockRedisRepo.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil)
				mockMongoRepo.EXPECT().GetDatabaseSchema(gomock.Any()).Return(mongoSchema, nil)
				mockRedisRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedStatus: fiber.StatusOK,
			expectError:    false,
		},
		{
			name:         "Error - Data source not found",
			dataSourceID: notFoundDSID,
			setupService: func() *services.UseCase {
				return &services.UseCase{
					Logger:              log.NewNop(),
					Tracer:              noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
					RedisRepo:           mockRedisRepo,
				}
			},
			mockSetup: func() {
				mockRedisRepo.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil)
			},
			expectedStatus: fiber.StatusBadRequest, // ErrMissingDataSource returns ValidationError (400)
			expectError:    true,
		},
		{
			name:         "Error - Database schema retrieval fails",
			dataSourceID: mongoDSID,
			setupService: func() *services.UseCase {
				return &services.UseCase{
					Logger: log.NewNop(),
					Tracer: noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						mongoDSID: {
							DatabaseType:      pkg.MongoDBType,
							MongoDBName:       "mongo_db",
							MongoDBRepository: mockMongoRepo,
							Initialized:       true,
						},
					}),
					RedisRepo: mockRedisRepo,
				}
			},
			mockSetup: func() {
				mockRedisRepo.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil)
				mockMongoRepo.EXPECT().GetDatabaseSchema(gomock.Any()).Return(nil, errors.New("db error"))
			},
			expectedStatus: fiber.StatusBadRequest, // ErrMissingDataSource returns ValidationError (400)
			expectError:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			app := setupTestApp()

			handler := &DataSourceHandler{
				service: tt.setupService(),
			}

			// Use the string middleware so c.Locals("dataSourceId") is a string,
			// matching the production route wiring.
			app.Get("/v1/data-sources/:dataSourceId", ParseStringPathParam("dataSourceId"), func(c *fiber.Ctx) error {
				c.SetUserContext(context.Background())
				return handler.GetDataSourceInformationByID(c)
			})

			req := httptest.NewRequest("GET", "/v1/data-sources/"+tt.dataSourceID, nil)
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if !tt.expectError {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var result model.DataSourceDetails
				err = json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Equal(t, tt.dataSourceID, result.Id)
			}
		})
	}
}

func TestDataSourceHandler_GetDataSourceInformationByID_InvalidID(t *testing.T) {
	t.Parallel()

	app := setupTestApp()

	handler := &DataSourceHandler{
		service: &services.UseCase{
			Logger:              log.NewNop(),
			Tracer:              noop.NewTracerProvider().Tracer("test"),
			ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
		},
	}

	app.Get("/v1/data-sources/:dataSourceId", ParseStringPathParam("dataSourceId"), func(c *fiber.Ctx) error {
		c.SetUserContext(context.Background())
		return handler.GetDataSourceInformationByID(c)
	})

	tests := []struct {
		name         string
		dataSourceID string
	}{
		{
			name:         "Error - Path traversal attempt",
			dataSourceID: "..%2F..%2Fetc%2Fpasswd",
		},
		{
			name:         "Error - Special characters",
			dataSourceID: "id;DROP%20TABLE",
		},
		{
			name:         "Error - Starts with number",
			dataSourceID: "123invalid",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("GET", "/v1/data-sources/"+tt.dataSourceID, nil)
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var errorResponse map[string]interface{}
			err = json.Unmarshal(body, &errorResponse)
			require.NoError(t, err)

			assert.Contains(t, errorResponse, "code")
		})
	}
}

func TestNewDataSourceHandler_NilService(t *testing.T) {
	t.Parallel()

	handler, err := NewDataSourceHandler(nil)

	assert.Nil(t, handler)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service must not be nil")
}

func TestNewDataSourceHandler_ValidService(t *testing.T) {
	t.Parallel()

	svc := &services.UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	handler, err := NewDataSourceHandler(svc)

	assert.NotNil(t, handler)
	require.NoError(t, err)
}
