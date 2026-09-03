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
	"strings"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libConstants "github.com/LerianStudio/lib-commons/v6/commons/constants"
	libHTTP "github.com/LerianStudio/lib-commons/v6/commons/net/http"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	operationroute "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
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
		{
			name:        "invalid pagination returns 400 before repository calls",
			queryParams: "?page=abc",
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID) {
			},
			expectedStatus: 400,
			validateBody:   assertInvalidQueryParameterResponse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
			transactionID := uuid.Must(libCommons.GenerateUUIDv7())

			mockTransactionRepo := transaction.NewMockRepository(ctrl)
			mockOperationRepo := operation.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			tt.setupMocks(mockTransactionRepo, mockOperationRepo, mockMetadataRepo, mockRedisRepo, orgID, ledgerID, transactionID)

			uc := &query.UseCase{
				TransactionRepo:         mockTransactionRepo,
				OperationRepo:           mockOperationRepo,
				TransactionMetadataRepo: mockMetadataRepo,
				TransactionRedisRepo:    mockRedisRepo,
			}
			handler := &TransactionHandler{Query: uc}

			app := buildHumaTransactionApp(t, handler, true)

			// Act
			req := httptest.NewRequest("GET",
				"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+tt.queryParams,
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
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
			transactionID := uuid.Must(libCommons.GenerateUUIDv7())

			mockTransactionRepo := transaction.NewMockRepository(ctrl)
			mockOperationRepo := operation.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)

			amount := decimal.NewFromInt(1000)
			txBody := mtransaction.Transaction{
				Send: mtransaction.Send{
					Source: mtransaction.Source{
						From: []mtransaction.FromTo{
							{AccountAlias: "@acc1"},
						},
					},
					Distribute: mtransaction.Distribute{
						To: []mtransaction.FromTo{
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

			// Mock: fetch transaction with its operations (commit/cancel fallback)
			mockTransactionRepo.EXPECT().
				FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
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
				TransactionRepo:         mockTransactionRepo,
				OperationRepo:           mockOperationRepo,
				TransactionMetadataRepo: mockMetadataRepo,
				TransactionRedisRepo:    mockRedisRepo,
			}
			commandUC := &command.UseCase{
				TransactionRedisRepo: mockRedisRepo,
			}
			handler := &TransactionHandler{Query: queryUC, Command: commandUC}

			app := buildHumaTransactionApp(t, handler, true)

			// Act
			req := httptest.NewRequest("POST",
				"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/commit",
				nil)
			resp, err := app.Test(req)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, 409, resp.StatusCode, "expected HTTP 409 for non-PENDING status")

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
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
			transactionID := uuid.Must(libCommons.GenerateUUIDv7())

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
				TransactionRepo:         mockTransactionRepo,
				TransactionMetadataRepo: mockMetadataRepo,
			}
			handler := &TransactionHandler{Query: queryUC, Command: &command.UseCase{TransactionReader: queryUC}}

			app := buildHumaTransactionApp(t, handler, true)

			// Act
			req := httptest.NewRequest("POST",
				"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
				nil)
			resp, err := app.Test(req)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, 409, resp.StatusCode, "expected HTTP 409 for non-APPROVED status")

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
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())
	existingRevertID := uuid.Must(libCommons.GenerateUUIDv7())

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
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}
	handler := &TransactionHandler{Query: queryUC, Command: &command.UseCase{TransactionReader: queryUC}}

	app := buildHumaTransactionApp(t, handler, true)

	// Act
	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
		nil)
	resp, err := app.Test(req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 409, resp.StatusCode, "expected HTTP 409 for already reverted transaction")

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
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())
	originalTransactionID := uuid.Must(libCommons.GenerateUUIDv7())

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
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}
	handler := &TransactionHandler{Query: queryUC, Command: &command.UseCase{TransactionReader: queryUC}}

	app := buildHumaTransactionApp(t, handler, true)

	// Act
	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
		nil)
	resp, err := app.Test(req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 409, resp.StatusCode, "expected HTTP 409 for transaction that is already a revert")

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
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())

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
	handler := &TransactionHandler{Query: queryUC, Command: &command.UseCase{TransactionReader: queryUC}}

	app := buildHumaTransactionApp(t, handler, true)

	// Act
	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
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
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())

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
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}
	handler := &TransactionHandler{Query: queryUC, Command: &command.UseCase{TransactionReader: queryUC}}

	app := buildHumaTransactionApp(t, handler, true)

	// Act
	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
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
// returns an empty result (transaction can't be reverted), HTTP 422 is returned.
// TransactionRevert.IsEmpty() returns true when AssetCode is empty and Amount is zero.
func TestRevertTransaction_EmptyRevert_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())

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
		Body: mtransaction.Transaction{},
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
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}
	handler := &TransactionHandler{Query: queryUC, Command: &command.UseCase{TransactionReader: queryUC}}

	app := buildHumaTransactionApp(t, handler, true)

	// Act
	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
		nil)
	resp, err := app.Test(req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 422, resp.StatusCode, "expected HTTP 422 for empty revert")

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
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())
	operationRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)

	amount := decimal.NewFromInt(1000)
	routeIDStr := operationRouteID.String()
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
				RouteID:      &routeIDStr,
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
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		OperationRouteRepo:      mockOperationRouteRepo,
	}
	// The handler needs Command for createTransaction; since we only test
	// that the bidirectional check passes (not the full createTransaction flow),
	// we use a Fiber error handler to catch panics from nil Command and verify
	// the bidirectional error was not returned.
	handler := &TransactionHandler{Query: queryUC, Command: &command.UseCase{TransactionReader: queryUC}}

	app := buildHumaTransactionApp(t, handler, true)

	// Act
	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
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
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())
	operationRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)

	amount := decimal.NewFromInt(1000)
	routeIDStr := operationRouteID.String()
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
				RouteID:      &routeIDStr,
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
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		OperationRouteRepo:      mockOperationRouteRepo,
	}
	handler := &TransactionHandler{Query: queryUC, Command: &command.UseCase{TransactionReader: queryUC}}

	app := buildHumaTransactionApp(t, handler, true)

	// Act
	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
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
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())

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
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}
	handler := &TransactionHandler{Query: queryUC, Command: &command.UseCase{TransactionReader: queryUC}}

	app := buildHumaTransactionApp(t, handler, true)

	// Act
	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
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
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())
	operationRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)

	amount := decimal.NewFromInt(1000)
	routeIDStr := operationRouteID.String()
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
				RouteID:      &routeIDStr,
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
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		OperationRouteRepo:      mockOperationRouteRepo,
	}
	handler := &TransactionHandler{Query: queryUC, Command: &command.UseCase{TransactionReader: queryUC}}

	app := buildHumaTransactionApp(t, handler, true)

	// Act
	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/revert",
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
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Mock: Write-behind cache miss (fall through to Postgres Find)
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("cache miss")).
		AnyTimes()

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

	queryUC := &query.UseCase{
		TransactionRepo:      mockTransactionRepo,
		TransactionRedisRepo: mockRedisRepo,
	}
	handler := &TransactionHandler{Query: queryUC, Command: &command.UseCase{TransactionReader: queryUC}}

	app := buildHumaTransactionApp(t, handler, true)

	// Act
	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/commit",
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
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	amount := decimal.NewFromInt(1000)
	txBody := mtransaction.Transaction{
		Send: mtransaction.Send{
			Asset: "USD",
			Value: amount,
			Source: mtransaction.Source{
				From: []mtransaction.FromTo{{AccountAlias: "@acc1"}},
			},
			Distribute: mtransaction.Distribute{
				To: []mtransaction.FromTo{{AccountAlias: "@acc2"}},
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

	// Mock: commit/cancel fallback fetch (with operations)
	mockTransactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
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
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}
	commandUC := &command.UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}
	handler := &TransactionHandler{Query: queryUC, Command: commandUC}

	app := buildHumaTransactionApp(t, handler, true)

	// Act
	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/commit",
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
// lock cannot be acquired (already being processed), HTTP 409 is returned with the
// concurrency-specific error code (distinct from the status-conflict 0099).
func TestCommitTransaction_LockNotAcquired_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Arrange
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	amount := decimal.NewFromInt(1000)
	txBody := mtransaction.Transaction{
		Send: mtransaction.Send{
			Asset: "USD",
			Value: amount,
			Source: mtransaction.Source{
				From: []mtransaction.FromTo{{AccountAlias: "@acc1"}},
			},
			Distribute: mtransaction.Distribute{
				To: []mtransaction.FromTo{{AccountAlias: "@acc2"}},
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

	// Mock: commit/cancel fallback fetch (with operations)
	mockTransactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
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
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}
	commandUC := &command.UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}
	handler := &TransactionHandler{Query: queryUC, Command: commandUC}

	app := buildHumaTransactionApp(t, handler, true)

	// Act
	req := httptest.NewRequest("POST",
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/commit",
		nil)
	resp, err := app.Test(req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 409, resp.StatusCode, "expected HTTP 409 for locked transaction")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	err = json.Unmarshal(body, &errResp)
	require.NoError(t, err, "error response should be valid JSON")

	assert.Equal(t, cn.ErrPendingTransactionLocked.Error(), errResp["code"],
		"expected error code 0486 (ErrPendingTransactionLocked) for lock contention")
}

// TestCreateTransactionJSON_NonPositiveValue_Returns422 validates that creating a transaction
// with send.value <= 0 returns HTTP 422 with error code 0125.
// Business rule: Transaction values must be greater than zero.
func TestCreateTransactionJSON_NonPositiveValue_Returns422(t *testing.T) {
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
			// Arrange
			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

			// No mocks needed - validation short-circuits before any repository call
			handler := &TransactionHandler{}

			app := buildHumaTransactionApp(t, handler, true)

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
				"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/json",
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
			// Arrange
			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

			// No mocks needed - validation short-circuits before any repository call
			handler := &TransactionHandler{}

			app := buildHumaTransactionApp(t, handler, true)

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
				"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/inflow",
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
			// Arrange
			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

			// No mocks needed - validation short-circuits before any repository call
			handler := &TransactionHandler{}

			app := buildHumaTransactionApp(t, handler, true)

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
				"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/outflow",
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

// ptr is defined in observability_test.go as a generic helper.

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
				transactionID := uuid.Must(libCommons.GenerateUUIDv7())
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
				transactionID := uuid.Must(libCommons.GenerateUUIDv7())
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
				transactionID := uuid.Must(libCommons.GenerateUUIDv7())
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
			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

			mockTransactionRepo := transaction.NewMockRepository(ctrl)
			mockOperationRepo := operation.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockTransactionRepo, mockOperationRepo, mockMetadataRepo, orgID, ledgerID)

			uc := &query.UseCase{
				TransactionRepo:         mockTransactionRepo,
				OperationRepo:           mockOperationRepo,
				TransactionMetadataRepo: mockMetadataRepo,
			}
			handler := &TransactionHandler{Query: uc}

			app := buildHumaTransactionApp(t, handler, true)

			// Act
			req := httptest.NewRequest("GET",
				"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions"+tt.queryParams,
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
			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
			transactionID := uuid.Must(libCommons.GenerateUUIDv7())

			mockTransactionRepo := transaction.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			mockOperationRepo := operation.NewMockRepository(ctrl)
			tt.setupMocks(mockTransactionRepo, mockMetadataRepo, mockOperationRepo, orgID, ledgerID, transactionID)

			queryUC := &query.UseCase{
				TransactionRepo:         mockTransactionRepo,
				TransactionMetadataRepo: mockMetadataRepo,
				OperationRepo:           mockOperationRepo,
			}
			commandUC := &command.UseCase{
				TransactionRepo:         mockTransactionRepo,
				TransactionMetadataRepo: mockMetadataRepo,
			}
			handler := &TransactionHandler{Query: queryUC, Command: commandUC}

			app := buildHumaTransactionApp(t, handler, true)

			// Act
			req := httptest.NewRequest("PATCH",
				"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String(),
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
			// Arrange
			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

			// No mocks needed - validation short-circuits before any repository call
			handler := &TransactionHandler{}

			app := buildHumaTransactionApp(t, handler, true)

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
				"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/annotation",
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

// TestCancelTransaction tests the CancelTransaction handler
func TestCancelTransaction(t *testing.T) {
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
					FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
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
			name: "transaction not PENDING returns 409",
			setupMocks: func(transactionRepo *transaction.MockRepository, metadataRepo *mongodb.MockRepository, operationRepo *operation.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID) {
				amount := decimal.NewFromInt(1000)
				txBody := mtransaction.Transaction{
					Send: mtransaction.Send{
						Source: mtransaction.Source{
							From: []mtransaction.FromTo{
								{AccountAlias: "@acc1"},
							},
						},
						Distribute: mtransaction.Distribute{
							To: []mtransaction.FromTo{
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

				// commit/cancel fallback fetch (with operations)
				transactionRepo.EXPECT().
					FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
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
			expectedStatus: 409,
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
				txBody := mtransaction.Transaction{
					Send: mtransaction.Send{
						Source: mtransaction.Source{
							From: []mtransaction.FromTo{
								{AccountAlias: "@acc1"},
							},
						},
						Distribute: mtransaction.Distribute{
							To: []mtransaction.FromTo{
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

				// commit/cancel fallback fetch (with operations)
				transactionRepo.EXPECT().
					FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
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
			name: "lock already acquired by another process returns 409",
			setupMocks: func(transactionRepo *transaction.MockRepository, metadataRepo *mongodb.MockRepository, operationRepo *operation.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, transactionID uuid.UUID) {
				amount := decimal.NewFromInt(1000)
				txBody := mtransaction.Transaction{
					Send: mtransaction.Send{
						Source: mtransaction.Source{
							From: []mtransaction.FromTo{
								{AccountAlias: "@acc1"},
							},
						},
						Distribute: mtransaction.Distribute{
							To: []mtransaction.FromTo{
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

				// commit/cancel fallback fetch (with operations)
				transactionRepo.EXPECT().
					FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
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
			expectedStatus: 409,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Equal(t, cn.ErrPendingTransactionLocked.Error(), errResp["code"],
					"expected error code 0486 (lock contention) when transaction is locked")
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

				// commit/cancel fallback fetch (with operations) succeeds
				transactionRepo.EXPECT().
					FindWithOperations(gomock.Any(), orgID, ledgerID, transactionID).
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
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
			transactionID := uuid.Must(libCommons.GenerateUUIDv7())

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
				TransactionRepo:         mockTransactionRepo,
				TransactionMetadataRepo: mockMetadataRepo,
				OperationRepo:           mockOperationRepo,
				TransactionRedisRepo:    mockRedisRepo,
			}
			commandUC := &command.UseCase{
				TransactionRedisRepo: mockRedisRepo,
			}
			handler := &TransactionHandler{Query: queryUC, Command: commandUC}

			app := buildHumaTransactionApp(t, handler, true)

			// Act
			req := httptest.NewRequest("POST",
				"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/"+transactionID.String()+"/cancel",
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

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	tranID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	queryUC := &query.UseCase{TransactionRedisRepo: mockRedisRepo}
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

	app := buildHumaTransactionApp(t, handler, true)

	req := httptest.NewRequest("GET", humaTransactionURL(orgID, ledgerID, "/"+tranID.String()), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "true", resp.Header.Get("X-Cache-Hit"))
}

// TestCancelTransaction_WriteBehindMiss_PostgresMiss verifies that CancelTransaction returns error
// when both write-behind and Postgres fail.
func TestCancelTransaction_WriteBehindMiss_PostgresMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	tranID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	queryUC := &query.UseCase{
		TransactionRedisRepo: mockRedisRepo,
		TransactionRepo:      mockTransactionRepo,
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
		FindWithOperations(gomock.Any(), orgID, ledgerID, tranID).
		Return(nil, errors.New("record not found")).
		Times(1)

	app := buildHumaTransactionApp(t, handler, true)

	req := httptest.NewRequest("POST", humaTransactionURL(orgID, ledgerID, "/"+tranID.String()+"/cancel"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	assert.True(t, resp.StatusCode >= 400, "Expected error status code, got %d", resp.StatusCode)
}

// TestCancelTransaction_WriteBehindMiss_PostgresHit verifies fallback to Postgres when write-behind misses.
func TestCancelTransaction_WriteBehindMiss_PostgresHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	tranID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	queryUC := &query.UseCase{
		TransactionRedisRepo:    mockRedisRepo,
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}
	handler := &TransactionHandler{
		Command: &command.UseCase{TransactionRedisRepo: mockRedisRepo},
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
		FindWithOperations(gomock.Any(), orgID, ledgerID, tranID).
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

	app := buildHumaTransactionApp(t, handler, true)

	req := httptest.NewRequest("POST", humaTransactionURL(orgID, ledgerID, "/"+tranID.String()+"/cancel"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	// Response is an error (from SetNX), but the important thing is Find WAS called (fallback worked)
	assert.True(t, resp.StatusCode >= 400)
}

// TestCancelTransaction_WriteBehindHit_PostgresNotCalled verifies that when write-behind hits,
// Postgres is not queried.
func TestCancelTransaction_WriteBehindHit_PostgresNotCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	tranID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	queryUC := &query.UseCase{TransactionRedisRepo: mockRedisRepo}
	handler := &TransactionHandler{
		Command: &command.UseCase{TransactionRedisRepo: mockRedisRepo},
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

	app := buildHumaTransactionApp(t, handler, true)

	req := httptest.NewRequest("POST", humaTransactionURL(orgID, ledgerID, "/"+tranID.String()+"/cancel"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	// Error from SetNX short-circuit, but write-behind was used and Postgres was NOT called
	assert.True(t, resp.StatusCode >= 400)
}

// TestCommitTransaction_WriteBehindMiss_PostgresMiss verifies that CommitTransaction returns error
// when both write-behind and Postgres fail.
func TestCommitTransaction_WriteBehindMiss_PostgresMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	tranID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	queryUC := &query.UseCase{
		TransactionRedisRepo: mockRedisRepo,
		TransactionRepo:      mockTransactionRepo,
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
		FindWithOperations(gomock.Any(), orgID, ledgerID, tranID).
		Return(nil, errors.New("record not found")).
		Times(1)

	app := buildHumaTransactionApp(t, handler, true)

	req := httptest.NewRequest("POST", humaTransactionURL(orgID, ledgerID, "/"+tranID.String()+"/commit"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	assert.True(t, resp.StatusCode >= 400, "Expected error status code, got %d", resp.StatusCode)
}

// TestCommitTransaction_WriteBehindMiss_PostgresHit verifies fallback to Postgres when write-behind misses.
func TestCommitTransaction_WriteBehindMiss_PostgresHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	tranID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	queryUC := &query.UseCase{
		TransactionRedisRepo:    mockRedisRepo,
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}
	handler := &TransactionHandler{
		Command: &command.UseCase{TransactionRedisRepo: mockRedisRepo},
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
		FindWithOperations(gomock.Any(), orgID, ledgerID, tranID).
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

	app := buildHumaTransactionApp(t, handler, true)

	req := httptest.NewRequest("POST", humaTransactionURL(orgID, ledgerID, "/"+tranID.String()+"/commit"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	// Error from SetNX short-circuit, but Find WAS called (fallback worked)
	assert.True(t, resp.StatusCode >= 400)
}

// TestCommitTransaction_WriteBehindHit_PostgresNotCalled verifies that when write-behind hits,
// Postgres is not queried.
func TestCommitTransaction_WriteBehindHit_PostgresNotCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	tranID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	queryUC := &query.UseCase{TransactionRedisRepo: mockRedisRepo}
	handler := &TransactionHandler{
		Command: &command.UseCase{TransactionRedisRepo: mockRedisRepo},
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

	app := buildHumaTransactionApp(t, handler, true)

	req := httptest.NewRequest("POST", humaTransactionURL(orgID, ledgerID, "/"+tranID.String()+"/commit"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	// Error from SetNX short-circuit, but write-behind was used and Postgres was NOT called
	assert.True(t, resp.StatusCode >= 400)
}

func TestPropagateRouteValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		isPending         bool
		from              map[string]mtransaction.Amount
		to                map[string]mtransaction.Amount
		expectedFromFlags map[string]bool
		expectedToFlags   map[string]bool
	}{
		{
			name:      "pending transaction sets RouteValidationEnabled on all From entries",
			isPending: true,
			from: map[string]mtransaction.Amount{
				"@source1": {
					Value:     decimal.NewFromInt(500),
					Operation: libConstants.ONHOLD,
				},
				"@source2": {
					Value:     decimal.NewFromInt(500),
					Operation: libConstants.ONHOLD,
				},
			},
			to: map[string]mtransaction.Amount{
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
			from: map[string]mtransaction.Amount{
				"@source1": {
					Value:     decimal.NewFromInt(1000),
					Operation: libConstants.DEBIT,
				},
			},
			to: map[string]mtransaction.Amount{
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
			from:              map[string]mtransaction.Amount{},
			to:                map[string]mtransaction.Amount{},
			expectedFromFlags: map[string]bool{},
			expectedToFlags:   map[string]bool{},
		},
		{
			name:      "pending transaction with single From entry",
			isPending: true,
			from: map[string]mtransaction.Amount{
				"@source1": {
					Value:                  decimal.NewFromInt(100),
					Operation:              libConstants.ONHOLD,
					RouteValidationEnabled: false,
				},
			},
			to: map[string]mtransaction.Amount{},
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

			validate := &mtransaction.Responses{
				From: tt.from,
				To:   tt.to,
			}

			transactionStatus := cn.CREATED
			if tt.isPending {
				transactionStatus = cn.PENDING
			}

			mtransaction.PropagateRouteValidation(ctx, validate, transactionStatus)

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

func TestPropagateRouteValidation_Canceled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		isPending         bool
		transactionStatus string
		from              map[string]mtransaction.Amount
		to                map[string]mtransaction.Amount
		expectedFromFlags map[string]bool
		expectedToFlags   map[string]bool
	}{
		{
			name:              "canceled transaction sets RouteValidationEnabled on all From entries",
			isPending:         false,
			transactionStatus: cn.CANCELED,
			from: map[string]mtransaction.Amount{
				"@source1": {
					Value:     decimal.NewFromInt(500),
					Operation: libConstants.RELEASE,
				},
			},
			to: map[string]mtransaction.Amount{
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
			from: map[string]mtransaction.Amount{
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
			to: map[string]mtransaction.Amount{},
			expectedFromFlags: map[string]bool{
				"@source1": true,
				"@source2": true,
				"@source3": true,
			},
			expectedToFlags: map[string]bool{},
		},
		{
			name:              "APPROVED transaction sets RouteValidationEnabled on From entries",
			isPending:         false,
			transactionStatus: cn.APPROVED,
			from: map[string]mtransaction.Amount{
				"@source1": {
					Value:     decimal.NewFromInt(100),
					Operation: libConstants.DEBIT,
				},
			},
			to: map[string]mtransaction.Amount{
				"@dest1": {
					Value:     decimal.NewFromInt(100),
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
			name:              "CREATED transaction does NOT set RouteValidationEnabled",
			isPending:         false,
			transactionStatus: cn.CREATED,
			from: map[string]mtransaction.Amount{
				"@source1": {
					Value:     decimal.NewFromInt(100),
					Operation: libConstants.DEBIT,
				},
			},
			to:                map[string]mtransaction.Amount{},
			expectedFromFlags: map[string]bool{"@source1": false},
			expectedToFlags:   map[string]bool{},
		},
		{
			name:              "canceled transaction with empty From map is a no-op",
			isPending:         false,
			transactionStatus: cn.CANCELED,
			from:              map[string]mtransaction.Amount{},
			to:                map[string]mtransaction.Amount{},
			expectedFromFlags: map[string]bool{},
			expectedToFlags:   map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			validate := &mtransaction.Responses{
				From: tt.from,
				To:   tt.to,
			}

			mtransaction.PropagateRouteValidation(ctx, validate, tt.transactionStatus)

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
