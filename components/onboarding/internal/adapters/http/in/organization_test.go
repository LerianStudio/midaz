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
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/organization"
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

func TestHandler_CreateOrganization(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.CreateOrganizationInput
		setupMocks     func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 201 with created organization",
			payload: &mmodel.CreateOrganizationInput{
				LegalName:     "Test Organization",
				LegalDocument: "12345678901234",
				Address: mmodel.Address{
					Country: "US",
				},
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
			},
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository) {
				orgRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx any, org *mmodel.Organization) (*mmodel.Organization, error) {
						org.ID = uuid.New().String()
						org.CreatedAt = time.Now()
						org.UpdatedAt = time.Now()
						return org, nil
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
				assert.Contains(t, result, "legalName", "response should contain legalName")
				assert.Equal(t, "Test Organization", result["legalName"])
			},
		},
		{
			name: "invalid country code returns 400",
			payload: &mmodel.CreateOrganizationInput{
				LegalName:     "Test Organization",
				LegalDocument: "12345678901234",
				Address: mmodel.Address{
					Country: "INVALID",
				},
			},
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository) {
				// No repo calls expected - validation fails first
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrInvalidCountryCode.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			payload: &mmodel.CreateOrganizationInput{
				LegalName:     "Test Organization",
				LegalDocument: "12345678901234",
				Address: mmodel.Address{
					Country: "BR",
				},
			},
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository) {
				orgRepo.EXPECT().
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
			mockOrgRepo := organization.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockOrgRepo, mockMetadataRepo)

			cmdUC := &command.UseCase{
				OrganizationRepo: mockOrgRepo,
				MetadataRepo:     mockMetadataRepo,
			}
			handler := &OrganizationHandler{Command: cmdUC}

			app := fiber.New()
			app.Post("/v1/organizations",
				func(c *fiber.Ctx) error {
					return handler.CreateOrganization(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("POST", "/v1/organizations", nil)
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

func TestHandler_UpdateOrganization(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.UpdateOrganizationInput
		setupMocks     func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository, id uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with updated organization",
			payload: &mmodel.UpdateOrganizationInput{
				LegalName: "Updated Organization Name",
			},
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository, id uuid.UUID) {
				// Update succeeds
				orgRepo.EXPECT().
					Update(gomock.Any(), id, gomock.Any()).
					Return(&mmodel.Organization{
						ID:        id.String(),
						LegalName: "Updated Organization Name",
						Status:    mmodel.Status{Code: "ACTIVE"},
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)

				// UpdateMetadata is called (metadata is nil, so it calls Update with empty map)
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Organization", id.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Retrieval after update
				orgRepo.EXPECT().
					Find(gomock.Any(), id).
					Return(&mmodel.Organization{
						ID:        id.String(),
						LegalName: "Updated Organization Name",
						Status:    mmodel.Status{Code: "ACTIVE"},
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)

				// GetOrganizationByID also fetches metadata
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Organization", id.String()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "legalName", "response should contain legalName")
				assert.Equal(t, "Updated Organization Name", result["legalName"])
			},
		},
		{
			name: "not found on update returns 404",
			payload: &mmodel.UpdateOrganizationInput{
				LegalName: "Updated Name",
			},
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository, id uuid.UUID) {
				orgRepo.EXPECT().
					Update(gomock.Any(), id, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrEntityNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "not found on retrieval returns 404",
			payload: &mmodel.UpdateOrganizationInput{
				LegalName: "Updated Name",
			},
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository, id uuid.UUID) {
				// Update succeeds
				orgRepo.EXPECT().
					Update(gomock.Any(), id, gomock.Any()).
					Return(&mmodel.Organization{ID: id.String()}, nil).
					Times(1)

				// UpdateMetadata succeeds
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Organization", id.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Retrieval fails
				orgRepo.EXPECT().
					Find(gomock.Any(), id).
					Return(nil, pkg.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())).
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
			payload: &mmodel.UpdateOrganizationInput{
				LegalName: "Updated Name",
			},
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository, id uuid.UUID) {
				orgRepo.EXPECT().
					Update(gomock.Any(), id, gomock.Any()).
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

			mockOrgRepo := organization.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockOrgRepo, mockMetadataRepo, orgID)

			cmdUC := &command.UseCase{
				OrganizationRepo: mockOrgRepo,
				MetadataRepo:     mockMetadataRepo,
			}
			queryUC := &query.UseCase{
				OrganizationRepo: mockOrgRepo,
				MetadataRepo:     mockMetadataRepo,
			}
			handler := &OrganizationHandler{
				Command: cmdUC,
				Query:   queryUC,
			}

			app := fiber.New()
			app.Patch("/v1/organizations/:id",
				func(c *fiber.Ctx) error {
					c.Locals("id", orgID)
					return c.Next()
				},
				func(c *fiber.Ctx) error {
					return handler.UpdateOrganization(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("PATCH", "/v1/organizations/"+orgID.String(), nil)
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

func TestHandler_GetOrganizationByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository, id uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with organization",
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository, id uuid.UUID) {
				orgRepo.EXPECT().
					Find(gomock.Any(), id).
					Return(&mmodel.Organization{
						ID:            id.String(),
						LegalName:     "Test Organization",
						LegalDocument: "12345678901234",
						Status:        mmodel.Status{Code: "ACTIVE"},
						CreatedAt:     time.Now(),
						UpdatedAt:     time.Now(),
					}, nil).
					Times(1)

				// GetOrganizationByID fetches metadata when org is found
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Organization", id.String()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "legalName", "response should contain legalName")
				assert.Equal(t, "Test Organization", result["legalName"])
			},
		},
		{
			name: "not found returns 404",
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository, id uuid.UUID) {
				orgRepo.EXPECT().
					Find(gomock.Any(), id).
					Return(nil, pkg.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrEntityNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository, id uuid.UUID) {
				orgRepo.EXPECT().
					Find(gomock.Any(), id).
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

			mockOrgRepo := organization.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockOrgRepo, mockMetadataRepo, orgID)

			queryUC := &query.UseCase{
				OrganizationRepo: mockOrgRepo,
				MetadataRepo:     mockMetadataRepo,
			}
			handler := &OrganizationHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:id",
				func(c *fiber.Ctx) error {
					c.Locals("id", orgID)
					return c.Next()
				},
				handler.GetOrganizationByID,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String(), nil)
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

func TestHandler_GetAllOrganizations(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with pagination structure",
			queryParams: "",
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository) {
				orgRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any()).
					Return([]*mmodel.Organization{}, nil).
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
			name:        "success with items returns organizations",
			queryParams: "?limit=5&page=1",
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository) {
				org1ID := uuid.New().String()
				org2ID := uuid.New().String()

				orgRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any()).
					Return([]*mmodel.Organization{
						{
							ID:            org1ID,
							LegalName:     "Org One",
							LegalDocument: "11111111111111",
							Status:        mmodel.Status{Code: "ACTIVE"},
							CreatedAt:     time.Now(),
							UpdatedAt:     time.Now(),
						},
						{
							ID:            org2ID,
							LegalName:     "Org Two",
							LegalDocument: "22222222222222",
							Status:        mmodel.Status{Code: "ACTIVE"},
							CreatedAt:     time.Now(),
							UpdatedAt:     time.Now(),
						},
					}, nil).
					Times(1)

				// GetAllOrganizations fetches metadata for all returned organizations
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Organization", gomock.Any()).
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
				assert.Len(t, items, 2, "should have two organizations")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "organization should have id field")
				assert.Contains(t, firstItem, "legalName", "organization should have legalName field")

				// Validate pagination
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(5), limit)
			},
		},
		{
			name:        "metadata filter returns filtered organizations",
			queryParams: "?metadata.tier=premium",
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository) {
				org1ID := uuid.New().String()
				org2ID := uuid.New().String()

				// MetadataRepo.FindList returns metadata matching the filter
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Organization", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: org1ID, Data: map[string]any{"tier": "premium"}},
						{EntityID: org2ID, Data: map[string]any{"tier": "premium"}},
					}, nil).
					Times(1)

				// OrganizationRepo.ListByIDs returns the organizations
				orgRepo.EXPECT().
					ListByIDs(gomock.Any(), gomock.Any()).
					Return([]*mmodel.Organization{
						{
							ID:            org1ID,
							LegalName:     "Premium Org One",
							LegalDocument: "11111111111111",
							Status:        mmodel.Status{Code: "ACTIVE"},
							CreatedAt:     time.Now(),
							UpdatedAt:     time.Now(),
						},
						{
							ID:            org2ID,
							LegalName:     "Premium Org Two",
							LegalDocument: "22222222222222",
							Status:        mmodel.Status{Code: "ACTIVE"},
							CreatedAt:     time.Now(),
							UpdatedAt:     time.Now(),
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
				assert.Len(t, items, 2, "should have two filtered organizations")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "organization should have id field")
				assert.Contains(t, firstItem, "legalName", "organization should have legalName field")
			},
		},
		{
			name:        "metadata filter with no matching metadata returns 404",
			queryParams: "?metadata.tier=nonexistent",
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository) {
				// MetadataRepo.FindList returns nil (no matching metadata)
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Organization", gomock.Any()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrNoOrganizationsFound.Error(), errResp["code"])
			},
		},
		{
			name:        "metadata filter with organizations not found returns 404",
			queryParams: "?metadata.tier=premium",
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository) {
				org1ID := uuid.New().String()

				// MetadataRepo.FindList returns metadata
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Organization", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: org1ID, Data: map[string]any{"tier": "premium"}},
					}, nil).
					Times(1)

				// OrganizationRepo.ListByIDs returns not found error
				orgRepo.EXPECT().
					ListByIDs(gomock.Any(), gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrNoOrganizationsFound, reflect.TypeOf(mmodel.Organization{}).Name())).
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
			setupMocks: func(orgRepo *organization.MockRepository, metadataRepo *mongodb.MockRepository) {
				orgRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any()).
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
			mockOrgRepo := organization.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockOrgRepo, mockMetadataRepo)

			queryUC := &query.UseCase{
				OrganizationRepo: mockOrgRepo,
				MetadataRepo:     mockMetadataRepo,
			}
			handler := &OrganizationHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations", handler.GetAllOrganizations)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations"+tt.queryParams, nil)
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

func TestHandler_DeleteOrganizationByID(t *testing.T) {
	tests := []struct {
		name           string
		envName        string
		setupMocks     func(orgRepo *organization.MockRepository, id uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:    "success returns 204 no content",
			envName: "development",
			setupMocks: func(orgRepo *organization.MockRepository, id uuid.UUID) {
				orgRepo.EXPECT().
					Delete(gomock.Any(), id).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil, // 204 has no body
		},
		{
			name:    "not found returns 404",
			envName: "development",
			setupMocks: func(orgRepo *organization.MockRepository, id uuid.UUID) {
				orgRepo.EXPECT().
					Delete(gomock.Any(), id).
					Return(pkg.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrEntityNotFound.Error(), errResp["code"])
			},
		},
		{
			name:    "production environment returns 400 validation error",
			envName: "production",
			setupMocks: func(orgRepo *organization.MockRepository, id uuid.UUID) {
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
			setupMocks: func(orgRepo *organization.MockRepository, id uuid.UUID) {
				orgRepo.EXPECT().
					Delete(gomock.Any(), id).
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

			mockOrgRepo := organization.NewMockRepository(ctrl)
			tt.setupMocks(mockOrgRepo, orgID)

			cmdUC := &command.UseCase{
				OrganizationRepo: mockOrgRepo,
			}
			handler := &OrganizationHandler{Command: cmdUC}

			app := fiber.New()
			app.Delete("/v1/organizations/:id",
				func(c *fiber.Ctx) error {
					c.Locals("id", orgID)
					return c.Next()
				},
				handler.DeleteOrganizationByID,
			)

			// Act
			req := httptest.NewRequest("DELETE", "/v1/organizations/"+orgID.String(), nil)
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

func TestHandler_CountOrganizations(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(orgRepo *organization.MockRepository)
		expectedStatus int
	}{
		{
			name: "success returns 204 with X-Total-Count header",
			setupMocks: func(orgRepo *organization.MockRepository) {
				orgRepo.EXPECT().
					Count(gomock.Any()).
					Return(int64(42), nil).
					Times(1)
			},
			expectedStatus: 204,
		},
		{
			name: "repository error returns 500",
			setupMocks: func(orgRepo *organization.MockRepository) {
				orgRepo.EXPECT().
					Count(gomock.Any()).
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
			mockOrgRepo := organization.NewMockRepository(ctrl)
			tt.setupMocks(mockOrgRepo)

			queryUC := &query.UseCase{
				OrganizationRepo: mockOrgRepo,
			}
			handler := &OrganizationHandler{Query: queryUC}

			app := fiber.New()
			app.Head("/v1/organizations/metrics/count", handler.CountOrganizations)

			// Act
			req := httptest.NewRequest("HEAD", "/v1/organizations/metrics/count", nil)
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
