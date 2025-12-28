package in

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestHandler_CreateLedger(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.CreateLedgerInput
		setupMocks     func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 201 with created ledger",
			payload: &mmodel.CreateLedgerInput{
				Name: "Test Ledger",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
			},
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID uuid.UUID) {
				// FindByName check for duplicate names (returns false = name available)
				ledgerRepo.EXPECT().
					FindByName(gomock.Any(), orgID, "Test Ledger").
					Return(false, nil).
					Times(1)

				ledgerRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx any, led *mmodel.Ledger) (*mmodel.Ledger, error) {
						led.ID = uuid.New().String()
						led.CreatedAt = time.Now()
						led.UpdatedAt = time.Now()
						return led, nil
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
				assert.Equal(t, "Test Ledger", result["name"])
			},
		},
		{
			name: "duplicate name returns error",
			payload: &mmodel.CreateLedgerInput{
				Name: "Existing Ledger",
			},
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID uuid.UUID) {
				// FindByName returns error for duplicate
				ledgerRepo.EXPECT().
					FindByName(gomock.Any(), orgID, "Existing Ledger").
					Return(false, pkg.ValidateBusinessError(cn.ErrLedgerNameConflict, reflect.TypeOf(mmodel.Ledger{}).Name())).
					Times(1)
			},
			expectedStatus: 409,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrLedgerNameConflict.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			payload: &mmodel.CreateLedgerInput{
				Name: "Test Ledger",
			},
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID uuid.UUID) {
				ledgerRepo.EXPECT().
					FindByName(gomock.Any(), orgID, "Test Ledger").
					Return(false, nil).
					Times(1)

				ledgerRepo.EXPECT().
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

			mockLedgerRepo := ledger.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockLedgerRepo, mockMetadataRepo, orgID)

			cmdUC := &command.UseCase{
				LedgerRepo:   mockLedgerRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &LedgerHandler{Command: cmdUC}

			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/ledgers",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					return c.Next()
				},
				func(c *fiber.Ctx) error {
					return handler.CreateLedger(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("POST", "/v1/organizations/"+orgID.String()+"/ledgers", nil)
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

func TestHandler_UpdateLedger(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.UpdateLedgerInput
		setupMocks     func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with updated ledger",
			payload: &mmodel.UpdateLedgerInput{
				Name: "Updated Ledger Name",
			},
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// Update succeeds
				ledgerRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(&mmodel.Ledger{
						ID:             ledgerID.String(),
						OrganizationID: orgID.String(),
						Name:           "Updated Ledger Name",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// UpdateMetadata is called
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Ledger", ledgerID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Retrieval after update
				ledgerRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID).
					Return(&mmodel.Ledger{
						ID:             ledgerID.String(),
						OrganizationID: orgID.String(),
						Name:           "Updated Ledger Name",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetLedgerByID also fetches metadata
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Ledger", ledgerID.String()).
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
				assert.Equal(t, "Updated Ledger Name", result["name"])
			},
		},
		{
			name: "not found on update returns 404",
			payload: &mmodel.UpdateLedgerInput{
				Name: "Updated Name",
			},
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				ledgerRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrLedgerIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "not found on retrieval returns 404",
			payload: &mmodel.UpdateLedgerInput{
				Name: "Updated Name",
			},
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// Update succeeds
				ledgerRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(&mmodel.Ledger{ID: ledgerID.String()}, nil).
					Times(1)

				// UpdateMetadata succeeds
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Ledger", ledgerID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Retrieval fails
				ledgerRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())).
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
			payload: &mmodel.UpdateLedgerInput{
				Name: "Updated Name",
			},
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				ledgerRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, gomock.Any()).
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

			mockLedgerRepo := ledger.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockLedgerRepo, mockMetadataRepo, orgID, ledgerID)

			cmdUC := &command.UseCase{
				LedgerRepo:   mockLedgerRepo,
				MetadataRepo: mockMetadataRepo,
			}
			queryUC := &query.UseCase{
				LedgerRepo:   mockLedgerRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &LedgerHandler{
				Command: cmdUC,
				Query:   queryUC,
			}

			app := fiber.New()
			app.Patch("/v1/organizations/:organization_id/ledgers/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("id", ledgerID)
					return c.Next()
				},
				func(c *fiber.Ctx) error {
					return handler.UpdateLedger(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("PATCH", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String(), nil)
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

func TestHandler_GetLedgerByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with ledger",
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				ledgerRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID).
					Return(&mmodel.Ledger{
						ID:             ledgerID.String(),
						OrganizationID: orgID.String(),
						Name:           "Test Ledger",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetLedgerByID fetches metadata when ledger is found
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Ledger", ledgerID.String()).
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
				assert.Equal(t, "Test Ledger", result["name"])
			},
		},
		{
			name: "not found returns 404",
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				ledgerRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrLedgerIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				ledgerRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID).
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

			mockLedgerRepo := ledger.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockLedgerRepo, mockMetadataRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				LedgerRepo:   mockLedgerRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &LedgerHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("id", ledgerID)
					return c.Next()
				},
				handler.GetLedgerByID,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String(), nil)
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

func TestHandler_GetAllLedgers(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with pagination structure",
			queryParams: "",
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID uuid.UUID) {
				ledgerRepo.EXPECT().
					FindAll(gomock.Any(), orgID, gomock.Any()).
					Return([]*mmodel.Ledger{}, nil).
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
			name:        "success with items returns ledgers",
			queryParams: "?limit=5&page=1",
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID uuid.UUID) {
				ledger1ID := uuid.New().String()
				ledger2ID := uuid.New().String()

				ledgerRepo.EXPECT().
					FindAll(gomock.Any(), orgID, gomock.Any()).
					Return([]*mmodel.Ledger{
						{
							ID:             ledger1ID,
							OrganizationID: orgID.String(),
							Name:           "Ledger One",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             ledger2ID,
							OrganizationID: orgID.String(),
							Name:           "Ledger Two",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
					}, nil).
					Times(1)

				// GetAllLedgers fetches metadata for all returned ledgers
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Ledger", gomock.Any()).
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
				assert.Len(t, items, 2, "should have two ledgers")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "ledger should have id field")
				assert.Contains(t, firstItem, "name", "ledger should have name field")

				// Validate pagination
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(5), limit)
			},
		},
		{
			name:        "metadata filter returns filtered ledgers",
			queryParams: "?metadata.tier=premium",
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID uuid.UUID) {
				ledger1ID := uuid.New().String()
				ledger2ID := uuid.New().String()

				// MetadataRepo.FindList returns metadata matching the filter
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Ledger", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: ledger1ID, Data: map[string]any{"tier": "premium"}},
						{EntityID: ledger2ID, Data: map[string]any{"tier": "premium"}},
					}, nil).
					Times(1)

				// LedgerRepo.ListByIDs returns the ledgers
				ledgerRepo.EXPECT().
					ListByIDs(gomock.Any(), orgID, gomock.Any()).
					Return([]*mmodel.Ledger{
						{
							ID:             ledger1ID,
							OrganizationID: orgID.String(),
							Name:           "Premium Ledger One",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             ledger2ID,
							OrganizationID: orgID.String(),
							Name:           "Premium Ledger Two",
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
				assert.Len(t, items, 2, "should have two filtered ledgers")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "ledger should have id field")
				assert.Contains(t, firstItem, "name", "ledger should have name field")
			},
		},
		{
			name:        "metadata filter with no matching metadata returns 404",
			queryParams: "?metadata.tier=nonexistent",
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID uuid.UUID) {
				// MetadataRepo.FindList returns nil (no matching metadata)
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Ledger", gomock.Any()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrNoLedgersFound.Error(), errResp["code"])
			},
		},
		{
			name:        "metadata filter with ledgers not found returns 404",
			queryParams: "?metadata.tier=premium",
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID uuid.UUID) {
				ledger1ID := uuid.New().String()

				// MetadataRepo.FindList returns metadata
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Ledger", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: ledger1ID, Data: map[string]any{"tier": "premium"}},
					}, nil).
					Times(1)

				// LedgerRepo.ListByIDs returns not found error
				ledgerRepo.EXPECT().
					ListByIDs(gomock.Any(), orgID, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrNoLedgersFound, reflect.TypeOf(mmodel.Ledger{}).Name())).
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
			setupMocks: func(ledgerRepo *ledger.MockRepository, metadataRepo *mongodb.MockRepository, orgID uuid.UUID) {
				ledgerRepo.EXPECT().
					FindAll(gomock.Any(), orgID, gomock.Any()).
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

			mockLedgerRepo := ledger.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockLedgerRepo, mockMetadataRepo, orgID)

			queryUC := &query.UseCase{
				LedgerRepo:   mockLedgerRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &LedgerHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					return c.Next()
				},
				handler.GetAllLedgers,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers"+tt.queryParams, nil)
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

func TestHandler_DeleteLedgerByID(t *testing.T) {
	tests := []struct {
		name           string
		envName        string
		setupMocks     func(ledgerRepo *ledger.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:    "success returns 204 no content",
			envName: "development",
			setupMocks: func(ledgerRepo *ledger.MockRepository, orgID, ledgerID uuid.UUID) {
				ledgerRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil, // 204 has no body
		},
		{
			name:    "not found returns 404",
			envName: "development",
			setupMocks: func(ledgerRepo *ledger.MockRepository, orgID, ledgerID uuid.UUID) {
				ledgerRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID).
					Return(pkg.ValidateBusinessError(cn.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrLedgerIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name:    "production environment returns 400 validation error",
			envName: "production",
			setupMocks: func(ledgerRepo *ledger.MockRepository, orgID, ledgerID uuid.UUID) {
				// No repository calls expected in production
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrActionNotPermitted.Error(), errResp["code"])
			},
		},
		{
			name:    "repository error returns 500",
			envName: "development",
			setupMocks: func(ledgerRepo *ledger.MockRepository, orgID, ledgerID uuid.UUID) {
				ledgerRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID).
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

			// Set environment variable for production check
			t.Setenv("ENV_NAME", tt.envName)

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()

			mockLedgerRepo := ledger.NewMockRepository(ctrl)
			tt.setupMocks(mockLedgerRepo, orgID, ledgerID)

			cmdUC := &command.UseCase{
				LedgerRepo: mockLedgerRepo,
			}
			handler := &LedgerHandler{Command: cmdUC}

			app := fiber.New()
			app.Delete("/v1/organizations/:organization_id/ledgers/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("id", ledgerID)
					return c.Next()
				},
				handler.DeleteLedgerByID,
			)

			// Act
			req := httptest.NewRequest("DELETE", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String(), nil)
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

func TestHandler_CountLedgers(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(ledgerRepo *ledger.MockRepository, orgID uuid.UUID)
		expectedStatus int
	}{
		{
			name: "success returns 204 with X-Total-Count header",
			setupMocks: func(ledgerRepo *ledger.MockRepository, orgID uuid.UUID) {
				ledgerRepo.EXPECT().
					Count(gomock.Any(), orgID).
					Return(int64(42), nil).
					Times(1)
			},
			expectedStatus: 204,
		},
		{
			name: "repository error returns 500",
			setupMocks: func(ledgerRepo *ledger.MockRepository, orgID uuid.UUID) {
				ledgerRepo.EXPECT().
					Count(gomock.Any(), orgID).
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

			mockLedgerRepo := ledger.NewMockRepository(ctrl)
			tt.setupMocks(mockLedgerRepo, orgID)

			queryUC := &query.UseCase{
				LedgerRepo: mockLedgerRepo,
			}
			handler := &LedgerHandler{Query: queryUC}

			app := fiber.New()
			app.Head("/v1/organizations/:organization_id/ledgers/metrics/count",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					return c.Next()
				},
				handler.CountLedgers,
			)

			// Act
			req := httptest.NewRequest("HEAD", "/v1/organizations/"+orgID.String()+"/ledgers/metrics/count", nil)
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

// Ensure libPostgres.Pagination is used (referenced in handler)
var _ = libPostgres.Pagination{}
