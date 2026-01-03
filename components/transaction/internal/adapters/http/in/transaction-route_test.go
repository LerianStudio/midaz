package in

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestTransactionRouteHandler_CreateTransactionRoute(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		setupMocks     func(transactionRouteRepo *transactionroute.MockRepository, operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 201 with created transaction route",
			jsonBody: `{
				"title": "Payment Settlement",
				"description": "Route for payment settlement transactions",
				"operationRoutes": ["01965ed9-7fa4-75b2-8872-fc9e8509ab0a", "01965ed9-7fa4-75b2-8872-fc9e8509ab0b"],
				"metadata": {"category": "settlement"}
			}`,
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID) {
				opRoute1ID := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")
				opRoute2ID := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b")

				// FindByIDs returns the operation routes
				operationRouteRepo.EXPECT().
					FindByIDs(gomock.Any(), orgID, ledgerID, []uuid.UUID{opRoute1ID, opRoute2ID}).
					Return([]*mmodel.OperationRoute{
						{
							ID:            opRoute1ID,
							OperationType: "source",
							Title:         "Source Route",
						},
						{
							ID:            opRoute2ID,
							OperationType: "destination",
							Title:         "Destination Route",
						},
					}, nil).
					Times(1)

				// Create transaction route
				transactionRouteRepo.EXPECT().
					Create(gomock.Any(), orgID, ledgerID, gomock.Any()).
					DoAndReturn(func(ctx any, oID, lID uuid.UUID, tr *mmodel.TransactionRoute) (*mmodel.TransactionRoute, error) {
						tr.CreatedAt = time.Now()
						tr.UpdatedAt = time.Now()
						return tr, nil
					}).
					Times(1)

				// Create metadata
				metadataRepo.EXPECT().
					Create(gomock.Any(), "TransactionRoute", gomock.Any()).
					Return(nil).
					Times(1)

				// Create cache (error is logged but not returned)
				redisRepo.EXPECT().
					SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "title", "response should contain title")
				assert.Contains(t, result, "description", "response should contain description")
				assert.Equal(t, "Payment Settlement", result["title"])
				assert.Equal(t, "Route for payment settlement transactions", result["description"])
			},
		},
		{
			name: "repository error returns 500",
			jsonBody: `{
				"title": "Payment Settlement",
				"description": "Route for payment settlement transactions",
				"operationRoutes": ["01965ed9-7fa4-75b2-8872-fc9e8509ab0a", "01965ed9-7fa4-75b2-8872-fc9e8509ab0b"]
			}`,
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID) {
				opRoute1ID := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")
				opRoute2ID := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b")

				// FindByIDs returns error
				operationRouteRepo.EXPECT().
					FindByIDs(gomock.Any(), orgID, ledgerID, []uuid.UUID{opRoute1ID, opRoute2ID}).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New()
			ledgerID := uuid.New()

			mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
			mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)

			tt.setupMocks(mockTransactionRouteRepo, mockOperationRouteRepo, mockMetadataRepo, mockRedisRepo, orgID, ledgerID)

			cmdUC := &command.UseCase{
				TransactionRouteRepo: mockTransactionRouteRepo,
				OperationRouteRepo:   mockOperationRouteRepo,
				MetadataRepo:         mockMetadataRepo,
				RedisRepo:            mockRedisRepo,
			}
			handler := &TransactionRouteHandler{Command: cmdUC}

			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				http.WithBody(new(mmodel.CreateTransactionRouteInput), handler.CreateTransactionRoute),
			)

			req := httptest.NewRequest("POST", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes", bytes.NewBufferString(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

func TestTransactionRouteHandler_GetTransactionRouteByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(transactionRouteRepo *transactionroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, transactionRouteID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with transaction route",
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, transactionRouteID uuid.UUID) {
				transactionRouteRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, transactionRouteID).
					Return(&mmodel.TransactionRoute{
						ID:             transactionRouteID,
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						Title:          "Payment Settlement",
						Description:    "Route for payment settlement",
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetTransactionRouteByID fetches metadata when transaction route is found
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "TransactionRoute", transactionRouteID.String()).
					Return(&mongodb.Metadata{
						EntityID:   transactionRouteID.String(),
						EntityName: "TransactionRoute",
						Data:       map[string]any{"category": "settlement"},
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "title", "response should contain title")
				assert.Contains(t, result, "description", "response should contain description")
				assert.Contains(t, result, "metadata", "response should contain metadata")
				assert.Equal(t, "Payment Settlement", result["title"])
			},
		},
		{
			name: "not found returns 404",
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, transactionRouteID uuid.UUID) {
				transactionRouteRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, transactionRouteID).
					Return(nil, pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, constant.ErrTransactionRouteNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, transactionRouteID uuid.UUID) {
				transactionRouteRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, transactionRouteID).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New()
			ledgerID := uuid.New()
			transactionRouteID := uuid.New()

			mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockTransactionRouteRepo, mockMetadataRepo, orgID, ledgerID, transactionRouteID)

			queryUC := &query.UseCase{
				TransactionRouteRepo: mockTransactionRouteRepo,
				MetadataRepo:         mockMetadataRepo,
			}
			handler := &TransactionRouteHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("transaction_route_id", transactionRouteID)
					return c.Next()
				},
				handler.GetTransactionRouteByID,
			)

			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes/"+transactionRouteID.String(), nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

func TestTransactionRouteHandler_UpdateTransactionRoute(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		setupMocks     func(transactionRouteRepo *transactionroute.MockRepository, operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionRouteID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with updated transaction route",
			jsonBody: `{
				"title": "Updated Payment Settlement",
				"description": "Updated route description",
				"metadata": {"category": "updated-settlement"}
			}`,
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionRouteID uuid.UUID) {
				// Update transaction route in command use case
				transactionRouteRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, transactionRouteID, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.TransactionRoute{
						ID:             transactionRouteID,
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						Title:          "Updated Payment Settlement",
						Description:    "Updated route description",
						CreatedAt:      time.Now().Add(-time.Hour),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// UpdateMetadata first calls FindByEntity
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "TransactionRoute", transactionRouteID.String()).
					Return(nil, nil).
					Times(1)

				// Then calls Update
				metadataRepo.EXPECT().
					Update(gomock.Any(), "TransactionRoute", transactionRouteID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// GetTransactionRouteByID in query use case to return updated transaction route
				transactionRouteRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, transactionRouteID).
					Return(&mmodel.TransactionRoute{
						ID:             transactionRouteID,
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						Title:          "Updated Payment Settlement",
						Description:    "Updated route description",
						CreatedAt:      time.Now().Add(-time.Hour),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetTransactionRouteByID also fetches metadata
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "TransactionRoute", transactionRouteID.String()).
					Return(&mongodb.Metadata{
						EntityID:   transactionRouteID.String(),
						EntityName: "TransactionRoute",
						Data:       map[string]any{"category": "updated-settlement"},
					}, nil).
					Times(1)

				// Create cache after update (error is logged but not returned)
				redisRepo.EXPECT().
					SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "title", "response should contain title")
				assert.Equal(t, "Updated Payment Settlement", result["title"])
				assert.Equal(t, "Updated route description", result["description"])
			},
		},
		{
			name: "not found returns 404",
			jsonBody: `{
				"title": "Updated title"
			}`,
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionRouteID uuid.UUID) {
				transactionRouteRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, transactionRouteID, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, constant.ErrTransactionRouteNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			jsonBody: `{
				"title": "Updated title"
			}`,
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionRouteID uuid.UUID) {
				transactionRouteRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, transactionRouteID, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New()
			ledgerID := uuid.New()
			transactionRouteID := uuid.New()

			mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
			mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)

			tt.setupMocks(mockTransactionRouteRepo, mockOperationRouteRepo, mockMetadataRepo, mockRedisRepo, orgID, ledgerID, transactionRouteID)

			cmdUC := &command.UseCase{
				TransactionRouteRepo: mockTransactionRouteRepo,
				OperationRouteRepo:   mockOperationRouteRepo,
				MetadataRepo:         mockMetadataRepo,
				RedisRepo:            mockRedisRepo,
			}
			queryUC := &query.UseCase{
				TransactionRouteRepo: mockTransactionRouteRepo,
				MetadataRepo:         mockMetadataRepo,
			}
			handler := &TransactionRouteHandler{
				Command: cmdUC,
				Query:   queryUC,
			}

			app := fiber.New()
			app.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("transaction_route_id", transactionRouteID)
					return c.Next()
				},
				http.WithBody(new(mmodel.UpdateTransactionRouteInput), handler.UpdateTransactionRoute),
			)

			req := httptest.NewRequest("PATCH", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes/"+transactionRouteID.String(), bytes.NewBufferString(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

func TestTransactionRouteHandler_DeleteTransactionRouteByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(transactionRouteRepo *transactionroute.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionRouteID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 204",
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionRouteID uuid.UUID) {
				// FindByID to get operation routes for deletion
				transactionRouteRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, transactionRouteID).
					Return(&mmodel.TransactionRoute{
						ID:             transactionRouteID,
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						Title:          "Payment Settlement",
						OperationRoutes: []mmodel.OperationRoute{
							{ID: uuid.New()},
							{ID: uuid.New()},
						},
					}, nil).
					Times(1)

				// Delete transaction route
				transactionRouteRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, transactionRouteID, gomock.Any()).
					Return(nil).
					Times(1)

				// Delete cache (error is logged but not returned)
				redisRepo.EXPECT().
					Del(gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil,
		},
		{
			name: "not found returns 404",
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionRouteID uuid.UUID) {
				transactionRouteRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, transactionRouteID).
					Return(nil, pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionRouteID uuid.UUID) {
				transactionRouteRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, transactionRouteID).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New()
			ledgerID := uuid.New()
			transactionRouteID := uuid.New()

			mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			tt.setupMocks(mockTransactionRouteRepo, mockRedisRepo, orgID, ledgerID, transactionRouteID)

			cmdUC := &command.UseCase{
				TransactionRouteRepo: mockTransactionRouteRepo,
				RedisRepo:            mockRedisRepo,
			}
			handler := &TransactionRouteHandler{Command: cmdUC}

			app := fiber.New()
			app.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("transaction_route_id", transactionRouteID)
					return c.Next()
				},
				handler.DeleteTransactionRouteByID,
			)

			req := httptest.NewRequest("DELETE", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes/"+transactionRouteID.String(), nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

func TestTransactionRouteHandler_GetAllTransactionRoutes(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(transactionRouteRepo *transactionroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with pagination structure",
			queryParams: "",
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				transactionRouteRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.TransactionRoute{}, libHTTP.CursorPagination{}, nil).
					Times(1)

				// When results are found (even empty), FindList is called
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "TransactionRoute", gomock.Any()).
					Return([]*mongodb.Metadata{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				// Validate pagination structure exists
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(10), limit)

				// Validate items is empty array
				items, ok := result["items"].([]any)
				require.True(t, ok, "items should be an array")
				assert.Empty(t, items, "items should be empty")
			},
		},
		{
			name:        "success with items returns transaction routes",
			queryParams: "?limit=5",
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				tr1ID := uuid.New()
				tr2ID := uuid.New()

				transactionRouteRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.TransactionRoute{
						{
							ID:             tr1ID,
							OrganizationID: orgID,
							LedgerID:       ledgerID,
							Title:          "Payment Settlement",
							Description:    "Route for payment settlement",
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             tr2ID,
							OrganizationID: orgID,
							LedgerID:       ledgerID,
							Title:          "Transfer Route",
							Description:    "Route for transfers",
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
					}, libHTTP.CursorPagination{
						Next: "next_cursor_value",
						Prev: "",
					}, nil).
					Times(1)

				// GetAllTransactionRoutes fetches metadata for all returned transaction routes
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "TransactionRoute", gomock.Any()).
					Return([]*mongodb.Metadata{
						{
							EntityID:   tr1ID.String(),
							EntityName: "TransactionRoute",
							Data:       map[string]any{"category": "settlement"},
						},
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				// Validate items array
				items, ok := result["items"].([]any)
				require.True(t, ok, "items should be an array")
				assert.Len(t, items, 2, "should have two transaction routes")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "transaction route should have id field")
				assert.Contains(t, firstItem, "title", "transaction route should have title field")
				assert.Equal(t, "Payment Settlement", firstItem["title"])

				// Validate pagination
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(5), limit)

				// Validate cursor pagination fields
				assert.Contains(t, result, "next_cursor", "response should contain next_cursor")
				assert.Equal(t, "next_cursor_value", result["next_cursor"])
			},
		},
		{
			name:        "with metadata filter returns filtered transaction routes",
			queryParams: "?metadata.category=settlement",
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				trID := uuid.New()

				// GetAllMetadataTransactionRoutes first calls FindList to get metadata matching filter
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "TransactionRoute", gomock.Any()).
					Return([]*mongodb.Metadata{
						{
							EntityID:   trID.String(),
							EntityName: "TransactionRoute",
							Data:       map[string]any{"category": "settlement"},
						},
					}, nil).
					Times(1)

				// Then calls FindAll to get transaction routes
				transactionRouteRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.TransactionRoute{
						{
							ID:             trID,
							OrganizationID: orgID,
							LedgerID:       ledgerID,
							Title:          "Payment Settlement",
							Description:    "Route for payment settlement",
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
					}, libHTTP.CursorPagination{
						Next: "",
						Prev: "",
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				// Validate items array
				items, ok := result["items"].([]any)
				require.True(t, ok, "items should be an array")
				assert.Len(t, items, 1, "should have one filtered transaction route")

				// Validate first item has metadata
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "metadata", "transaction route should have metadata field")
			},
		},
		{
			name:        "repository error returns 500",
			queryParams: "",
			setupMocks: func(transactionRouteRepo *transactionroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				transactionRouteRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(nil, libHTTP.CursorPagination{}, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New()
			ledgerID := uuid.New()

			mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockTransactionRouteRepo, mockMetadataRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				TransactionRouteRepo: mockTransactionRouteRepo,
				MetadataRepo:         mockMetadataRepo,
			}
			handler := &TransactionRouteHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.GetAllTransactionRoutes,
			)

			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes"+tt.queryParams, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

// Ensure libPostgres.Pagination is used (referenced in handler)
var _ = libPostgres.Pagination{}
