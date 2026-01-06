package in

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
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

func TestAliasHandler_CreateAlias(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		setupMocks     func(aliasRepo *alias.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 201 with created alias",
			jsonBody: `{
				"ledgerId": "00000000-0000-0000-0000-000000000001",
				"accountId": "00000000-0000-0000-0000-000000000002"
			}`,
			setupMocks: func(aliasRepo *alias.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				document := "12345678901"
				holderType := "individual"

				holderRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, false).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &document,
						Type:     &holderType,
					}, nil).
					Times(1)

				aliasRepo.EXPECT().
					Create(gomock.Any(), orgID, gomock.Any()).
					DoAndReturn(func(ctx any, org string, a *mmodel.Alias) (*mmodel.Alias, error) {
						a.CreatedAt = time.Now()
						a.UpdatedAt = time.Now()
						return a, nil
					}).
					Times(1)
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "ledgerId", "response should contain ledgerId")
				assert.Equal(t, "00000000-0000-0000-0000-000000000001", result["ledgerId"])
			},
		},
		{
			name: "holder not found returns 404",
			jsonBody: `{
				"ledgerId": "00000000-0000-0000-0000-000000000001",
				"accountId": "00000000-0000-0000-0000-000000000002"
			}`,
			setupMocks: func(aliasRepo *alias.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
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
			name: "repository error returns 500",
			jsonBody: `{
				"ledgerId": "00000000-0000-0000-0000-000000000001",
				"accountId": "00000000-0000-0000-0000-000000000002"
			}`,
			setupMocks: func(aliasRepo *alias.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				document := "12345678901"
				holderType := "individual"

				holderRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, false).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &document,
						Type:     &holderType,
					}, nil).
					Times(1)

				aliasRepo.EXPECT().
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
			handler := &AliasHandler{Service: uc}

			app := fiber.New()
			app.Post("/v1/holders/:holder_id/aliases",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				http.WithBody(new(mmodel.CreateAliasInput), handler.CreateAlias),
			)

			req := httptest.NewRequest("POST", "/v1/holders/"+holderID.String()+"/aliases", bytes.NewBufferString(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Organization-Id", orgID)
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

func TestAliasHandler_GetAliasByID(t *testing.T) {
	tests := []struct {
		name           string
		includeDeleted string
		setupMocks     func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:           "success returns 200 with alias",
			includeDeleted: "",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID) {
				ledgerID := "00000000-0000-0000-0000-000000000001"
				accountID := "00000000-0000-0000-0000-000000000002"
				document := "12345678901"
				holderType := "individual"

				aliasRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, aliasID, false).
					Return(&mmodel.Alias{
						ID:        &aliasID,
						LedgerID:  &ledgerID,
						AccountID: &accountID,
						HolderID:  &holderID,
						Document:  &document,
						Type:      &holderType,
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
				assert.Contains(t, result, "ledgerId", "response should contain ledgerId")
				assert.Contains(t, result, "accountId", "response should contain accountId")
			},
		},
		{
			name:           "not found returns 404",
			includeDeleted: "",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID) {
				aliasRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, aliasID, false).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAliasNotFound.Error(), errResp["code"])
			},
		},
		{
			name:           "repository error returns 500",
			includeDeleted: "",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID) {
				aliasRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, aliasID, false).
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
			aliasID := uuid.New()

			mockAliasRepo := alias.NewMockRepository(ctrl)
			tt.setupMocks(mockAliasRepo, orgID, holderID, aliasID)

			uc := &services.UseCase{
				AliasRepo: mockAliasRepo,
			}
			handler := &AliasHandler{Service: uc}

			app := fiber.New()
			app.Get("/v1/holders/:holder_id/aliases/:id",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Locals("id", aliasID)
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				handler.GetAliasByID,
			)

			url := "/v1/holders/" + holderID.String() + "/aliases/" + aliasID.String()
			if tt.includeDeleted != "" {
				url += "?include_deleted=" + tt.includeDeleted
			}
			req := httptest.NewRequest("GET", url, nil)
			req.Header.Set("X-Organization-Id", orgID)
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

func TestAliasHandler_UpdateAlias(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		setupMocks     func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with updated alias",
			jsonBody: `{
				"metadata": {"key": "value"}
			}`,
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID) {
				ledgerID := "00000000-0000-0000-0000-000000000001"
				accountID := "00000000-0000-0000-0000-000000000002"
				document := "12345678901"
				holderType := "individual"

				aliasRepo.EXPECT().
					Update(gomock.Any(), orgID, holderID, aliasID, gomock.Any(), gomock.Any()).
					Return(&mmodel.Alias{
						ID:        &aliasID,
						LedgerID:  &ledgerID,
						AccountID: &accountID,
						HolderID:  &holderID,
						Document:  &document,
						Type:      &holderType,
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
				assert.Contains(t, result, "metadata", "response should contain metadata")
				metadata, ok := result["metadata"].(map[string]any)
				require.True(t, ok, "metadata should be an object")
				assert.Equal(t, "value", metadata["key"])
			},
		},
		{
			name: "not found returns 404",
			jsonBody: `{
				"metadata": {"key": "value"}
			}`,
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID) {
				aliasRepo.EXPECT().
					Update(gomock.Any(), orgID, holderID, aliasID, gomock.Any(), gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAliasNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			jsonBody: `{
				"metadata": {"key": "value"}
			}`,
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID) {
				aliasRepo.EXPECT().
					Update(gomock.Any(), orgID, holderID, aliasID, gomock.Any(), gomock.Any()).
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
			aliasID := uuid.New()

			mockAliasRepo := alias.NewMockRepository(ctrl)
			tt.setupMocks(mockAliasRepo, orgID, holderID, aliasID)

			uc := &services.UseCase{
				AliasRepo: mockAliasRepo,
			}
			handler := &AliasHandler{Service: uc}

			app := fiber.New()
			app.Patch("/v1/holders/:holder_id/aliases/:id",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Locals("id", aliasID)
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				http.WithBody(new(mmodel.UpdateAliasInput), handler.UpdateAlias),
			)

			req := httptest.NewRequest("PATCH", "/v1/holders/"+holderID.String()+"/aliases/"+aliasID.String(), bytes.NewBufferString(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Organization-Id", orgID)
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

func TestAliasHandler_DeleteAliasByID(t *testing.T) {
	tests := []struct {
		name           string
		hardDelete     string
		setupMocks     func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:       "success returns 204 no content",
			hardDelete: "",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID) {
				aliasRepo.EXPECT().
					Delete(gomock.Any(), orgID, holderID, aliasID, false).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil,
		},
		{
			name:       "success with hard delete returns 204",
			hardDelete: "true",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID) {
				aliasRepo.EXPECT().
					Delete(gomock.Any(), orgID, holderID, aliasID, true).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil,
		},
		{
			name:       "not found returns 404",
			hardDelete: "",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID) {
				aliasRepo.EXPECT().
					Delete(gomock.Any(), orgID, holderID, aliasID, false).
					Return(pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrAliasNotFound.Error(), errResp["code"])
			},
		},
		{
			name:       "repository error returns 500",
			hardDelete: "",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID) {
				aliasRepo.EXPECT().
					Delete(gomock.Any(), orgID, holderID, aliasID, false).
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
			aliasID := uuid.New()

			mockAliasRepo := alias.NewMockRepository(ctrl)
			tt.setupMocks(mockAliasRepo, orgID, holderID, aliasID)

			uc := &services.UseCase{
				AliasRepo: mockAliasRepo,
			}
			handler := &AliasHandler{Service: uc}

			app := fiber.New()
			app.Delete("/v1/holders/:holder_id/aliases/:id",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Locals("id", aliasID)
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				handler.DeleteAliasByID,
			)

			url := "/v1/holders/" + holderID.String() + "/aliases/" + aliasID.String()
			if tt.hardDelete != "" {
				url += "?hard_delete=" + tt.hardDelete
			}
			req := httptest.NewRequest("DELETE", url, nil)
			req.Header.Set("X-Organization-Id", orgID)
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

func TestAliasHandler_GetAllAliases(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(aliasRepo *alias.MockRepository, orgID string)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with pagination structure",
			queryParams: "",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string) {
				aliasRepo.EXPECT().
					FindAll(gomock.Any(), orgID, uuid.Nil, gomock.Any(), false).
					Return([]*mmodel.Alias{}, nil).
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
			name:        "success with items returns aliases",
			queryParams: "?limit=5&page=1",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string) {
				alias1ID := uuid.New()
				alias2ID := uuid.New()
				holderID := uuid.New()
				ledgerID := "00000000-0000-0000-0000-000000000001"
				accountID := "00000000-0000-0000-0000-000000000002"
				document := "12345678901"
				holderType := "individual"

				aliasRepo.EXPECT().
					FindAll(gomock.Any(), orgID, uuid.Nil, gomock.Any(), false).
					Return([]*mmodel.Alias{
						{
							ID:        &alias1ID,
							LedgerID:  &ledgerID,
							AccountID: &accountID,
							HolderID:  &holderID,
							Document:  &document,
							Type:      &holderType,
							CreatedAt: time.Now(),
							UpdatedAt: time.Now(),
						},
						{
							ID:        &alias2ID,
							LedgerID:  &ledgerID,
							AccountID: &accountID,
							HolderID:  &holderID,
							Document:  &document,
							Type:      &holderType,
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
				assert.Len(t, items, 2, "should have two aliases")

				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "alias should have id field")
				assert.Contains(t, firstItem, "ledgerId", "alias should have ledgerId field")

				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(5), limit)
			},
		},
		{
			name:        "repository error returns 500",
			queryParams: "",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string) {
				aliasRepo.EXPECT().
					FindAll(gomock.Any(), orgID, uuid.Nil, gomock.Any(), false).
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

			mockAliasRepo := alias.NewMockRepository(ctrl)
			tt.setupMocks(mockAliasRepo, orgID)

			uc := &services.UseCase{
				AliasRepo: mockAliasRepo,
			}
			handler := &AliasHandler{Service: uc}

			app := fiber.New()
			app.Get("/v1/aliases",
				func(c *fiber.Ctx) error {
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				handler.GetAllAliases,
			)

			req := httptest.NewRequest("GET", "/v1/aliases"+tt.queryParams, nil)
			req.Header.Set("X-Organization-Id", orgID)
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
