package in

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	testutils "github.com/LerianStudio/midaz/v3/tests/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestAccountHandler_CreateAccount(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.CreateAccountInput
		setupMocks     func(accountRepo *account.MockRepository, assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, balancePort *mbootstrap.MockBalancePort, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 201 with created account",
			payload: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				AssetCode: "USD",
				Type:      "deposit",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
			},
			setupMocks: func(accountRepo *account.MockRepository, assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, balancePort *mbootstrap.MockBalancePort, orgID, ledgerID uuid.UUID) {
				// CheckHealth is called first to verify balance service availability
				balancePort.EXPECT().
					CheckHealth(gomock.Any()).
					Return(nil).
					Times(1)

				// FindByNameOrCode to check if asset exists
				assetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), orgID, ledgerID, "", "USD").
					Return(true, nil).
					Times(1)

				// Create account
				accountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx any, acc *mmodel.Account) (*mmodel.Account, error) {
						acc.ID = uuid.New().String()
						acc.OrganizationID = orgID.String()
						acc.LedgerID = ledgerID.String()
						acc.CreatedAt = time.Now()
						acc.UpdatedAt = time.Now()
						return acc, nil
					}).
					Times(1)

				// CreateBalanceSync for the account
				balancePort.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(&mmodel.Balance{}, nil).
					Times(1)

				// No metadata in request, so MetadataRepo.Create won't be called
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "name", "response should contain name")
				assert.Equal(t, "Test Account", result["name"])
				assert.Equal(t, "USD", result["assetCode"])
				assert.Equal(t, "deposit", result["type"])
			},
		},
		{
			name: "asset not found returns 404",
			payload: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				AssetCode: "UNKNOWN",
				Type:      "deposit",
			},
			setupMocks: func(accountRepo *account.MockRepository, assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, balancePort *mbootstrap.MockBalancePort, orgID, ledgerID uuid.UUID) {
				// CheckHealth is called first to verify balance service availability
				balancePort.EXPECT().
					CheckHealth(gomock.Any()).
					Return(nil).
					Times(1)

				// FindByNameOrCode returns false - asset not found
				assetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), orgID, ledgerID, "", "UNKNOWN").
					Return(false, nil).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAssetCodeNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			payload: &mmodel.CreateAccountInput{
				Name:      "Test Account",
				AssetCode: "USD",
				Type:      "deposit",
			},
			setupMocks: func(accountRepo *account.MockRepository, assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, balancePort *mbootstrap.MockBalancePort, orgID, ledgerID uuid.UUID) {
				// CheckHealth is called first to verify balance service availability
				balancePort.EXPECT().
					CheckHealth(gomock.Any()).
					Return(nil).
					Times(1)

				// Asset exists
				assetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), orgID, ledgerID, "", "USD").
					Return(true, nil).
					Times(1)

				// Account create fails
				accountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
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

			mockAccountRepo := account.NewMockRepository(ctrl)
			mockAssetRepo := asset.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			mockBalancePort := mbootstrap.NewMockBalancePort(ctrl)
			tt.setupMocks(mockAccountRepo, mockAssetRepo, mockMetadataRepo, mockBalancePort, orgID, ledgerID)

			cmdUC := &command.UseCase{
				AccountRepo:  mockAccountRepo,
				AssetRepo:    mockAssetRepo,
				MetadataRepo: mockMetadataRepo,
				BalancePort:  mockBalancePort,
			}
			handler := &AccountHandler{Command: cmdUC}

			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				func(c *fiber.Ctx) error {
					return handler.CreateAccount(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("POST", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts", nil)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")
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

func TestAccountHandler_GetAllAccounts(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with pagination structure",
			queryParams: "",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Nil(), gomock.Any()).
					Return([]*mmodel.Account{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				// Validate page-based pagination structure exists
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(10), limit)

				page, ok := result["page"].(float64)
				require.True(t, ok, "page should be a number")
				assert.Equal(t, float64(1), page)
			},
		},
		{
			name:        "success with items returns accounts",
			queryParams: "?limit=5&page=1",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				account1ID := uuid.New().String()
				account2ID := uuid.New().String()

				accountRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Nil(), gomock.Any()).
					Return([]*mmodel.Account{
						{
							ID:             account1ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Account One",
							AssetCode:      "USD",
							Type:           "deposit",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             account2ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Account Two",
							AssetCode:      "EUR",
							Type:           "savings",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
					}, nil).
					Times(1)

				// GetAllAccounts fetches metadata for all returned accounts
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Account", gomock.Any()).
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
				assert.Len(t, items, 2, "should have two accounts")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "account should have id field")
				assert.Contains(t, firstItem, "name", "account should have name field")
				assert.Contains(t, firstItem, "assetCode", "account should have assetCode field")
				assert.Contains(t, firstItem, "type", "account should have type field")

				// Validate page-based pagination
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(5), limit)

				page, ok := result["page"].(float64)
				require.True(t, ok, "page should be a number")
				assert.Equal(t, float64(1), page)
			},
		},
		{
			name:        "metadata filter returns filtered accounts",
			queryParams: "?metadata.tier=premium",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				account1ID := uuid.New().String()
				account2ID := uuid.New().String()

				// MetadataRepo.FindList returns metadata matching the filter
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: account1ID, Data: map[string]any{"tier": "premium"}},
						{EntityID: account2ID, Data: map[string]any{"tier": "premium"}},
					}, nil).
					Times(1)

				// AccountRepo.ListByIDs returns the accounts
				accountRepo.EXPECT().
					ListByIDs(gomock.Any(), orgID, ledgerID, gomock.Nil(), gomock.Any()).
					Return([]*mmodel.Account{
						{
							ID:             account1ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Premium Account One",
							AssetCode:      "USD",
							Type:           "deposit",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             account2ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Premium Account Two",
							AssetCode:      "EUR",
							Type:           "savings",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
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
				assert.Len(t, items, 2, "should have two filtered accounts")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "account should have id field")
				assert.Contains(t, firstItem, "name", "account should have name field")
			},
		},
		{
			name:        "metadata filter with no matching metadata returns 404",
			queryParams: "?metadata.tier=nonexistent",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// MetadataRepo.FindList returns nil (no matching metadata)
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrNoAccountsFound.Error(), errResp["code"])
			},
		},
		{
			name:        "metadata filter with accounts not found returns 404",
			queryParams: "?metadata.tier=premium",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				account1ID := uuid.New().String()

				// MetadataRepo.FindList returns metadata
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: account1ID, Data: map[string]any{"tier": "premium"}},
					}, nil).
					Times(1)

				// AccountRepo.ListByIDs returns not found error
				accountRepo.EXPECT().
					ListByIDs(gomock.Any(), orgID, ledgerID, gomock.Nil(), gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())).
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
			name:        "repository error returns 500",
			queryParams: "",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Nil(), gomock.Any()).
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

			mockAccountRepo := account.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAccountRepo, mockMetadataRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				AccountRepo:  mockAccountRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &AccountHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.GetAllAccounts,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts"+tt.queryParams, nil)
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

func TestAccountHandler_GetAccountByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with account",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				accountRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
					Return(&mmodel.Account{
						ID:             accountID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Test Account",
						AssetCode:      "USD",
						Type:           "deposit",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetAccountByID fetches metadata when account is found
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Account", accountID.String()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "name", "response should contain name")
				assert.Equal(t, "Test Account", result["name"])
				assert.Equal(t, "USD", result["assetCode"])
				assert.Equal(t, "deposit", result["type"])
			},
		},
		{
			name: "not found returns 404",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				accountRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAccountIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				accountRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
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

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()
			accountID := uuid.New()

			mockAccountRepo := account.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAccountRepo, mockMetadataRepo, orgID, ledgerID, accountID)

			queryUC := &query.UseCase{
				AccountRepo:  mockAccountRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &AccountHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", accountID)
					return c.Next()
				},
				handler.GetAccountByID,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String(), nil)
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

func TestAccountHandler_GetAccountExternalByCode(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		setupMocks     func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with external account",
			code: "USD",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountID := uuid.New().String()
				// GetAccountExternalByCode queries by alias with @external/ prefix
				accountRepo.EXPECT().
					FindAlias(gomock.Any(), orgID, ledgerID, gomock.Nil(), "@external/USD").
					Return(&mmodel.Account{
						ID:             accountID,
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "External USD Account",
						AssetCode:      "USD",
						Type:           "external",
						Alias:          testutils.Ptr("@external/USD"),
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetAccountByAlias fetches metadata using the alias, not the account ID
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Account", "@external/USD").
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "alias", "response should contain alias")
				assert.Equal(t, "@external/USD", result["alias"])
				assert.Equal(t, "external", result["type"])
			},
		},
		{
			name: "not found returns 404",
			code: "UNKNOWN",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountRepo.EXPECT().
					FindAlias(gomock.Any(), orgID, ledgerID, gomock.Nil(), "@external/UNKNOWN").
					Return(nil, pkg.ValidateBusinessError(cn.ErrAccountAliasNotFound, reflect.TypeOf(mmodel.Account{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAccountAliasNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			code: "BRL",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountRepo.EXPECT().
					FindAlias(gomock.Any(), orgID, ledgerID, gomock.Nil(), "@external/BRL").
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

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()

			mockAccountRepo := account.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAccountRepo, mockMetadataRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				AccountRepo:  mockAccountRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &AccountHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.GetAccountExternalByCode,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/external/"+tt.code, nil)
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

func TestAccountHandler_GetAccountByAlias(t *testing.T) {
	tests := []struct {
		name           string
		alias          string
		setupMocks     func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:  "success returns 200 with account",
			alias: "@person1",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountID := uuid.New().String()
				accountRepo.EXPECT().
					FindAlias(gomock.Any(), orgID, ledgerID, gomock.Nil(), "@person1").
					Return(&mmodel.Account{
						ID:             accountID,
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Person 1 Account",
						AssetCode:      "USD",
						Type:           "deposit",
						Alias:          testutils.Ptr("@person1"),
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetAccountByAlias fetches metadata using the alias
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Account", "@person1").
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "alias", "response should contain alias")
				assert.Equal(t, "@person1", result["alias"])
			},
		},
		{
			name:  "not found returns 404",
			alias: "@unknown",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountRepo.EXPECT().
					FindAlias(gomock.Any(), orgID, ledgerID, gomock.Nil(), "@unknown").
					Return(nil, pkg.ValidateBusinessError(cn.ErrAccountAliasNotFound, reflect.TypeOf(mmodel.Account{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAccountAliasNotFound.Error(), errResp["code"])
			},
		},
		{
			name:  "repository error returns 500",
			alias: "@test",
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountRepo.EXPECT().
					FindAlias(gomock.Any(), orgID, ledgerID, gomock.Nil(), "@test").
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

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()

			mockAccountRepo := account.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAccountRepo, mockMetadataRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				AccountRepo:  mockAccountRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &AccountHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.GetAccountByAlias,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/alias/"+tt.alias, nil)
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

func TestAccountHandler_UpdateAccount(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.UpdateAccountInput
		setupMocks     func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with updated account",
			payload: &mmodel.UpdateAccountInput{
				Name: "Updated Account Name",
			},
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				// First find the account to check if it's external
				accountRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
					Return(&mmodel.Account{
						ID:             accountID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Original Account Name",
						AssetCode:      "USD",
						Type:           "deposit",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// Update succeeds
				accountRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID, gomock.Any()).
					Return(&mmodel.Account{
						ID:             accountID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Updated Account Name",
						AssetCode:      "USD",
						Type:           "deposit",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// UpdateMetadata is called
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Account", accountID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Retrieval after update (query use case GetAccountByID)
				accountRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
					Return(&mmodel.Account{
						ID:             accountID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Updated Account Name",
						AssetCode:      "USD",
						Type:           "deposit",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetAccountByID also fetches metadata
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Account", accountID.String()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "name", "response should contain name")
				assert.Equal(t, "Updated Account Name", result["name"])
			},
		},
		{
			name: "not found on update returns 404",
			payload: &mmodel.UpdateAccountInput{
				Name: "Updated Name",
			},
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				// First find returns not found
				accountRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAccountIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "not found on retrieval returns 404",
			payload: &mmodel.UpdateAccountInput{
				Name: "Updated Name",
			},
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				// First find succeeds
				accountRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
					Return(&mmodel.Account{
						ID:   accountID.String(),
						Type: "deposit",
					}, nil).
					Times(1)

				// Update succeeds
				accountRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID, gomock.Any()).
					Return(&mmodel.Account{ID: accountID.String()}, nil).
					Times(1)

				// UpdateMetadata succeeds
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Account", accountID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Retrieval fails
				accountRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())).
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
			payload: &mmodel.UpdateAccountInput{
				Name: "Updated Name",
			},
			setupMocks: func(accountRepo *account.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				// First find returns error
				accountRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
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

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()
			accountID := uuid.New()

			mockAccountRepo := account.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAccountRepo, mockMetadataRepo, orgID, ledgerID, accountID)

			cmdUC := &command.UseCase{
				AccountRepo:  mockAccountRepo,
				MetadataRepo: mockMetadataRepo,
			}
			queryUC := &query.UseCase{
				AccountRepo:  mockAccountRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &AccountHandler{
				Command: cmdUC,
				Query:   queryUC,
			}

			app := fiber.New()
			app.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", accountID)
					return c.Next()
				},
				func(c *fiber.Ctx) error {
					return handler.UpdateAccount(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("PATCH", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String(), nil)
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

func TestAccountHandler_DeleteAccountByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(accountRepo *account.MockRepository, balancePort *mbootstrap.MockBalancePort, orgID, ledgerID, accountID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 204 no content",
			setupMocks: func(accountRepo *account.MockRepository, balancePort *mbootstrap.MockBalancePort, orgID, ledgerID, accountID uuid.UUID) {
				// Find account first
				accountRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
					Return(&mmodel.Account{
						ID:             accountID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Test Account",
						AssetCode:      "USD",
						Type:           "deposit",
					}, nil).
					Times(1)

				// Delete all balances for the account
				balancePort.EXPECT().
					DeleteAllBalancesByAccountID(gomock.Any(), orgID, ledgerID, accountID, gomock.Any()).
					Return(nil).
					Times(1)

				// Delete account
				accountRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil, // 204 has no body
		},
		{
			name: "not found returns 404",
			setupMocks: func(accountRepo *account.MockRepository, balancePort *mbootstrap.MockBalancePort, orgID, ledgerID, accountID uuid.UUID) {
				accountRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAccountIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(accountRepo *account.MockRepository, balancePort *mbootstrap.MockBalancePort, orgID, ledgerID, accountID uuid.UUID) {
				accountRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
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

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()
			accountID := uuid.New()

			mockAccountRepo := account.NewMockRepository(ctrl)
			mockBalancePort := mbootstrap.NewMockBalancePort(ctrl)
			tt.setupMocks(mockAccountRepo, mockBalancePort, orgID, ledgerID, accountID)

			cmdUC := &command.UseCase{
				AccountRepo: mockAccountRepo,
				BalancePort: mockBalancePort,
			}
			handler := &AccountHandler{Command: cmdUC}

			app := fiber.New()
			app.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", accountID)
					return c.Next()
				},
				handler.DeleteAccountByID,
			)

			// Act
			req := httptest.NewRequest("DELETE", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String(), nil)
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

func TestAccountHandler_CountAccounts(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(accountRepo *account.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
	}{
		{
			name: "success returns 204 with X-Total-Count header",
			setupMocks: func(accountRepo *account.MockRepository, orgID, ledgerID uuid.UUID) {
				accountRepo.EXPECT().
					Count(gomock.Any(), orgID, ledgerID).
					Return(int64(42), nil).
					Times(1)
			},
			expectedStatus: 204,
		},
		{
			name: "repository error returns 500",
			setupMocks: func(accountRepo *account.MockRepository, orgID, ledgerID uuid.UUID) {
				accountRepo.EXPECT().
					Count(gomock.Any(), orgID, ledgerID).
					Return(int64(0), pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()

			mockAccountRepo := account.NewMockRepository(ctrl)
			tt.setupMocks(mockAccountRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				AccountRepo: mockAccountRepo,
			}
			handler := &AccountHandler{Query: queryUC}

			app := fiber.New()
			app.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/metrics/count",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.CountAccounts,
			)

			// Act
			req := httptest.NewRequest("HEAD", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/metrics/count", nil)
			resp, err := app.Test(req)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == 204 {
				// Validate X-Total-Count header
				totalCount := resp.Header.Get(cn.XTotalCount)
				assert.Equal(t, "42", totalCount, "X-Total-Count header should contain the count")

				contentLength := resp.Header.Get(cn.ContentLength)
				assert.Equal(t, "0", contentLength, "Content-Length should be 0")
			}
		})
	}
}
