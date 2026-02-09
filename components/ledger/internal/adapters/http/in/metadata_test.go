// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

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
		setupMocks     func(*mbootstrap.MockMetadataIndexRepository, *mbootstrap.MockMetadataIndexRepository)
		expectedStatus int
	}{
		{
			name:       "success - transaction entity",
			entityName: "transaction",
			payload: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "tier",
				Unique:      false,
			},
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				transaction.EXPECT().
					CreateIndex(gomock.Any(), "transaction", gomock.Any()).
					DoAndReturn(func(_ context.Context, collection string, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
						return &mmodel.MetadataIndex{
							IndexName:   "metadata.tier_1",
							EntityName:  collection,
							MetadataKey: input.MetadataKey,
							Unique:      input.Unique,
							Sparse:      true,
						}, nil
					})
			},
			expectedStatus: fiber.StatusCreated,
		},
		{
			name:       "success - onboarding entity (account)",
			entityName: "account",
			payload: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "category",
				Unique:      true,
			},
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				onboarding.EXPECT().
					CreateIndex(gomock.Any(), "account", gomock.Any()).
					DoAndReturn(func(_ context.Context, collection string, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
						return &mmodel.MetadataIndex{
							IndexName:   "metadata.category_1",
							EntityName:  collection,
							MetadataKey: input.MetadataKey,
							Unique:      input.Unique,
							Sparse:      true,
						}, nil
					})
			},
			expectedStatus: fiber.StatusCreated,
		},
		{
			name:       "success - onboarding entity (organization)",
			entityName: "organization",
			payload: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "region",
				Unique:      false,
			},
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				onboarding.EXPECT().
					CreateIndex(gomock.Any(), "organization", gomock.Any()).
					DoAndReturn(func(_ context.Context, collection string, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
						return &mmodel.MetadataIndex{
							IndexName:   "metadata.region_1",
							EntityName:  collection,
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
			setupMocks:     func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {},
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name:       "error - repo failure",
			entityName: "transaction",
			payload: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "tier",
				Unique:      false,
			},
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				transaction.EXPECT().
					CreateIndex(gomock.Any(), "transaction", gomock.Any()).
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

			mockOnboardingRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
			mockTransactionRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
			tt.setupMocks(mockOnboardingRepo, mockTransactionRepo)

			handler := &MetadataIndexHandler{
				OnboardingMetadataRepo:  mockOnboardingRepo,
				TransactionMetadataRepo: mockTransactionRepo,
			}

			app := fiber.New()

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

			mockOnboardingRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
			mockTransactionRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)

			handler := &MetadataIndexHandler{
				OnboardingMetadataRepo:  mockOnboardingRepo,
				TransactionMetadataRepo: mockTransactionRepo,
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

func TestMetadataIndexHandler_CreateMetadataIndex_EmptyEntityName(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockOnboardingRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
	mockTransactionRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)

	handler := &MetadataIndexHandler{
		OnboardingMetadataRepo:  mockOnboardingRepo,
		TransactionMetadataRepo: mockTransactionRepo,
	}

	payload := &mmodel.CreateMetadataIndexInput{
		MetadataKey: "tier",
	}

	app := fiber.New()

	// Route without :entity_name param so c.Params("entity_name") returns ""
	app.Post("/v1/settings/metadata-indexes/entities", func(c *fiber.Ctx) error {
		c.SetUserContext(context.Background())
		return handler.CreateMetadataIndex(payload, c)
	})

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/v1/settings/metadata-indexes/entities", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestMetadataIndexHandler_GetAllMetadataIndexes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(*mbootstrap.MockMetadataIndexRepository, *mbootstrap.MockMetadataIndexRepository)
		expectedStatus int
		validateBody   func(*testing.T, []*mmodel.MetadataIndex)
	}{
		{
			name:        "success - filter by transaction entity",
			queryParams: "?entity_name=transaction",
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				transaction.EXPECT().
					FindAllIndexes(gomock.Any(), "transaction").
					Return([]*mmodel.MetadataIndex{
						{
							IndexName:   "metadata.tier_1",
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
			name:        "success - filter by onboarding entity (account)",
			queryParams: "?entity_name=account",
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				onboarding.EXPECT().
					FindAllIndexes(gomock.Any(), "account").
					Return([]*mmodel.MetadataIndex{
						{
							IndexName:   "metadata.category_1",
							MetadataKey: "category",
						},
					}, nil)
			},
			expectedStatus: fiber.StatusOK,
			validateBody: func(t *testing.T, result []*mmodel.MetadataIndex) {
				require.Len(t, result, 1)
				assert.Equal(t, "account", result[0].EntityName)
			},
		},
		{
			name:        "success - empty result",
			queryParams: "?entity_name=ledger",
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				onboarding.EXPECT().
					FindAllIndexes(gomock.Any(), "ledger").
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
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				// No mock call expected - validation fails before repo call
			},
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name:        "error - repo failure",
			queryParams: "?entity_name=operation",
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				transaction.EXPECT().
					FindAllIndexes(gomock.Any(), "operation").
					Return(nil, errors.New("database error"))
			},
			expectedStatus: fiber.StatusInternalServerError,
		},
		{
			name:        "success - no filter returns all entities",
			queryParams: "",
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				onboarding.EXPECT().
					FindAllIndexes(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, collection string) ([]*mmodel.MetadataIndex, error) {
						if collection == "account" {
							return []*mmodel.MetadataIndex{
								{IndexName: "metadata.category_1", MetadataKey: "category"},
							}, nil
						}
						return []*mmodel.MetadataIndex{}, nil
					}).
					AnyTimes()

				transaction.EXPECT().
					FindAllIndexes(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, collection string) ([]*mmodel.MetadataIndex, error) {
						if collection == "transaction" {
							return []*mmodel.MetadataIndex{
								{IndexName: "metadata.tier_1", MetadataKey: "tier"},
							}, nil
						}
						return []*mmodel.MetadataIndex{}, nil
					}).
					AnyTimes()
			},
			expectedStatus: fiber.StatusOK,
			validateBody: func(t *testing.T, result []*mmodel.MetadataIndex) {
				require.GreaterOrEqual(t, len(result), 2)

				var foundAccount, foundTransaction bool
				for _, idx := range result {
					assert.NotEmpty(t, idx.EntityName)
					if idx.EntityName == "account" && idx.MetadataKey == "category" {
						foundAccount = true
					}
					if idx.EntityName == "transaction" && idx.MetadataKey == "tier" {
						foundTransaction = true
					}
				}
				assert.True(t, foundAccount, "should contain account index")
				assert.True(t, foundTransaction, "should contain transaction index")
			},
		},
		{
			name:        "success - no filter with partial repo failures",
			queryParams: "",
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				onboarding.EXPECT().
					FindAllIndexes(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, collection string) ([]*mmodel.MetadataIndex, error) {
						if collection == "organization" {
							return nil, errors.New("connection refused")
						}
						if collection == "ledger" {
							return []*mmodel.MetadataIndex{
								{IndexName: "metadata.env_1", MetadataKey: "env"},
							}, nil
						}
						return []*mmodel.MetadataIndex{}, nil
					}).
					AnyTimes()

				transaction.EXPECT().
					FindAllIndexes(gomock.Any(), gomock.Any()).
					Return([]*mmodel.MetadataIndex{}, nil).
					AnyTimes()
			},
			expectedStatus: fiber.StatusOK,
			validateBody: func(t *testing.T, result []*mmodel.MetadataIndex) {
				// Should still return results from non-failing collections
				var foundLedger bool
				for _, idx := range result {
					if idx.EntityName == "ledger" && idx.MetadataKey == "env" {
						foundLedger = true
					}
				}
				assert.True(t, foundLedger, "should contain ledger index despite other failures")
			},
		},
		{
			name:        "success - no filter all empty",
			queryParams: "",
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				onboarding.EXPECT().
					FindAllIndexes(gomock.Any(), gomock.Any()).
					Return([]*mmodel.MetadataIndex{}, nil).
					AnyTimes()

				transaction.EXPECT().
					FindAllIndexes(gomock.Any(), gomock.Any()).
					Return([]*mmodel.MetadataIndex{}, nil).
					AnyTimes()
			},
			expectedStatus: fiber.StatusOK,
			validateBody: func(t *testing.T, result []*mmodel.MetadataIndex) {
				assert.Empty(t, result)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockOnboardingRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
			mockTransactionRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
			tt.setupMocks(mockOnboardingRepo, mockTransactionRepo)

			handler := &MetadataIndexHandler{
				OnboardingMetadataRepo:  mockOnboardingRepo,
				TransactionMetadataRepo: mockTransactionRepo,
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
		setupMocks     func(*mbootstrap.MockMetadataIndexRepository, *mbootstrap.MockMetadataIndexRepository)
		expectedStatus int
	}{
		{
			name:       "success - transaction entity",
			entityName: "transaction",
			indexKey:   "tier",
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				transaction.EXPECT().
					DeleteIndex(gomock.Any(), "transaction", "metadata.tier_1").
					Return(nil)
			},
			expectedStatus: fiber.StatusNoContent,
		},
		{
			name:       "success - onboarding entity (account)",
			entityName: "account",
			indexKey:   "category",
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				onboarding.EXPECT().
					DeleteIndex(gomock.Any(), "account", "metadata.category_1").
					Return(nil)
			},
			expectedStatus: fiber.StatusNoContent,
		},
		{
			name:       "success - operation_route entity",
			entityName: "operation_route",
			indexKey:   "region",
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				transaction.EXPECT().
					DeleteIndex(gomock.Any(), "operation_route", "metadata.region_1").
					Return(nil)
			},
			expectedStatus: fiber.StatusNoContent,
		},
		{
			name:           "error - invalid entity_name",
			entityName:     "invalid_entity",
			indexKey:       "tier",
			setupMocks:     func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {},
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name:       "error - repo failure (not found)",
			entityName: "transaction",
			indexKey:   "tier",
			setupMocks: func(onboarding, transaction *mbootstrap.MockMetadataIndexRepository) {
				transaction.EXPECT().
					DeleteIndex(gomock.Any(), "transaction", "metadata.tier_1").
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

			mockOnboardingRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
			mockTransactionRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
			tt.setupMocks(mockOnboardingRepo, mockTransactionRepo)

			handler := &MetadataIndexHandler{
				OnboardingMetadataRepo:  mockOnboardingRepo,
				TransactionMetadataRepo: mockTransactionRepo,
			}

			app := fiber.New()

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

func TestMetadataIndexHandler_DeleteMetadataIndex_EmptyEntityName(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockOnboardingRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
	mockTransactionRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)

	handler := &MetadataIndexHandler{
		OnboardingMetadataRepo:  mockOnboardingRepo,
		TransactionMetadataRepo: mockTransactionRepo,
	}

	app := fiber.New()

	// Route without :entity_name param so c.Params("entity_name") returns ""
	app.Delete("/v1/settings/metadata-indexes/entities", func(c *fiber.Ctx) error {
		c.SetUserContext(context.Background())
		return handler.DeleteMetadataIndex(c)
	})

	req := httptest.NewRequest("DELETE", "/v1/settings/metadata-indexes/entities", nil)

	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestMetadataIndexHandler_DeleteMetadataIndex_EmptyIndexKey(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockOnboardingRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
	mockTransactionRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)

	handler := &MetadataIndexHandler{
		OnboardingMetadataRepo:  mockOnboardingRepo,
		TransactionMetadataRepo: mockTransactionRepo,
	}

	app := fiber.New()

	// Register route without :index_key param so c.Params("index_key") returns ""
	app.Delete("/v1/settings/metadata-indexes/entities/:entity_name/key", func(c *fiber.Ctx) error {
		c.SetUserContext(context.Background())
		return handler.DeleteMetadataIndex(c)
	})

	req := httptest.NewRequest("DELETE", "/v1/settings/metadata-indexes/entities/transaction/key", nil)

	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}
