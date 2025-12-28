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
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
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
