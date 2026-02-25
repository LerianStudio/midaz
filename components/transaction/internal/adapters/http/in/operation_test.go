// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v3/commons/net/http"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestOperationHandler_GetAllOperationsByAccount(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with pagination structure",
			queryParams: "",
			setupMocks: func(operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				operationRepo.EXPECT().
					FindAllByAccount(gomock.Any(), orgID, ledgerID, accountID, gomock.Any(), gomock.Any()).
					Return([]*operation.Operation{}, libHTTP.CursorPagination{}, nil).
					Times(1)
				// When empty results, FindByEntityIDs is not called
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
			name:        "success with items returns operations",
			queryParams: "?limit=5",
			setupMocks: func(operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				op1ID := uuid.New().String()
				op2ID := uuid.New().String()
				transactionID := uuid.New().String()
				amount := decimal.NewFromInt(1500)
				available := decimal.NewFromInt(10000)
				onHold := decimal.NewFromInt(500)

				operationRepo.EXPECT().
					FindAllByAccount(gomock.Any(), orgID, ledgerID, accountID, gomock.Any(), gomock.Any()).
					Return([]*operation.Operation{
						{
							ID:              op1ID,
							TransactionID:   transactionID,
							Description:     "First operation",
							Type:            "DEBIT",
							AssetCode:       "BRL",
							ChartOfAccounts: "1000",
							Amount:          operation.Amount{Value: &amount},
							Balance:         operation.Balance{Available: &available, OnHold: &onHold},
							BalanceAfter:    operation.Balance{Available: &available, OnHold: &onHold},
							Status:          operation.Status{Code: "ACTIVE"},
							AccountID:       accountID.String(),
							AccountAlias:    "@person1",
							OrganizationID:  orgID.String(),
							LedgerID:        ledgerID.String(),
							CreatedAt:       time.Now(),
							UpdatedAt:       time.Now(),
						},
						{
							ID:              op2ID,
							TransactionID:   transactionID,
							Description:     "Second operation",
							Type:            "CREDIT",
							AssetCode:       "BRL",
							ChartOfAccounts: "2000",
							Amount:          operation.Amount{Value: &amount},
							Balance:         operation.Balance{Available: &available, OnHold: &onHold},
							BalanceAfter:    operation.Balance{Available: &available, OnHold: &onHold},
							Status:          operation.Status{Code: "ACTIVE"},
							AccountID:       accountID.String(),
							AccountAlias:    "@person2",
							OrganizationID:  orgID.String(),
							LedgerID:        ledgerID.String(),
							CreatedAt:       time.Now(),
							UpdatedAt:       time.Now(),
						},
					}, libHTTP.CursorPagination{
						Next: "next_cursor_value",
						Prev: "",
					}, nil).
					Times(1)

				// When operations are found, FindByEntityIDs is called to get metadata
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Operation", []string{op1ID, op2ID}).
					Return([]*mongodb.Metadata{}, nil).
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
				assert.Len(t, items, 2, "should have two operations")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "operation should have id field")
				assert.Contains(t, firstItem, "transactionId", "operation should have transactionId field")
				assert.Contains(t, firstItem, "type", "operation should have type field")
				assert.Contains(t, firstItem, "assetCode", "operation should have assetCode field")
				assert.Equal(t, "DEBIT", firstItem["type"])
				assert.Equal(t, "BRL", firstItem["assetCode"])

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
			name:        "with metadata filter returns filtered operations",
			queryParams: "?metadata.key=value",
			setupMocks: func(operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				opID := uuid.New().String()
				transactionID := uuid.New().String()
				amount := decimal.NewFromInt(1500)
				available := decimal.NewFromInt(10000)
				onHold := decimal.NewFromInt(500)

				// GetAllMetadataOperations first calls FindList to get metadata matching filter
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Operation", gomock.Any()).
					Return([]*mongodb.Metadata{
						{
							EntityID:   opID,
							EntityName: "Operation",
							Data:       map[string]any{"key": "value"},
						},
					}, nil).
					Times(1)

				// Then calls FindAllByAccount to get operations
				operationRepo.EXPECT().
					FindAllByAccount(gomock.Any(), orgID, ledgerID, accountID, gomock.Any(), gomock.Any()).
					Return([]*operation.Operation{
						{
							ID:              opID,
							TransactionID:   transactionID,
							Description:     "Filtered operation",
							Type:            "DEBIT",
							AssetCode:       "BRL",
							ChartOfAccounts: "1000",
							Amount:          operation.Amount{Value: &amount},
							Balance:         operation.Balance{Available: &available, OnHold: &onHold},
							BalanceAfter:    operation.Balance{Available: &available, OnHold: &onHold},
							Status:          operation.Status{Code: "ACTIVE"},
							AccountID:       accountID.String(),
							AccountAlias:    "@person1",
							OrganizationID:  orgID.String(),
							LedgerID:        ledgerID.String(),
							CreatedAt:       time.Now(),
							UpdatedAt:       time.Now(),
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
				assert.Len(t, items, 1, "should have one filtered operation")

				// Validate first item has metadata
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "metadata", "operation should have metadata field")
			},
		},
		{
			name:        "repository error returns 500",
			queryParams: "",
			setupMocks: func(operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				operationRepo.EXPECT().
					FindAllByAccount(gomock.Any(), orgID, ledgerID, accountID, gomock.Any(), gomock.Any()).
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
			accountID := uuid.New()

			mockOperationRepo := operation.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockOperationRepo, mockMetadataRepo, orgID, ledgerID, accountID)

			queryUC := &query.UseCase{
				OperationRepo: mockOperationRepo,
				MetadataRepo:  mockMetadataRepo,
			}
			handler := &OperationHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("account_id", accountID)
					return c.Next()
				},
				handler.GetAllOperationsByAccount,
			)

			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations"+tt.queryParams, nil)
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

func TestOperationHandler_GetOperationByAccount(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID, operationID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with operation",
			setupMocks: func(operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID, operationID uuid.UUID) {
				transactionID := uuid.New().String()
				amount := decimal.NewFromInt(1500)
				available := decimal.NewFromInt(10000)
				onHold := decimal.NewFromInt(500)

				operationRepo.EXPECT().
					FindByAccount(gomock.Any(), orgID, ledgerID, accountID, operationID).
					Return(&operation.Operation{
						ID:              operationID.String(),
						TransactionID:   transactionID,
						Description:     "Test operation",
						Type:            "DEBIT",
						AssetCode:       "BRL",
						ChartOfAccounts: "1000",
						Amount:          operation.Amount{Value: &amount},
						Balance:         operation.Balance{Available: &available, OnHold: &onHold},
						BalanceAfter:    operation.Balance{Available: &available, OnHold: &onHold},
						Status:          operation.Status{Code: "ACTIVE"},
						AccountID:       accountID.String(),
						AccountAlias:    "@person1",
						OrganizationID:  orgID.String(),
						LedgerID:        ledgerID.String(),
						CreatedAt:       time.Now(),
						UpdatedAt:       time.Now(),
					}, nil).
					Times(1)

				// GetOperationByAccount fetches metadata when operation is found
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Operation", operationID.String()).
					Return(&mongodb.Metadata{
						EntityID:   operationID.String(),
						EntityName: "Operation",
						Data:       map[string]any{"reason": "Purchase refund"},
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "transactionId", "response should contain transactionId")
				assert.Contains(t, result, "type", "response should contain type")
				assert.Contains(t, result, "assetCode", "response should contain assetCode")
				assert.Contains(t, result, "amount", "response should contain amount")
				assert.Contains(t, result, "balance", "response should contain balance")
				assert.Contains(t, result, "balanceAfter", "response should contain balanceAfter")
				assert.Contains(t, result, "status", "response should contain status")
				assert.Contains(t, result, "metadata", "response should contain metadata")
				assert.Equal(t, "DEBIT", result["type"])
				assert.Equal(t, "BRL", result["assetCode"])
			},
		},
		{
			name: "not found returns 404",
			setupMocks: func(operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID, operationID uuid.UUID) {
				operationRepo.EXPECT().
					FindByAccount(gomock.Any(), orgID, ledgerID, accountID, operationID).
					Return(nil, pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, constant.ErrNoOperationsFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID, operationID uuid.UUID) {
				operationRepo.EXPECT().
					FindByAccount(gomock.Any(), orgID, ledgerID, accountID, operationID).
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
			accountID := uuid.New()
			operationID := uuid.New()

			mockOperationRepo := operation.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockOperationRepo, mockMetadataRepo, orgID, ledgerID, accountID, operationID)

			queryUC := &query.UseCase{
				OperationRepo: mockOperationRepo,
				MetadataRepo:  mockMetadataRepo,
			}
			handler := &OperationHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations/:operation_id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("account_id", accountID)
					c.Locals("operation_id", operationID)
					return c.Next()
				},
				handler.GetOperationByAccount,
			)

			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations/"+operationID.String(), nil)
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

func TestOperationHandler_UpdateOperation(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		setupMocks     func(operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, transactionID, operationID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with updated operation",
			jsonBody: `{
				"description": "Updated operation description",
				"metadata": {"reason": "Purchase refund", "reference": "INV-12345"}
			}`,
			setupMocks: func(operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, transactionID, operationID uuid.UUID) {
				amount := decimal.NewFromInt(1500)
				available := decimal.NewFromInt(10000)
				onHold := decimal.NewFromInt(500)
				accountID := uuid.New()

				// Update operation in command use case
				operationRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, transactionID, operationID, gomock.Any()).
					Return(&operation.Operation{
						ID:              operationID.String(),
						TransactionID:   transactionID.String(),
						Description:     "Updated operation description",
						Type:            "DEBIT",
						AssetCode:       "BRL",
						ChartOfAccounts: "1000",
						Amount:          operation.Amount{Value: &amount},
						Balance:         operation.Balance{Available: &available, OnHold: &onHold},
						BalanceAfter:    operation.Balance{Available: &available, OnHold: &onHold},
						Status:          operation.Status{Code: "ACTIVE"},
						AccountID:       accountID.String(),
						AccountAlias:    "@person1",
						OrganizationID:  orgID.String(),
						LedgerID:        ledgerID.String(),
						CreatedAt:       time.Now(),
						UpdatedAt:       time.Now(),
					}, nil).
					Times(1)

				// UpdateMetadata first calls FindByEntity
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Operation", operationID.String()).
					Return(nil, nil).
					Times(1)

				// Then calls Update
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Operation", operationID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// GetOperationByID in query use case to return updated operation
				operationRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, transactionID, operationID).
					Return(&operation.Operation{
						ID:              operationID.String(),
						TransactionID:   transactionID.String(),
						Description:     "Updated operation description",
						Type:            "DEBIT",
						AssetCode:       "BRL",
						ChartOfAccounts: "1000",
						Amount:          operation.Amount{Value: &amount},
						Balance:         operation.Balance{Available: &available, OnHold: &onHold},
						BalanceAfter:    operation.Balance{Available: &available, OnHold: &onHold},
						Status:          operation.Status{Code: "ACTIVE"},
						AccountID:       accountID.String(),
						AccountAlias:    "@person1",
						OrganizationID:  orgID.String(),
						LedgerID:        ledgerID.String(),
						CreatedAt:       time.Now(),
						UpdatedAt:       time.Now(),
					}, nil).
					Times(1)

				// GetOperationByID also fetches metadata
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Operation", operationID.String()).
					Return(&mongodb.Metadata{
						EntityID:   operationID.String(),
						EntityName: "Operation",
						Data:       map[string]any{"reason": "Purchase refund", "reference": "INV-12345"},
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "description", "response should contain description")
				assert.Equal(t, "Updated operation description", result["description"])
				assert.Contains(t, result, "metadata", "response should contain metadata")
				metadata, ok := result["metadata"].(map[string]any)
				require.True(t, ok, "metadata should be an object")
				assert.Equal(t, "Purchase refund", metadata["reason"])
				assert.Equal(t, "INV-12345", metadata["reference"])
			},
		},
		{
			name: "not found returns 404",
			jsonBody: `{
				"description": "Updated description"
			}`,
			setupMocks: func(operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, transactionID, operationID uuid.UUID) {
				operationRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, transactionID, operationID, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, constant.ErrNoOperationsFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			jsonBody: `{
				"description": "Updated description"
			}`,
			setupMocks: func(operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, transactionID, operationID uuid.UUID) {
				operationRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, transactionID, operationID, gomock.Any()).
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
			transactionID := uuid.New()
			operationID := uuid.New()

			mockOperationRepo := operation.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockOperationRepo, mockMetadataRepo, orgID, ledgerID, transactionID, operationID)

			cmdUC := &command.UseCase{
				OperationRepo: mockOperationRepo,
				MetadataRepo:  mockMetadataRepo,
			}
			queryUC := &query.UseCase{
				OperationRepo: mockOperationRepo,
				MetadataRepo:  mockMetadataRepo,
			}
			handler := &OperationHandler{
				Command: cmdUC,
				Query:   queryUC,
			}

			app := fiber.New()
			app.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("transaction_id", transactionID)
					c.Locals("operation_id", operationID)
					return c.Next()
				},
				http.WithBody(new(operation.UpdateOperationInput), handler.UpdateOperation),
			)

			req := httptest.NewRequest("PATCH", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/operations/"+operationID.String(), bytes.NewBufferString(tt.jsonBody))
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

// Ensure libPostgres.Pagination is used (referenced in handler)
var _ = libPostgres.Pagination{}
