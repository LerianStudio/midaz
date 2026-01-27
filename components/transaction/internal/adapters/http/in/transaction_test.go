package in

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				// Redis cache miss (empty string = not found)
				redisRepo.EXPECT().
					Get(gomock.Any(), gomock.Any()).
					Return("", nil).
					Times(1)
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
				// Redis cache miss (empty string = not found)
				redisRepo.EXPECT().
					Get(gomock.Any(), gomock.Any()).
					Return("", nil).
					Times(1)
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
				// Redis cache miss (empty string = not found)
				redisRepo.EXPECT().
					Get(gomock.Any(), gomock.Any()).
					Return("", nil).
					Times(1)
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
				// Redis cache miss (empty string = not found)
				redisRepo.EXPECT().
					Get(gomock.Any(), gomock.Any()).
					Return("", nil).
					Times(1)
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
				// Redis cache miss (empty string = not found)
				redisRepo.EXPECT().
					Get(gomock.Any(), gomock.Any()).
					Return("", nil).
					Times(1)
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

			queryUC := &query.UseCase{
				TransactionRepo: mockTransactionRepo,
				OperationRepo:   mockOperationRepo,
				MetadataRepo:    mockMetadataRepo,
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
		"expected error code 0097 (ErrTransactionCantRevert)")
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

	// Mock: Transaction lookup returns error
	// GetTransactionByID calls TransactionRepo.Find directly (no Redis cache)
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

	// Mock: Redis lock NOT acquired (returns false, nil) - transaction already being processed
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).
		Times(1)

	queryUC := &query.UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
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

			queryUC := &query.UseCase{
				TransactionRepo: mockTransactionRepo,
				MetadataRepo:    mockMetadataRepo,
				OperationRepo:   mockOperationRepo,
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
