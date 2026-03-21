// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

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
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestHandler_CreateAsset(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.CreateAssetInput
		setupMocks     func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, accountRepo *account.MockRepository, balancePort *mbootstrap.MockBalancePort, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 201 with created asset",
			payload: &mmodel.CreateAssetInput{
				Name: "Test Asset",
				Code: "TST",
				Type: "commodity",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
			},
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, accountRepo *account.MockRepository, balancePort *mbootstrap.MockBalancePort, orgID, ledgerID uuid.UUID) {
				// CheckHealth is called first to verify balance service availability
				balancePort.EXPECT().
					CheckHealth(gomock.Any()).
					Return(nil).
					Times(1)

				// FindByNameOrCode check for duplicate names/codes (returns false = name/code available)
				assetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), orgID, ledgerID, "Test Asset", "TST").
					Return(false, nil).
					Times(1)

				assetRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx any, a *mmodel.Asset) (*mmodel.Asset, error) {
						a.ID = uuid.New().String()
						a.CreatedAt = time.Now()
						a.UpdatedAt = time.Now()
						return a, nil
					}).
					Times(1)

				// No metadata in request, so MetadataRepo.Create won't be called

				// ListAccountsByAlias to check for existing external account
				accountRepo.EXPECT().
					ListAccountsByAlias(gomock.Any(), orgID, ledgerID, []string{"@external/TST"}).
					Return([]*mmodel.Account{}, nil).
					Times(1)

				// Create external account
				accountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx any, acc *mmodel.Account) (*mmodel.Account, error) {
						return acc, nil
					}).
					Times(1)

				// CreateBalanceSync for external account
				balancePort.EXPECT().
					CreateBalanceSync(gomock.Any(), gomock.Any()).
					Return(&mmodel.Balance{}, nil).
					Times(1)
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "name", "response should contain name")
				assert.Equal(t, "Test Asset", result["name"])
				assert.Equal(t, "TST", result["code"])
			},
		},
		{
			name: "duplicate code returns 409 conflict",
			payload: &mmodel.CreateAssetInput{
				Name: "Existing Asset",
				Code: "EXS",
				Type: "commodity",
			},
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, accountRepo *account.MockRepository, balancePort *mbootstrap.MockBalancePort, orgID, ledgerID uuid.UUID) {
				// CheckHealth is called first to verify balance service availability
				balancePort.EXPECT().
					CheckHealth(gomock.Any()).
					Return(nil).
					Times(1)

				// FindByNameOrCode returns error for duplicate
				assetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), orgID, ledgerID, "Existing Asset", "EXS").
					Return(false, pkg.ValidateBusinessError(cn.ErrAssetNameOrCodeDuplicate, reflect.TypeOf(mmodel.Asset{}).Name())).
					Times(1)
			},
			expectedStatus: 409,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAssetNameOrCodeDuplicate.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			payload: &mmodel.CreateAssetInput{
				Name: "Test Asset",
				Code: "TST",
				Type: "commodity",
			},
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, accountRepo *account.MockRepository, balancePort *mbootstrap.MockBalancePort, orgID, ledgerID uuid.UUID) {
				// CheckHealth is called first to verify balance service availability
				balancePort.EXPECT().
					CheckHealth(gomock.Any()).
					Return(nil).
					Times(1)

				assetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), orgID, ledgerID, "Test Asset", "TST").
					Return(false, nil).
					Times(1)

				assetRepo.EXPECT().
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

			mockAssetRepo := asset.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			mockAccountRepo := account.NewMockRepository(ctrl)
			mockBalancePort := mbootstrap.NewMockBalancePort(ctrl)
			tt.setupMocks(mockAssetRepo, mockMetadataRepo, mockAccountRepo, mockBalancePort, orgID, ledgerID)

			cmdUC := &command.UseCase{
				AssetRepo:    mockAssetRepo,
				MetadataRepo: mockMetadataRepo,
				AccountRepo:  mockAccountRepo,
				BalancePort:  mockBalancePort,
			}
			handler := &AssetHandler{Command: cmdUC}

			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/assets",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				func(c *fiber.Ctx) error {
					return handler.CreateAsset(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("POST", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets", nil)
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

func TestHandler_UpdateAsset(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.UpdateAssetInput
		setupMocks     func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, assetID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with updated asset",
			payload: &mmodel.UpdateAssetInput{
				Name: "Updated Asset Name",
			},
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, assetID uuid.UUID) {
				// Update succeeds
				assetRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, assetID, gomock.Any()).
					Return(&mmodel.Asset{
						ID:             assetID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Updated Asset Name",
						Code:           "TST",
						Type:           "commodity",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// UpdateMetadata is called
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Asset", assetID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Retrieval after update
				assetRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, assetID).
					Return(&mmodel.Asset{
						ID:             assetID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Updated Asset Name",
						Code:           "TST",
						Type:           "commodity",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetAssetByID also fetches metadata
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Asset", assetID.String()).
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
				assert.Equal(t, "Updated Asset Name", result["name"])
			},
		},
		{
			name: "not found on update returns 404",
			payload: &mmodel.UpdateAssetInput{
				Name: "Updated Name",
			},
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, assetID uuid.UUID) {
				assetRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, assetID, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAssetIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "not found on retrieval returns 404",
			payload: &mmodel.UpdateAssetInput{
				Name: "Updated Name",
			},
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, assetID uuid.UUID) {
				// Update succeeds
				assetRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, assetID, gomock.Any()).
					Return(&mmodel.Asset{ID: assetID.String()}, nil).
					Times(1)

				// UpdateMetadata succeeds
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Asset", assetID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Retrieval fails
				assetRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, assetID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name())).
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
			payload: &mmodel.UpdateAssetInput{
				Name: "Updated Name",
			},
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, assetID uuid.UUID) {
				assetRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, assetID, gomock.Any()).
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
			assetID := uuid.New()

			mockAssetRepo := asset.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAssetRepo, mockMetadataRepo, orgID, ledgerID, assetID)

			cmdUC := &command.UseCase{
				AssetRepo:    mockAssetRepo,
				MetadataRepo: mockMetadataRepo,
			}
			queryUC := &query.UseCase{
				AssetRepo:    mockAssetRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &AssetHandler{
				Command: cmdUC,
				Query:   queryUC,
			}

			app := fiber.New()
			app.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", assetID)
					return c.Next()
				},
				func(c *fiber.Ctx) error {
					return handler.UpdateAsset(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("PATCH", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(), nil)
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

func TestHandler_GetAssetByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, assetID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with asset",
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, assetID uuid.UUID) {
				assetRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, assetID).
					Return(&mmodel.Asset{
						ID:             assetID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Test Asset",
						Code:           "TST",
						Type:           "commodity",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetAssetByID fetches metadata when asset is found
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Asset", assetID.String()).
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
				assert.Equal(t, "Test Asset", result["name"])
				assert.Equal(t, "TST", result["code"])
			},
		},
		{
			name: "not found returns 404",
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, assetID uuid.UUID) {
				assetRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, assetID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAssetIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, assetID uuid.UUID) {
				assetRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, assetID).
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
			assetID := uuid.New()

			mockAssetRepo := asset.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAssetRepo, mockMetadataRepo, orgID, ledgerID, assetID)

			queryUC := &query.UseCase{
				AssetRepo:    mockAssetRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &AssetHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", assetID)
					return c.Next()
				},
				handler.GetAssetByID,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(), nil)
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

func TestHandler_GetAllAssets(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with pagination structure",
			queryParams: "",
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				assetRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.Asset{}, nil).
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

				page, ok := result["page"].(float64)
				require.True(t, ok, "page should be a number")
				assert.Equal(t, float64(1), page)
			},
		},
		{
			name:        "success with items returns assets",
			queryParams: "?limit=5&page=1",
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				asset1ID := uuid.New().String()
				asset2ID := uuid.New().String()

				assetRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.Asset{
						{
							ID:             asset1ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Asset One",
							Code:           "A01",
							Type:           "commodity",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             asset2ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Asset Two",
							Code:           "A02",
							Type:           "currency",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
					}, nil).
					Times(1)

				// GetAllAssets fetches metadata for all returned assets
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Asset", gomock.Any()).
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
				assert.Len(t, items, 2, "should have two assets")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "asset should have id field")
				assert.Contains(t, firstItem, "name", "asset should have name field")
				assert.Contains(t, firstItem, "code", "asset should have code field")

				// Validate pagination
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(5), limit)
			},
		},
		{
			name:        "metadata filter returns filtered assets",
			queryParams: "?metadata.tier=premium",
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				asset1ID := uuid.New().String()
				asset2ID := uuid.New().String()

				// MetadataRepo.FindList returns metadata matching the filter
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Asset", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: asset1ID, Data: map[string]any{"tier": "premium"}},
						{EntityID: asset2ID, Data: map[string]any{"tier": "premium"}},
					}, nil).
					Times(1)

				// AssetRepo.ListByIDs returns the assets
				assetRepo.EXPECT().
					ListByIDs(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.Asset{
						{
							ID:             asset1ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Premium Asset One",
							Code:           "P01",
							Type:           "commodity",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             asset2ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Premium Asset Two",
							Code:           "P02",
							Type:           "currency",
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
				assert.Len(t, items, 2, "should have two filtered assets")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "asset should have id field")
				assert.Contains(t, firstItem, "name", "asset should have name field")
			},
		},
		{
			name:        "metadata filter with no matching metadata returns 404",
			queryParams: "?metadata.tier=nonexistent",
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// MetadataRepo.FindList returns nil (no matching metadata)
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Asset", gomock.Any()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrNoAssetsFound.Error(), errResp["code"])
			},
		},
		{
			name:        "metadata filter with assets not found returns 404",
			queryParams: "?metadata.tier=premium",
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				asset1ID := uuid.New().String()

				// MetadataRepo.FindList returns metadata
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Asset", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: asset1ID, Data: map[string]any{"tier": "premium"}},
					}, nil).
					Times(1)

				// AssetRepo.ListByIDs returns not found error
				assetRepo.EXPECT().
					ListByIDs(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrNoAssetsFound, reflect.TypeOf(mmodel.Asset{}).Name())).
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
			setupMocks: func(assetRepo *asset.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				assetRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
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

			mockAssetRepo := asset.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAssetRepo, mockMetadataRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				AssetRepo:    mockAssetRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &AssetHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.GetAllAssets,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets"+tt.queryParams, nil)
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

func TestHandler_DeleteAssetByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(assetRepo *asset.MockRepository, accountRepo *account.MockRepository, orgID, ledgerID, assetID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 204 no content",
			setupMocks: func(assetRepo *asset.MockRepository, accountRepo *account.MockRepository, orgID, ledgerID, assetID uuid.UUID) {
				// Find asset first
				assetRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, assetID).
					Return(&mmodel.Asset{
						ID:             assetID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Test Asset",
						Code:           "TST",
						Type:           "commodity",
					}, nil).
					Times(1)

				// ListAccountsByAlias to find external account
				accountRepo.EXPECT().
					ListAccountsByAlias(gomock.Any(), orgID, ledgerID, []string{"@external/TST"}).
					Return([]*mmodel.Account{}, nil).
					Times(1)

				// Delete asset
				assetRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, assetID).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil, // 204 has no body
		},
		{
			name: "not found returns 404",
			setupMocks: func(assetRepo *asset.MockRepository, accountRepo *account.MockRepository, orgID, ledgerID, assetID uuid.UUID) {
				assetRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, assetID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAssetIDNotFound, reflect.TypeOf(mmodel.Asset{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAssetIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(assetRepo *asset.MockRepository, accountRepo *account.MockRepository, orgID, ledgerID, assetID uuid.UUID) {
				assetRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, assetID).
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
			assetID := uuid.New()

			mockAssetRepo := asset.NewMockRepository(ctrl)
			mockAccountRepo := account.NewMockRepository(ctrl)
			tt.setupMocks(mockAssetRepo, mockAccountRepo, orgID, ledgerID, assetID)

			cmdUC := &command.UseCase{
				AssetRepo:   mockAssetRepo,
				AccountRepo: mockAccountRepo,
			}
			handler := &AssetHandler{Command: cmdUC}

			app := fiber.New()
			app.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", assetID)
					return c.Next()
				},
				handler.DeleteAssetByID,
			)

			// Act
			req := httptest.NewRequest("DELETE", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(), nil)
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

func TestHandler_CountAssets(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(assetRepo *asset.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
	}{
		{
			name: "success returns 204 with X-Total-Count header",
			setupMocks: func(assetRepo *asset.MockRepository, orgID, ledgerID uuid.UUID) {
				assetRepo.EXPECT().
					Count(gomock.Any(), orgID, ledgerID).
					Return(int64(42), nil).
					Times(1)
			},
			expectedStatus: 204,
		},
		{
			name: "repository error returns 500",
			setupMocks: func(assetRepo *asset.MockRepository, orgID, ledgerID uuid.UUID) {
				assetRepo.EXPECT().
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

			mockAssetRepo := asset.NewMockRepository(ctrl)
			tt.setupMocks(mockAssetRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				AssetRepo: mockAssetRepo,
			}
			handler := &AssetHandler{Query: queryUC}

			app := fiber.New()
			app.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/metrics/count",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.CountAssets,
			)

			// Act
			req := httptest.NewRequest("HEAD", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/metrics/count", nil)
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
