package in

import (
	"encoding/json"
	"io"
	"net/http/httptest"
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
		setupMocks     func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, transactionID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "success returns 200 with transaction",
			queryParams: "",
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, transactionID uuid.UUID) {
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
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, transactionID uuid.UUID) {
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
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, transactionID uuid.UUID) {
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
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, transactionID uuid.UUID) {
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
			setupMocks: func(transactionRepo *transaction.MockRepository, operationRepo *operation.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, transactionID uuid.UUID) {
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
			tt.setupMocks(mockTransactionRepo, mockOperationRepo, mockMetadataRepo, orgID, ledgerID, transactionID)

			uc := &query.UseCase{
				TransactionRepo: mockTransactionRepo,
				OperationRepo:   mockOperationRepo,
				MetadataRepo:    mockMetadataRepo,
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

// ptr is a helper function to create a pointer to a string.
func ptr(s string) *string {
	return &s
}
