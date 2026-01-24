package in

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services"
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

func TestHolderHandler_CreateHolder(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		setupMocks     func(holderRepo *holder.MockRepository, orgID string)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 201 with created holder",
			jsonBody: `{
				"type": "NATURAL_PERSON",
				"name": "John Doe",
				"document": "91315026015"
			}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				holderRepo.EXPECT().
					Create(gomock.Any(), orgID, gomock.Cond(func(x any) bool {
						h, ok := x.(*mmodel.Holder)
						if !ok {
							return false
						}
						// Validate required fields
						return h.Type != nil && *h.Type == "NATURAL_PERSON" &&
							h.Name != nil && *h.Name == "John Doe" &&
							h.Document != nil && *h.Document == "91315026015"
					})).
					DoAndReturn(func(ctx any, org string, h *mmodel.Holder) (*mmodel.Holder, error) {
						h.CreatedAt = time.Now()
						h.UpdatedAt = time.Now()
						return h, nil
					}).
					Times(1)
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "name", "response should contain name")
				assert.Equal(t, "John Doe", result["name"])
				assert.Contains(t, result, "type", "response should contain type")
				assert.Equal(t, "NATURAL_PERSON", result["type"])
				assert.Contains(t, result, "document", "response should contain document")
				assert.Equal(t, "91315026015", result["document"])
			},
		},
		{
			name: "success with optional fields returns 201",
			jsonBody: `{
				"type": "NATURAL_PERSON",
				"name": "John Doe",
				"document": "91315026015",
				"externalId": "EXT-123",
				"metadata": {"key": "value"}
			}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				holderRepo.EXPECT().
					Create(gomock.Any(), orgID, gomock.Cond(func(x any) bool {
						h, ok := x.(*mmodel.Holder)
						if !ok {
							return false
						}
						// Validate required fields
						if h.Type == nil || *h.Type != "NATURAL_PERSON" ||
							h.Name == nil || *h.Name != "John Doe" ||
							h.Document == nil || *h.Document != "91315026015" {
							return false
						}
						// Validate optional fields
						if h.ExternalID == nil || *h.ExternalID != "EXT-123" {
							return false
						}
						if h.Metadata == nil || h.Metadata["key"] != "value" {
							return false
						}
						return true
					})).
					DoAndReturn(func(ctx any, org string, h *mmodel.Holder) (*mmodel.Holder, error) {
						h.CreatedAt = time.Now()
						h.UpdatedAt = time.Now()
						return h, nil
					}).
					Times(1)
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "externalId", "response should contain externalId")
				assert.Equal(t, "EXT-123", result["externalId"])
				assert.Contains(t, result, "metadata", "response should contain metadata")
				metadata, ok := result["metadata"].(map[string]any)
				require.True(t, ok, "metadata should be an object")
				assert.Equal(t, "value", metadata["key"])
			},
		},
		{
			name: "repository error returns 500",
			jsonBody: `{
				"type": "NATURAL_PERSON",
				"name": "John Doe",
				"document": "91315026015"
			}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				holderRepo.EXPECT().
					Create(gomock.Any(), orgID, gomock.Any()).
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
		{
			name: "missing type field returns 400",
			jsonBody: `{
				"name": "John Doe",
				"document": "91315026015"
			}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				// No mock expectations - validation should fail before reaching repository
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
		{
			name: "missing name field returns 400",
			jsonBody: `{
				"type": "NATURAL_PERSON",
				"document": "91315026015"
			}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				// No mock expectations - validation should fail before reaching repository
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
		{
			name: "missing document field returns 400",
			jsonBody: `{
				"type": "NATURAL_PERSON",
				"name": "John Doe"
			}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				// No mock expectations - validation should fail before reaching repository
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
		{
			name: "oversized metadata key returns 400",
			jsonBody: `{
				"type": "NATURAL_PERSON",
				"name": "John Doe",
				"document": "91315026015",
				"metadata": {"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": "value"}
			}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				// No mock expectations - validation should fail before reaching repository
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
		{
			name: "oversized metadata value returns 400",
			jsonBody: `{
				"type": "NATURAL_PERSON",
				"name": "John Doe",
				"document": "91315026015",
				"metadata": {"key": "` + strings.Repeat("a", 2001) + `"}
			}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				// No mock expectations - validation should fail before reaching repository
			},
			expectedStatus: 400,
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

			orgID := uuid.New().String()

			mockHolderRepo := holder.NewMockRepository(ctrl)
			tt.setupMocks(mockHolderRepo, orgID)

			uc := &services.UseCase{
				HolderRepo: mockHolderRepo,
			}
			handler := &HolderHandler{Service: uc}

			app := fiber.New()
			app.Post("/v1/holders",
				func(c *fiber.Ctx) error {
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				http.WithBody(new(mmodel.CreateHolderInput), handler.CreateHolder),
			)

			req := httptest.NewRequest("POST", "/v1/holders", bytes.NewBufferString(tt.jsonBody))
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

func TestHolderHandler_GetHolderByID(t *testing.T) {
	tests := []struct {
		name           string
		includeDeleted string
		setupMocks     func(holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:           "success returns 200 with holder",
			includeDeleted: "",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				holderType := "NATURAL_PERSON"
				holderName := "John Doe"
				document := "91315026015"

				holderRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, false).
					Return(&mmodel.Holder{
						ID:        &holderID,
						Type:      &holderType,
						Name:      &holderName,
						Document:  &document,
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "name", "response should contain name")
				assert.Contains(t, result, "type", "response should contain type")
				assert.Contains(t, result, "document", "response should contain document")
			},
		},
		{
			name:           "success with include_deleted returns 200",
			includeDeleted: "true",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				holderType := "NATURAL_PERSON"
				holderName := "John Doe"
				document := "91315026015"
				deletedAt := time.Now()

				holderRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, true).
					Return(&mmodel.Holder{
						ID:        &holderID,
						Type:      &holderType,
						Name:      &holderName,
						Document:  &document,
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
						DeletedAt: &deletedAt,
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "deletedAt", "response should contain deletedAt")
			},
		},
		{
			name:           "not found returns 404",
			includeDeleted: "",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				holderRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, false).
					Return(nil, pkg.ValidateBusinessError(cn.ErrHolderNotFound, reflect.TypeOf(mmodel.Holder{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrHolderNotFound.Error(), errResp["code"])
			},
		},
		{
			name:           "repository error returns 500",
			includeDeleted: "",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				holderRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, false).
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

			orgID := uuid.New().String()
			holderID := uuid.New()

			mockHolderRepo := holder.NewMockRepository(ctrl)
			tt.setupMocks(mockHolderRepo, orgID, holderID)

			uc := &services.UseCase{
				HolderRepo: mockHolderRepo,
			}
			handler := &HolderHandler{Service: uc}

			app := fiber.New()
			app.Get("/v1/holders/:id",
				func(c *fiber.Ctx) error {
					c.Locals("id", holderID)
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				handler.GetHolderByID,
			)

			url := "/v1/holders/" + holderID.String()
			if tt.includeDeleted != "" {
				url += "?include_deleted=" + tt.includeDeleted
			}
			req := httptest.NewRequest("GET", url, nil)
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

func TestHolderHandler_UpdateHolder(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		setupMocks     func(holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with updated holder",
			jsonBody: `{
				"name": "Jane Doe",
				"metadata": {"key": "value"}
			}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				holderType := "NATURAL_PERSON"
				holderName := "Jane Doe"
				document := "91315026015"

				holderRepo.EXPECT().
					Update(gomock.Any(), orgID, holderID, gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{
						ID:        &holderID,
						Type:      &holderType,
						Name:      &holderName,
						Document:  &document,
						Metadata:  map[string]any{"key": "value"},
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "name", "response should contain name")
				assert.Equal(t, "Jane Doe", result["name"])
				assert.Contains(t, result, "metadata", "response should contain metadata")
				metadata, ok := result["metadata"].(map[string]any)
				require.True(t, ok, "metadata should be an object")
				assert.Equal(t, "value", metadata["key"])
			},
		},
		{
			name: "success with external id update returns 200",
			jsonBody: `{
				"externalId": "NEW-EXT-123"
			}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				holderType := "NATURAL_PERSON"
				holderName := "John Doe"
				document := "91315026015"
				externalID := "NEW-EXT-123"

				holderRepo.EXPECT().
					Update(gomock.Any(), orgID, holderID, gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{
						ID:         &holderID,
						Type:       &holderType,
						Name:       &holderName,
						Document:   &document,
						ExternalID: &externalID,
						CreatedAt:  time.Now(),
						UpdatedAt:  time.Now(),
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "externalId", "response should contain externalId")
				assert.Equal(t, "NEW-EXT-123", result["externalId"])
			},
		},
		{
			name: "not found returns 404",
			jsonBody: `{
				"name": "Jane Doe"
			}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				holderRepo.EXPECT().
					Update(gomock.Any(), orgID, holderID, gomock.Any(), gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrHolderNotFound, reflect.TypeOf(mmodel.Holder{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrHolderNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			jsonBody: `{
				"name": "Jane Doe"
			}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				holderRepo.EXPECT().
					Update(gomock.Any(), orgID, holderID, gomock.Any(), gomock.Any()).
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

			orgID := uuid.New().String()
			holderID := uuid.New()

			mockHolderRepo := holder.NewMockRepository(ctrl)
			tt.setupMocks(mockHolderRepo, orgID, holderID)

			uc := &services.UseCase{
				HolderRepo: mockHolderRepo,
			}
			handler := &HolderHandler{Service: uc}

			app := fiber.New()
			app.Patch("/v1/holders/:id",
				func(c *fiber.Ctx) error {
					c.Locals("id", holderID)
					c.Locals("patchRemove", []string{})
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				http.WithBody(new(mmodel.UpdateHolderInput), handler.UpdateHolder),
			)

			req := httptest.NewRequest("PATCH", "/v1/holders/"+holderID.String(), bytes.NewBufferString(tt.jsonBody))
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

func TestHolderHandler_DeleteHolderByID(t *testing.T) {
	tests := []struct {
		name           string
		hardDelete     string
		setupMocks     func(aliasRepo *alias.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:       "success soft delete returns 204",
			hardDelete: "",
			setupMocks: func(aliasRepo *alias.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				aliasRepo.EXPECT().
					Count(gomock.Any(), orgID, holderID).
					Return(int64(0), nil).
					Times(1)

				holderRepo.EXPECT().
					Delete(gomock.Any(), orgID, holderID, false).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil,
		},
		{
			name:       "success hard delete returns 204",
			hardDelete: "true",
			setupMocks: func(aliasRepo *alias.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				aliasRepo.EXPECT().
					Count(gomock.Any(), orgID, holderID).
					Return(int64(0), nil).
					Times(1)

				holderRepo.EXPECT().
					Delete(gomock.Any(), orgID, holderID, true).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil,
		},
		{
			name:       "holder has aliases returns 400",
			hardDelete: "",
			setupMocks: func(aliasRepo *alias.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				aliasRepo.EXPECT().
					Count(gomock.Any(), orgID, holderID).
					Return(int64(1), nil).
					Times(1)
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrHolderHasAliases.Error(), errResp["code"])
			},
		},
		{
			name:       "not found returns 404",
			hardDelete: "",
			setupMocks: func(aliasRepo *alias.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				aliasRepo.EXPECT().
					Count(gomock.Any(), orgID, holderID).
					Return(int64(0), nil).
					Times(1)

				holderRepo.EXPECT().
					Delete(gomock.Any(), orgID, holderID, false).
					Return(pkg.ValidateBusinessError(cn.ErrHolderNotFound, reflect.TypeOf(mmodel.Holder{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrHolderNotFound.Error(), errResp["code"])
			},
		},
		{
			name:       "repository error on count returns 500",
			hardDelete: "",
			setupMocks: func(aliasRepo *alias.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				aliasRepo.EXPECT().
					Count(gomock.Any(), orgID, holderID).
					Return(int64(0), pkg.InternalServerError{
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
		{
			name:       "repository error on delete returns 500",
			hardDelete: "",
			setupMocks: func(aliasRepo *alias.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				aliasRepo.EXPECT().
					Count(gomock.Any(), orgID, holderID).
					Return(int64(0), nil).
					Times(1)

				holderRepo.EXPECT().
					Delete(gomock.Any(), orgID, holderID, false).
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

			orgID := uuid.New().String()
			holderID := uuid.New()

			mockAliasRepo := alias.NewMockRepository(ctrl)
			mockHolderRepo := holder.NewMockRepository(ctrl)
			tt.setupMocks(mockAliasRepo, mockHolderRepo, orgID, holderID)

			uc := &services.UseCase{
				AliasRepo:  mockAliasRepo,
				HolderRepo: mockHolderRepo,
			}
			handler := &HolderHandler{Service: uc}

			app := fiber.New()
			app.Delete("/v1/holders/:id",
				func(c *fiber.Ctx) error {
					c.Locals("id", holderID)
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				handler.DeleteHolderByID,
			)

			url := "/v1/holders/" + holderID.String()
			if tt.hardDelete != "" {
				url += "?hard_delete=" + tt.hardDelete
			}
			req := httptest.NewRequest("DELETE", url, nil)
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

func TestHolderHandler_GetAllHolders(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(holderRepo *holder.MockRepository, orgID string)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with pagination structure",
			queryParams: "",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				holderRepo.EXPECT().
					FindAll(gomock.Any(), orgID, gomock.Any(), false).
					Return([]*mmodel.Holder{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(10), limit)

				page, ok := result["page"].(float64)
				require.True(t, ok, "page should be a number")
				assert.Equal(t, float64(1), page)
			},
		},
		{
			name:        "success with items returns holders",
			queryParams: "?limit=5&page=1",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				holder1ID := uuid.New()
				holder2ID := uuid.New()
				holderType := "NATURAL_PERSON"
				name1 := "John Doe"
				name2 := "Jane Doe"
				document1 := "91315026015"
				document2 := "91315026016"

				holderRepo.EXPECT().
					FindAll(gomock.Any(), orgID, gomock.Any(), false).
					Return([]*mmodel.Holder{
						{
							ID:        &holder1ID,
							Type:      &holderType,
							Name:      &name1,
							Document:  &document1,
							CreatedAt: time.Now(),
							UpdatedAt: time.Now(),
						},
						{
							ID:        &holder2ID,
							Type:      &holderType,
							Name:      &name2,
							Document:  &document2,
							CreatedAt: time.Now(),
							UpdatedAt: time.Now(),
						},
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				items, ok := result["items"].([]any)
				require.True(t, ok, "items should be an array")
				assert.Len(t, items, 2, "should have two holders")

				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "holder should have id field")
				assert.Contains(t, firstItem, "name", "holder should have name field")
				assert.Contains(t, firstItem, "type", "holder should have type field")
				assert.Contains(t, firstItem, "document", "holder should have document field")

				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(5), limit)
			},
		},
		{
			name:        "success with include_deleted returns deleted holders",
			queryParams: "?include_deleted=true",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				holderID := uuid.New()
				holderType := "NATURAL_PERSON"
				holderName := "John Doe"
				document := "91315026015"
				deletedAt := time.Now()

				holderRepo.EXPECT().
					FindAll(gomock.Any(), orgID, gomock.Any(), true).
					Return([]*mmodel.Holder{
						{
							ID:        &holderID,
							Type:      &holderType,
							Name:      &holderName,
							Document:  &document,
							CreatedAt: time.Now(),
							UpdatedAt: time.Now(),
							DeletedAt: &deletedAt,
						},
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				items, ok := result["items"].([]any)
				require.True(t, ok, "items should be an array")
				assert.Len(t, items, 1, "should have one holder")

				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "deletedAt", "holder should have deletedAt field")
			},
		},
		{
			name:        "success with sort_order param",
			queryParams: "?sort_order=desc",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				holderRepo.EXPECT().
					FindAll(gomock.Any(), orgID, gomock.Any(), false).
					Return([]*mmodel.Holder{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				// sortOrder is not serialized in JSON response (json:"-" tag)
				// but the handler should still process it successfully
				assert.Contains(t, result, "limit", "response should contain limit")
				assert.Contains(t, result, "page", "response should contain page")
			},
		},
		{
			name:        "repository error returns 500",
			queryParams: "",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				holderRepo.EXPECT().
					FindAll(gomock.Any(), orgID, gomock.Any(), false).
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
		{
			name:        "invalid sort_order returns 400",
			queryParams: "?sort_order=invalid",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				// No mock expectations - validation should fail before reaching repository
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
		{
			name:        "limit exceeds max returns 400",
			queryParams: "?limit=101",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				// No mock expectations - validation should fail before reaching repository
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
		{
			name:        "zero limit passes through to repository",
			queryParams: "?limit=0",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				holderRepo.EXPECT().
					FindAll(gomock.Any(), orgID, gomock.Cond(func(x any) bool {
						params, ok := x.(http.QueryHeader)
						return ok && params.Limit == 0
					}), false).
					Return([]*mmodel.Holder{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(0), limit)
			},
		},
		{
			name:        "negative limit passes through to repository",
			queryParams: "?limit=-5",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				holderRepo.EXPECT().
					FindAll(gomock.Any(), orgID, gomock.Cond(func(x any) bool {
						params, ok := x.(http.QueryHeader)
						return ok && params.Limit == -5
					}), false).
					Return([]*mmodel.Holder{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(-5), limit)
			},
		},
		{
			name:        "negative page passes through to repository",
			queryParams: "?page=-1",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				holderRepo.EXPECT().
					FindAll(gomock.Any(), orgID, gomock.Cond(func(x any) bool {
						params, ok := x.(http.QueryHeader)
						return ok && params.Page == -1
					}), false).
					Return([]*mmodel.Holder{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				page, ok := result["page"].(float64)
				require.True(t, ok, "page should be a number")
				assert.Equal(t, float64(-1), page)
			},
		},
		{
			name:        "non-numeric limit becomes zero",
			queryParams: "?limit=abc",
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				holderRepo.EXPECT().
					FindAll(gomock.Any(), orgID, gomock.Cond(func(x any) bool {
						params, ok := x.(http.QueryHeader)
						return ok && params.Limit == 0
					}), false).
					Return([]*mmodel.Holder{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(0), limit)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New().String()

			mockHolderRepo := holder.NewMockRepository(ctrl)
			tt.setupMocks(mockHolderRepo, orgID)

			uc := &services.UseCase{
				HolderRepo: mockHolderRepo,
			}
			handler := &HolderHandler{Service: uc}

			app := fiber.New()
			app.Get("/v1/holders",
				func(c *fiber.Ctx) error {
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				handler.GetAllHolders,
			)

			req := httptest.NewRequest("GET", "/v1/holders"+tt.queryParams, nil)
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

