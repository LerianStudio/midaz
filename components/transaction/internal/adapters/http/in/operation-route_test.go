package in

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestOperationRouteHandler_CreateOperationRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		payload        *mmodel.CreateOperationRouteInput
		setupMocks     func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 201 with created operation route",
			payload: &mmodel.CreateOperationRouteInput{
				Title:         "Cashin Route",
				Description:   "Route for cashin operations",
				Code:          "CASHIN-001",
				OperationType: "source",
				Metadata:      map[string]any{"category": "income"},
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				operationRouteRepo.EXPECT().
					Create(gomock.Any(), orgID, ledgerID, gomock.Any()).
					DoAndReturn(func(ctx any, oID, lID uuid.UUID, or *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
						or.ID = uuid.New()
						or.OrganizationID = oID
						or.LedgerID = lID
						or.CreatedAt = time.Now()
						or.UpdatedAt = time.Now()
						return or, nil
					}).
					Times(1)

				metadataRepo.EXPECT().
					Create(gomock.Any(), "OperationRoute", gomock.Any()).
					Return(nil).
					Times(1)
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "title", "response should contain title")
				assert.Contains(t, result, "description", "response should contain description")
				assert.Contains(t, result, "operationType", "response should contain operationType")
				assert.Equal(t, "Cashin Route", result["title"])
				assert.Equal(t, "source", result["operationType"])
			},
		},
		{
			name: "success with alias account rule returns 201",
			payload: &mmodel.CreateOperationRouteInput{
				Title:         "Alias Route",
				OperationType: "destination",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@cash_account",
				},
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				operationRouteRepo.EXPECT().
					Create(gomock.Any(), orgID, ledgerID, gomock.Any()).
					DoAndReturn(func(ctx any, oID, lID uuid.UUID, or *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
						or.ID = uuid.New()
						or.OrganizationID = oID
						or.LedgerID = lID
						or.CreatedAt = time.Now()
						or.UpdatedAt = time.Now()
						return or, nil
					}).
					Times(1)
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "account", "response should contain account")

				account, ok := result["account"].(map[string]any)
				require.True(t, ok, "account should be an object")
				assert.Equal(t, "alias", account["ruleType"])
				assert.Equal(t, "@cash_account", account["validIf"])
			},
		},
		{
			name: "success with account_type rule returns 201",
			payload: &mmodel.CreateOperationRouteInput{
				Title:         "Account Type Route",
				OperationType: "source",
				Account: &mmodel.AccountRule{
					RuleType: "account_type",
					ValidIf:  []string{"deposit", "savings"},
				},
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				operationRouteRepo.EXPECT().
					Create(gomock.Any(), orgID, ledgerID, gomock.Any()).
					DoAndReturn(func(ctx any, oID, lID uuid.UUID, or *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
						or.ID = uuid.New()
						or.OrganizationID = oID
						or.LedgerID = lID
						or.CreatedAt = time.Now()
						or.UpdatedAt = time.Now()
						return or, nil
					}).
					Times(1)
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "account", "response should contain account")
			},
		},
		// NOTE: The following two tests document ACTUAL behavior where ErrMissingFieldsInRequest
		// is NOT mapped in ValidateBusinessError, causing the handler to return 500 instead of 400.
		// This is a known limitation in the current error handling implementation.
		{
			name: "validation error - ruleType without validIf returns 500 (unmapped error)",
			payload: &mmodel.CreateOperationRouteInput{
				Title:         "Invalid Route",
				OperationType: "source",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  nil,
				},
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// No repository calls expected - validation fails first
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				// Response contains error text (unmapped error returns raw text)
				assert.NotEmpty(t, body, "error response should not be empty")
			},
		},
		{
			name: "validation error - validIf without ruleType returns 500 (unmapped error)",
			payload: &mmodel.CreateOperationRouteInput{
				Title:         "Invalid Route",
				OperationType: "source",
				Account: &mmodel.AccountRule{
					RuleType: "",
					ValidIf:  "@some_alias",
				},
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// No repository calls expected - validation fails first
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				// Response contains error text (unmapped error returns raw text)
				assert.NotEmpty(t, body, "error response should not be empty")
			},
		},
		{
			name: "validation error - alias ruleType with non-string validIf returns 400",
			payload: &mmodel.CreateOperationRouteInput{
				Title:         "Invalid Alias Route",
				OperationType: "source",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  []string{"not", "a", "string"},
				},
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// No repository calls expected - validation fails first
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, constant.ErrInvalidAccountRuleValue.Error(), errResp["code"], "should return invalid account rule value error code")
			},
		},
		{
			name: "validation error - account_type ruleType with non-array validIf returns 400",
			payload: &mmodel.CreateOperationRouteInput{
				Title:         "Invalid Account Type Route",
				OperationType: "source",
				Account: &mmodel.AccountRule{
					RuleType: "account_type",
					ValidIf:  "should_be_array",
				},
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// No repository calls expected - validation fails first
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, constant.ErrInvalidAccountRuleValue.Error(), errResp["code"], "should return invalid account rule value error code")
			},
		},
		{
			name: "validation error - account_type with invalid array element returns 400",
			payload: &mmodel.CreateOperationRouteInput{
				Title:         "Invalid Account Type Route",
				OperationType: "source",
				Account: &mmodel.AccountRule{
					RuleType: "account_type",
					ValidIf:  []any{"valid", 123, "also_valid"},
				},
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// No repository calls expected - validation fails first
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, constant.ErrInvalidAccountRuleValue.Error(), errResp["code"], "should return invalid account rule value error code")
			},
		},
		{
			name: "validation error - invalid ruleType returns 400",
			payload: &mmodel.CreateOperationRouteInput{
				Title:         "Invalid Rule Type Route",
				OperationType: "source",
				Account: &mmodel.AccountRule{
					RuleType: "unknown_type",
					ValidIf:  "some_value",
				},
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// No repository calls expected - validation fails first
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, constant.ErrInvalidAccountRuleType.Error(), errResp["code"], "should return invalid account rule type error code")
			},
		},
		{
			name: "repository error returns 500",
			payload: &mmodel.CreateOperationRouteInput{
				Title:         "Error Route",
				OperationType: "source",
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				operationRouteRepo.EXPECT().
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
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()

			mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockOperationRouteRepo, mockMetadataRepo, orgID, ledgerID)

			cmdUC := &command.UseCase{
				OperationRouteRepo: mockOperationRouteRepo,
				MetadataRepo:       mockMetadataRepo,
			}
			handler := &OperationRouteHandler{Command: cmdUC}

			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				func(c *fiber.Ctx) error {
					return handler.CreateOperationRoute(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("POST", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/operation-routes", nil)
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

func TestOperationRouteHandler_GetOperationRouteByID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setupMocks     func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, operationRouteID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with operation route",
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				operationRouteRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, operationRouteID).
					Return(&mmodel.OperationRoute{
						ID:             operationRouteID,
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						Title:          "Cashin Route",
						Description:    "Route for cashin operations",
						Code:           "CASHIN-001",
						OperationType:  "source",
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "OperationRoute", operationRouteID.String()).
					Return(&mongodb.Metadata{
						EntityID:   operationRouteID.String(),
						EntityName: "OperationRoute",
						Data:       map[string]any{"category": "income"},
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "title", "response should contain title")
				assert.Contains(t, result, "operationType", "response should contain operationType")
				assert.Contains(t, result, "metadata", "response should contain metadata")
				assert.Equal(t, "Cashin Route", result["title"])
				assert.Equal(t, "source", result["operationType"])
			},
		},
		{
			name: "success without metadata returns 200",
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				operationRouteRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, operationRouteID).
					Return(&mmodel.OperationRoute{
						ID:             operationRouteID,
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						Title:          "Simple Route",
						OperationType:  "destination",
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "OperationRoute", operationRouteID.String()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "title", "response should contain title")
				assert.Equal(t, "Simple Route", result["title"])
			},
		},
		{
			name: "not found returns 404",
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				operationRouteRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, operationRouteID).
					Return(nil, pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, constant.ErrOperationRouteNotFound.Error(), errResp["code"], "should return operation route not found error code")
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				operationRouteRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, operationRouteID).
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
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()
			operationRouteID := uuid.New()

			mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockOperationRouteRepo, mockMetadataRepo, orgID, ledgerID, operationRouteID)

			queryUC := &query.UseCase{
				OperationRouteRepo: mockOperationRouteRepo,
				MetadataRepo:       mockMetadataRepo,
			}
			handler := &OperationRouteHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("operation_route_id", operationRouteID)
					return c.Next()
				},
				handler.GetOperationRouteByID,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/operation-routes/"+operationRouteID.String(), nil)
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

func TestOperationRouteHandler_UpdateOperationRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		payload        *mmodel.UpdateOperationRouteInput
		setupMocks     func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, operationRouteID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with updated operation route",
			payload: &mmodel.UpdateOperationRouteInput{
				Title:       "Updated Cashin Route",
				Description: "Updated description",
				Metadata:    map[string]any{"category": "updated"},
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				// Command.UpdateOperationRoute
				operationRouteRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, operationRouteID, gomock.Any()).
					Return(&mmodel.OperationRoute{
						ID:             operationRouteID,
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						Title:          "Updated Cashin Route",
						Description:    "Updated description",
						OperationType:  "source",
						CreatedAt:      time.Now().Add(-time.Hour),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// UpdateMetadata first calls FindByEntity
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "OperationRoute", operationRouteID.String()).
					Return(nil, nil).
					Times(1)

				// Then calls Update
				metadataRepo.EXPECT().
					Update(gomock.Any(), "OperationRoute", operationRouteID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Query.GetOperationRouteByID
				operationRouteRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, operationRouteID).
					Return(&mmodel.OperationRoute{
						ID:             operationRouteID,
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						Title:          "Updated Cashin Route",
						Description:    "Updated description",
						OperationType:  "source",
						CreatedAt:      time.Now().Add(-time.Hour),
						UpdatedAt:      time.Now(),
					}, nil).
					Times(1)

				// GetOperationRouteByID also fetches metadata
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "OperationRoute", operationRouteID.String()).
					Return(&mongodb.Metadata{
						EntityID:   operationRouteID.String(),
						EntityName: "OperationRoute",
						Data:       map[string]any{"category": "updated"},
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "title", "response should contain title")
				assert.Equal(t, "Updated Cashin Route", result["title"])
				assert.Equal(t, "Updated description", result["description"])
			},
		},
		{
			name: "success with account rule update triggers cache reload",
			payload: &mmodel.UpdateOperationRouteInput{
				Title: "Route with Account",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@new_account",
				},
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				// Command.UpdateOperationRoute
				operationRouteRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, operationRouteID, gomock.Any()).
					Return(&mmodel.OperationRoute{
						ID:             operationRouteID,
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						Title:          "Route with Account",
						OperationType:  "source",
						Account: &mmodel.AccountRule{
							RuleType: "alias",
							ValidIf:  "@new_account",
						},
						CreatedAt: time.Now().Add(-time.Hour),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)

				// UpdateMetadata with nil metadata skips FindByEntity and directly calls Update with empty map
				metadataRepo.EXPECT().
					Update(gomock.Any(), "OperationRoute", operationRouteID.String(), gomock.Any()).
					Return(nil).
					Times(1)

				// Query.GetOperationRouteByID
				operationRouteRepo.EXPECT().
					FindByID(gomock.Any(), orgID, ledgerID, operationRouteID).
					Return(&mmodel.OperationRoute{
						ID:             operationRouteID,
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						Title:          "Route with Account",
						OperationType:  "source",
						Account: &mmodel.AccountRule{
							RuleType: "alias",
							ValidIf:  "@new_account",
						},
						CreatedAt: time.Now().Add(-time.Hour),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)

				// GetOperationRouteByID also fetches metadata
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "OperationRoute", operationRouteID.String()).
					Return(nil, nil).
					Times(1)

				// ReloadOperationRouteCache is called when Account is updated
				operationRouteRepo.EXPECT().
					FindTransactionRouteIDs(gomock.Any(), operationRouteID).
					Return([]uuid.UUID{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "account", "response should contain account")
			},
		},
		{
			name: "not found returns 404",
			payload: &mmodel.UpdateOperationRouteInput{
				Title: "Updated title",
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				operationRouteRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, operationRouteID, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, constant.ErrOperationRouteNotFound.Error(), errResp["code"], "should return operation route not found error code")
			},
		},
		// NOTE: This test documents ACTUAL behavior where ErrMissingFieldsInRequest
		// is NOT mapped in ValidateBusinessError, causing the handler to return 500 instead of 400.
		{
			name: "validation error - ruleType without validIf returns 500 (unmapped error)",
			payload: &mmodel.UpdateOperationRouteInput{
				Title: "Invalid Update",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  nil,
				},
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				// No repository calls expected - validation fails first
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				// Response contains error text (unmapped error returns raw text)
				assert.NotEmpty(t, body, "error response should not be empty")
			},
		},
		{
			name: "validation error - alias ruleType with non-string validIf returns 400",
			payload: &mmodel.UpdateOperationRouteInput{
				Title: "Invalid Alias Update",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  123,
				},
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				// No repository calls expected - validation fails first
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, constant.ErrInvalidAccountRuleValue.Error(), errResp["code"], "should return invalid account rule value error code")
			},
		},
		{
			name: "repository error returns 500",
			payload: &mmodel.UpdateOperationRouteInput{
				Title: "Error Update",
			},
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				operationRouteRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, operationRouteID, gomock.Any()).
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
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()
			operationRouteID := uuid.New()

			mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			tt.setupMocks(mockOperationRouteRepo, mockMetadataRepo, mockRedisRepo, orgID, ledgerID, operationRouteID)

			cmdUC := &command.UseCase{
				OperationRouteRepo: mockOperationRouteRepo,
				MetadataRepo:       mockMetadataRepo,
				RedisRepo:          mockRedisRepo,
			}
			queryUC := &query.UseCase{
				OperationRouteRepo: mockOperationRouteRepo,
				MetadataRepo:       mockMetadataRepo,
			}
			handler := &OperationRouteHandler{
				Command: cmdUC,
				Query:   queryUC,
			}

			app := fiber.New()
			app.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("operation_route_id", operationRouteID)
					return c.Next()
				},
				func(c *fiber.Ctx) error {
					return handler.UpdateOperationRoute(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("PATCH", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/operation-routes/"+operationRouteID.String(), nil)
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

func TestOperationRouteHandler_DeleteOperationRouteByID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setupMocks     func(operationRouteRepo *operationroute.MockRepository, orgID, ledgerID, operationRouteID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 204",
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				// Check for transaction route links
				operationRouteRepo.EXPECT().
					HasTransactionRouteLinks(gomock.Any(), operationRouteID).
					Return(false, nil).
					Times(1)

				// Delete operation route
				operationRouteRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, operationRouteID).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil, // 204 has no body
		},
		{
			name: "not found returns 404",
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				// Check for transaction route links
				operationRouteRepo.EXPECT().
					HasTransactionRouteLinks(gomock.Any(), operationRouteID).
					Return(false, nil).
					Times(1)

				operationRouteRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, operationRouteID).
					Return(pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, constant.ErrOperationRouteNotFound.Error(), errResp["code"], "should return operation route not found error code")
			},
		},
		{
			name: "linked to transaction routes returns 422",
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				// Check for transaction route links - returns true
				operationRouteRepo.EXPECT().
					HasTransactionRouteLinks(gomock.Any(), operationRouteID).
					Return(true, nil).
					Times(1)
				// Delete should NOT be called
			},
			expectedStatus: 422, // UnprocessableOperationError returns 422
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, constant.ErrOperationRouteLinkedToTransactionRoutes.Error(), errResp["code"], "should return linked to transaction routes error code")
			},
		},
		{
			name: "has links check error returns 500",
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				operationRouteRepo.EXPECT().
					HasTransactionRouteLinks(gomock.Any(), operationRouteID).
					Return(false, pkg.InternalServerError{
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
		{
			name: "repository delete error returns 500",
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, orgID, ledgerID, operationRouteID uuid.UUID) {
				operationRouteRepo.EXPECT().
					HasTransactionRouteLinks(gomock.Any(), operationRouteID).
					Return(false, nil).
					Times(1)

				operationRouteRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, operationRouteID).
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
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Contains(t, errResp, "message", "error response should contain message field")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()
			operationRouteID := uuid.New()

			mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
			tt.setupMocks(mockOperationRouteRepo, orgID, ledgerID, operationRouteID)

			cmdUC := &command.UseCase{
				OperationRouteRepo: mockOperationRouteRepo,
			}
			handler := &OperationRouteHandler{Command: cmdUC}

			app := fiber.New()
			app.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("operation_route_id", operationRouteID)
					return c.Next()
				},
				handler.DeleteOperationRouteByID,
			)

			// Act
			req := httptest.NewRequest("DELETE", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/operation-routes/"+operationRouteID.String(), nil)
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

func TestOperationRouteHandler_GetAllOperationRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with pagination structure",
			queryParams: "",
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				operationRouteRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.OperationRoute{}, libHTTP.CursorPagination{}, nil).
					Times(1)

				// When results are found (even empty), FindList is called
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "OperationRoute", gomock.Any()).
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
			name:        "success with items returns operation routes",
			queryParams: "?limit=5",
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				or1ID := uuid.New()
				or2ID := uuid.New()

				operationRouteRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.OperationRoute{
						{
							ID:             or1ID,
							OrganizationID: orgID,
							LedgerID:       ledgerID,
							Title:          "Cashin Route",
							Description:    "Route for cashin",
							OperationType:  "source",
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
						{
							ID:             or2ID,
							OrganizationID: orgID,
							LedgerID:       ledgerID,
							Title:          "Cashout Route",
							Description:    "Route for cashout",
							OperationType:  "destination",
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
					}, libHTTP.CursorPagination{
						Next: "next_cursor_value",
						Prev: "",
					}, nil).
					Times(1)

				// GetAllOperationRoutes fetches metadata for all returned operation routes
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "OperationRoute", gomock.Any()).
					Return([]*mongodb.Metadata{
						{
							EntityID:   or1ID.String(),
							EntityName: "OperationRoute",
							Data:       map[string]any{"category": "income"},
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
				assert.Len(t, items, 2, "should have two operation routes")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "operation route should have id field")
				assert.Contains(t, firstItem, "title", "operation route should have title field")
				assert.Equal(t, "Cashin Route", firstItem["title"])

				// Validate pagination
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(5), limit)

				// Validate cursor pagination fields
				assert.Contains(t, result, "next_cursor", "response should contain next_cursor")
				assert.Equal(t, "next_cursor_value", result["next_cursor"])
			},
		},
		{
			name:        "with metadata filter returns filtered operation routes",
			queryParams: "?metadata.category=income",
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				orID := uuid.New()

				// GetAllMetadataOperationRoutes first calls FindList to get metadata matching filter
				metadataRepo.EXPECT().
					FindList(gomock.Any(), "OperationRoute", gomock.Any()).
					Return([]*mongodb.Metadata{
						{
							EntityID:   orID.String(),
							EntityName: "OperationRoute",
							Data:       map[string]any{"category": "income"},
						},
					}, nil).
					Times(1)

				// Then calls FindAll to get operation routes
				operationRouteRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.OperationRoute{
						{
							ID:             orID,
							OrganizationID: orgID,
							LedgerID:       ledgerID,
							Title:          "Cashin Route",
							Description:    "Route for cashin",
							OperationType:  "source",
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
					}, libHTTP.CursorPagination{
						Next: "",
						Prev: "",
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
				assert.Len(t, items, 1, "should have one filtered operation route")

				// Validate first item has metadata
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "metadata", "operation route should have metadata field")
			},
		},
		{
			name:        "repository error returns 500",
			queryParams: "",
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				operationRouteRepo.EXPECT().
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
		{
			name:        "invalid sort_order query parameter returns 400",
			queryParams: "?sort_order=invalid",
			setupMocks: func(operationRouteRepo *operationroute.MockRepository, metadataRepo *mongodb.MockRepository, orgID, ledgerID uuid.UUID) {
				// No repository calls expected - validation fails first
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, constant.ErrInvalidSortOrder.Error(), errResp["code"], "should return invalid sort order error code")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Arrange
			orgID := uuid.New()
			ledgerID := uuid.New()

			mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			tt.setupMocks(mockOperationRouteRepo, mockMetadataRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				OperationRouteRepo: mockOperationRouteRepo,
				MetadataRepo:       mockMetadataRepo,
			}
			handler := &OperationRouteHandler{Query: queryUC}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.GetAllOperationRoutes,
			)

			// Act
			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/operation-routes"+tt.queryParams, nil)
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

// Ensure libPostgres.Pagination is used (referenced in handler)
var _ = libPostgres.Pagination{}
