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
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/segment"
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

func TestHandler_CreateSegment(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.CreateSegmentInput
		setupMocks     func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 201 with created segment",
			payload: &mmodel.CreateSegmentInput{
				Name: "Test Segment",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
			},
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// FindByName check for duplicate names (returns false = name available)
				segmentRepo.EXPECT().
					FindByName(gomock.Any(), orgID, ledgerID, "Test Segment").
					Return(false, nil).
					Times(1)

				segmentRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx any, seg *mmodel.Segment) (*mmodel.Segment, error) {
						seg.ID = uuid.New().String()
						seg.CreatedAt = time.Now()
						seg.UpdatedAt = time.Now()
						return seg, nil
					}).
					Times(1)
				// No metadata in request, so MetadataRepo.Create won't be called
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				// Core identity fields
				assert.Contains(t, result, "id", "response should contain id")
				assert.NotEmpty(t, result["id"], "id should not be empty")

				// Name field
				assert.Contains(t, result, "name", "response should contain name")
				assert.Equal(t, "Test Segment", result["name"])

				// Status field
				assert.Contains(t, result, "status", "response should contain status")
				status, ok := result["status"].(map[string]any)
				require.True(t, ok, "status should be an object")
				assert.Equal(t, "ACTIVE", status["code"], "status code should match input")

				// Relationship fields
				assert.Contains(t, result, "organizationId", "response should contain organizationId")
				assert.Contains(t, result, "ledgerId", "response should contain ledgerId")

				// Timestamp fields
				assert.Contains(t, result, "createdAt", "response should contain createdAt")
				assert.Contains(t, result, "updatedAt", "response should contain updatedAt")
			},
		},
		{
			name: "duplicate name returns 409 conflict",
			payload: &mmodel.CreateSegmentInput{
				Name: "Existing Segment",
			},
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// FindByName returns error for duplicate
				segmentRepo.EXPECT().
					FindByName(gomock.Any(), orgID, ledgerID, "Existing Segment").
					Return(true, pkg.ValidateBusinessError(cn.ErrDuplicateSegmentName, reflect.TypeOf(mmodel.Segment{}).Name(), "Existing Segment", ledgerID)).
					Times(1)
			},
			expectedStatus: 409,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrDuplicateSegmentName.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			payload: &mmodel.CreateSegmentInput{
				Name: "Test Segment",
			},
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				segmentRepo.EXPECT().
					FindByName(gomock.Any(), orgID, ledgerID, "Test Segment").
					Return(false, nil).
					Times(1)

				segmentRepo.EXPECT().
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

			mockSegmentRepo := segment.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockSegmentRepo, mockMetadataRepo, orgID, ledgerID)

			cmdUC := &command.UseCase{
				SegmentRepo:  mockSegmentRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &SegmentHandler{Command: cmdUC}

			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/segments",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				func(c *fiber.Ctx) error {
					return handler.CreateSegment(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("POST", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments", nil)
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

func TestHandler_UpdateSegment(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.UpdateSegmentInput
		setupMocks     func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, segmentID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with updated segment",
			payload: &mmodel.UpdateSegmentInput{
				Name: "Updated Segment Name",
			},
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, segmentID uuid.UUID) {
				// Update succeeds
				segmentRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, segmentID, gomock.Any()).
					Return(&mmodel.Segment{
						ID:             segmentID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Updated Segment Name",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// UpdateMetadata is called
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Segment", segmentID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Retrieval after update
				segmentRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, segmentID).
					Return(&mmodel.Segment{
						ID:             segmentID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Updated Segment Name",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetSegmentByID also fetches metadata
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Segment", segmentID.String()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				// Core identity fields
				assert.Contains(t, result, "id", "response should contain id")
				assert.NotEmpty(t, result["id"], "id should not be empty")

				// Name field - verify update was applied
				assert.Contains(t, result, "name", "response should contain name")
				assert.Equal(t, "Updated Segment Name", result["name"], "name should reflect the update")

				// Status field
				assert.Contains(t, result, "status", "response should contain status")
				status, ok := result["status"].(map[string]any)
				require.True(t, ok, "status should be an object")
				assert.Equal(t, "ACTIVE", status["code"], "status code should be preserved")

				// Relationship fields
				assert.Contains(t, result, "organizationId", "response should contain organizationId")
				assert.Contains(t, result, "ledgerId", "response should contain ledgerId")

				// Timestamp fields
				assert.Contains(t, result, "createdAt", "response should contain createdAt")
				assert.Contains(t, result, "updatedAt", "response should contain updatedAt")
			},
		},
		{
			name: "not found on update returns 404",
			payload: &mmodel.UpdateSegmentInput{
				Name: "Updated Name",
			},
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, segmentID uuid.UUID) {
				segmentRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, segmentID, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrSegmentIDNotFound, reflect.TypeOf(mmodel.Segment{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrSegmentIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "not found on retrieval returns 404",
			payload: &mmodel.UpdateSegmentInput{
				Name: "Updated Name",
			},
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, segmentID uuid.UUID) {
				// Update succeeds
				segmentRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, segmentID, gomock.Any()).
					Return(&mmodel.Segment{ID: segmentID.String()}, nil).
					Times(1)

				// UpdateMetadata succeeds
				metadataRepo.EXPECT().
					Update(gomock.Any(), "Segment", segmentID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Retrieval fails
				segmentRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, segmentID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrSegmentIDNotFound, reflect.TypeOf(mmodel.Segment{}).Name())).
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
			payload: &mmodel.UpdateSegmentInput{
				Name: "Updated Name",
			},
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, segmentID uuid.UUID) {
				segmentRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, segmentID, gomock.Any()).
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
			segmentID := uuid.New()

			mockSegmentRepo := segment.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockSegmentRepo, mockMetadataRepo, orgID, ledgerID, segmentID)

			cmdUC := &command.UseCase{
				SegmentRepo:  mockSegmentRepo,
				MetadataRepo: mockMetadataRepo,
			}
			queryUC := &query.UseCase{
				SegmentRepo:  mockSegmentRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &SegmentHandler{
				Command: cmdUC,
				Query:   queryUC,
			}

			app := fiber.New()
			app.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", segmentID)
					return c.Next()
				},
				func(c *fiber.Ctx) error {
					return handler.UpdateSegment(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("PATCH", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments/"+segmentID.String(), nil)
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

func TestHandler_GetSegmentByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, segmentID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with segment",
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, segmentID uuid.UUID) {
				segmentRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, segmentID).
					Return(&mmodel.Segment{
						ID:             segmentID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Name:           "Test Segment",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetSegmentByID fetches metadata when segment is found
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Segment", segmentID.String()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				// Core identity fields
				assert.Contains(t, result, "id", "response should contain id")
				assert.NotEmpty(t, result["id"], "id should not be empty")

				// Name field
				assert.Contains(t, result, "name", "response should contain name")
				assert.Equal(t, "Test Segment", result["name"])

				// Status field
				assert.Contains(t, result, "status", "response should contain status")
				status, ok := result["status"].(map[string]any)
				require.True(t, ok, "status should be an object")
				assert.Equal(t, "ACTIVE", status["code"], "status code should match")

				// Relationship fields
				assert.Contains(t, result, "organizationId", "response should contain organizationId")
				assert.Contains(t, result, "ledgerId", "response should contain ledgerId")

				// Timestamp fields
				assert.Contains(t, result, "createdAt", "response should contain createdAt")
				assert.Contains(t, result, "updatedAt", "response should contain updatedAt")
			},
		},
		{
			name: "not found returns 404",
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, segmentID uuid.UUID) {
				segmentRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, segmentID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrSegmentIDNotFound, reflect.TypeOf(mmodel.Segment{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrSegmentIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, segmentID uuid.UUID) {
				segmentRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, segmentID).
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
			segmentID := uuid.New()

			mockSegmentRepo := segment.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockSegmentRepo, mockMetadataRepo, orgID, ledgerID, segmentID)

			queryUC := &query.UseCase{
				SegmentRepo:  mockSegmentRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &SegmentHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", segmentID)
					return c.Next()
				},
				handler.GetSegmentByID,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments/"+segmentID.String(), nil)
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

func TestHandler_GetAllSegments(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with pagination structure",
			queryParams: "",
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				segmentRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.Segment{}, nil).
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
			name:        "success with items returns segments",
			queryParams: "?limit=5&page=1",
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				segment1ID := uuid.New().String()
				segment2ID := uuid.New().String()

				segmentRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.Segment{
						{
							ID:             segment1ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Segment One",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             segment2ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Segment Two",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
					}, nil).
					Times(1)

				// GetAllSegments fetches metadata for all returned segments
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Segment", gomock.Any()).
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
				assert.Len(t, items, 2, "should have two segments")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "segment should have id field")
				assert.Contains(t, firstItem, "name", "segment should have name field")

				// Validate pagination
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(5), limit)
			},
		},
		{
			name:        "metadata filter returns filtered segments",
			queryParams: "?metadata.tier=premium",
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				segment1ID := uuid.New().String()
				segment2ID := uuid.New().String()

				// MetadataRepo.FindList returns metadata matching the filter
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Segment", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: segment1ID, Data: map[string]any{"tier": "premium"}},
						{EntityID: segment2ID, Data: map[string]any{"tier": "premium"}},
					}, nil).
					Times(1)

				// SegmentRepo.FindByIDs returns the segments
				segmentRepo.EXPECT().
					FindByIDs(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.Segment{
						{
							ID:             segment1ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Premium Segment One",
							Status:         mmodel.Status{Code: "ACTIVE"},
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             segment2ID,
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Name:           "Premium Segment Two",
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
				assert.Len(t, items, 2, "should have two filtered segments")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "segment should have id field")
				assert.Contains(t, firstItem, "name", "segment should have name field")
			},
		},
		{
			name:        "metadata filter with no matching metadata returns 404",
			queryParams: "?metadata.tier=nonexistent",
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// MetadataRepo.FindList returns nil (no matching metadata)
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Segment", gomock.Any()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrNoSegmentsFound.Error(), errResp["code"])
			},
		},
		{
			name:        "metadata filter with segments not found returns 404",
			queryParams: "?metadata.tier=premium",
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				segment1ID := uuid.New().String()

				// MetadataRepo.FindList returns metadata
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "Segment", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: segment1ID, Data: map[string]any{"tier": "premium"}},
					}, nil).
					Times(1)

				// SegmentRepo.FindByIDs returns not found error
				segmentRepo.EXPECT().
					FindByIDs(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrNoSegmentsFound, reflect.TypeOf(mmodel.Segment{}).Name())).
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
			setupMocks: func(segmentRepo *segment.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				segmentRepo.EXPECT().
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

			mockSegmentRepo := segment.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockSegmentRepo, mockMetadataRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				SegmentRepo:  mockSegmentRepo,
				MetadataRepo: mockMetadataRepo,
			}
			handler := &SegmentHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/segments",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.GetAllSegments,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments"+tt.queryParams, nil)
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

func TestHandler_DeleteSegmentByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(segmentRepo *segment.MockRepository, orgID, ledgerID, segmentID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 204 no content",
			setupMocks: func(segmentRepo *segment.MockRepository, orgID, ledgerID, segmentID uuid.UUID) {
				segmentRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, segmentID).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil, // 204 has no body
		},
		{
			name: "not found returns 404",
			setupMocks: func(segmentRepo *segment.MockRepository, orgID, ledgerID, segmentID uuid.UUID) {
				segmentRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, segmentID).
					Return(pkg.ValidateBusinessError(cn.ErrSegmentIDNotFound, reflect.TypeOf(mmodel.Segment{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrSegmentIDNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(segmentRepo *segment.MockRepository, orgID, ledgerID, segmentID uuid.UUID) {
				segmentRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, segmentID).
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
			segmentID := uuid.New()

			mockSegmentRepo := segment.NewMockRepository(ctrl)
			tt.setupMocks(mockSegmentRepo, orgID, ledgerID, segmentID)

			cmdUC := &command.UseCase{
				SegmentRepo: mockSegmentRepo,
			}
			handler := &SegmentHandler{Command: cmdUC}

			app := fiber.New()
			app.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("id", segmentID)
					return c.Next()
				},
				handler.DeleteSegmentByID,
			)

			// Act
			req := httptest.NewRequest("DELETE", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments/"+segmentID.String(), nil)
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

func TestHandler_CountSegments(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(segmentRepo *segment.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
	}{
		{
			name: "success returns 204 with X-Total-Count header",
			setupMocks: func(segmentRepo *segment.MockRepository, orgID, ledgerID uuid.UUID) {
				segmentRepo.EXPECT().
					Count(gomock.Any(), orgID, ledgerID).
					Return(int64(42), nil).
					Times(1)
			},
			expectedStatus: 204,
		},
		{
			name: "repository error returns 500",
			setupMocks: func(segmentRepo *segment.MockRepository, orgID, ledgerID uuid.UUID) {
				segmentRepo.EXPECT().
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

			mockSegmentRepo := segment.NewMockRepository(ctrl)
			tt.setupMocks(mockSegmentRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				SegmentRepo: mockSegmentRepo,
			}
			handler := &SegmentHandler{Query: queryUC}

			app := fiber.New()
			app.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/metrics/count",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.CountSegments,
			)

			// Act
			req := httptest.NewRequest("HEAD", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments/metrics/count", nil)
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
