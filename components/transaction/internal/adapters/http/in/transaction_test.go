// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libConstants "github.com/LerianStudio/lib-commons/v3/commons/constants"
	libHTTP "github.com/LerianStudio/lib-commons/v3/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	operationroute "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"
)

func TestTransactionHandler_GetTransaction(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "success returns 200 with transaction",
			queryParams: "",
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID) {
				// Write-behind cache miss
				redisRepo.EXPECT().
					GetBytes(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("cache miss")).
					AnyTimes()
				amount := decimal.NewFromInt(1000)
				transactionRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, transactionID).
					Return(&transaction.Transaction{
						ID:             transactionID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Description:    "Test transaction",
						AssetCode:      "USD",
						Amount:         &amount,
						Status: transaction.Status{
							Code: cn.APPROVED,
						},
					}, nil).
					Times(1)
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
					Return(nil, nil).
					Times(1)
				operationRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, transactionID, gomock.Any()).
					Return([]*operation.Operation{}, libHTTP.CursorPagination{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "transaction should have id field")
				assert.Contains(t, result, "organizationId", "transaction should have organizationId field")
				assert.Contains(t, result, "ledgerId", "transaction should have ledgerId field")
				assert.Contains(t, result, "status", "transaction should have status field")
				assert.Equal(t, "USD", result["assetCode"])

				status, ok := result["status"].(map[string]any)
				require.True(t, ok, "status should be an object")
				assert.Equal(t, cn.APPROVED, status["code"])
			},
		},
		{
			name:        "not found returns 404",
			queryParams: "",
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID) {
				// Write-behind cache miss
				redisRepo.EXPECT().
					GetBytes(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("cache miss")).
					AnyTimes()
				transactionRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, transactionID).
					Return(nil, pkg.EntityNotFoundError{
						EntityType: "Transaction",
						Code:       cn.ErrEntityNotFound.Error(),
						Title:      "Entity Not Found",
						Message:    "Transaction not found",
					}).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrEntityNotFound.Error(), errResp["code"])
			},
		},
		{
			name:        "repository error returns 500",
			queryParams: "",
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID) {
				// Write-behind cache miss
				redisRepo.EXPECT().
					GetBytes(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("cache miss")).
					AnyTimes()
				transactionRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, transactionID).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "The server encountered an unexpected error.",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Contains(t, errResp, "message", "error response should contain message field")
			},
		},
		{
			name:        "metadata error returns 500",
			queryParams: "",
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID) {
				// Write-behind cache miss
				redisRepo.EXPECT().
					GetBytes(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("cache miss")).
					AnyTimes()
				amount := decimal.NewFromInt(1000)
				transactionRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, transactionID).
					Return(&transaction.Transaction{
						ID:             transactionID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Description:    "Test transaction",
						AssetCode:      "USD",
						Amount:         &amount,
						Status: transaction.Status{
							Code: cn.APPROVED,
						},
					}, nil).
					Times(1)
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Metadata service error.",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
			},
		},
		{
			name:        "operations error returns 500",
			queryParams: "",
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID) {
				// Write-behind cache miss
				redisRepo.EXPECT().
					GetBytes(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("cache miss")).
					AnyTimes()
				amount := decimal.NewFromInt(1000)
				transactionRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, transactionID).
					Return(&transaction.Transaction{
						ID:             transactionID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Description:    "Test transaction",
						AssetCode:      "USD",
						Amount:         &amount,
						Status: transaction.Status{
							Code: cn.APPROVED,
						},
					}, nil).
					Times(1)
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
					Return(nil, nil).
					Times(1)
				operationRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, transactionID, gomock.Any()).
					Return(nil, libHTTP.CursorPagination{}, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Operations service error.",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()
			transactionID := uuid.New()

			mockTransactionRepo := transaction.NewMockRepository(ctrl)
			mockOperationRepo := operation.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			tt.setupMocks(mockTransactionRepo, mockOperationRepo, mockMetadataRepo, mockRedisRepo, orgID, ledgerID, transactionID)

			uc := &query.UseCase{
				TransactionRepo: mockTransactionRepo,
				OperationRepo:   mockOperationRepo,
				MetadataRepo:    mockMetadataRepo,
				RedisRepo:       mockRedisRepo,
			}
			handler := &TransactionHandler{Query: uc}

			app := fiber.New()
			app.Get("/test/:organization_id/:ledger_id/transactions/:transaction_id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("transaction_id", transactionID)
					return c.Next()
				},
				handler.GetTransaction,
			)

			// Act
			req := httptest.NewRequest("GET",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+tt.queryParams,
				nil)
			resp, err := app.Test(req)

			// Assert
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

// TestCommitTransaction_InvalidStatus_ReturnsError validates that committing a transaction
// with a status other than PENDING returns HTTP 422 with error code 0099.
func TestCommitTransaction_InvalidStatus_ReturnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		currentStatus string
	}{
		{name: "status CREATED cannot be committed", currentStatus: cn.CREATED},
		{name: "status APPROVED cannot be committed", currentStatus: cn.APPROVED},
		{name: "status CANCELED cannot be committed", currentStatus: cn.CANCELED},
		{name: "status NOTED cannot be committed", currentStatus: cn.NOTED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()
			transactionID := uuid.New()

			mockTransactionRepo := transaction.NewMockRepository(ctrl)
			mockOperationRepo := operation.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)

			amount := decimal.NewFromInt(1000)
			txBody := pkgTransaction.Transaction{
				Send: pkgTransaction.Send{
					Source: pkgTransaction.Source{
						From: []pkgTransaction.FromTo{
							{AccountAlias: "@acc1"},
						},
					},
					Distribute: pkgTransaction.Distribute{
						To: []pkgTransaction.FromTo{
							{AccountAlias: "@acc2"},
						},
					},
				},
			}
			tran := &transaction.Transaction{
				ID:             transactionID.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				Description:    "Test transaction",
				AssetCode:      "USD",
				Amount:         &amount,
				Status: transaction.Status{
					Code: tt.currentStatus,
				},
				Body: txBody,
			}

			// Mock: Find transaction
			mockTransactionRepo.EXPECT().
				Find(gomock.Any(), orgID, ledgerID, transactionID).
				Return(tran, nil).
				Times(1)

			// Mock: Metadata lookup
			mockMetadataRepo.EXPECT().
				FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
				Return(nil, nil).
				Times(1)

			// Mock: Redis lock acquired successfully
			mockRedisRepo.EXPECT().
				SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(true, nil).
				Times(1)

			// Mock: Redis lock cleanup after error
			mockRedisRepo.EXPECT().
				Del(gomock.Any(), gomock.Any()).
				Return(nil).
				Times(1)

			// Write-behind cache miss (fall through to Postgres Find)
			mockRedisRepo.EXPECT().
				GetBytes(gomock.Any(), gomock.Any()).
				Return(nil, errors.New("cache miss")).
				AnyTimes()

			queryUC := &query.UseCase{
				TransactionRepo: mockTransactionRepo,
				OperationRepo:   mockOperationRepo,
				MetadataRepo:    mockMetadataRepo,
				RedisRepo:       mockRedisRepo,
			}
			commandUC := &command.UseCase{
				RedisRepo: mockRedisRepo,
			}
			handler := &TransactionHandler{Query: queryUC, Command: commandUC}

			app := fiber.New()
			app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/commit",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("transaction_id", transactionID)
					return c.Next()
				},
				handler.CommitTransaction,
			)

			// Act
			req := httptest.NewRequest("POST",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/commit",
				nil)
			resp, err := app.Test(req)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, 422, resp.StatusCode, "expected HTTP 422 for non-PENDING status")

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var errResp map[string]any
			err = json.Unmarshal(body, &errResp)
			require.NoError(t, err, "error response should be valid JSON")

			assert.Equal(t, cn.ErrCommitTransactionNotPending.Error(), errResp["code"],
				"expected error code 0099 (ErrCommitTransactionNotPending)")
		})
	}
}

// TestRevertTransaction_InvalidStatus_ReturnsError validates that reverting a transaction
// with a status other than APPROVED returns HTTP 422 with error code 0099.
func TestRevertTransaction_InvalidStatus_ReturnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		currentStatus string
	}{
		{name: "status PENDING cannot be reverted", currentStatus: cn.PENDING},
		{name: "status CREATED cannot be reverted", currentStatus: cn.CREATED},
		{name: "status CANCELED cannot be reverted", currentStatus: cn.CANCELED},
		{name: "status NOTED cannot be reverted", currentStatus: cn.NOTED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()
			transactionID := uuid.New()

			mockTransactionRepo := transaction.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)

			amount := decimal.NewFromInt(1000)
			tran := &transaction.Transaction{
				ID:                  transactionID.String(),
				OrganizationID:      orgID.String(),
				LedgerID:            ledgerID.String(),
				ParentTransactionID: nil, // Not a revert transaction
				Description:         "Test transaction",
				AssetCode:           "USD",
				Amount:              &amount,
				Status: transaction.Status{
					Code: tt.currentStatus,
				},
			}

			// Mock: No existing revert (parent lookup returns nil)
			mockTransactionRepo.EXPECT().
				FindByParentID(gomock.Any(), orgID, ledgerID, transactionID).
				Return(nil, nil).
				Times(1)

			// Mock: Find transaction with operations
			mockTransactionRepo.EXPECT().
				FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
				Return(tran, nil).
				Times(1)

			// Mock: Metadata lookup
			mockMetadataRepo.EXPECT().
				FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
				Return(nil, nil).
				Times(1)

			queryUC := &query.UseCase{
				TransactionRepo: mockTransactionRepo,
				MetadataRepo:    mockMetadataRepo,
			}
			handler := &TransactionHandler{Query: queryUC}

			app := fiber.New()
			app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/revert",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("transaction_id", transactionID)
					return c.Next()
				},
				handler.RevertTransaction,
			)

			// Act
			req := httptest.NewRequest("POST",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
				nil)
			resp, err := app.Test(req)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, 422, resp.StatusCode, "expected HTTP 422 for non-APPROVED status")

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var errResp map[string]any
			err = json.Unmarshal(body, &errResp)
			require.NoError(t, err, "error response should be valid JSON")

			assert.Equal(t, cn.ErrCommitTransactionNotPending.Error(), errResp["code"],
				"expected error code 0099 (transaction status invalid for revert)")
		})
	}
}

// TestRevertTransaction_AlreadyHasRevert_ReturnsError validates that reverting a transaction
// that already has a revert returns HTTP 422 with error code 0087.
func TestRevertTransaction_AlreadyHasRevert_ReturnsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	existingRevertID := uuid.New()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	// Existing revert transaction
	existingRevert := &transaction.Transaction{
		ID:                  existingRevertID.String(),
		ParentTransactionID: ptr(transactionID.String()),
	}

	// Mock: Parent lookup returns existing revert
	mockTransactionRepo.EXPECT().
		FindByParentID(gomock.Any(), orgID, ledgerID, transactionID).
		Return(existingRevert, nil).
		Times(1)

	// Mock: Metadata lookup for the existing revert
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", existingRevertID.String()).
		Return(nil, nil).
		Times(1)

	queryUC := &query.UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}
	handler := &TransactionHandler{Query: queryUC}

	app := fiber.New()
	app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/revert",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgID)
			c.Locals("ledger_id", ledgerID)
			c.Locals("transaction_id", transactionID)
			return c.Next()
		},
		handler.RevertTransaction,
	)

	// Act
	req := httptest.NewRequest("POST",
		"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
		nil)
	resp, err := app.Test(req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode, "expected HTTP 400 for already reverted transaction")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	err = json.Unmarshal(body, &errResp)
	require.NoError(t, err, "error response should be valid JSON")

	assert.Equal(t, cn.ErrTransactionIDHasAlreadyParentTransaction.Error(), errResp["code"],
		"expected error code 0087 (ErrTransactionIDHasAlreadyParentTransaction)")
}

// TestRevertTransaction_IsAlreadyARevert_ReturnsError validates that reverting a transaction
// that is itself a revert returns HTTP 422 with error code 0088.
func TestRevertTransaction_IsAlreadyARevert_ReturnsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	originalTransactionID := uuid.New()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	amount := decimal.NewFromInt(1000)
	// Transaction that IS a revert (has ParentTransactionID)
	tran := &transaction.Transaction{
		ID:                  transactionID.String(),
		OrganizationID:      orgID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: ptr(originalTransactionID.String()), // This IS a revert
		Description:         "Revert transaction",
		AssetCode:           "USD",
		Amount:              &amount,
		Status: transaction.Status{
			Code: cn.APPROVED,
		},
	}

	// Mock: No existing revert of this transaction
	mockTransactionRepo.EXPECT().
		FindByParentID(gomock.Any(), orgID, ledgerID, transactionID).
		Return(nil, nil).
		Times(1)

	// Mock: Find transaction - it's already a revert
	mockTransactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
		Return(tran, nil).
		Times(1)

	// Mock: Metadata lookup
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, nil).
		Times(1)

	queryUC := &query.UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}
	handler := &TransactionHandler{Query: queryUC}

	app := fiber.New()
	app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/revert",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgID)
			c.Locals("ledger_id", ledgerID)
			c.Locals("transaction_id", transactionID)
			return c.Next()
		},
		handler.RevertTransaction,
	)

	// Act
	req := httptest.NewRequest("POST",
		"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
		nil)
	resp, err := app.Test(req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode, "expected HTTP 400 for transaction that is already a revert")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	err = json.Unmarshal(body, &errResp)
	require.NoError(t, err, "error response should be valid JSON")

	assert.Equal(t, cn.ErrTransactionIDIsAlreadyARevert.Error(), errResp["code"],
		"expected error code 0088 (ErrTransactionIDIsAlreadyARevert)")
}

// TestRevertTransaction_GetParentError_ReturnsError validates that errors from
// GetParentByTransactionID are properly propagated.
func TestRevertTransaction_GetParentError_ReturnsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	// Mock: Parent lookup returns error
	mockTransactionRepo.EXPECT().
		FindByParentID(gomock.Any(), orgID, ledgerID, transactionID).
		Return(nil, pkg.InternalServerError{
			Code:    "0046",
			Title:   "Internal Server Error",
			Message: "Database connection failed",
		}).
		Times(1)

	queryUC := &query.UseCase{
		TransactionRepo: mockTransactionRepo,
	}
	handler := &TransactionHandler{Query: queryUC}

	app := fiber.New()
	app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/revert",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgID)
			c.Locals("ledger_id", ledgerID)
			c.Locals("transaction_id", transactionID)
			return c.Next()
		},
		handler.RevertTransaction,
	)

	// Act
	req := httptest.NewRequest("POST",
		"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
		nil)
	resp, err := app.Test(req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode, "expected HTTP 500 for database error")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	err = json.Unmarshal(body, &errResp)
	require.NoError(t, err, "error response should be valid JSON")

	assert.Contains(t, errResp, "code", "error response should contain code field")
}

// TestRevertTransaction_GetTransactionError_ReturnsError validates that errors from
// GetTransactionWithOperationsByID are properly propagated.
func TestRevertTransaction_GetTransactionError_ReturnsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	// Mock: No existing revert (parent lookup returns nil)
	mockTransactionRepo.EXPECT().
		FindByParentID(gomock.Any(), orgID, ledgerID, transactionID).
		Return(nil, nil).
		Times(1)

	// Mock: Transaction lookup returns error
	mockTransactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
		Return(nil, pkg.EntityNotFoundError{
			EntityType: "Transaction",
			Code:       cn.ErrEntityNotFound.Error(),
			Title:      "Entity Not Found",
			Message:    "Transaction not found",
		}).
		Times(1)

	// Mock: Metadata lookup (conditional - may not be called if transaction lookup fails first)
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, nil).
		AnyTimes()

	queryUC := &query.UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}
	handler := &TransactionHandler{Query: queryUC}

	app := fiber.New()
	app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/revert",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgID)
			c.Locals("ledger_id", ledgerID)
			c.Locals("transaction_id", transactionID)
			return c.Next()
		},
		handler.RevertTransaction,
	)

	// Act
	req := httptest.NewRequest("POST",
		"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
		nil)
	resp, err := app.Test(req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode, "expected HTTP 404 for not found")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	err = json.Unmarshal(body, &errResp)
	require.NoError(t, err, "error response should be valid JSON")

	assert.Equal(t, cn.ErrEntityNotFound.Error(), errResp["code"],
		"expected error code for entity not found")
}

// TestRevertTransaction_EmptyRevert_ReturnsError validates that when TransactionRevert
// returns an empty result (transaction can't be reverted), HTTP 400 is returned.
// TransactionRevert.IsEmpty() returns true when AssetCode is empty and Amount is zero.
func TestRevertTransaction_EmptyRevert_ReturnsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	// Transaction with APPROVED status but empty AssetCode and zero Amount
	// This causes TransactionRevert().IsEmpty() to return true
	zeroAmount := decimal.Zero
	tran := &transaction.Transaction{
		ID:                  transactionID.String(),
		OrganizationID:      orgID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: nil,
		Description:         "Test transaction",
		AssetCode:           "", // Empty asset code
		Amount:              &zeroAmount,
		Status: transaction.Status{
			Code: cn.APPROVED,
		},
		Body: pkgTransaction.Transaction{},
	}

	// Mock: No existing revert (parent lookup returns nil)
	mockTransactionRepo.EXPECT().
		FindByParentID(gomock.Any(), orgID, ledgerID, transactionID).
		Return(nil, nil).
		Times(1)

	// Mock: Find transaction with operations
	mockTransactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
		Return(tran, nil).
		Times(1)

	// Mock: Metadata lookup
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, nil).
		Times(1)

	queryUC := &query.UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}
	handler := &TransactionHandler{Query: queryUC}

	app := fiber.New()
	app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/revert",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgID)
			c.Locals("ledger_id", ledgerID)
			c.Locals("transaction_id", transactionID)
			return c.Next()
		},
		handler.RevertTransaction,
	)

	// Act
	req := httptest.NewRequest("POST",
		"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
		nil)
	resp, err := app.Test(req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode, "expected HTTP 400 for empty revert")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	err = json.Unmarshal(body, &errResp)
	require.NoError(t, err, "error response should be valid JSON")

	assert.Equal(t, cn.ErrTransactionCantRevert.Error(), errResp["code"],
		"expected error code 0089 (ErrTransactionCantRevert)")
}

// TestRevertTransaction_BidirectionalRouteAllows validates that a revert is allowed
// when the operation route has OperationType "bidirectional".
func TestRevertTransaction_BidirectionalRouteAllows(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	operationRouteID := uuid.New()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)

	amount := decimal.NewFromInt(1000)
	tran := &transaction.Transaction{
		ID:                  transactionID.String(),
		OrganizationID:      orgID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: nil,
		Description:         "Test transaction with bidirectional route",
		AssetCode:           "USD",
		Amount:              &amount,
		Status: transaction.Status{
			Code: cn.APPROVED,
		},
		Operations: []*operation.Operation{
			{
				Type:         libConstants.CREDIT,
				AccountAlias: "@receiver",
				AssetCode:    "USD",
				Amount:       operation.Amount{Value: &amount},
				Route:        operationRouteID.String(),
			},
		},
	}

	// Mock: No existing revert
	mockTransactionRepo.EXPECT().
		FindByParentID(gomock.Any(), orgID, ledgerID, transactionID).
		Return(nil, nil).
		Times(1)

	// Mock: Find transaction with operations
	mockTransactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
		Return(tran, nil).
		Times(1)

	// Mock: Metadata lookup
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, nil).
		Times(1)

	// Mock: Operation route is bidirectional
	mockOperationRouteRepo.EXPECT().
		FindByID(gomock.Any(), orgID, ledgerID, operationRouteID).
		Return(&mmodel.OperationRoute{
			ID:            operationRouteID,
			OperationType: "bidirectional",
		}, nil).
		Times(1)

	// Mock: Metadata for the operation route
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "OperationRoute", operationRouteID.String()).
		Return(nil, nil).
		AnyTimes()

	queryUC := &query.UseCase{
		TransactionRepo:    mockTransactionRepo,
		MetadataRepo:       mockMetadataRepo,
		OperationRouteRepo: mockOperationRouteRepo,
	}
	// The handler needs Command for createTransaction; since we only test
	// that the bidirectional check passes (not the full createTransaction flow),
	// we use a Fiber error handler to catch panics from nil Command and verify
	// the bidirectional error was not returned.
	handler := &TransactionHandler{Query: queryUC}

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal"})
		},
	})
	app.Use(func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				_ = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "recovered"})
			}
		}()
		return c.Next()
	})
	app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/revert",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgID)
			c.Locals("ledger_id", ledgerID)
			c.Locals("transaction_id", transactionID)
			return c.Next()
		},
		handler.RevertTransaction,
	)

	// Act
	req := httptest.NewRequest("POST",
		"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
		nil)
	resp, err := app.Test(req)

	// Assert: should NOT return the bidirectional error.
	// The handler passes the bidirectional gate but may fail downstream
	// (e.g., nil Command for createTransaction). We verify the gate passed,
	// not the full revert flow.
	require.NoError(t, err)
	if resp.StatusCode >= 400 {
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var errResp map[string]any
		require.NoError(t, json.Unmarshal(body, &errResp))

		// If there's an error, it must NOT be the bidirectional error
		assert.NotEqual(t, cn.ErrRouteNotBidirectional.Error(), errResp["code"],
			"bidirectional route should allow revert; gate check must pass")
	}
}

// TestRevertTransaction_NonBidirectionalRouteRejects validates that a revert is rejected
// when the operation route has OperationType other than "bidirectional" (e.g., "source").
func TestRevertTransaction_NonBidirectionalRouteRejects(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	operationRouteID := uuid.New()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)

	amount := decimal.NewFromInt(1000)
	tran := &transaction.Transaction{
		ID:                  transactionID.String(),
		OrganizationID:      orgID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: nil,
		Description:         "Test transaction with non-bidirectional route",
		AssetCode:           "USD",
		Amount:              &amount,
		Status: transaction.Status{
			Code: cn.APPROVED,
		},
		Operations: []*operation.Operation{
			{
				Type:         libConstants.CREDIT,
				AccountAlias: "@receiver",
				AssetCode:    "USD",
				Amount:       operation.Amount{Value: &amount},
				Route:        operationRouteID.String(),
			},
		},
	}

	// Mock: No existing revert
	mockTransactionRepo.EXPECT().
		FindByParentID(gomock.Any(), orgID, ledgerID, transactionID).
		Return(nil, nil).
		Times(1)

	// Mock: Find transaction with operations
	mockTransactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
		Return(tran, nil).
		Times(1)

	// Mock: Metadata lookup
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, nil).
		Times(1)

	// Mock: Operation route is NOT bidirectional (type "source")
	mockOperationRouteRepo.EXPECT().
		FindByID(gomock.Any(), orgID, ledgerID, operationRouteID).
		Return(&mmodel.OperationRoute{
			ID:            operationRouteID,
			OperationType: "source",
		}, nil).
		Times(1)

	// Mock: Metadata for the operation route
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "OperationRoute", operationRouteID.String()).
		Return(nil, nil).
		AnyTimes()

	queryUC := &query.UseCase{
		TransactionRepo:    mockTransactionRepo,
		MetadataRepo:       mockMetadataRepo,
		OperationRouteRepo: mockOperationRouteRepo,
	}
	handler := &TransactionHandler{Query: queryUC}

	app := fiber.New()
	app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/revert",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgID)
			c.Locals("ledger_id", ledgerID)
			c.Locals("transaction_id", transactionID)
			return c.Next()
		},
		handler.RevertTransaction,
	)

	// Act
	req := httptest.NewRequest("POST",
		"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
		nil)
	resp, err := app.Test(req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 422, resp.StatusCode, "expected HTTP 422 for non-bidirectional route")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	err = json.Unmarshal(body, &errResp)
	require.NoError(t, err, "error response should be valid JSON")

	assert.Equal(t, cn.ErrRouteNotBidirectional.Error(), errResp["code"],
		"expected ErrRouteNotBidirectional error code")
}

// TestRevertTransaction_NoRouteRevertsNormally validates that operations without
// a route_id skip the bidirectional check and revert normally.
func TestRevertTransaction_NoRouteRevertsNormally(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	amount := decimal.NewFromInt(1000)
	tran := &transaction.Transaction{
		ID:                  transactionID.String(),
		OrganizationID:      orgID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: nil,
		Description:         "Test transaction without routes",
		AssetCode:           "USD",
		Amount:              &amount,
		Status: transaction.Status{
			Code: cn.APPROVED,
		},
		Operations: []*operation.Operation{
			{
				Type:         libConstants.CREDIT,
				AccountAlias: "@receiver",
				AssetCode:    "USD",
				Amount:       operation.Amount{Value: &amount},
				// No Route set
			},
		},
	}

	// Mock: No existing revert
	mockTransactionRepo.EXPECT().
		FindByParentID(gomock.Any(), orgID, ledgerID, transactionID).
		Return(nil, nil).
		Times(1)

	// Mock: Find transaction with operations
	mockTransactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
		Return(tran, nil).
		Times(1)

	// Mock: Metadata lookup
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, nil).
		Times(1)

	// No OperationRouteRepo mock needed -- no route to look up

	queryUC := &query.UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}
	handler := &TransactionHandler{Query: queryUC}

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal"})
		},
	})
	app.Use(func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				_ = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "recovered"})
			}
		}()
		return c.Next()
	})
	app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/revert",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgID)
			c.Locals("ledger_id", ledgerID)
			c.Locals("transaction_id", transactionID)
			return c.Next()
		},
		handler.RevertTransaction,
	)

	// Act
	req := httptest.NewRequest("POST",
		"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
		nil)
	resp, err := app.Test(req)

	// Assert: should NOT return a bidirectional error.
	// The handler passes the bidirectional gate (skipped for no-route ops)
	// but may fail downstream (e.g., nil Command). We verify the gate passed,
	// not the full revert flow.
	require.NoError(t, err)
	if resp.StatusCode >= 400 {
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var errResp map[string]any
		require.NoError(t, json.Unmarshal(body, &errResp))

		assert.NotEqual(t, cn.ErrRouteNotBidirectional.Error(), errResp["code"],
			"operations without route should skip bidirectional check")
	}
}

// TestRevertTransaction_RouteLookupError_ReturnsError validates that when the
// route lookup fails, the revert is blocked (fail-closed behavior).
func TestRevertTransaction_RouteLookupError_ReturnsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	operationRouteID := uuid.New()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)

	amount := decimal.NewFromInt(1000)
	tran := &transaction.Transaction{
		ID:                  transactionID.String(),
		OrganizationID:      orgID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: nil,
		Description:         "Test transaction with route lookup failure",
		AssetCode:           "USD",
		Amount:              &amount,
		Status: transaction.Status{
			Code: cn.APPROVED,
		},
		Operations: []*operation.Operation{
			{
				Type:         libConstants.CREDIT,
				AccountAlias: "@receiver",
				AssetCode:    "USD",
				Amount:       operation.Amount{Value: &amount},
				Route:        operationRouteID.String(),
			},
		},
	}

	routeLookupErr := errors.New("database connection error")

	// Mock: No existing revert
	mockTransactionRepo.EXPECT().
		FindByParentID(gomock.Any(), orgID, ledgerID, transactionID).
		Return(nil, nil).
		Times(1)

	// Mock: Find transaction with operations
	mockTransactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
		Return(tran, nil).
		Times(1)

	// Mock: Metadata lookup
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, nil).
		Times(1)

	// Mock: Operation route lookup fails
	mockOperationRouteRepo.EXPECT().
		FindByID(gomock.Any(), orgID, ledgerID, operationRouteID).
		Return(nil, routeLookupErr).
		Times(1)

	queryUC := &query.UseCase{
		TransactionRepo:    mockTransactionRepo,
		MetadataRepo:       mockMetadataRepo,
		OperationRouteRepo: mockOperationRouteRepo,
	}
	handler := &TransactionHandler{Query: queryUC}

	app := fiber.New()
	app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/revert",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgID)
			c.Locals("ledger_id", ledgerID)
			c.Locals("transaction_id", transactionID)
			return c.Next()
		},
		handler.RevertTransaction,
	)

	// Act
	req := httptest.NewRequest("POST",
		"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
		nil)
	resp, err := app.Test(req)

	// Assert: route lookup failure must block the revert (fail-closed)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, resp.StatusCode, 400,
		"route lookup failure should return an error status")
}

// TestCommitTransaction_GetTransactionError_ReturnsError validates that errors from
// GetTransactionByID are properly propagated.
func TestCommitTransaction_GetTransactionError_ReturnsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Mock: Write-behind cache miss (fall through to Postgres Find)
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("cache miss")).
		AnyTimes()

	// Mock: Transaction lookup returns error
	mockTransactionRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, transactionID).
		Return(nil, pkg.EntityNotFoundError{
			EntityType: "Transaction",
			Code:       cn.ErrEntityNotFound.Error(),
			Title:      "Entity Not Found",
			Message:    "Transaction not found",
		}).
		Times(1)

	queryUC := &query.UseCase{
		TransactionRepo: mockTransactionRepo,
		RedisRepo:       mockRedisRepo,
	}
	handler := &TransactionHandler{Query: queryUC}

	app := fiber.New()
	app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/commit",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgID)
			c.Locals("ledger_id", ledgerID)
			c.Locals("transaction_id", transactionID)
			return c.Next()
		},
		handler.CommitTransaction,
	)

	// Act
	req := httptest.NewRequest("POST",
		"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/commit",
		nil)
	resp, err := app.Test(req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode, "expected HTTP 404 for not found")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	err = json.Unmarshal(body, &errResp)
	require.NoError(t, err, "error response should be valid JSON")

	assert.Equal(t, cn.ErrEntityNotFound.Error(), errResp["code"],
		"expected error code for entity not found")
}

// TestCommitTransaction_RedisLockError_ReturnsError validates that errors from
// Redis SetNX (lock acquisition) are properly propagated.
func TestCommitTransaction_RedisLockError_ReturnsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	amount := decimal.NewFromInt(1000)
	txBody := pkgTransaction.Transaction{
		Send: pkgTransaction.Send{
			Asset: "USD",
			Value: amount,
			Source: pkgTransaction.Source{
				From: []pkgTransaction.FromTo{{AccountAlias: "@acc1"}},
			},
			Distribute: pkgTransaction.Distribute{
				To: []pkgTransaction.FromTo{{AccountAlias: "@acc2"}},
			},
		},
	}
	tran := &transaction.Transaction{
		ID:             transactionID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Description:    "Test transaction",
		AssetCode:      "USD",
		Amount:         &amount,
		Status: transaction.Status{
			Code: cn.PENDING,
		},
		Body: txBody,
	}

	// Mock: Transaction found successfully
	mockTransactionRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, transactionID).
		Return(tran, nil).
		Times(1)

	// Mock: Metadata lookup
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, nil).
		Times(1)

	// Mock: Write-behind cache miss (fall through to Postgres Find)
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("cache miss")).
		AnyTimes()

	// Mock: Redis lock acquisition fails with error
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, pkg.InternalServerError{
			Code:    "0046",
			Title:   "Internal Server Error",
			Message: "Redis connection failed",
		}).
		Times(1)

	queryUC := &query.UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
		RedisRepo:       mockRedisRepo,
	}
	commandUC := &command.UseCase{
		RedisRepo: mockRedisRepo,
	}
	handler := &TransactionHandler{Query: queryUC, Command: commandUC}

	app := fiber.New()
	app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/commit",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgID)
			c.Locals("ledger_id", ledgerID)
			c.Locals("transaction_id", transactionID)
			return c.Next()
		},
		handler.CommitTransaction,
	)

	// Act
	req := httptest.NewRequest("POST",
		"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/commit",
		nil)
	resp, err := app.Test(req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode, "expected HTTP 500 for Redis error")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	err = json.Unmarshal(body, &errResp)
	require.NoError(t, err, "error response should be valid JSON")

	assert.Contains(t, errResp, "code", "error response should contain code field")
}

// TestCommitTransaction_LockNotAcquired_ReturnsError validates that when the transaction
// lock cannot be acquired (already being processed), HTTP 422 is returned.
func TestCommitTransaction_LockNotAcquired_ReturnsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	amount := decimal.NewFromInt(1000)
	txBody := pkgTransaction.Transaction{
		Send: pkgTransaction.Send{
			Asset: "USD",
			Value: amount,
			Source: pkgTransaction.Source{
				From: []pkgTransaction.FromTo{{AccountAlias: "@acc1"}},
			},
			Distribute: pkgTransaction.Distribute{
				To: []pkgTransaction.FromTo{{AccountAlias: "@acc2"}},
			},
		},
	}
	tran := &transaction.Transaction{
		ID:             transactionID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Description:    "Test transaction",
		AssetCode:      "USD",
		Amount:         &amount,
		Status: transaction.Status{
			Code: cn.PENDING,
		},
		Body: txBody,
	}

	// Mock: Transaction found successfully
	mockTransactionRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, transactionID).
		Return(tran, nil).
		Times(1)

	// Mock: Metadata lookup
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, nil).
		Times(1)

	// Mock: Write-behind cache miss (fall through to Postgres Find)
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("cache miss")).
		AnyTimes()

	// Mock: Redis lock NOT acquired (returns false, nil) - transaction already being processed
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).
		Times(1)

	queryUC := &query.UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
		RedisRepo:       mockRedisRepo,
	}
	commandUC := &command.UseCase{
		RedisRepo: mockRedisRepo,
	}
	handler := &TransactionHandler{Query: queryUC, Command: commandUC}

	app := fiber.New()
	app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/commit",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgID)
			c.Locals("ledger_id", ledgerID)
			c.Locals("transaction_id", transactionID)
			return c.Next()
		},
		handler.CommitTransaction,
	)

	// Act
	req := httptest.NewRequest("POST",
		"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/commit",
		nil)
	resp, err := app.Test(req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 422, resp.StatusCode, "expected HTTP 422 for locked transaction")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	err = json.Unmarshal(body, &errResp)
	require.NoError(t, err, "error response should be valid JSON")

	assert.Equal(t, cn.ErrCommitTransactionNotPending.Error(), errResp["code"],
		"expected error code 0099 (ErrCommitTransactionNotPending)")
}

// TestCreateTransactionJSON_NonPositiveValue_Returns422 validates that creating a transaction
// with send.value <= 0 returns HTTP 422 with error code 0125.
// Business rule: Transaction values must be greater than zero.
func TestCreateTransactionJSON_NonPositiveValue_Returns422(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sendValue string
	}{
		{name: "zero value is rejected", sendValue: "0"},
		{name: "negative value is rejected", sendValue: "-1"},
		{name: "negative decimal is rejected", sendValue: "-0.01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()

			// No mocks needed - validation short-circuits before any repository call
			handler := &TransactionHandler{}

			app := fiber.New()
			app.Post("/test/:organization_id/:ledger_id/transactions/json",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				http.WithBody(new(transaction.CreateTransactionInput), handler.CreateTransactionJSON),
			)

			// Build request body with non-positive value
			requestBody := `{
				"send": {
					"asset": "USD",
					"value": "` + tt.sendValue + `",
					"source": {
						"from": [{"accountAlias": "@source", "amount": {"asset": "USD", "value": "100"}}]
					},
					"distribute": {
						"to": [{"accountAlias": "@dest", "amount": {"asset": "USD", "value": "100"}}]
					}
				}
			}`

			// Act
			req := httptest.NewRequest("POST",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/json",
				strings.NewReader(requestBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, 422, resp.StatusCode, "expected HTTP 422 for non-positive transaction value")

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var errResp map[string]any
			err = json.Unmarshal(body, &errResp)
			require.NoError(t, err, "error response should be valid JSON")

			assert.Equal(t, cn.ErrInvalidTransactionNonPositiveValue.Error(), errResp["code"],
				"expected error code 0125 (ErrInvalidTransactionNonPositiveValue)")

			// Verify error message is present and descriptive
			msg, ok := errResp["message"].(string)
			assert.True(t, ok, "error response should contain message field")
			assert.Contains(t, msg, "zero", "error message should mention zero values")
		})
	}
}

// TestCreateTransactionInflow_NonPositiveValue_Returns422 validates that creating an inflow transaction
// with send.value <= 0 returns HTTP 422 with error code 0125.
// Business rule: Transaction values must be greater than zero.
func TestCreateTransactionInflow_NonPositiveValue_Returns422(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sendValue string
	}{
		{name: "zero value is rejected", sendValue: "0"},
		{name: "negative value is rejected", sendValue: "-1"},
		{name: "negative decimal is rejected", sendValue: "-0.01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()

			// No mocks needed - validation short-circuits before any repository call
			handler := &TransactionHandler{}

			app := fiber.New()
			app.Post("/test/:organization_id/:ledger_id/transactions/inflow",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				http.WithBody(new(transaction.CreateTransactionInflowInput), handler.CreateTransactionInflow),
			)

			// Build request body with non-positive value (inflow has no source, only distribute.to)
			requestBody := `{
				"send": {
					"asset": "USD",
					"value": "` + tt.sendValue + `",
					"distribute": {
						"to": [{"accountAlias": "@dest", "amount": {"asset": "USD", "value": "100"}}]
					}
				}
			}`

			// Act
			req := httptest.NewRequest("POST",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/inflow",
				strings.NewReader(requestBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, 422, resp.StatusCode, "expected HTTP 422 for non-positive transaction value")

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var errResp map[string]any
			err = json.Unmarshal(body, &errResp)
			require.NoError(t, err, "error response should be valid JSON")

			assert.Equal(t, cn.ErrInvalidTransactionNonPositiveValue.Error(), errResp["code"],
				"expected error code 0125 (ErrInvalidTransactionNonPositiveValue)")

			// Verify error message is present and descriptive
			msg, ok := errResp["message"].(string)
			assert.True(t, ok, "error response should contain message field")
			assert.Contains(t, msg, "zero", "error message should mention zero values")
		})
	}
}

// TestCreateTransactionOutflow_NonPositiveValue_Returns422 validates that creating an outflow transaction
// with send.value <= 0 returns HTTP 422 with error code 0125.
// Business rule: Transaction values must be greater than zero.
func TestCreateTransactionOutflow_NonPositiveValue_Returns422(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sendValue string
	}{
		{name: "zero value is rejected", sendValue: "0"},
		{name: "negative value is rejected", sendValue: "-1"},
		{name: "negative decimal is rejected", sendValue: "-0.01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()

			// No mocks needed - validation short-circuits before any repository call
			handler := &TransactionHandler{}

			app := fiber.New()
			app.Post("/test/:organization_id/:ledger_id/transactions/outflow",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				http.WithBody(new(transaction.CreateTransactionOutflowInput), handler.CreateTransactionOutflow),
			)

			// Build request body with non-positive value (outflow has no distribute.to, only source.from)
			requestBody := `{
				"send": {
					"asset": "USD",
					"value": "` + tt.sendValue + `",
					"source": {
						"from": [{"accountAlias": "@source", "amount": {"asset": "USD", "value": "100"}}]
					}
				}
			}`

			// Act
			req := httptest.NewRequest("POST",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/outflow",
				strings.NewReader(requestBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, 422, resp.StatusCode, "expected HTTP 422 for non-positive transaction value")

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var errResp map[string]any
			err = json.Unmarshal(body, &errResp)
			require.NoError(t, err, "error response should be valid JSON")

			assert.Equal(t, cn.ErrInvalidTransactionNonPositiveValue.Error(), errResp["code"],
				"expected error code 0125 (ErrInvalidTransactionNonPositiveValue)")

			// Verify error message is present and descriptive
			msg, ok := errResp["message"].(string)
			assert.True(t, ok, "error response should contain message field")
			assert.Contains(t, msg, "zero", "error message should mention zero values")
		})
	}
}

// ptr is a helper function to create a pointer to a string.
func ptr(s string) *string {
	return &s
}

// TestTransactionHandler_GetAllTransactions tests the GetAllTransactions handler
func TestTransactionHandler_GetAllTransactions(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "success returns 200 with pagination (cursor-based)",
			queryParams: "?limit=10&sort_order=desc",
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				transactionID := uuid.New()
				amount := decimal.NewFromInt(1000)
				transactionRepo.EXPECT().
					FindOrListAllWithOperations(gomock.Any(), orgID, ledgerID, []uuid.UUID{}, gomock.Any()).
					Return([]*transaction.Transaction{
						{
							ID:             transactionID.String(),
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Description:    "Test transaction",
							AssetCode:      "USD",
							Amount:         &amount,
							Status: transaction.Status{
								Code: cn.APPROVED,
							},
							Operations: []*operation.Operation{},
						},
					}, libHTTP.CursorPagination{
						Next: "next-cursor-token",
						Prev: "prev-cursor-token",
					}, nil).
					Times(1)
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Transaction", []string{transactionID.String()}).
					Return([]*mongodb.Metadata{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "items", "response should have items field")
				assert.Contains(t, result, "next_cursor", "response should have next_cursor for pagination")
				assert.Contains(t, result, "prev_cursor", "response should have prev_cursor for pagination")

				items, ok := result["items"].([]any)
				require.True(t, ok, "items should be an array")
				assert.Len(t, items, 1, "should have one transaction")
			},
		},
		{
			name:        "success returns 200 with metadata filter (dual code path)",
			queryParams: "?metadata.category=payment",
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				transactionID := uuid.New()
				amount := decimal.NewFromInt(500)

				// First: FindList is called for metadata filtering
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Transaction", gomock.Any()).
					Return([]*mongodb.Metadata{
						{
							EntityID: transactionID.String(),
							Data:     map[string]any{"category": "payment"},
						},
					}, nil).
					Times(1)

				// Then: FindOrListAllWithOperations is called with the filtered IDs
				transactionRepo.EXPECT().
					FindOrListAllWithOperations(gomock.Any(), orgID, ledgerID, []uuid.UUID{transactionID}, gomock.Any()).
					Return([]*transaction.Transaction{
						{
							ID:             transactionID.String(),
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Description:    "Payment transaction",
							AssetCode:      "USD",
							Amount:         &amount,
							Status: transaction.Status{
								Code: cn.APPROVED,
							},
							Operations: []*operation.Operation{},
						},
					}, libHTTP.CursorPagination{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "items", "response should have items field")

				items, ok := result["items"].([]any)
				require.True(t, ok, "items should be an array")
				assert.Len(t, items, 1, "should have one transaction matching metadata filter")
			},
		},
		{
			name:        "success returns 200 without metadata filter",
			queryParams: "",
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				transactionID := uuid.New()
				amount := decimal.NewFromInt(2000)

				transactionRepo.EXPECT().
					FindOrListAllWithOperations(gomock.Any(), orgID, ledgerID, []uuid.UUID{}, gomock.Any()).
					Return([]*transaction.Transaction{
						{
							ID:             transactionID.String(),
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Description:    "Regular transaction",
							AssetCode:      "EUR",
							Amount:         &amount,
							Status: transaction.Status{
								Code: cn.APPROVED,
							},
							Operations: []*operation.Operation{},
						},
					}, libHTTP.CursorPagination{}, nil).
					Times(1)
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Transaction", []string{transactionID.String()}).
					Return([]*mongodb.Metadata{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "items", "response should have items field")
			},
		},
		{
			name:        "invalid query parameters returns 400",
			queryParams: "?start_date=invalid-date-format",
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// No mocks needed - validation fails before repository calls
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
			},
		},
		{
			name:        "repository error returns 500",
			queryParams: "",
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				transactionRepo.EXPECT().
					FindOrListAllWithOperations(gomock.Any(), orgID, ledgerID, []uuid.UUID{}, gomock.Any()).
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
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Contains(t, errResp, "message", "error response should contain message field")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()

			mockTransactionRepo := transaction.NewMockRepository(ctrl)
			mockOperationRepo := operation.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockTransactionRepo, mockOperationRepo, mockMetadataRepo, orgID, ledgerID)

			uc := &query.UseCase{
				TransactionRepo: mockTransactionRepo,
				OperationRepo:   mockOperationRepo,
				MetadataRepo:    mockMetadataRepo,
			}
			handler := &TransactionHandler{Query: uc}

			app := fiber.New()
			app.Get("/test/:organization_id/:ledger_id/transactions",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.GetAllTransactions,
			)

			// Act
			req := httptest.NewRequest("GET",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions"+tt.queryParams,
				nil)
			resp, err := app.Test(req)

			// Assert
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

// TestTransactionHandler_UpdateTransaction tests the UpdateTransaction handler
func TestTransactionHandler_UpdateTransaction(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    string
		setupMocks     func(transactionRepo *transaction.MockRepository, metadataRepo *mongodb.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, transactionID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "success returns 200 with updated transaction",
			requestBody: `{"description": "Updated description", "metadata": {"key": "value"}}`,
			setupMocks: func(transactionRepo *transaction.MockRepository, metadataRepo *mongodb.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, transactionID uuid.UUID) {
				amount := decimal.NewFromInt(1000)

				// Command.UpdateTransaction calls TransactionRepo.Update
				transactionRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, transactionID, gomock.Any()).
					Return(&transaction.Transaction{
						ID:             transactionID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Description:    "Updated description",
						AssetCode:      "USD",
						Amount:         &amount,
						Status: transaction.Status{
							Code: cn.APPROVED,
						},
					}, nil).
					Times(1)

				// Command.UpdateMetadata first calls FindByEntity to get existing metadata
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
					Return(nil, nil).
					Times(1)

				// Command.UpdateMetadata then calls MetadataRepo.Update
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Transaction", transactionID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Query.GetTransactionByID (read-after-write pattern) calls TransactionRepo.Find
				transactionRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, transactionID).
					Return(&transaction.Transaction{
						ID:             transactionID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Description:    "Updated description",
						AssetCode:      "USD",
						Amount:         &amount,
						Status: transaction.Status{
							Code: cn.APPROVED,
						},
					}, nil).
					Times(1)

				// Query.GetTransactionByID calls FindByEntity for transaction metadata
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
					Return(&mongodb.Metadata{
						EntityID: transactionID.String(),
						Data:     map[string]any{"key": "value"},
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Equal(t, "Updated description", result["description"])
				assert.Contains(t, result, "metadata", "response should have metadata field")
			},
		},
		{
			name:        "not found returns 404",
			requestBody: `{"description": "Updated description"}`,
			setupMocks: func(transactionRepo *transaction.MockRepository, metadataRepo *mongodb.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, transactionID uuid.UUID) {
				transactionRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, transactionID, gomock.Any()).
					Return(nil, pkg.EntityNotFoundError{
						EntityType: "Transaction",
						Code:       cn.ErrTransactionIDNotFound.Error(),
						Title:      "Entity Not Found",
						Message:    "Transaction not found",
					}).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrTransactionIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name:        "repository update error returns 500",
			requestBody: `{"description": "Updated description"}`,
			setupMocks: func(transactionRepo *transaction.MockRepository, metadataRepo *mongodb.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, transactionID uuid.UUID) {
				transactionRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, transactionID, gomock.Any()).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database update failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
			},
		},
		{
			name:        "repository get error after update returns 500",
			requestBody: `{"description": "Updated description", "metadata": {"key": "value"}}`,
			setupMocks: func(transactionRepo *transaction.MockRepository, metadataRepo *mongodb.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, transactionID uuid.UUID) {
				amount := decimal.NewFromInt(1000)

				// Update succeeds
				transactionRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, transactionID, gomock.Any()).
					Return(&transaction.Transaction{
						ID:             transactionID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Description:    "Updated description",
						AssetCode:      "USD",
						Amount:         &amount,
						Status: transaction.Status{
							Code: cn.APPROVED,
						},
					}, nil).
					Times(1)

				// UpdateMetadata first calls FindByEntity to check existing metadata
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
					Return(nil, nil).
					Times(1)

				// UpdateMetadata then calls Update
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Transaction", transactionID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Get after update fails
				transactionRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, transactionID).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database read failed after update",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()
			transactionID := uuid.New()

			mockTransactionRepo := transaction.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			mockOperationRepo := operation.NewMockRepository(ctrl)
			tt.setupMocks(mockTransactionRepo, mockMetadataRepo, mockOperationRepo, orgID, ledgerID, transactionID)

			queryUC := &query.UseCase{
				TransactionRepo: mockTransactionRepo,
				MetadataRepo:    mockMetadataRepo,
				OperationRepo:   mockOperationRepo,
			}
			commandUC := &command.UseCase{
				TransactionRepo: mockTransactionRepo,
				MetadataRepo:    mockMetadataRepo,
			}
			handler := &TransactionHandler{Query: queryUC, Command: commandUC}

			app := fiber.New()
			app.Patch("/test/:organization_id/:ledger_id/transactions/:transaction_id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("transaction_id", transactionID)
					return c.Next()
				},
				http.WithBody(new(transaction.UpdateTransactionInput), handler.UpdateTransaction),
			)

			// Act
			req := httptest.NewRequest("PATCH",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String(),
				strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			// Assert
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

// TestCreateTransactionAnnotation_NonPositiveValue_Returns422 validates that creating an annotation
// with send.value <= 0 returns HTTP 422 with error code 0125.
// Business rule: Transaction values must be greater than zero, even for annotations.
func TestCreateTransactionAnnotation_NonPositiveValue_Returns422(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sendValue string
	}{
		{name: "zero value is rejected", sendValue: "0"},
		{name: "negative value is rejected", sendValue: "-1"},
		{name: "negative decimal is rejected", sendValue: "-0.01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()

			// No mocks needed - validation short-circuits before any repository call
			handler := &TransactionHandler{}

			app := fiber.New()
			app.Post("/test/:organization_id/:ledger_id/transactions/annotation",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				http.WithBody(new(transaction.CreateTransactionInput), handler.CreateTransactionAnnotation),
			)

			// Build request body with non-positive value
			requestBody := `{
				"send": {
					"asset": "USD",
					"value": "` + tt.sendValue + `",
					"source": {
						"from": [{"accountAlias": "@source", "amount": {"asset": "USD", "value": "100"}}]
					},
					"distribute": {
						"to": [{"accountAlias": "@dest", "amount": {"asset": "USD", "value": "100"}}]
					}
				}
			}`

			// Act
			req := httptest.NewRequest("POST",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/annotation",
				strings.NewReader(requestBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, 422, resp.StatusCode, "expected HTTP 422 for non-positive transaction value")

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var errResp map[string]any
			err = json.Unmarshal(body, &errResp)
			require.NoError(t, err, "error response should be valid JSON")

			assert.Equal(t, cn.ErrInvalidTransactionNonPositiveValue.Error(), errResp["code"],
				"expected error code 0125 (ErrInvalidTransactionNonPositiveValue)")

			// Verify error message is present and descriptive
			msg, ok := errResp["message"].(string)
			assert.True(t, ok, "error response should contain message field")
			assert.Contains(t, msg, "zero", "error message should mention zero values")
		})
	}
}

// TestCreateTransactionDSL_DeprecationHeaders validates that the deprecated DSL endpoint
// returns RFC 8594 compliant deprecation headers.
// RFC 8594 specifies standard HTTP headers for communicating API deprecation status:
// - Deprecation: indicates the resource is deprecated
// - Sunset: specifies when the deprecated resource will become unavailable
// - Link with rel="successor-version": points to the replacement resource
func TestCreateTransactionDSL_DeprecationHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		setupRequest     func(orgID, ledgerID uuid.UUID) *nethttp.Request
		expectedStatus   int
		validateHeaders  func(t *testing.T, resp *nethttp.Response, orgID, ledgerID uuid.UUID)
		validateResponse func(t *testing.T, body []byte)
	}{
		{
			name: "deprecation headers present on missing file error",
			setupRequest: func(orgID, ledgerID uuid.UUID) *nethttp.Request {
				// Request without file - will fail validation but should still have deprecation headers
				req := httptest.NewRequest(nethttp.MethodPost,
					"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/dsl",
					nil)
				req.Header.Set("Content-Type", "multipart/form-data")
				return req
			},
			expectedStatus: 400,
			validateHeaders: func(t *testing.T, resp *nethttp.Response, orgID, ledgerID uuid.UUID) {
				// Verify Deprecation header
				assert.Equal(t, "true", resp.Header.Get("Deprecation"),
					"Deprecation header should be 'true'")

				// Verify Sunset header with correct date format
				assert.Equal(t, "Sat, 01 Aug 2026 00:00:00 GMT", resp.Header.Get("Sunset"),
					"Sunset header should have correct RFC 1123 date format")

				// Verify Link header with successor-version
				expectedLink := "</v1/organizations/" + orgID.String() +
					"/ledgers/" + ledgerID.String() +
					"/transactions/json>; rel=\"successor-version\""
				assert.Equal(t, expectedLink, resp.Header.Get("Link"),
					"Link header should point to JSON endpoint with successor-version rel")
			},
			validateResponse: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")
				assert.Contains(t, errResp, "code", "error response should contain code field")
			},
		},
		{
			name: "deprecation headers present with invalid query parameters",
			setupRequest: func(orgID, ledgerID uuid.UUID) *nethttp.Request {
				req := httptest.NewRequest(nethttp.MethodPost,
					"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/dsl?start_date=invalid-format",
					nil)
				req.Header.Set("Content-Type", "multipart/form-data")
				return req
			},
			expectedStatus: 400,
			validateHeaders: func(t *testing.T, resp *nethttp.Response, orgID, ledgerID uuid.UUID) {
				// Even on validation errors, deprecation headers should be present
				assert.Equal(t, "true", resp.Header.Get("Deprecation"),
					"Deprecation header should be present even on error")
				assert.Equal(t, "Sat, 01 Aug 2026 00:00:00 GMT", resp.Header.Get("Sunset"),
					"Sunset header should be present even on error")
				assert.Contains(t, resp.Header.Get("Link"), "successor-version",
					"Link header should contain successor-version rel")
			},
			validateResponse: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")
				assert.Contains(t, errResp, "code", "error response should contain code field")
			},
		},
		{
			name: "Link header contains dynamic organization_id and ledger_id",
			setupRequest: func(orgID, ledgerID uuid.UUID) *nethttp.Request {
				req := httptest.NewRequest(nethttp.MethodPost,
					"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/dsl",
					nil)
				req.Header.Set("Content-Type", "multipart/form-data")
				return req
			},
			expectedStatus: 400,
			validateHeaders: func(t *testing.T, resp *nethttp.Response, orgID, ledgerID uuid.UUID) {
				linkHeader := resp.Header.Get("Link")

				// Verify Link header contains the specific organization_id
				assert.Contains(t, linkHeader, orgID.String(),
					"Link header should contain the organization_id from the request")

				// Verify Link header contains the specific ledger_id
				assert.Contains(t, linkHeader, ledgerID.String(),
					"Link header should contain the ledger_id from the request")

				// Verify Link header has correct structure
				assert.Contains(t, linkHeader, "/v1/organizations/",
					"Link header should have /v1/organizations/ path prefix")
				assert.Contains(t, linkHeader, "/ledgers/",
					"Link header should have /ledgers/ path segment")
				assert.Contains(t, linkHeader, "/transactions/json",
					"Link header should point to /transactions/json endpoint")
				assert.Contains(t, linkHeader, "rel=\"successor-version\"",
					"Link header should have rel=\"successor-version\"")
			},
			validateResponse: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()

			// No mocks needed - these tests focus on response headers
			// which are set before any business logic executes
			handler := &TransactionHandler{}

			app := fiber.New()
			app.Post("/test/:organization_id/:ledger_id/transactions/dsl",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.CreateTransactionDSL,
			)

			// Act
			req := tt.setupRequest(orgID, ledgerID)
			resp, err := app.Test(req)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			// Validate deprecation headers
			if tt.validateHeaders != nil {
				tt.validateHeaders(t, resp, orgID, ledgerID)
			}

			// Validate response body
			if tt.validateResponse != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateResponse(t, body)
			}
		})
	}
}

// TestCreateTransactionDSL_DeprecationHeaders_DifferentIDs validates that the Link header
// correctly uses the organization_id and ledger_id from each unique request.
func TestCreateTransactionDSL_DeprecationHeaders_DifferentIDs(t *testing.T) {
	t.Parallel()

	// Test with multiple different ID combinations to ensure dynamic header construction
	testCases := []struct {
		name string
	}{
		{name: "first unique ID pair"},
		{name: "second unique ID pair"},
		{name: "third unique ID pair"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Generate unique IDs for each test
			orgID := uuid.New()
			ledgerID := uuid.New()

			handler := &TransactionHandler{}

			app := fiber.New()
			app.Post("/test/:organization_id/:ledger_id/transactions/dsl",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.CreateTransactionDSL,
			)

			req := httptest.NewRequest(nethttp.MethodPost,
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/dsl",
				nil)
			req.Header.Set("Content-Type", "multipart/form-data")

			resp, err := app.Test(req)
			require.NoError(t, err)

			// Build expected Link header with this test's specific IDs
			expectedLink := "</v1/organizations/" + orgID.String() +
				"/ledgers/" + ledgerID.String() +
				"/transactions/json>; rel=\"successor-version\""

			assert.Equal(t, expectedLink, resp.Header.Get("Link"),
				"Link header should use the organization_id and ledger_id from this specific request")
		})
	}
}

// TestCancelTransaction tests the CancelTransaction handler
func TestCancelTransaction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setupMocks     func(transactionRepo *transaction.MockRepository, metadataRepo *mongodb.MockRepository, operationRepo *operation.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "transaction not found returns 404",
			setupMocks: func(transactionRepo *transaction.MockRepository, metadataRepo *mongodb.MockRepository, operationRepo *operation.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID) {
				transactionRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, transactionID).
					Return(nil, pkg.EntityNotFoundError{
						EntityType: "Transaction",
						Code:       cn.ErrEntityNotFound.Error(),
						Title:      "Entity Not Found",
						Message:    "Transaction not found",
					}).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrEntityNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "transaction not PENDING returns 422",
			setupMocks: func(transactionRepo *transaction.MockRepository, metadataRepo *mongodb.MockRepository, operationRepo *operation.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID) {
				amount := decimal.NewFromInt(1000)
				txBody := pkgTransaction.Transaction{
					Send: pkgTransaction.Send{
						Source: pkgTransaction.Source{
							From: []pkgTransaction.FromTo{
								{AccountAlias: "@acc1"},
							},
						},
						Distribute: pkgTransaction.Distribute{
							To: []pkgTransaction.FromTo{
								{AccountAlias: "@acc2"},
							},
						},
					},
				}
				tran := &transaction.Transaction{
					ID:             transactionID.String(),
					OrganizationID: orgID.String(),
					LedgerID:       ledgerID.String(),
					Description:    "Test transaction",
					AssetCode:      "USD",
					Amount:         &amount,
					Status: transaction.Status{
						Code: cn.APPROVED, // Not PENDING
					},
					Body: txBody,
				}

				// Query.GetTransactionByID
				transactionRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, transactionID).
					Return(tran, nil).
					Times(1)

				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
					Return(nil, nil).
					Times(1)

				// Redis lock acquired successfully
				redisRepo.EXPECT().
					SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(true, nil).
					Times(1)

				// Redis lock cleanup after error
				redisRepo.EXPECT().
					Del(gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
			},
			expectedStatus: 422,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Equal(t, cn.ErrCommitTransactionNotPending.Error(), errResp["code"],
					"expected error code 0099 (ErrCommitTransactionNotPending)")
			},
		},
		{
			name: "Redis lock failure returns 500",
			setupMocks: func(transactionRepo *transaction.MockRepository, metadataRepo *mongodb.MockRepository, operationRepo *operation.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID) {
				amount := decimal.NewFromInt(1000)
				txBody := pkgTransaction.Transaction{
					Send: pkgTransaction.Send{
						Source: pkgTransaction.Source{
							From: []pkgTransaction.FromTo{
								{AccountAlias: "@acc1"},
							},
						},
						Distribute: pkgTransaction.Distribute{
							To: []pkgTransaction.FromTo{
								{AccountAlias: "@acc2"},
							},
						},
					},
				}
				tran := &transaction.Transaction{
					ID:             transactionID.String(),
					OrganizationID: orgID.String(),
					LedgerID:       ledgerID.String(),
					Description:    "Test transaction",
					AssetCode:      "USD",
					Amount:         &amount,
					Status: transaction.Status{
						Code: cn.PENDING,
					},
					Body: txBody,
				}

				// Query.GetTransactionByID
				transactionRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, transactionID).
					Return(tran, nil).
					Times(1)

				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
					Return(nil, nil).
					Times(1)

				// Redis SetNX returns error
				redisRepo.EXPECT().
					SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Redis connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
			},
		},
		{
			name: "lock already acquired by another process returns 422",
			setupMocks: func(transactionRepo *transaction.MockRepository, metadataRepo *mongodb.MockRepository, operationRepo *operation.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID) {
				amount := decimal.NewFromInt(1000)
				txBody := pkgTransaction.Transaction{
					Send: pkgTransaction.Send{
						Source: pkgTransaction.Source{
							From: []pkgTransaction.FromTo{
								{AccountAlias: "@acc1"},
							},
						},
						Distribute: pkgTransaction.Distribute{
							To: []pkgTransaction.FromTo{
								{AccountAlias: "@acc2"},
							},
						},
					},
				}
				tran := &transaction.Transaction{
					ID:             transactionID.String(),
					OrganizationID: orgID.String(),
					LedgerID:       ledgerID.String(),
					Description:    "Test transaction",
					AssetCode:      "USD",
					Amount:         &amount,
					Status: transaction.Status{
						Code: cn.PENDING,
					},
					Body: txBody,
				}

				// Query.GetTransactionByID
				transactionRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, transactionID).
					Return(tran, nil).
					Times(1)

				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
					Return(nil, nil).
					Times(1)

				// Redis SetNX returns false (lock already held by another process)
				redisRepo.EXPECT().
					SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).
					Times(1)
			},
			expectedStatus: 422,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Equal(t, cn.ErrCommitTransactionNotPending.Error(), errResp["code"],
					"expected error code 0099 when transaction is locked")
			},
		},
		{
			name: "metadata retrieval error returns 500",
			setupMocks: func(transactionRepo *transaction.MockRepository, metadataRepo *mongodb.MockRepository, operationRepo *operation.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID) {
				amount := decimal.NewFromInt(1000)
				tran := &transaction.Transaction{
					ID:             transactionID.String(),
					OrganizationID: orgID.String(),
					LedgerID:       ledgerID.String(),
					Description:    "Test transaction",
					AssetCode:      "USD",
					Amount:         &amount,
					Status: transaction.Status{
						Code: cn.PENDING,
					},
				}

				// Query.GetTransactionByID - Find succeeds
				transactionRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, transactionID).
					Return(tran, nil).
					Times(1)

				// Metadata retrieval fails
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "MongoDB connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()
			transactionID := uuid.New()

			mockTransactionRepo := transaction.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			mockOperationRepo := operation.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			tt.setupMocks(mockTransactionRepo, mockMetadataRepo, mockOperationRepo, mockRedisRepo, orgID, ledgerID, transactionID)

			// Write-behind cache miss (fall through to Postgres Find)
			mockRedisRepo.EXPECT().
				GetBytes(gomock.Any(), gomock.Any()).
				Return(nil, errors.New("cache miss")).
				AnyTimes()

			queryUC := &query.UseCase{
				TransactionRepo: mockTransactionRepo,
				MetadataRepo:    mockMetadataRepo,
				OperationRepo:   mockOperationRepo,
				RedisRepo:       mockRedisRepo,
			}
			commandUC := &command.UseCase{
				RedisRepo: mockRedisRepo,
			}
			handler := &TransactionHandler{Query: queryUC, Command: commandUC}

			app := fiber.New()
			app.Post("/test/:organization_id/:ledger_id/transactions/:transaction_id/cancel",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("transaction_id", transactionID)
					return c.Next()
				},
				handler.CancelTransaction,
			)

			// Act
			req := httptest.NewRequest("POST",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/cancel",
				nil)
			resp, err := app.Test(req)

			// Assert
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

// --- Write-behind cache tests (from maintenance branch) ---

func newTestTransactionData(orgID, ledgerID, tranID uuid.UUID) *transaction.Transaction {
	return &transaction.Transaction{
		ID:             tranID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AssetCode:      "BRL",
		Status:         transaction.Status{Code: "PENDING"},
	}
}

// TestGetTransaction_WriteBehindHit verifies that GetTransaction returns 200 from write-behind cache,
// skipping both Postgres lookup and operations query.
func TestGetTransaction_WriteBehindHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	tranID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	queryUC := &query.UseCase{RedisRepo: mockRedisRepo}
	handler := &TransactionHandler{
		Command: &command.UseCase{},
		Query:   queryUC,
	}

	// Write-behind hit
	tran := newTestTransactionData(orgID, ledgerID, tranID)
	wbData, err := msgpack.Marshal(tran)
	require.NoError(t, err)

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(wbData, nil).
		Times(1)

	// No TransactionRepo mock -> proves Postgres is never called

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", orgID)
		c.Locals("ledger_id", ledgerID)
		c.Locals("transaction_id", tranID)
		return handler.GetTransaction(c)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "true", resp.Header.Get("X-Cache-Hit"))
}

// TestCancelTransaction_WriteBehindMiss_PostgresMiss verifies that CancelTransaction returns error
// when both write-behind and Postgres fail.
func TestCancelTransaction_WriteBehindMiss_PostgresMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	tranID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	queryUC := &query.UseCase{
		RedisRepo:       mockRedisRepo,
		TransactionRepo: mockTransactionRepo,
	}
	handler := &TransactionHandler{
		Command: &command.UseCase{},
		Query:   queryUC,
	}

	// Write-behind miss
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("redis: nil")).
		Times(1)

	// Postgres miss
	mockTransactionRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, tranID).
		Return(nil, errors.New("record not found")).
		Times(1)

	app := fiber.New()
	app.Post("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", orgID)
		c.Locals("ledger_id", ledgerID)
		c.Locals("transaction_id", tranID)
		return handler.CancelTransaction(c)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.True(t, resp.StatusCode >= 400, "Expected error status code, got %d", resp.StatusCode)
}

// TestCancelTransaction_WriteBehindMiss_PostgresHit verifies fallback to Postgres when write-behind misses.
func TestCancelTransaction_WriteBehindMiss_PostgresHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	tranID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	queryUC := &query.UseCase{
		RedisRepo:       mockRedisRepo,
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}
	handler := &TransactionHandler{
		Command: &command.UseCase{RedisRepo: mockRedisRepo},
		Query:   queryUC,
	}

	tran := newTestTransactionData(orgID, ledgerID, tranID)

	// Write-behind miss
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("redis: nil")).
		Times(1)

	// Postgres hit
	mockTransactionRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, tranID).
		Return(tran, nil).
		Times(1)

	// Metadata lookup (returns nil = no metadata)
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)

	// commitOrCancelTransaction: SetNX short-circuits (we're only testing the lookup path)
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, errors.New("lock error")).
		Times(1)

	app := fiber.New()
	app.Post("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", orgID)
		c.Locals("ledger_id", ledgerID)
		c.Locals("transaction_id", tranID)
		return handler.CancelTransaction(c)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	// Response is an error (from SetNX), but the important thing is Find WAS called (fallback worked)
	assert.True(t, resp.StatusCode >= 400)
}

// TestCancelTransaction_WriteBehindHit_PostgresNotCalled verifies that when write-behind hits,
// Postgres is not queried.
func TestCancelTransaction_WriteBehindHit_PostgresNotCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	tranID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	queryUC := &query.UseCase{RedisRepo: mockRedisRepo}
	handler := &TransactionHandler{
		Command: &command.UseCase{RedisRepo: mockRedisRepo},
		Query:   queryUC,
	}

	// Write-behind hit
	tran := newTestTransactionData(orgID, ledgerID, tranID)
	wbData, err := msgpack.Marshal(tran)
	require.NoError(t, err)

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(wbData, nil).
		Times(1)

	// No TransactionRepo mock -> proves Postgres is never called

	// commitOrCancelTransaction: SetNX short-circuits
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, errors.New("lock error")).
		Times(1)

	app := fiber.New()
	app.Post("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", orgID)
		c.Locals("ledger_id", ledgerID)
		c.Locals("transaction_id", tranID)
		return handler.CancelTransaction(c)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	// Error from SetNX short-circuit, but write-behind was used and Postgres was NOT called
	assert.True(t, resp.StatusCode >= 400)
}

// TestCommitTransaction_WriteBehindMiss_PostgresMiss verifies that CommitTransaction returns error
// when both write-behind and Postgres fail.
func TestCommitTransaction_WriteBehindMiss_PostgresMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	tranID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	queryUC := &query.UseCase{
		RedisRepo:       mockRedisRepo,
		TransactionRepo: mockTransactionRepo,
	}
	handler := &TransactionHandler{
		Command: &command.UseCase{},
		Query:   queryUC,
	}

	// Write-behind miss
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("redis: nil")).
		Times(1)

	// Postgres miss
	mockTransactionRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, tranID).
		Return(nil, errors.New("record not found")).
		Times(1)

	app := fiber.New()
	app.Post("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", orgID)
		c.Locals("ledger_id", ledgerID)
		c.Locals("transaction_id", tranID)
		return handler.CommitTransaction(c)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.True(t, resp.StatusCode >= 400, "Expected error status code, got %d", resp.StatusCode)
}

// TestCommitTransaction_WriteBehindMiss_PostgresHit verifies fallback to Postgres when write-behind misses.
func TestCommitTransaction_WriteBehindMiss_PostgresHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	tranID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	queryUC := &query.UseCase{
		RedisRepo:       mockRedisRepo,
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}
	handler := &TransactionHandler{
		Command: &command.UseCase{RedisRepo: mockRedisRepo},
		Query:   queryUC,
	}

	tran := newTestTransactionData(orgID, ledgerID, tranID)

	// Write-behind miss
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("redis: nil")).
		Times(1)

	// Postgres hit
	mockTransactionRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, tranID).
		Return(tran, nil).
		Times(1)

	// Metadata lookup
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)

	// commitOrCancelTransaction: SetNX short-circuits
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, errors.New("lock error")).
		Times(1)

	app := fiber.New()
	app.Post("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", orgID)
		c.Locals("ledger_id", ledgerID)
		c.Locals("transaction_id", tranID)
		return handler.CommitTransaction(c)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	// Error from SetNX short-circuit, but Find WAS called (fallback worked)
	assert.True(t, resp.StatusCode >= 400)
}

// TestCommitTransaction_WriteBehindHit_PostgresNotCalled verifies that when write-behind hits,
// Postgres is not queried.
func TestCommitTransaction_WriteBehindHit_PostgresNotCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	tranID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	queryUC := &query.UseCase{RedisRepo: mockRedisRepo}
	handler := &TransactionHandler{
		Command: &command.UseCase{RedisRepo: mockRedisRepo},
		Query:   queryUC,
	}

	// Write-behind hit
	tran := newTestTransactionData(orgID, ledgerID, tranID)
	wbData, err := msgpack.Marshal(tran)
	require.NoError(t, err)

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(wbData, nil).
		Times(1)

	// No TransactionRepo mock -> proves Postgres is never called

	// commitOrCancelTransaction: SetNX short-circuits
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, errors.New("lock error")).
		Times(1)

	app := fiber.New()
	app.Post("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", orgID)
		c.Locals("ledger_id", ledgerID)
		c.Locals("transaction_id", tranID)
		return handler.CommitTransaction(c)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	// Error from SetNX short-circuit, but write-behind was used and Postgres was NOT called
	assert.True(t, resp.StatusCode >= 400)
}

func TestPropagateRouteValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		isPending         bool
		from              map[string]pkgTransaction.Amount
		to                map[string]pkgTransaction.Amount
		expectedFromFlags map[string]bool
		expectedToFlags   map[string]bool
	}{
		{
			name:      "pending transaction sets RouteValidationEnabled on all From entries",
			isPending: true,
			from: map[string]pkgTransaction.Amount{
				"@source1": {
					Value:     decimal.NewFromInt(500),
					Operation: libConstants.ONHOLD,
				},
				"@source2": {
					Value:     decimal.NewFromInt(500),
					Operation: libConstants.ONHOLD,
				},
			},
			to: map[string]pkgTransaction.Amount{
				"@dest1": {
					Value:     decimal.NewFromInt(1000),
					Operation: libConstants.CREDIT,
				},
			},
			expectedFromFlags: map[string]bool{
				"@source1": true,
				"@source2": true,
			},
			expectedToFlags: map[string]bool{
				"@dest1": false,
			},
		},
		{
			name:      "non-pending transaction does not set RouteValidationEnabled",
			isPending: false,
			from: map[string]pkgTransaction.Amount{
				"@source1": {
					Value:     decimal.NewFromInt(1000),
					Operation: libConstants.DEBIT,
				},
			},
			to: map[string]pkgTransaction.Amount{
				"@dest1": {
					Value:     decimal.NewFromInt(1000),
					Operation: libConstants.CREDIT,
				},
			},
			expectedFromFlags: map[string]bool{
				"@source1": false,
			},
			expectedToFlags: map[string]bool{
				"@dest1": false,
			},
		},
		{
			name:              "pending transaction with empty From map is a no-op",
			isPending:         true,
			from:              map[string]pkgTransaction.Amount{},
			to:                map[string]pkgTransaction.Amount{},
			expectedFromFlags: map[string]bool{},
			expectedToFlags:   map[string]bool{},
		},
		{
			name:      "pending transaction with single From entry",
			isPending: true,
			from: map[string]pkgTransaction.Amount{
				"@source1": {
					Value:                  decimal.NewFromInt(100),
					Operation:              libConstants.ONHOLD,
					RouteValidationEnabled: false,
				},
			},
			to: map[string]pkgTransaction.Amount{},
			expectedFromFlags: map[string]bool{
				"@source1": true,
			},
			expectedToFlags: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			validate := &pkgTransaction.Responses{
				From: tt.from,
				To:   tt.to,
			}

			transactionStatus := cn.CREATED
			if tt.isPending {
				transactionStatus = cn.PENDING
			}

			propagateRouteValidation(ctx, validate, tt.isPending, transactionStatus)

			for key, expectedFlag := range tt.expectedFromFlags {
				amt, exists := validate.From[key]
				assert.True(t, exists, "From map should contain key %s", key)
				assert.Equal(t, expectedFlag, amt.RouteValidationEnabled,
					"From[%s].RouteValidationEnabled should be %v", key, expectedFlag)
			}

			for key, expectedFlag := range tt.expectedToFlags {
				amt, exists := validate.To[key]
				assert.True(t, exists, "To map should contain key %s", key)
				assert.Equal(t, expectedFlag, amt.RouteValidationEnabled,
					"To[%s].RouteValidationEnabled should not be modified", key)
			}
		})
	}
}

func TestBuildDoubleEntryPendingOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		balance            *mmodel.Balance
		fromTo             pkgTransaction.FromTo
		amount             pkgTransaction.Amount
		balanceAfter       pkgTransaction.Balance
		tran               transaction.Transaction
		transactionInput   pkgTransaction.Transaction
		isAnnotation       bool
		expectedOpCount    int
		expectedOp1Type    string
		expectedOp2Type    string
		checkVersionChain  bool
		checkBalanceFields bool
	}{
		{
			name: "generates exactly 2 operations with correct types",
			balance: &mmodel.Balance{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
				AccountID:      uuid.New().String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(0),
				Version:        5,
			},
			fromTo: pkgTransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
				Description:  "test operation",
			},
			amount: pkgTransaction.Amount{
				Value:                  decimal.NewFromInt(300),
				Operation:              libConstants.ONHOLD,
				TransactionType:        cn.PENDING,
				RouteValidationEnabled: true,
			},
			balanceAfter: pkgTransaction.Balance{
				Available: decimal.NewFromInt(700),
				OnHold:    decimal.NewFromInt(300),
				Version:   7,
			},
			tran: transaction.Transaction{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
			},
			transactionInput: pkgTransaction.Transaction{
				Pending: true,
				Send:    pkgTransaction.Send{Asset: "BRL"},
			},
			isAnnotation:       false,
			expectedOpCount:    2,
			expectedOp1Type:    cn.DEBIT,
			expectedOp2Type:    libConstants.ONHOLD,
			checkVersionChain:  true,
			checkBalanceFields: true,
		},
		{
			name: "annotation mode zeroes all balance fields",
			balance: &mmodel.Balance{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
				AccountID:      uuid.New().String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(0),
				Version:        5,
			},
			fromTo: pkgTransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amount: pkgTransaction.Amount{
				Value:                  decimal.NewFromInt(200),
				Operation:              libConstants.ONHOLD,
				TransactionType:        cn.PENDING,
				RouteValidationEnabled: true,
			},
			balanceAfter: pkgTransaction.Balance{
				Available: decimal.NewFromInt(800),
				OnHold:    decimal.NewFromInt(200),
				Version:   7,
			},
			tran: transaction.Transaction{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
			},
			transactionInput: pkgTransaction.Transaction{
				Pending:     true,
				Description: "annotation test",
				Send:        pkgTransaction.Send{Asset: "BRL"},
			},
			isAnnotation:    true,
			expectedOpCount: 2,
			expectedOp1Type: cn.DEBIT,
			expectedOp2Type: libConstants.ONHOLD,
		},
		{
			name: "uses transaction description when fromTo description is empty",
			balance: &mmodel.Balance{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
				AccountID:      uuid.New().String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(500),
				OnHold:         decimal.NewFromInt(0),
				Version:        1,
			},
			fromTo: pkgTransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
				Description:  "",
			},
			amount: pkgTransaction.Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              libConstants.ONHOLD,
				TransactionType:        cn.PENDING,
				RouteValidationEnabled: true,
			},
			balanceAfter: pkgTransaction.Balance{
				Available: decimal.NewFromInt(400),
				OnHold:    decimal.NewFromInt(100),
				Version:   3,
			},
			tran: transaction.Transaction{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
			},
			transactionInput: pkgTransaction.Transaction{
				Pending:     true,
				Description: "fallback description",
				Send:        pkgTransaction.Send{Asset: "USD"},
			},
			isAnnotation:    false,
			expectedOpCount: 2,
			expectedOp1Type: cn.DEBIT,
			expectedOp2Type: libConstants.ONHOLD,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			handler := &TransactionHandler{}
			transactionDate := time.Now()

			ops := handler.buildDoubleEntryPendingOps(
				ctx,
				tt.balance,
				tt.fromTo,
				tt.amount,
				tt.balanceAfter,
				tt.tran,
				tt.transactionInput,
				transactionDate,
				tt.isAnnotation,
			)

			require.Len(t, ops, tt.expectedOpCount, "should generate exactly %d operations", tt.expectedOpCount)

			op1 := ops[0]
			op2 := ops[1]

			// Verify operation types
			assert.Equal(t, tt.expectedOp1Type, op1.Type, "op1 should be DEBIT")
			assert.Equal(t, tt.expectedOp2Type, op2.Type, "op2 should be ON_HOLD")

			// Both ops share the same transaction and balance IDs
			assert.Equal(t, tt.tran.ID, op1.TransactionID)
			assert.Equal(t, tt.tran.ID, op2.TransactionID)
			assert.Equal(t, tt.balance.ID, op1.BalanceID)
			assert.Equal(t, tt.balance.ID, op2.BalanceID)

			// Both ops have same amount value
			assert.True(t, tt.amount.Value.Equal(*op1.Amount.Value), "op1 amount should match input")
			assert.True(t, tt.amount.Value.Equal(*op2.Amount.Value), "op2 amount should match input")

			// Op IDs are different (each is a distinct UUIDv7)
			assert.NotEqual(t, op1.ID, op2.ID, "op1 and op2 should have distinct IDs")

			// BalanceAffected flag
			assert.Equal(t, !tt.isAnnotation, op1.BalanceAffected, "op1 BalanceAffected")
			assert.Equal(t, !tt.isAnnotation, op2.BalanceAffected, "op2 BalanceAffected")

			if tt.checkVersionChain && !tt.isAnnotation {
				// Version chaining: op1 starts at original, ends at original+1
				// op2 starts at original+1, ends at original+2
				originalVersion := tt.balance.Version

				assert.Equal(t, originalVersion, *op1.Balance.Version,
					"op1 balance before should have original version")
				assert.Equal(t, originalVersion+1, *op1.BalanceAfter.Version,
					"op1 balance after should be original+1")
				assert.Equal(t, originalVersion+1, *op2.Balance.Version,
					"op2 balance before should chain from op1 (original+1)")
				assert.Equal(t, originalVersion+2, *op2.BalanceAfter.Version,
					"op2 balance after should be original+2")
			}

			if tt.checkBalanceFields && !tt.isAnnotation {
				// Op1 (DEBIT): only Available changes, OnHold unchanged
				expectedDebitAvailable := tt.balance.Available.Sub(tt.amount.Value)
				assert.True(t, expectedDebitAvailable.Equal(*op1.BalanceAfter.Available),
					"op1 should decrease Available by amount: want %s got %s",
					expectedDebitAvailable.String(), op1.BalanceAfter.Available.String())
				assert.True(t, tt.balance.OnHold.Equal(*op1.BalanceAfter.OnHold),
					"op1 should not change OnHold: want %s got %s",
					tt.balance.OnHold.String(), op1.BalanceAfter.OnHold.String())

				// Op2 (ONHOLD): OnHold increases, Available stays at op1's result
				expectedOnHoldValue := tt.balance.OnHold.Add(tt.amount.Value)
				assert.True(t, expectedDebitAvailable.Equal(*op2.Balance.Available),
					"op2 balance before Available should match op1 after Available")
				assert.True(t, expectedOnHoldValue.Equal(*op2.BalanceAfter.OnHold),
					"op2 should increase OnHold by amount: want %s got %s",
					expectedOnHoldValue.String(), op2.BalanceAfter.OnHold.String())
			}

			if tt.isAnnotation {
				// All balance fields should be zeroed
				zero := decimal.NewFromInt(0)
				zeroVersion := int64(0)

				assert.True(t, zero.Equal(*op1.Balance.Available), "annotation op1 balance Available should be zero")
				assert.True(t, zero.Equal(*op1.Balance.OnHold), "annotation op1 balance OnHold should be zero")
				assert.Equal(t, zeroVersion, *op1.Balance.Version, "annotation op1 balance Version should be zero")
				assert.True(t, zero.Equal(*op1.BalanceAfter.Available), "annotation op1 balanceAfter Available should be zero")
				assert.True(t, zero.Equal(*op1.BalanceAfter.OnHold), "annotation op1 balanceAfter OnHold should be zero")
				assert.Equal(t, zeroVersion, *op1.BalanceAfter.Version, "annotation op1 balanceAfter Version should be zero")

				assert.True(t, zero.Equal(*op2.Balance.Available), "annotation op2 balance Available should be zero")
				assert.True(t, zero.Equal(*op2.Balance.OnHold), "annotation op2 balance OnHold should be zero")
				assert.Equal(t, zeroVersion, *op2.Balance.Version, "annotation op2 balance Version should be zero")
				assert.True(t, zero.Equal(*op2.BalanceAfter.Available), "annotation op2 balanceAfter Available should be zero")
				assert.True(t, zero.Equal(*op2.BalanceAfter.OnHold), "annotation op2 balanceAfter OnHold should be zero")
				assert.Equal(t, zeroVersion, *op2.BalanceAfter.Version, "annotation op2 balanceAfter Version should be zero")
			}

			// Description fallback
			if tt.fromTo.Description != "" {
				assert.Equal(t, tt.fromTo.Description, op1.Description, "should use fromTo description")
				assert.Equal(t, tt.fromTo.Description, op2.Description, "should use fromTo description")
			} else {
				assert.Equal(t, tt.transactionInput.Description, op1.Description, "should fall back to transaction description")
				assert.Equal(t, tt.transactionInput.Description, op2.Description, "should fall back to transaction description")
			}
		})
	}
}

func TestPropagateRouteValidation_Canceled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		isPending         bool
		transactionStatus string
		from              map[string]pkgTransaction.Amount
		to                map[string]pkgTransaction.Amount
		expectedFromFlags map[string]bool
		expectedToFlags   map[string]bool
	}{
		{
			name:              "canceled transaction sets RouteValidationEnabled on all From entries",
			isPending:         false,
			transactionStatus: cn.CANCELED,
			from: map[string]pkgTransaction.Amount{
				"@source1": {
					Value:     decimal.NewFromInt(500),
					Operation: libConstants.RELEASE,
				},
			},
			to: map[string]pkgTransaction.Amount{
				"@dest1": {
					Value:     decimal.NewFromInt(500),
					Operation: libConstants.CREDIT,
				},
			},
			expectedFromFlags: map[string]bool{
				"@source1": true,
			},
			expectedToFlags: map[string]bool{
				"@dest1": false,
			},
		},
		{
			name:              "canceled transaction with multiple From entries sets flag on all",
			isPending:         false,
			transactionStatus: cn.CANCELED,
			from: map[string]pkgTransaction.Amount{
				"@source1": {
					Value:     decimal.NewFromInt(300),
					Operation: libConstants.RELEASE,
				},
				"@source2": {
					Value:     decimal.NewFromInt(200),
					Operation: libConstants.RELEASE,
				},
				"@source3": {
					Value:     decimal.NewFromInt(500),
					Operation: libConstants.RELEASE,
				},
			},
			to: map[string]pkgTransaction.Amount{},
			expectedFromFlags: map[string]bool{
				"@source1": true,
				"@source2": true,
				"@source3": true,
			},
			expectedToFlags: map[string]bool{},
		},
		{
			name:              "APPROVED transaction does NOT set RouteValidationEnabled",
			isPending:         false,
			transactionStatus: cn.APPROVED,
			from: map[string]pkgTransaction.Amount{
				"@source1": {
					Value:     decimal.NewFromInt(100),
					Operation: libConstants.DEBIT,
				},
			},
			to: map[string]pkgTransaction.Amount{
				"@dest1": {
					Value:     decimal.NewFromInt(100),
					Operation: libConstants.CREDIT,
				},
			},
			expectedFromFlags: map[string]bool{
				"@source1": false,
			},
			expectedToFlags: map[string]bool{
				"@dest1": false,
			},
		},
		{
			name:              "CREATED transaction does NOT set RouteValidationEnabled",
			isPending:         false,
			transactionStatus: cn.CREATED,
			from: map[string]pkgTransaction.Amount{
				"@source1": {
					Value:     decimal.NewFromInt(100),
					Operation: libConstants.DEBIT,
				},
			},
			to:                map[string]pkgTransaction.Amount{},
			expectedFromFlags: map[string]bool{"@source1": false},
			expectedToFlags:   map[string]bool{},
		},
		{
			name:              "canceled transaction with empty From map is a no-op",
			isPending:         false,
			transactionStatus: cn.CANCELED,
			from:              map[string]pkgTransaction.Amount{},
			to:                map[string]pkgTransaction.Amount{},
			expectedFromFlags: map[string]bool{},
			expectedToFlags:   map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			validate := &pkgTransaction.Responses{
				From: tt.from,
				To:   tt.to,
			}

			propagateRouteValidation(ctx, validate, tt.isPending, tt.transactionStatus)

			for key, expectedFlag := range tt.expectedFromFlags {
				amt, exists := validate.From[key]
				assert.True(t, exists, "From map should contain key %s", key)
				assert.Equal(t, expectedFlag, amt.RouteValidationEnabled,
					"From[%s].RouteValidationEnabled should be %v", key, expectedFlag)
			}

			for key, expectedFlag := range tt.expectedToFlags {
				amt, exists := validate.To[key]
				assert.True(t, exists, "To map should contain key %s", key)
				assert.Equal(t, expectedFlag, amt.RouteValidationEnabled,
					"To[%s].RouteValidationEnabled should not be modified", key)
			}
		})
	}
}

func TestBuildDoubleEntryCanceledOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		balance            *mmodel.Balance
		fromTo             pkgTransaction.FromTo
		amount             pkgTransaction.Amount
		balanceAfter       pkgTransaction.Balance
		tran               transaction.Transaction
		transactionInput   pkgTransaction.Transaction
		isAnnotation       bool
		expectedOpCount    int
		expectedOp1Type    string
		expectedOp2Type    string
		checkVersionChain  bool
		checkBalanceFields bool
	}{
		{
			name: "generates exactly 2 operations RELEASE+CREDIT with correct types",
			balance: &mmodel.Balance{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
				AccountID:      uuid.New().String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(500),
				OnHold:         decimal.NewFromInt(300),
				Version:        7,
			},
			fromTo: pkgTransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
				Description:  "canceled operation",
			},
			amount: pkgTransaction.Amount{
				Value:                  decimal.NewFromInt(300),
				Operation:              libConstants.RELEASE,
				TransactionType:        cn.CANCELED,
				RouteValidationEnabled: true,
			},
			balanceAfter: pkgTransaction.Balance{
				Available: decimal.NewFromInt(800),
				OnHold:    decimal.NewFromInt(0),
				Version:   10,
			},
			tran: transaction.Transaction{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
			},
			transactionInput: pkgTransaction.Transaction{
				Pending: false,
				Send:    pkgTransaction.Send{Asset: "BRL"},
			},
			isAnnotation:       false,
			expectedOpCount:    2,
			expectedOp1Type:    cn.RELEASE,
			expectedOp2Type:    cn.CREDIT,
			checkVersionChain:  true,
			checkBalanceFields: true,
		},
		{
			name: "annotation mode zeroes all balance fields",
			balance: &mmodel.Balance{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
				AccountID:      uuid.New().String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(500),
				OnHold:         decimal.NewFromInt(300),
				Version:        7,
			},
			fromTo: pkgTransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amount: pkgTransaction.Amount{
				Value:                  decimal.NewFromInt(300),
				Operation:              libConstants.RELEASE,
				TransactionType:        cn.CANCELED,
				RouteValidationEnabled: true,
			},
			balanceAfter: pkgTransaction.Balance{
				Available: decimal.NewFromInt(800),
				OnHold:    decimal.NewFromInt(0),
				Version:   10,
			},
			tran: transaction.Transaction{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
			},
			transactionInput: pkgTransaction.Transaction{
				Pending:     false,
				Description: "annotation canceled",
				Send:        pkgTransaction.Send{Asset: "BRL"},
			},
			isAnnotation:    true,
			expectedOpCount: 2,
			expectedOp1Type: cn.RELEASE,
			expectedOp2Type: cn.CREDIT,
		},
		{
			name: "uses transaction description when fromTo description is empty",
			balance: &mmodel.Balance{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
				AccountID:      uuid.New().String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(200),
				Version:        1,
			},
			fromTo: pkgTransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
				Description:  "",
			},
			amount: pkgTransaction.Amount{
				Value:                  decimal.NewFromInt(200),
				Operation:              libConstants.RELEASE,
				TransactionType:        cn.CANCELED,
				RouteValidationEnabled: true,
			},
			balanceAfter: pkgTransaction.Balance{
				Available: decimal.NewFromInt(1200),
				OnHold:    decimal.NewFromInt(0),
				Version:   4,
			},
			tran: transaction.Transaction{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
			},
			transactionInput: pkgTransaction.Transaction{
				Pending:     false,
				Description: "fallback canceled description",
				Send:        pkgTransaction.Send{Asset: "USD"},
			},
			isAnnotation:    false,
			expectedOpCount: 2,
			expectedOp1Type: cn.RELEASE,
			expectedOp2Type: cn.CREDIT,
		},
		{
			name: "zero amount produces 2 operations with unchanged balances",
			balance: &mmodel.Balance{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
				AccountID:      uuid.New().String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(500),
				Version:        5,
			},
			fromTo: pkgTransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
				Description:  "zero amount test",
			},
			amount: pkgTransaction.Amount{
				Value:                  decimal.NewFromInt(0),
				Operation:              libConstants.RELEASE,
				TransactionType:        cn.CANCELED,
				RouteValidationEnabled: true,
			},
			balanceAfter: pkgTransaction.Balance{
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.NewFromInt(500),
				Version:   7,
			},
			tran: transaction.Transaction{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
			},
			transactionInput: pkgTransaction.Transaction{
				Pending: false,
				Send:    pkgTransaction.Send{Asset: "BRL"},
			},
			isAnnotation:       false,
			expectedOpCount:    2,
			expectedOp1Type:    cn.RELEASE,
			expectedOp2Type:    cn.CREDIT,
			checkVersionChain:  true,
			checkBalanceFields: true,
		},
		{
			name: "version starting at 0 chains correctly",
			balance: &mmodel.Balance{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
				AccountID:      uuid.New().String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(100),
				OnHold:         decimal.NewFromInt(100),
				Version:        0,
			},
			fromTo: pkgTransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
				Description:  "version zero test",
			},
			amount: pkgTransaction.Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              libConstants.RELEASE,
				TransactionType:        cn.CANCELED,
				RouteValidationEnabled: true,
			},
			balanceAfter: pkgTransaction.Balance{
				Available: decimal.NewFromInt(200),
				OnHold:    decimal.NewFromInt(0),
				Version:   2,
			},
			tran: transaction.Transaction{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
			},
			transactionInput: pkgTransaction.Transaction{
				Pending: false,
				Send:    pkgTransaction.Send{Asset: "BRL"},
			},
			isAnnotation:       false,
			expectedOpCount:    2,
			expectedOp1Type:    cn.RELEASE,
			expectedOp2Type:    cn.CREDIT,
			checkVersionChain:  true,
			checkBalanceFields: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			handler := &TransactionHandler{}
			transactionDate := time.Now()

			ops := handler.buildDoubleEntryCanceledOps(
				ctx,
				tt.balance,
				tt.fromTo,
				tt.amount,
				tt.balanceAfter,
				tt.tran,
				tt.transactionInput,
				transactionDate,
				tt.isAnnotation,
			)

			require.Len(t, ops, tt.expectedOpCount, "should generate exactly %d operations", tt.expectedOpCount)

			op1 := ops[0]
			op2 := ops[1]

			// Verify operation types
			assert.Equal(t, tt.expectedOp1Type, op1.Type, "op1 should be RELEASE")
			assert.Equal(t, tt.expectedOp2Type, op2.Type, "op2 should be CREDIT")

			// Both ops share the same transaction and balance IDs
			assert.Equal(t, tt.tran.ID, op1.TransactionID)
			assert.Equal(t, tt.tran.ID, op2.TransactionID)
			assert.Equal(t, tt.balance.ID, op1.BalanceID)
			assert.Equal(t, tt.balance.ID, op2.BalanceID)

			// Both ops have same amount value
			assert.True(t, tt.amount.Value.Equal(*op1.Amount.Value), "op1 amount should match input")
			assert.True(t, tt.amount.Value.Equal(*op2.Amount.Value), "op2 amount should match input")

			// Op IDs are different (each is a distinct UUIDv7)
			assert.NotEqual(t, op1.ID, op2.ID, "op1 and op2 should have distinct IDs")

			// BalanceAffected flag
			assert.Equal(t, !tt.isAnnotation, op1.BalanceAffected, "op1 BalanceAffected")
			assert.Equal(t, !tt.isAnnotation, op2.BalanceAffected, "op2 BalanceAffected")

			if tt.checkVersionChain && !tt.isAnnotation {
				// Version chaining: op1 starts at original, ends at original+1
				// op2 starts at original+1 (release version), ends at original+2
				originalVersion := tt.balance.Version

				assert.Equal(t, originalVersion, *op1.Balance.Version,
					"op1 balance before should have original version")
				assert.Equal(t, originalVersion+1, *op1.BalanceAfter.Version,
					"op1 balance after should be original+1")
				assert.Equal(t, originalVersion+1, *op2.Balance.Version,
					"op2 balance before should chain from op1 (original+1)")
				assert.Equal(t, originalVersion+2, *op2.BalanceAfter.Version,
					"op2 balance after should be original+2")
			}

			if tt.checkBalanceFields && !tt.isAnnotation {
				// Op1 (RELEASE): only OnHold changes, Available unchanged
				expectedReleaseOnHold := tt.balance.OnHold.Sub(tt.amount.Value)
				assert.True(t, tt.balance.Available.Equal(*op1.BalanceAfter.Available),
					"op1 should NOT change Available: want %s got %s",
					tt.balance.Available.String(), op1.BalanceAfter.Available.String())
				assert.True(t, expectedReleaseOnHold.Equal(*op1.BalanceAfter.OnHold),
					"op1 should decrease OnHold by amount: want %s got %s",
					expectedReleaseOnHold.String(), op1.BalanceAfter.OnHold.String())

				// Op2 (CREDIT): Available increases, OnHold stays at op1's result
				expectedCreditAvailable := tt.balance.Available.Add(tt.amount.Value)
				assert.True(t, expectedReleaseOnHold.Equal(*op2.BalanceAfter.OnHold),
					"op2 OnHold should remain at op1 result: want %s got %s",
					expectedReleaseOnHold.String(), op2.BalanceAfter.OnHold.String())
				assert.True(t, expectedCreditAvailable.Equal(*op2.BalanceAfter.Available),
					"op2 should increase Available by amount: want %s got %s",
					expectedCreditAvailable.String(), op2.BalanceAfter.Available.String())
			}

			if tt.isAnnotation {
				// All balance fields should be zeroed
				zero := decimal.NewFromInt(0)
				zeroVersion := int64(0)

				assert.True(t, zero.Equal(*op1.Balance.Available), "annotation op1 balance Available should be zero")
				assert.True(t, zero.Equal(*op1.Balance.OnHold), "annotation op1 balance OnHold should be zero")
				assert.Equal(t, zeroVersion, *op1.Balance.Version, "annotation op1 balance Version should be zero")
				assert.True(t, zero.Equal(*op1.BalanceAfter.Available), "annotation op1 balanceAfter Available should be zero")
				assert.True(t, zero.Equal(*op1.BalanceAfter.OnHold), "annotation op1 balanceAfter OnHold should be zero")
				assert.Equal(t, zeroVersion, *op1.BalanceAfter.Version, "annotation op1 balanceAfter Version should be zero")

				assert.True(t, zero.Equal(*op2.Balance.Available), "annotation op2 balance Available should be zero")
				assert.True(t, zero.Equal(*op2.Balance.OnHold), "annotation op2 balance OnHold should be zero")
				assert.Equal(t, zeroVersion, *op2.Balance.Version, "annotation op2 balance Version should be zero")
				assert.True(t, zero.Equal(*op2.BalanceAfter.Available), "annotation op2 balanceAfter Available should be zero")
				assert.True(t, zero.Equal(*op2.BalanceAfter.OnHold), "annotation op2 balanceAfter OnHold should be zero")
				assert.Equal(t, zeroVersion, *op2.BalanceAfter.Version, "annotation op2 balanceAfter Version should be zero")
			}

			// Description fallback
			if tt.fromTo.Description != "" {
				assert.Equal(t, tt.fromTo.Description, op1.Description, "should use fromTo description")
				assert.Equal(t, tt.fromTo.Description, op2.Description, "should use fromTo description")
			} else {
				assert.Equal(t, tt.transactionInput.Description, op1.Description, "should fall back to transaction description")
				assert.Equal(t, tt.transactionInput.Description, op2.Description, "should fall back to transaction description")
			}
		})
	}
}

func TestTryBuildDoubleEntryOps(t *testing.T) {
	t.Parallel()

	baseBalance := &mmodel.Balance{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
		LedgerID:       uuid.New().String(),
		AccountID:      uuid.New().String(),
		Alias:          "@source1",
		Key:            "default",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.NewFromInt(200),
		Version:        5,
	}

	baseTran := transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
		LedgerID:       uuid.New().String(),
	}

	baseBalanceAfter := pkgTransaction.Balance{
		Available: decimal.NewFromInt(800),
		OnHold:    decimal.NewFromInt(400),
		Version:   7,
	}

	tests := []struct {
		name                   string
		ft                     pkgTransaction.FromTo
		amt                    pkgTransaction.Amount
		transactionInput       pkgTransaction.Transaction
		routeValidationEnabled bool
		processedDoubleEntry   map[string]bool
		expectedOps            int
		expectedHandled        bool
	}{
		{
			name: "returns (nil, false) when routeValidationEnabled is false",
			ft: pkgTransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amt: pkgTransaction.Amount{
				Value:           decimal.NewFromInt(100),
				Operation:       libConstants.ONHOLD,
				TransactionType: cn.PENDING,
			},
			transactionInput: pkgTransaction.Transaction{
				Pending: true,
				Send:    pkgTransaction.Send{Asset: "USD"},
			},
			routeValidationEnabled: false,
			processedDoubleEntry:   make(map[string]bool),
			expectedOps:            0,
			expectedHandled:        false,
		},
		{
			name: "returns (nil, false) when IsFrom is false",
			ft: pkgTransaction.FromTo{
				AccountAlias: "@dest1",
				BalanceKey:   "default",
				IsFrom:       false,
			},
			amt: pkgTransaction.Amount{
				Value:           decimal.NewFromInt(100),
				Operation:       libConstants.ONHOLD,
				TransactionType: cn.PENDING,
			},
			transactionInput: pkgTransaction.Transaction{
				Pending: true,
				Send:    pkgTransaction.Send{Asset: "USD"},
			},
			routeValidationEnabled: true,
			processedDoubleEntry:   make(map[string]bool),
			expectedOps:            0,
			expectedHandled:        false,
		},
		{
			name: "returns (nil, true) for already-processed alias (deduplication)",
			ft: pkgTransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amt: pkgTransaction.Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              libConstants.ONHOLD,
				TransactionType:        cn.PENDING,
				RouteValidationEnabled: true,
			},
			transactionInput: pkgTransaction.Transaction{
				Pending: true,
				Send:    pkgTransaction.Send{Asset: "USD"},
			},
			routeValidationEnabled: true,
			processedDoubleEntry:   map[string]bool{"@source1": true},
			expectedOps:            0,
			expectedHandled:        true,
		},
		{
			name: "returns (nil, false) for non-double-entry operation (DEBIT+CREATED)",
			ft: pkgTransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amt: pkgTransaction.Amount{
				Value:           decimal.NewFromInt(100),
				Operation:       cn.DEBIT,
				TransactionType: cn.CREATED,
			},
			transactionInput: pkgTransaction.Transaction{
				Send: pkgTransaction.Send{Asset: "USD"},
			},
			routeValidationEnabled: true,
			processedDoubleEntry:   make(map[string]bool),
			expectedOps:            0,
			expectedHandled:        false,
		},
		{
			name: "dispatches to pending path for PENDING+ONHOLD",
			ft: pkgTransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amt: pkgTransaction.Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              libConstants.ONHOLD,
				TransactionType:        cn.PENDING,
				RouteValidationEnabled: true,
			},
			transactionInput: pkgTransaction.Transaction{
				Pending: true,
				Send:    pkgTransaction.Send{Asset: "USD"},
			},
			routeValidationEnabled: true,
			processedDoubleEntry:   make(map[string]bool),
			expectedOps:            2,
			expectedHandled:        true,
		},
		{
			name: "dispatches to canceled path for CANCELED+RELEASE",
			ft: pkgTransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amt: pkgTransaction.Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              cn.RELEASE,
				TransactionType:        cn.CANCELED,
				RouteValidationEnabled: true,
			},
			transactionInput: pkgTransaction.Transaction{
				Send: pkgTransaction.Send{Asset: "USD"},
			},
			routeValidationEnabled: true,
			processedDoubleEntry:   make(map[string]bool),
			expectedOps:            2,
			expectedHandled:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			handler := &TransactionHandler{}
			transactionDate := time.Now()

			ops, handled := handler.tryBuildDoubleEntryOps(
				ctx,
				baseBalance,
				tt.ft,
				tt.amt,
				baseBalanceAfter,
				baseTran,
				tt.transactionInput,
				transactionDate,
				false, // isAnnotation
				tt.routeValidationEnabled,
				tt.processedDoubleEntry,
			)

			assert.Equal(t, tt.expectedHandled, handled, "handled flag mismatch")

			if tt.expectedOps == 0 {
				assert.Nil(t, ops, "expected nil ops")
			} else {
				require.Len(t, ops, tt.expectedOps, "expected %d operations", tt.expectedOps)

				// Verify ops have distinct IDs
				assert.NotEqual(t, ops[0].ID, ops[1].ID, "operations should have distinct IDs")

				// Verify alias was marked as processed
				assert.True(t, tt.processedDoubleEntry[baseBalance.Alias],
					"alias should be marked as processed in the deduplication map")
			}
		})
	}
}
