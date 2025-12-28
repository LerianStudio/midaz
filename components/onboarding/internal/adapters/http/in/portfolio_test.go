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
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/portfolio"
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

func TestHandler_CreatePortfolio(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.CreatePortfolioInput
		setupMocks     func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 201 with created portfolio",
			payload: &mmodel.CreatePortfolioInput{
				Name: "Test Portfolio",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
			},
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				portfolioRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx any, port *mmodel.Portfolio) (*mmodel.Portfolio, error) {
						port.ID = uuid.New().String()
						port.CreatedAt = time.Now()
						port.UpdatedAt = time.Now()
						return port, nil
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
				assert.Equal(t, "Test Portfolio", result["name"])
			},
		},
		{
			name: "conflict returns 409",
			payload: &mmodel.CreatePortfolioInput{
				Name: "Existing Portfolio",
			},
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				portfolioRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())).
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
			payload: &mmodel.CreatePortfolioInput{
				Name: "Test Portfolio",
			},
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				portfolioRepo.EXPECT().
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

			mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockPortfolioRepo, mockMetadataRepo, orgID, ledgerID)

			cmdUC := &command.UseCase{
				PortfolioRepo: mockPortfolioRepo,
				MetadataRepo:  mockMetadataRepo,
			}
			handler := &PortfolioHandler{Command: cmdUC}

			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				func(c *fiber.Ctx) error {
					return handler.CreatePortfolio(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("POST", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios", nil)
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

func TestHandler_UpdatePortfolio(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.UpdatePortfolioInput
		setupMocks     func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, portfolioID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with updated portfolio",
			payload: &mmodel.UpdatePortfolioInput{
				Name: "Updated Portfolio Name",
			},
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, portfolioID uuid.UUID) {
				// Update succeeds
				portfolioRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, portfolioID, gomock.Any()).
					Return(&mmodel.Portfolio{
						ID:             portfolioID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Updated Portfolio Name",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// UpdateMetadata is called
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Portfolio", portfolioID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Retrieval after update (GetPortfolioByID)
				portfolioRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, portfolioID).
					Return(&mmodel.Portfolio{
						ID:             portfolioID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Updated Portfolio Name",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetPortfolioByID also fetches metadata
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Portfolio", portfolioID.String()).
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
				assert.Equal(t, "Updated Portfolio Name", result["name"])
			},
		},
		{
			name: "not found on update returns 404",
			payload: &mmodel.UpdatePortfolioInput{
				Name: "Updated Name",
			},
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, portfolioID uuid.UUID) {
				portfolioRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, portfolioID, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrPortfolioIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "not found on retrieval returns 404",
			payload: &mmodel.UpdatePortfolioInput{
				Name: "Updated Name",
			},
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, portfolioID uuid.UUID) {
				// Update succeeds
				portfolioRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, portfolioID, gomock.Any()).
					Return(&mmodel.Portfolio{ID: portfolioID.String()}, nil).
					Times(1)

				// UpdateMetadata succeeds
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Portfolio", portfolioID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Retrieval fails
				portfolioRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, portfolioID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())).
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
			payload: &mmodel.UpdatePortfolioInput{
				Name: "Updated Name",
			},
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, portfolioID uuid.UUID) {
				portfolioRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, portfolioID, gomock.Any()).
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
			portfolioID := uuid.New()

			mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockPortfolioRepo, mockMetadataRepo, orgID, ledgerID, portfolioID)

			cmdUC := &command.UseCase{
				PortfolioRepo: mockPortfolioRepo,
				MetadataRepo:  mockMetadataRepo,
			}
			queryUC := &query.UseCase{
				PortfolioRepo: mockPortfolioRepo,
				MetadataRepo:  mockMetadataRepo,
			}
			handler := &PortfolioHandler{
				Command: cmdUC,
				Query:   queryUC,
			}

			app := fiber.New()
			app.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", portfolioID)
					return c.Next()
				},
				func(c *fiber.Ctx) error {
					return handler.UpdatePortfolio(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("PATCH", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios/"+portfolioID.String(), nil)
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

func TestHandler_GetPortfolioByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, portfolioID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with portfolio",
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, portfolioID uuid.UUID) {
				portfolioRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, portfolioID).
					Return(&mmodel.Portfolio{
						ID:             portfolioID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Test Portfolio",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetPortfolioByID fetches metadata when portfolio is found
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Portfolio", portfolioID.String()).
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
				assert.Equal(t, "Test Portfolio", result["name"])
			},
		},
		{
			name: "not found returns 404",
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, portfolioID uuid.UUID) {
				portfolioRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, portfolioID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrPortfolioIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, portfolioID uuid.UUID) {
				portfolioRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, portfolioID).
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
			portfolioID := uuid.New()

			mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockPortfolioRepo, mockMetadataRepo, orgID, ledgerID, portfolioID)

			queryUC := &query.UseCase{
				PortfolioRepo: mockPortfolioRepo,
				MetadataRepo:  mockMetadataRepo,
			}
			handler := &PortfolioHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", portfolioID)
					return c.Next()
				},
				handler.GetPortfolioByID,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios/"+portfolioID.String(), nil)
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

func TestHandler_GetAllPortfolios(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with pagination structure",
			queryParams: "",
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				portfolioRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.Portfolio{}, nil).
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
			name:        "success with items returns portfolios",
			queryParams: "?limit=5&page=1",
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				portfolio1ID := uuid.New().String()
				portfolio2ID := uuid.New().String()

				portfolioRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.Portfolio{
						{
							ID:             portfolio1ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Portfolio One",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             portfolio2ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Portfolio Two",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
					}, nil).
					Times(1)

				// GetAllPortfolios fetches metadata for all returned portfolios
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Portfolio", gomock.Any()).
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
				assert.Len(t, items, 2, "should have two portfolios")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "portfolio should have id field")
				assert.Contains(t, firstItem, "name", "portfolio should have name field")

				// Validate pagination
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(5), limit)
			},
		},
		{
			name:        "metadata filter returns filtered portfolios",
			queryParams: "?metadata.tier=premium",
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				portfolio1ID := uuid.New().String()
				portfolio2ID := uuid.New().String()

				// MetadataRepo.FindList returns metadata matching the filter
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Portfolio", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: portfolio1ID, Data: map[string]any{"tier": "premium"}},
						{EntityID: portfolio2ID, Data: map[string]any{"tier": "premium"}},
					}, nil).
					Times(1)

				// PortfolioRepo.ListByIDs returns the portfolios
				portfolioRepo.EXPECT().
					ListByIDs(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.Portfolio{
						{
							ID:             portfolio1ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Premium Portfolio One",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             portfolio2ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Premium Portfolio Two",
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
				assert.Len(t, items, 2, "should have two filtered portfolios")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "portfolio should have id field")
				assert.Contains(t, firstItem, "name", "portfolio should have name field")
			},
		},
		{
			name:        "metadata filter with no matching metadata returns 404",
			queryParams: "?metadata.tier=nonexistent",
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// MetadataRepo.FindList returns nil (no matching metadata)
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Portfolio", gomock.Any()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrNoPortfoliosFound.Error(), errResp["code"])
			},
		},
		{
			name:        "metadata filter with portfolios not found returns 404",
			queryParams: "?metadata.tier=premium",
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				portfolio1ID := uuid.New().String()

				// MetadataRepo.FindList returns metadata
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Portfolio", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: portfolio1ID, Data: map[string]any{"tier": "premium"}},
					}, nil).
					Times(1)

				// PortfolioRepo.ListByIDs returns not found error
				portfolioRepo.EXPECT().
					ListByIDs(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrNoPortfoliosFound, reflect.TypeOf(mmodel.Portfolio{}).Name())).
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
			setupMocks: func(portfolioRepo *portfolio.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				portfolioRepo.EXPECT().
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

			mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockPortfolioRepo, mockMetadataRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				PortfolioRepo: mockPortfolioRepo,
				MetadataRepo:  mockMetadataRepo,
			}
			handler := &PortfolioHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.GetAllPortfolios,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios"+tt.queryParams, nil)
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

func TestHandler_DeletePortfolioByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(portfolioRepo *portfolio.MockRepository, orgID, ledgerID, portfolioID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 204 no content",
			setupMocks: func(portfolioRepo *portfolio.MockRepository, orgID, ledgerID, portfolioID uuid.UUID) {
				portfolioRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, portfolioID).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil, // 204 has no body
		},
		{
			name: "not found returns 404",
			setupMocks: func(portfolioRepo *portfolio.MockRepository, orgID, ledgerID, portfolioID uuid.UUID) {
				portfolioRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, portfolioID).
					Return(pkg.ValidateBusinessError(cn.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrPortfolioIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(portfolioRepo *portfolio.MockRepository, orgID, ledgerID, portfolioID uuid.UUID) {
				portfolioRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, portfolioID).
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
			portfolioID := uuid.New()

			mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
			tt.setupMocks(mockPortfolioRepo, orgID, ledgerID, portfolioID)

			cmdUC := &command.UseCase{
				PortfolioRepo: mockPortfolioRepo,
			}
			handler := &PortfolioHandler{Command: cmdUC}

			app := fiber.New()
			app.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", portfolioID)
					return c.Next()
				},
				handler.DeletePortfolioByID,
			)

			// Act
			req := httptest.NewRequest("DELETE", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios/"+portfolioID.String(), nil)
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

func TestHandler_CountPortfolios(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(portfolioRepo *portfolio.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
	}{
		{
			name: "success returns 204 with X-Total-Count header",
			setupMocks: func(portfolioRepo *portfolio.MockRepository, orgID, ledgerID uuid.UUID) {
				portfolioRepo.EXPECT().
					Count(gomock.Any(), orgID, ledgerID).
					Return(int64(42), nil).
					Times(1)
			},
			expectedStatus: 204,
		},
		{
			name: "repository error returns 500",
			setupMocks: func(portfolioRepo *portfolio.MockRepository, orgID, ledgerID uuid.UUID) {
				portfolioRepo.EXPECT().
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

			mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
			tt.setupMocks(mockPortfolioRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				PortfolioRepo: mockPortfolioRepo,
			}
			handler := &PortfolioHandler{Query: queryUC}

			app := fiber.New()
			app.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/metrics/count",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.CountPortfolios,
			)

			// Act
			req := httptest.NewRequest("HEAD", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios/metrics/count", nil)
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
