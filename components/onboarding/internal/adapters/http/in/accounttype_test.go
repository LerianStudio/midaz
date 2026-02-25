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
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestHandler_CreateAccountType(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.CreateAccountTypeInput
		setupMocks     func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 201 with created account type",
			payload: &mmodel.CreateAccountTypeInput{
				Name:     "Test Account Type",
				KeyValue: "test_account_type",
			},
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountTypeRepo.EXPECT().
					Create(gomock.Any(), orgID, ledgerID, gomock.Any()).
					DoAndReturn(func(ctx any, oID, lID uuid.UUID, at *mmodel.AccountType) (*mmodel.AccountType, error) {
						at.ID = uuid.New()
						at.CreatedAt = time.Now()
						at.UpdatedAt = time.Now()
						return at, nil
					}).
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
				assert.Equal(t, "Test Account Type", result["name"])
				assert.Contains(t, result, "keyValue", "response should contain keyValue")
				assert.Equal(t, "test_account_type", result["keyValue"])
			},
		},
		{
			name: "duplicate key value returns 409 conflict",
			payload: &mmodel.CreateAccountTypeInput{
				Name:     "Existing Account Type",
				KeyValue: "existing_key",
			},
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountTypeRepo.EXPECT().
					Create(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrDuplicateAccountTypeKeyValue, reflect.TypeOf(mmodel.AccountType{}).Name())).
					Times(1)
			},
			expectedStatus: 409,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrDuplicateAccountTypeKeyValue.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			payload: &mmodel.CreateAccountTypeInput{
				Name:     "Test Account Type",
				KeyValue: "test_account_type",
			},
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountTypeRepo.EXPECT().
					Create(gomock.Any(), orgID, ledgerID, gomock.Any()).
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

			mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAccountTypeRepo, mockMetadataRepo, orgID, ledgerID)

			cmdUC := &command.UseCase{
				AccountTypeRepo: mockAccountTypeRepo,
				MetadataRepo:    mockMetadataRepo,
			}
			handler := &AccountTypeHandler{Command: cmdUC}

			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				http.WithBody(new(mmodel.CreateAccountTypeInput), handler.CreateAccountType),
			)

			// Act
			bodyBytes, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types", bytes.NewReader(bodyBytes))
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

func TestHandler_UpdateAccountType(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.UpdateAccountTypeInput
		setupMocks     func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountTypeID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with updated account type",
			payload: &mmodel.UpdateAccountTypeInput{
				Name: "Updated Account Type Name",
			},
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountTypeID uuid.UUID) {
				// Update succeeds
				accountTypeRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, accountTypeID, gomock.Any()).
					Return(&mmodel.AccountType{
						ID:             accountTypeID,
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						Name:           "Updated Account Type Name",
						KeyValue:       "test_key",
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// UpdateMetadata is called
				metadataRepo.EXPECT().
					Update(gomock.Any(), "AccountType", accountTypeID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Retrieval after update
				accountTypeRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, accountTypeID).
					Return(&mmodel.AccountType{
						ID:             accountTypeID,
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						Name:           "Updated Account Type Name",
						KeyValue:       "test_key",
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetAccountTypeByID also fetches metadata
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "AccountType", accountTypeID.String()).
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
				assert.Equal(t, "Updated Account Type Name", result["name"])
			},
		},
		{
			name: "not found on update returns 404",
			payload: &mmodel.UpdateAccountTypeInput{
				Name: "Updated Name",
			},
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountTypeID uuid.UUID) {
				accountTypeRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, accountTypeID, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAccountTypeNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAccountTypeNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "not found on retrieval returns 404",
			payload: &mmodel.UpdateAccountTypeInput{
				Name: "Updated Name",
			},
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountTypeID uuid.UUID) {
				// Update succeeds
				accountTypeRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, accountTypeID, gomock.Any()).
					Return(&mmodel.AccountType{ID: accountTypeID}, nil).
					Times(1)

				// UpdateMetadata succeeds
				metadataRepo.EXPECT().
					Update(gomock.Any(), "AccountType", accountTypeID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Retrieval fails
				accountTypeRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, accountTypeID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAccountTypeNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())).
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
			payload: &mmodel.UpdateAccountTypeInput{
				Name: "Updated Name",
			},
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountTypeID uuid.UUID) {
				accountTypeRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, accountTypeID, gomock.Any()).
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
			accountTypeID := uuid.New()

			mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAccountTypeRepo, mockMetadataRepo, orgID, ledgerID, accountTypeID)

			cmdUC := &command.UseCase{
				AccountTypeRepo: mockAccountTypeRepo,
				MetadataRepo:    mockMetadataRepo,
			}
			queryUC := &query.UseCase{
				AccountTypeRepo: mockAccountTypeRepo,
				MetadataRepo:    mockMetadataRepo,
			}
			handler := &AccountTypeHandler{
				Command: cmdUC,
				Query:   queryUC,
			}

			app := fiber.New()
			app.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", accountTypeID)
					return c.Next()
				},
				http.WithBody(new(mmodel.UpdateAccountTypeInput), handler.UpdateAccountType),
			)

			// Act
			bodyBytes, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			req := httptest.NewRequest("PATCH", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types/"+accountTypeID.String(), bytes.NewReader(bodyBytes))
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

func TestHandler_GetAccountTypeByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountTypeID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with account type",
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountTypeID uuid.UUID) {
				accountTypeRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, accountTypeID).
					Return(&mmodel.AccountType{
						ID:             accountTypeID,
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						Name:           "Test Account Type",
						KeyValue:       "test_key",
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetAccountTypeByID fetches metadata when account type is found
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "AccountType", accountTypeID.String()).
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
				assert.Equal(t, "Test Account Type", result["name"])
			},
		},
		{
			name: "not found returns 404",
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountTypeID uuid.UUID) {
				accountTypeRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, accountTypeID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAccountTypeNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAccountTypeNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, accountTypeID uuid.UUID) {
				accountTypeRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, accountTypeID).
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
			accountTypeID := uuid.New()

			mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAccountTypeRepo, mockMetadataRepo, orgID, ledgerID, accountTypeID)

			queryUC := &query.UseCase{
				AccountTypeRepo: mockAccountTypeRepo,
				MetadataRepo:    mockMetadataRepo,
			}
			handler := &AccountTypeHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", accountTypeID)
					return c.Next()
				},
				handler.GetAccountTypeByID,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types/"+accountTypeID.String(), nil)
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

func TestHandler_GetAllAccountTypes(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with pagination structure",
			queryParams: "",
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountTypeRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.AccountType{}, libHTTP.CursorPagination{}, nil).
					Times(1)

				// Empty list still calls metadata lookup
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "AccountType", gomock.Any()).
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
			},
		},
		{
			name:        "success with items returns account types with cursor pagination",
			queryParams: "?limit=5",
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountType1ID := uuid.New()
				accountType2ID := uuid.New()

				accountTypeRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.AccountType{
						{
							ID:             accountType1ID,
							OrganizationID: orgID,
							LedgerID:       ledgerID,
							Name:           "Account Type One",
							KeyValue:       "key_one",
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             accountType2ID,
							OrganizationID: orgID,
							LedgerID:       ledgerID,
							Name:           "Account Type Two",
							KeyValue:       "key_two",
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
					}, libHTTP.CursorPagination{Next: "next_cursor", Prev: "prev_cursor"}, nil).
					Times(1)

				// GetAllAccountTypes fetches metadata for all returned account types
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "AccountType", gomock.Any()).
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
				assert.Len(t, items, 2, "should have two account types")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "account type should have id field")
				assert.Contains(t, firstItem, "name", "account type should have name field")

				// Validate pagination limit
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(5), limit)

				// Validate cursor pagination
				nextCursor, ok := result["next_cursor"].(string)
				require.True(t, ok, "next_cursor should be a string")
				assert.Equal(t, "next_cursor", nextCursor)

				prevCursor, ok := result["prev_cursor"].(string)
				require.True(t, ok, "prev_cursor should be a string")
				assert.Equal(t, "prev_cursor", prevCursor)
			},
		},
		{
			name:        "metadata filter returns filtered account types",
			queryParams: "?metadata.tier=premium",
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountType1ID := uuid.New()
				accountType2ID := uuid.New()

				// MetadataRepo.FindList returns metadata matching the filter
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "AccountType", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: accountType1ID.String(), Data: map[string]any{"tier": "premium"}},
						{EntityID: accountType2ID.String(), Data: map[string]any{"tier": "premium"}},
					}, nil).
					Times(1)

				// AccountTypeRepo.ListByIDs returns the account types
				accountTypeRepo.EXPECT().
					ListByIDs(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.AccountType{
						{
							ID:             accountType1ID,
							OrganizationID: orgID,
							LedgerID:       ledgerID,
							Name:           "Premium Account Type One",
							KeyValue:       "premium_key_one",
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             accountType2ID,
							OrganizationID: orgID,
							LedgerID:       ledgerID,
							Name:           "Premium Account Type Two",
							KeyValue:       "premium_key_two",
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
				assert.Len(t, items, 2, "should have two filtered account types")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "account type should have id field")
				assert.Contains(t, firstItem, "name", "account type should have name field")
			},
		},
		{
			name:        "metadata filter with no matching metadata returns 404",
			queryParams: "?metadata.tier=nonexistent",
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// MetadataRepo.FindList returns nil (no matching metadata)
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "AccountType", gomock.Any()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrNoAccountTypesFound.Error(), errResp["code"])
			},
		},
		{
			name:        "account types not found returns 404",
			queryParams: "",
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountTypeRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(cn.ErrNoAccountTypesFound, reflect.TypeOf(mmodel.AccountType{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrNoAccountTypesFound.Error(), errResp["code"])
			},
		},
		{
			name:        "repository error returns 500",
			queryParams: "",
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				accountTypeRepo.EXPECT().
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

			mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAccountTypeRepo, mockMetadataRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				AccountTypeRepo: mockAccountTypeRepo,
				MetadataRepo:    mockMetadataRepo,
			}
			handler := &AccountTypeHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.GetAllAccountTypes,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types"+tt.queryParams, nil)
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

func TestHandler_DeleteAccountTypeByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(accountTypeRepo *accounttype.MockRepository, orgID, ledgerID, accountTypeID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 204 no content",
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, orgID, ledgerID, accountTypeID uuid.UUID) {
				accountTypeRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, accountTypeID).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil, // 204 has no body
		},
		{
			name: "not found returns 404",
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, orgID, ledgerID, accountTypeID uuid.UUID) {
				accountTypeRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, accountTypeID).
					Return(pkg.ValidateBusinessError(cn.ErrAccountTypeNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAccountTypeNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(accountTypeRepo *accounttype.MockRepository, orgID, ledgerID, accountTypeID uuid.UUID) {
				accountTypeRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, accountTypeID).
					Return(pkg.InternalServerError{
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
			accountTypeID := uuid.New()

			mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
			tt.setupMocks(mockAccountTypeRepo, orgID, ledgerID, accountTypeID)

			cmdUC := &command.UseCase{
				AccountTypeRepo: mockAccountTypeRepo,
			}
			handler := &AccountTypeHandler{Command: cmdUC}

			app := fiber.New()
			app.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", accountTypeID)
					return c.Next()
				},
				handler.DeleteAccountTypeByID,
			)

			// Act
			req := httptest.NewRequest("DELETE", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types/"+accountTypeID.String(), nil)
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
