package in

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestMetadataIndexHandler_CreateMetadataIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		entityName     string
		payload        *mmodel.CreateMetadataIndexInput
		setupMock      func(*mbootstrap.MockMetadataIndexPort)
		expectedStatus int
	}{
		{
			name:       "success",
			entityName: "transaction",
			payload: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "tier",
				Unique:      false,
			},
			setupMock: func(m *mbootstrap.MockMetadataIndexPort) {
				m.EXPECT().
					CreateMetadataIndex(gomock.Any(), "transaction", gomock.Any()).
					DoAndReturn(func(_ context.Context, entityName string, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
						return &mmodel.MetadataIndex{
							IndexName:   "metadata.tier_1",
							EntityName:  entityName,
							MetadataKey: input.MetadataKey,
							Unique:      input.Unique,
							Sparse:      true,
						}, nil
					})
			},
			expectedStatus: fiber.StatusCreated,
		},
		{
			name:       "success - operation entity",
			entityName: "operation",
			payload: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "category",
				Unique:      true,
			},
			setupMock: func(m *mbootstrap.MockMetadataIndexPort) {
				m.EXPECT().
					CreateMetadataIndex(gomock.Any(), "operation", gomock.Any()).
					DoAndReturn(func(_ context.Context, entityName string, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
						return &mmodel.MetadataIndex{
							IndexName:   "metadata.category_1",
							EntityName:  entityName,
							MetadataKey: input.MetadataKey,
							Unique:      input.Unique,
							Sparse:      true,
						}, nil
					})
			},
			expectedStatus: fiber.StatusCreated,
		},
		{
			name:       "error - invalid entity_name in path",
			entityName: "invalid_entity",
			payload: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "tier",
			},
			setupMock:      func(m *mbootstrap.MockMetadataIndexPort) {},
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name:       "error - port failure",
			entityName: "transaction",
			payload: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "tier",
				Unique:      false,
			},
			setupMock: func(m *mbootstrap.MockMetadataIndexPort) {
				m.EXPECT().
					CreateMetadataIndex(gomock.Any(), "transaction", gomock.Any()).
					Return(nil, errors.New("index already exists"))
			},
			expectedStatus: fiber.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockPort := mbootstrap.NewMockMetadataIndexPort(ctrl)
			tt.setupMock(mockPort)

			handler := &MetadataIndexHandler{
				MetadataIndexPort: mockPort,
			}

			app := fiber.New()

			// Route matches the new pattern: /v1/settings/metadata-indexes/entities/:entity_name
			app.Post("/v1/settings/metadata-indexes/entities/:entity_name", func(c *fiber.Ctx) error {
				c.SetUserContext(context.Background())
				return handler.CreateMetadataIndex(tt.payload, c)
			})

			body, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			url := "/v1/settings/metadata-indexes/entities/" + tt.entityName
			req := httptest.NewRequest("POST", url, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			// Validate response body for success case
			if tt.expectedStatus == fiber.StatusCreated {
				respBody, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var result mmodel.MetadataIndex
				err = json.Unmarshal(respBody, &result)
				require.NoError(t, err)

				assert.NotEmpty(t, result.IndexName)
				assert.Equal(t, tt.entityName, result.EntityName)
				assert.Equal(t, tt.payload.MetadataKey, result.MetadataKey)
			}
		})
	}
}

func TestMetadataIndexHandler_CreateMetadataIndex_InvalidPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		payload        any
		expectedStatus int
	}{
		{
			name:           "invalid payload type",
			payload:        "invalid",
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name:           "nil payload",
			payload:        nil,
			expectedStatus: fiber.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockPort := mbootstrap.NewMockMetadataIndexPort(ctrl)

			handler := &MetadataIndexHandler{
				MetadataIndexPort: mockPort,
			}

			app := fiber.New()

			app.Post("/v1/settings/metadata-indexes/entities/:entity_name", func(c *fiber.Ctx) error {
				c.SetUserContext(context.Background())
				return handler.CreateMetadataIndex(tt.payload, c)
			})

			req := httptest.NewRequest("POST", "/v1/settings/metadata-indexes/entities/transaction", bytes.NewReader([]byte("{}")))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestMetadataIndexHandler_GetAllMetadataIndexes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		queryParams    string
		setupMock      func(*mbootstrap.MockMetadataIndexPort)
		expectedStatus int
		validateBody   func(*testing.T, []*mmodel.MetadataIndex)
	}{
		{
			name:        "success - no filter",
			queryParams: "",
			setupMock: func(m *mbootstrap.MockMetadataIndexPort) {
				m.EXPECT().
					GetAllMetadataIndexes(gomock.Any(), gomock.Any()).
					Return([]*mmodel.MetadataIndex{
						{
							IndexName:   "metadata.tier_1",
							EntityName:  "transaction",
							MetadataKey: "tier",
							Unique:      false,
							Sparse:      true,
						},
					}, nil)
			},
			expectedStatus: fiber.StatusOK,
			validateBody: func(t *testing.T, result []*mmodel.MetadataIndex) {
				require.Len(t, result, 1)
				assert.Equal(t, "metadata.tier_1", result[0].IndexName)
				assert.Equal(t, "transaction", result[0].EntityName)
			},
		},
		{
			name:        "success - with entity filter",
			queryParams: "?entity_name=operation",
			setupMock: func(m *mbootstrap.MockMetadataIndexPort) {
				m.EXPECT().
					GetAllMetadataIndexes(gomock.Any(), gomock.Any()).
					Return([]*mmodel.MetadataIndex{
						{
							IndexName:   "metadata.tier_1",
							EntityName:  "operation",
							MetadataKey: "tier",
						},
					}, nil)
			},
			expectedStatus: fiber.StatusOK,
			validateBody: func(t *testing.T, result []*mmodel.MetadataIndex) {
				require.Len(t, result, 1)
				assert.Equal(t, "operation", result[0].EntityName)
			},
		},
		{
			name:        "success - empty result",
			queryParams: "",
			setupMock: func(m *mbootstrap.MockMetadataIndexPort) {
				m.EXPECT().
					GetAllMetadataIndexes(gomock.Any(), gomock.Any()).
					Return([]*mmodel.MetadataIndex{}, nil)
			},
			expectedStatus: fiber.StatusOK,
			validateBody: func(t *testing.T, result []*mmodel.MetadataIndex) {
				assert.Empty(t, result)
			},
		},
		{
			name:        "error - invalid entity_name filter",
			queryParams: "?entity_name=invalid_entity",
			setupMock: func(m *mbootstrap.MockMetadataIndexPort) {
				// No mock call expected - validation fails before port call
			},
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name:        "error - port failure",
			queryParams: "",
			setupMock: func(m *mbootstrap.MockMetadataIndexPort) {
				m.EXPECT().
					GetAllMetadataIndexes(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database error"))
			},
			expectedStatus: fiber.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockPort := mbootstrap.NewMockMetadataIndexPort(ctrl)
			tt.setupMock(mockPort)

			handler := &MetadataIndexHandler{
				MetadataIndexPort: mockPort,
			}

			app := fiber.New()

			app.Get("/v1/settings/metadata-indexes", func(c *fiber.Ctx) error {
				c.SetUserContext(context.Background())
				return handler.GetAllMetadataIndexes(c)
			})

			req := httptest.NewRequest("GET", "/v1/settings/metadata-indexes"+tt.queryParams, nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil && resp.StatusCode == fiber.StatusOK {
				respBody, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var result []*mmodel.MetadataIndex
				err = json.Unmarshal(respBody, &result)
				require.NoError(t, err)

				tt.validateBody(t, result)
			}
		})
	}
}

func TestMetadataIndexHandler_DeleteMetadataIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		entityName     string
		indexKey       string
		setupMock      func(*mbootstrap.MockMetadataIndexPort)
		expectedStatus int
	}{
		{
			name:       "success",
			entityName: "transaction",
			indexKey:   "tier",
			setupMock: func(m *mbootstrap.MockMetadataIndexPort) {
				// Handler builds index name as: metadata.{key}_1
				m.EXPECT().
					DeleteMetadataIndex(gomock.Any(), "transaction", "metadata.tier_1").
					Return(nil)
			},
			expectedStatus: fiber.StatusNoContent,
		},
		{
			name:       "success - operation entity",
			entityName: "operation",
			indexKey:   "category",
			setupMock: func(m *mbootstrap.MockMetadataIndexPort) {
				m.EXPECT().
					DeleteMetadataIndex(gomock.Any(), "operation", "metadata.category_1").
					Return(nil)
			},
			expectedStatus: fiber.StatusNoContent,
		},
		{
			name:       "success - operation_route entity",
			entityName: "operation_route",
			indexKey:   "region",
			setupMock: func(m *mbootstrap.MockMetadataIndexPort) {
				m.EXPECT().
					DeleteMetadataIndex(gomock.Any(), "operation_route", "metadata.region_1").
					Return(nil)
			},
			expectedStatus: fiber.StatusNoContent,
		},
		{
			name:           "error - invalid entity_name",
			entityName:     "invalid_entity",
			indexKey:       "tier",
			setupMock:      func(m *mbootstrap.MockMetadataIndexPort) {},
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name:       "error - port failure (not found)",
			entityName: "transaction",
			indexKey:   "tier",
			setupMock: func(m *mbootstrap.MockMetadataIndexPort) {
				m.EXPECT().
					DeleteMetadataIndex(gomock.Any(), "transaction", "metadata.tier_1").
					Return(errors.New("index not found"))
			},
			expectedStatus: fiber.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockPort := mbootstrap.NewMockMetadataIndexPort(ctrl)
			tt.setupMock(mockPort)

			handler := &MetadataIndexHandler{
				MetadataIndexPort: mockPort,
			}

			app := fiber.New()

			// Route matches the new pattern: /v1/settings/metadata-indexes/entities/:entity_name/key/:index_key
			app.Delete("/v1/settings/metadata-indexes/entities/:entity_name/key/:index_key", func(c *fiber.Ctx) error {
				c.SetUserContext(context.Background())
				return handler.DeleteMetadataIndex(c)
			})

			url := "/v1/settings/metadata-indexes/entities/" + tt.entityName + "/key/" + tt.indexKey
			req := httptest.NewRequest("DELETE", url, nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}
