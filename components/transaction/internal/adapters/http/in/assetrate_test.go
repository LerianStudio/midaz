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

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestAssetRateHandler_CreateOrUpdateAssetRate(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		setupMocks     func(assetRateRepo *assetrate.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 201 with created asset rate",
			jsonBody: `{
				"from": "USD",
				"to": "BRL",
				"rate": 500,
				"scale": 2,
				"ttl": 3600,
				"metadata": {"provider": "Central Bank"}
			}`,
			setupMocks: func(assetRateRepo *assetrate.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// FindByCurrencyPair returns nil (no existing asset rate)
				assetRateRepo.EXPECT().
					FindByCurrencyPair(gomock.Any(), orgID, ledgerID, "USD", "BRL").
					Return(nil, nil).
					Times(1)

				// Create new asset rate
				assetRateRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx any, ar *assetrate.AssetRate) (*assetrate.AssetRate, error) {
						ar.CreatedAt = time.Now()
						ar.UpdatedAt = time.Now()
						return ar, nil
					}).
					Times(1)

				// Create metadata for new asset rate
				metadataRepo.EXPECT().
					Create(gomock.Any(), "AssetRate", gomock.Any()).
					Return(nil).
					Times(1)
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "from", "response should contain from")
				assert.Contains(t, result, "to", "response should contain to")
				assert.Equal(t, "USD", result["from"])
				assert.Equal(t, "BRL", result["to"])
			},
		},
		{
			name: "update existing asset rate returns 201",
			jsonBody: `{
				"from": "USD",
				"to": "EUR",
				"rate": 110,
				"scale": 2,
				"ttl": 7200,
				"metadata": {"provider": "Updated Provider"}
			}`,
			setupMocks: func(assetRateRepo *assetrate.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				existingID := uuid.New().String()
				scale := float64(2)

				// FindByCurrencyPair returns existing asset rate
				assetRateRepo.EXPECT().
					FindByCurrencyPair(gomock.Any(), orgID, ledgerID, "USD", "EUR").
					Return(&assetrate.AssetRate{
						ID:             existingID,
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						ExternalID:     uuid.New().String(),
						From:           "USD",
						To:             "EUR",
						Rate:           100,
						Scale:          &scale,
						TTL:            3600,
						CreatedAt:      time.Now().Add(-time.Hour),
						UpdatedAt:      time.Now().Add(-time.Hour),
					}, nil).
					Times(1)

				// Update existing asset rate
				assetRateRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx any, oID, lID, id uuid.UUID, ar *assetrate.AssetRate) (*assetrate.AssetRate, error) {
						ar.UpdatedAt = time.Now()
						return ar, nil
					}).
					Times(1)

				// UpdateMetadata first calls FindByEntity
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "AssetRate", existingID).
					Return(nil, nil).
					Times(1)

				// Then calls Update
				metadataRepo.EXPECT().
					Update(gomock.Any(), "AssetRate", existingID, gomock.Any()).
					Return(nil).
					Times(1)
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "from", "response should contain from")
				assert.Equal(t, "USD", result["from"])
				assert.Equal(t, "EUR", result["to"])
			},
		},
		{
			name: "repository error returns 500",
			jsonBody: `{
				"from": "USD",
				"to": "BRL",
				"rate": 500,
				"scale": 2,
				"ttl": 3600
			}`,
			setupMocks: func(assetRateRepo *assetrate.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				assetRateRepo.EXPECT().
					FindByCurrencyPair(gomock.Any(), orgID, ledgerID, "USD", "BRL").
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

			mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAssetRateRepo, mockMetadataRepo, orgID, ledgerID)

			cmdUC := &command.UseCase{
				AssetRateRepo: mockAssetRateRepo,
				MetadataRepo:  mockMetadataRepo,
			}
			handler := &AssetRateHandler{Command: cmdUC}

			app := fiber.New()
			app.Put("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				http.WithBody(new(assetrate.CreateAssetRateInput), handler.CreateOrUpdateAssetRate),
			)

			req := httptest.NewRequest("PUT", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/asset-rates", bytes.NewBufferString(tt.jsonBody))
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

func TestAssetRateHandler_GetAssetRateByExternalID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(assetRateRepo *assetrate.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, externalID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with asset rate",
			setupMocks: func(assetRateRepo *assetrate.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, externalID uuid.UUID) {
				assetRateID := uuid.New().String()
				scale := float64(2)

				assetRateRepo.EXPECT().
					FindByExternalID(gomock.Any(), orgID, ledgerID, externalID).
					Return(&assetrate.AssetRate{
						ID:             assetRateID,
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						ExternalID:     externalID.String(),
						From:           "USD",
						To:             "BRL",
						Rate:           500,
						Scale:          &scale,
						TTL:            3600,
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetAssetRateByExternalID fetches metadata when asset rate is found
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "AssetRate", assetRateID).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "from", "response should contain from")
				assert.Contains(t, result, "to", "response should contain to")
				assert.Contains(t, result, "rate", "response should contain rate")
				assert.Equal(t, "USD", result["from"])
				assert.Equal(t, "BRL", result["to"])
			},
		},
		{
			name: "not found returns 404",
			setupMocks: func(assetRateRepo *assetrate.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, externalID uuid.UUID) {
				assetRateRepo.EXPECT().
					FindByExternalID(gomock.Any(), orgID, ledgerID, externalID).
					Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(assetrate.AssetRate{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, constant.ErrEntityNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(assetRateRepo *assetrate.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, externalID uuid.UUID) {
				assetRateRepo.EXPECT().
					FindByExternalID(gomock.Any(), orgID, ledgerID, externalID).
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
			externalID := uuid.New()

			mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAssetRateRepo, mockMetadataRepo, orgID, ledgerID, externalID)

			queryUC := &query.UseCase{
				AssetRateRepo: mockAssetRateRepo,
				MetadataRepo:  mockMetadataRepo,
			}
			handler := &AssetRateHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates/:external_id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("external_id", externalID)
					return c.Next()
				},
				handler.GetAssetRateByExternalID,
			)

			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/asset-rates/"+externalID.String(), nil)
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

func TestAssetRateHandler_GetAllAssetRatesByAssetCode(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(assetRateRepo *assetrate.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with pagination structure",
			queryParams: "",
			setupMocks: func(assetRateRepo *assetrate.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				assetRateRepo.EXPECT().
					FindAllByAssetCodes(gomock.Any(), orgID, ledgerID, "USD", gomock.Any(), gomock.Any()).
					Return([]*assetrate.AssetRate{}, libHTTP.CursorPagination{}, nil).
					Times(1)

				metadataRepo.EXPECT().
					FindList(gomock.Any(), "AssetRate", gomock.Any()).
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
			name:        "success with items returns asset rates",
			queryParams: "?limit=5&to=BRL,EUR",
			setupMocks: func(assetRateRepo *assetrate.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				assetRate1ID := uuid.New().String()
				assetRate2ID := uuid.New().String()
				scale := float64(2)

				assetRateRepo.EXPECT().
					FindAllByAssetCodes(gomock.Any(), orgID, ledgerID, "USD", gomock.Any(), gomock.Any()).
					Return([]*assetrate.AssetRate{
						{
							ID:             assetRate1ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							ExternalID:     uuid.New().String(),
							From:           "USD",
							To:             "BRL",
							Rate:           500,
							Scale:          &scale,
							TTL:            3600,
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             assetRate2ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							ExternalID:     uuid.New().String(),
							From:           "USD",
							To:             "EUR",
							Rate:           110,
							Scale:          &scale,
							TTL:            3600,
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
					}, libHTTP.CursorPagination{
						Next: "next_cursor_value",
						Prev: "",
					}, nil).
					Times(1)

				// GetAllAssetRatesByAssetCode fetches metadata for all returned asset rates
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "AssetRate", gomock.Any()).
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
				assert.Len(t, items, 2, "should have two asset rates")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "asset rate should have id field")
				assert.Contains(t, firstItem, "from", "asset rate should have from field")
				assert.Contains(t, firstItem, "to", "asset rate should have to field")
				assert.Contains(t, firstItem, "rate", "asset rate should have rate field")

				// Validate pagination
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(5), limit)

				// Validate cursor pagination fields
				assert.Contains(t, result, "next_cursor", "response should contain next_cursor")
			},
		},
		{
			name:        "repository error returns 500",
			queryParams: "",
			setupMocks: func(assetRateRepo *assetrate.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				assetRateRepo.EXPECT().
					FindAllByAssetCodes(gomock.Any(), orgID, ledgerID, "USD", gomock.Any(), gomock.Any()).
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
			assetCode := "USD"

			mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockAssetRateRepo, mockMetadataRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				AssetRateRepo: mockAssetRateRepo,
				MetadataRepo:  mockMetadataRepo,
			}
			handler := &AssetRateHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates/from/:asset_code",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.GetAllAssetRatesByAssetCode,
			)

			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/asset-rates/from/"+assetCode+tt.queryParams, nil)
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
