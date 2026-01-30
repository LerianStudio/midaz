// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestBalanceHandler_GetAllBalances(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with empty items array",
			queryParams: "",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID) {
				balanceRepo.EXPECT().
					ListAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.Balance{}, libHTTP.CursorPagination{}, nil).
					Times(1)
				// Redis not called when no balances found
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				// Validate items is nil/empty (use case returns nil for empty, pagination sets it)
				items := result["items"]
				assert.Nil(t, items, "items should be nil for empty result")

				// Validate limit is present
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(10), limit)
			},
		},
		{
			name:        "success with items returns proper pagination structure",
			queryParams: "?limit=5",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID) {
				balanceRepo.EXPECT().
					ListAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return([]*mmodel.Balance{
						{
							ID:             uuid.New().String(),
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Alias:          "@user1",
							Key:            "default",
							AssetCode:      "USD",
						},
						{
							ID:             uuid.New().String(),
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Alias:          "@user2",
							Key:            "default",
							AssetCode:      "BRL",
						},
					}, libHTTP.CursorPagination{Next: "next-cursor", Prev: "prev-cursor"}, nil).
					Times(1)
				redisRepo.EXPECT().
					MGet(gomock.Any(), gomock.Any()).
					Return(map[string]string{}, nil).
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
				assert.Len(t, items, 2, "should have two balances")

				// Validate pagination structure
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(5), limit)

				nextCursor, ok := result["next_cursor"].(string)
				require.True(t, ok, "next_cursor should be a string")
				assert.Equal(t, "next-cursor", nextCursor)

				prevCursor, ok := result["prev_cursor"].(string)
				require.True(t, ok, "prev_cursor should be a string")
				assert.Equal(t, "prev-cursor", prevCursor)
			},
		},
		{
			name:        "repository error returns 500",
			queryParams: "",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID) {
				balanceRepo.EXPECT().
					ListAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(nil, libHTTP.CursorPagination{}, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "The server encountered an unexpected error.",
					}).
					Times(1)
				// Redis not called when balance repo fails
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

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			tt.setupMocks(mockBalanceRepo, mockRedisRepo, orgID, ledgerID)

			uc := &query.UseCase{
				BalanceRepo: mockBalanceRepo,
				RedisRepo:   mockRedisRepo,
			}
			handler := &BalanceHandler{Query: uc}

			app := fiber.New()
			app.Get("/test/:organization_id/:ledger_id/balances",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.GetAllBalances,
			)

			// Act
			req := httptest.NewRequest("GET",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/balances"+tt.queryParams,
				nil)
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

func TestBalanceHandler_GetAllBalancesByAccountID(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, accountID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with empty items array",
			queryParams: "",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, accountID uuid.UUID) {
				balanceRepo.EXPECT().
					ListAllByAccountID(gomock.Any(), orgID, ledgerID, accountID, gomock.Any()).
					Return([]*mmodel.Balance{}, libHTTP.CursorPagination{}, nil).
					Times(1)
				// Redis not called when no balances found
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				// Validate items is empty array (use case returns empty slice which serializes as [])
				items, ok := result["items"].([]any)
				require.True(t, ok, "items should be an array")
				assert.Empty(t, items, "items should be empty for no balances")

				// Validate limit is present
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(10), limit)
			},
		},
		{
			name:        "success with items returns proper pagination structure",
			queryParams: "?limit=20",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, accountID uuid.UUID) {
				balanceRepo.EXPECT().
					ListAllByAccountID(gomock.Any(), orgID, ledgerID, accountID, gomock.Any()).
					Return([]*mmodel.Balance{
						{
							ID:             uuid.New().String(),
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							AccountID:      accountID.String(),
							Alias:          "@account-alias",
							Key:            "default",
							AssetCode:      "USD",
						},
					}, libHTTP.CursorPagination{Next: "next-page", Prev: ""}, nil).
					Times(1)
				redisRepo.EXPECT().
					MGet(gomock.Any(), gomock.Any()).
					Return(map[string]string{}, nil).
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
				assert.Len(t, items, 1, "should have one balance")

				// Validate first item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "balance should have id field")
				assert.Contains(t, firstItem, "assetCode", "balance should have assetCode field")

				// Validate pagination structure
				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(20), limit)

				nextCursor, ok := result["next_cursor"].(string)
				require.True(t, ok, "next_cursor should be a string")
				assert.Equal(t, "next-page", nextCursor)
			},
		},
		{
			name:        "repository error returns 500",
			queryParams: "",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, accountID uuid.UUID) {
				balanceRepo.EXPECT().
					ListAllByAccountID(gomock.Any(), orgID, ledgerID, accountID, gomock.Any()).
					Return(nil, libHTTP.CursorPagination{}, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "The server encountered an unexpected error.",
					}).
					Times(1)
				// Redis not called when balance repo fails
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
			accountID := uuid.New()

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			tt.setupMocks(mockBalanceRepo, mockRedisRepo, orgID, ledgerID, accountID)

			uc := &query.UseCase{
				BalanceRepo: mockBalanceRepo,
				RedisRepo:   mockRedisRepo,
			}
			handler := &BalanceHandler{Query: uc}

			app := fiber.New()
			app.Get("/test/:organization_id/:ledger_id/accounts/:account_id/balances",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("account_id", accountID)
					return c.Next()
				},
				handler.GetAllBalancesByAccountID,
			)

			// Act
			req := httptest.NewRequest("GET",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/accounts/"+accountID.String()+"/balances"+tt.queryParams,
				nil)
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

func TestBalanceHandler_GetBalancesByAlias(t *testing.T) {
	tests := []struct {
		name           string
		alias          string
		setupMocks     func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID, alias string)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:  "unknown alias returns 200 with empty list",
			alias: "@unknown-alias",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID, alias string) {
				balanceRepo.EXPECT().
					ListByAliases(gomock.Any(), orgID, ledgerID, []string{alias}).
					Return([]*mmodel.Balance{}, nil).
					Times(1)
				// Redis not called when no balances found
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				items, ok := result["items"].([]any)
				require.True(t, ok, "items should be an array")
				assert.Empty(t, items, "items should be empty for unknown alias")

				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(10), limit)
			},
		},
		{
			name:  "found balances returns 200 with items",
			alias: "@existing-alias",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID, alias string) {
				balanceRepo.EXPECT().
					ListByAliases(gomock.Any(), orgID, ledgerID, []string{alias}).
					Return([]*mmodel.Balance{
						{
							ID:             uuid.New().String(),
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Alias:          alias,
							Key:            "USD",
							AssetCode:      "USD",
						},
					}, nil).
					Times(1)
				// Redis is called to get cached balance values
				redisRepo.EXPECT().
					MGet(gomock.Any(), gomock.Any()).
					Return(map[string]string{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				items, ok := result["items"].([]any)
				require.True(t, ok, "items should be an array")
				assert.Len(t, items, 1, "should have one balance")

				// Validate item has expected fields
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "alias", "balance should have alias field")
				assert.Contains(t, firstItem, "assetCode", "balance should have assetCode field")
				assert.Equal(t, "@existing-alias", firstItem["alias"], "alias should match requested alias")
			},
		},
		{
			name:  "repository error returns 500",
			alias: "@error-alias",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID, alias string) {
				balanceRepo.EXPECT().
					ListByAliases(gomock.Any(), orgID, ledgerID, []string{alias}).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "The server encountered an unexpected error.",
					}).
					Times(1)
				// Redis not called when balance repo fails
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

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			tt.setupMocks(mockBalanceRepo, mockRedisRepo, orgID, ledgerID, tt.alias)

			uc := &query.UseCase{
				BalanceRepo: mockBalanceRepo,
				RedisRepo:   mockRedisRepo,
			}
			handler := &BalanceHandler{Query: uc}

			app := fiber.New()
			app.Get("/test/:organization_id/:ledger_id/:alias",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.GetBalancesByAlias,
			)

			// Act
			req := httptest.NewRequest("GET",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/"+tt.alias,
				nil)
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

func TestBalanceHandler_GetBalanceByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, balanceID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with balance",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, balanceID uuid.UUID) {
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, balanceID).
					Return(&mmodel.Balance{
						ID:             balanceID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						AccountID:      uuid.New().String(),
						Alias:          "@user1",
						Key:            "default",
						AssetCode:      "USD",
						Available:      decimal.NewFromInt(1000),
						OnHold:         decimal.NewFromInt(0),
					}, nil).
					Times(1)
				redisRepo.EXPECT().
					Get(gomock.Any(), gomock.Any()).
					Return("", nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "balance should have id field")
				assert.Contains(t, result, "assetCode", "balance should have assetCode field")
				assert.Contains(t, result, "available", "balance should have available field")
				assert.Equal(t, "USD", result["assetCode"])
			},
		},
		{
			name: "not found returns 404",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, balanceID uuid.UUID) {
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, balanceID).
					Return(nil, nil).
					Times(1)
				// Redis not called when balance not found
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrEntityNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, balanceID uuid.UUID) {
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, balanceID).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "The server encountered an unexpected error.",
					}).
					Times(1)
				// Redis not called when balance repo fails
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
			balanceID := uuid.New()

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			tt.setupMocks(mockBalanceRepo, mockRedisRepo, orgID, ledgerID, balanceID)

			uc := &query.UseCase{
				BalanceRepo: mockBalanceRepo,
				RedisRepo:   mockRedisRepo,
			}
			handler := &BalanceHandler{Query: uc}

			app := fiber.New()
			app.Get("/test/:organization_id/:ledger_id/balances/:balance_id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("balance_id", balanceID)
					return c.Next()
				},
				handler.GetBalanceByID,
			)

			// Act
			req := httptest.NewRequest("GET",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/balances/"+balanceID.String(),
				nil)
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

func TestBalanceHandler_DeleteBalanceByID(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(balanceRepo *balance.MockRepository, orgID, ledgerID, balanceID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 204 no content",
			setupMocks: func(balanceRepo *balance.MockRepository, orgID, ledgerID, balanceID uuid.UUID) {
				// Balance found with zero amounts (can be deleted)
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, balanceID).
					Return(&mmodel.Balance{
						ID:             balanceID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Available:      decimal.Zero,
						OnHold:         decimal.Zero,
					}, nil).
					Times(1)
				balanceRepo.EXPECT().
					Delete(gomock.Any(), orgID, ledgerID, balanceID).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil, // 204 has no body
		},
		{
			name: "not found returns 404",
			setupMocks: func(balanceRepo *balance.MockRepository, orgID, ledgerID, balanceID uuid.UUID) {
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, balanceID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrEntityNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "balance with non-zero funds returns 400 bad request",
			setupMocks: func(balanceRepo *balance.MockRepository, orgID, ledgerID, balanceID uuid.UUID) {
				// Test both Available and OnHold scenarios in subtests
				// Balance found with non-zero amounts (cannot be deleted)
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, balanceID).
					Return(&mmodel.Balance{
						ID:             balanceID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Available:      decimal.NewFromInt(1000),
						OnHold:         decimal.NewFromInt(500),
					}, nil).
					Times(1)
				// Delete should NOT be called
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err, "error response should be valid JSON")

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrBalancesCantBeDeleted.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(balanceRepo *balance.MockRepository, orgID, ledgerID, balanceID uuid.UUID) {
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, balanceID).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "The server encountered an unexpected error.",
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
			balanceID := uuid.New()

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			tt.setupMocks(mockBalanceRepo, orgID, ledgerID, balanceID)

			uc := &command.UseCase{
				BalanceRepo: mockBalanceRepo,
			}
			handler := &BalanceHandler{Command: uc}

			app := fiber.New()
			app.Delete("/test/:organization_id/:ledger_id/balances/:balance_id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("balance_id", balanceID)
					return c.Next()
				},
				handler.DeleteBalanceByID,
			)

			// Act
			req := httptest.NewRequest("DELETE",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/balances/"+balanceID.String(),
				nil)
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

func TestBalanceHandler_GetBalancesExternalByCode(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		setupMocks     func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID, code string)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "unknown code returns 200 with empty list",
			code: "XYZ",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID, code string) {
				expectedAlias := cn.DefaultExternalAccountAliasPrefix + code
				balanceRepo.EXPECT().
					ListByAliases(gomock.Any(), orgID, ledgerID, []string{expectedAlias}).
					Return([]*mmodel.Balance{}, nil).
					Times(1)
				// Redis not called when no balances found
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				items, ok := result["items"].([]any)
				require.True(t, ok, "items should be an array")
				assert.Empty(t, items, "items should be empty for unknown code")

				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(10), limit)
			},
		},
		{
			name: "found external balances returns 200 with items",
			code: "USD",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID, code string) {
				expectedAlias := cn.DefaultExternalAccountAliasPrefix + code
				balanceRepo.EXPECT().
					ListByAliases(gomock.Any(), orgID, ledgerID, []string{expectedAlias}).
					Return([]*mmodel.Balance{
						{
							ID:             uuid.New().String(),
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							Alias:          expectedAlias,
							Key:            "default",
							AssetCode:      code,
						},
					}, nil).
					Times(1)
				// Redis is called to get cached balance values
				redisRepo.EXPECT().
					MGet(gomock.Any(), gomock.Any()).
					Return(map[string]string{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				items, ok := result["items"].([]any)
				require.True(t, ok, "items should be an array")
				assert.Len(t, items, 1, "should have one external balance")

				// Validate first item has external alias prefix
				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				alias, ok := firstItem["alias"].(string)
				require.True(t, ok, "alias should be a string")
				assert.Contains(t, alias, cn.DefaultExternalAccountAliasPrefix, "alias should have external prefix")
			},
		},
		{
			name: "repository error returns 500",
			code: "BRL",
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID uuid.UUID, code string) {
				expectedAlias := cn.DefaultExternalAccountAliasPrefix + code
				balanceRepo.EXPECT().
					ListByAliases(gomock.Any(), orgID, ledgerID, []string{expectedAlias}).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "The server encountered an unexpected error.",
					}).
					Times(1)
				// Redis not called when balance repo fails
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

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			tt.setupMocks(mockBalanceRepo, mockRedisRepo, orgID, ledgerID, tt.code)

			uc := &query.UseCase{
				BalanceRepo: mockBalanceRepo,
				RedisRepo:   mockRedisRepo,
			}
			handler := &BalanceHandler{Query: uc}

			app := fiber.New()
			app.Get("/test/:organization_id/:ledger_id/accounts/external/:code/balances",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.GetBalancesExternalByCode,
			)

			// Act
			req := httptest.NewRequest("GET",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/accounts/external/"+tt.code+"/balances",
				nil)
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

func TestBalanceHandler_UpdateBalance(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.UpdateBalance
		setupMocks     func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, balanceID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with updated balance",
			payload: &mmodel.UpdateBalance{
				AllowSending:   boolPtr(false),
				AllowReceiving: boolPtr(true),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, balanceID uuid.UUID) {
				// Command.Update returns the updated balance directly (using RETURNING clause)
				balanceRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, balanceID, mmodel.UpdateBalance{
						AllowSending:   boolPtr(false),
						AllowReceiving: boolPtr(true),
					}).
					Return(&mmodel.Balance{
						ID:             balanceID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						Alias:          "@user1",
						Key:            "default",
						AssetCode:      "USD",
						AllowSending:   false,
						AllowReceiving: true,
					}, nil).
					Times(1)
				// Redis overlay for freshest balance amounts (service layer calls RedisRepo.Get)
				redisRepo.EXPECT().
					Get(gomock.Any(), gomock.Any()).
					Return("", nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "alias", "response should contain alias")
				assert.Contains(t, result, "assetCode", "response should contain assetCode")

				allowSending, ok := result["allowSending"].(bool)
				require.True(t, ok, "allowSending should be a boolean")
				assert.False(t, allowSending, "allowSending should be false after update")

				allowReceiving, ok := result["allowReceiving"].(bool)
				require.True(t, ok, "allowReceiving should be a boolean")
				assert.True(t, allowReceiving, "allowReceiving should be true after update")
			},
		},
		{
			name: "balance not found on update returns 404",
			payload: &mmodel.UpdateBalance{
				AllowSending: boolPtr(true),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, balanceID uuid.UUID) {
				balanceRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, balanceID, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
		{
			name: "repository error on update returns 500",
			payload: &mmodel.UpdateBalance{
				AllowReceiving: boolPtr(false),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, balanceID uuid.UUID) {
				balanceRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, balanceID, gomock.Any()).
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
			balanceID := uuid.New()

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			tt.setupMocks(mockBalanceRepo, mockRedisRepo, orgID, ledgerID, balanceID)

			cmdUC := &command.UseCase{
				BalanceRepo: mockBalanceRepo,
				RedisRepo:   mockRedisRepo,
			}
			queryUC := &query.UseCase{
				BalanceRepo: mockBalanceRepo,
				RedisRepo:   mockRedisRepo,
			}
			handler := &BalanceHandler{
				Command: cmdUC,
				Query:   queryUC,
			}

			app := fiber.New()
			app.Patch("/test/:organization_id/:ledger_id/balances/:balance_id",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("balance_id", balanceID)
					return c.Next()
				},
				// Simulate WithBody middleware by calling handler directly with parsed payload
				func(c *fiber.Ctx) error {
					return handler.UpdateBalance(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("PATCH",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/balances/"+balanceID.String(),
				nil)
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

func TestBalanceHandler_CreateAdditionalBalance(t *testing.T) {
	tests := []struct {
		name           string
		payload        *mmodel.CreateAdditionalBalance
		setupMocks     func(balanceRepo *balance.MockRepository, orgID, ledgerID, accountID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 201 with created balance",
			payload: &mmodel.CreateAdditionalBalance{
				Key:            "freeze-assets",
				AllowSending:   boolPtr(false),
				AllowReceiving: boolPtr(true),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				// Check if balance with key already exists - returns not found (allows creation)
				balanceRepo.EXPECT().
					FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "freeze-assets").
					Return(nil, pkg.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())).
					Times(1)

				// Get default balance to copy properties
				balanceRepo.EXPECT().
					FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "default").
					Return(&mmodel.Balance{
						ID:             uuid.New().String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						AccountID:      accountID.String(),
						Alias:          "@user1",
						Key:            "default",
						AssetCode:      "USD",
						AccountType:    "deposit",
						AllowSending:   true,
						AllowReceiving: true,
					}, nil).
					Times(1)

				// Create the additional balance
				balanceRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "alias", "response should contain alias")

				key, ok := result["key"].(string)
				require.True(t, ok, "key should be a string")
				assert.Equal(t, "freeze-assets", key, "key should match created key")

				allowSending, ok := result["allowSending"].(bool)
				require.True(t, ok, "allowSending should be a boolean")
				assert.False(t, allowSending, "allowSending should be false as specified")

				allowReceiving, ok := result["allowReceiving"].(bool)
				require.True(t, ok, "allowReceiving should be a boolean")
				assert.True(t, allowReceiving, "allowReceiving should be true as specified")
			},
		},
		{
			name: "duplicate key returns 409 conflict",
			payload: &mmodel.CreateAdditionalBalance{
				Key:            "existing-key",
				AllowSending:   boolPtr(true),
				AllowReceiving: boolPtr(true),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				// Check if balance with key already exists - returns existing balance
				balanceRepo.EXPECT().
					FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "existing-key").
					Return(&mmodel.Balance{
						ID:  uuid.New().String(),
						Key: "existing-key",
					}, nil).
					Times(1)
				// No further calls when duplicate is detected
			},
			expectedStatus: 409,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")

				message, ok := errResp["message"].(string)
				require.True(t, ok, "message should be a string")
				assert.Contains(t, message, "already exists", "message should indicate duplicate")
			},
		},
		{
			name: "external account type returns 400 validation error",
			payload: &mmodel.CreateAdditionalBalance{
				Key:            "new-key",
				AllowSending:   boolPtr(true),
				AllowReceiving: boolPtr(true),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				// Check if balance with key already exists - returns not found
				balanceRepo.EXPECT().
					FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "new-key").
					Return(nil, pkg.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())).
					Times(1)

				// Get default balance - returns external account type
				balanceRepo.EXPECT().
					FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "default").
					Return(&mmodel.Balance{
						ID:             uuid.New().String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						AccountID:      accountID.String(),
						Alias:          "@external:USD",
						Key:            "default",
						AssetCode:      "USD",
						AccountType:    cn.ExternalAccountType,
						AllowSending:   true,
						AllowReceiving: true,
					}, nil).
					Times(1)
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")

				message, ok := errResp["message"].(string)
				require.True(t, ok, "message should be a string")
				assert.Contains(t, message, "external", "message should indicate external account restriction")
			},
		},
		{
			name: "default balance not found returns 404",
			payload: &mmodel.CreateAdditionalBalance{
				Key:            "new-balance",
				AllowSending:   boolPtr(true),
				AllowReceiving: boolPtr(true),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				// Check if balance with key already exists - returns not found
				balanceRepo.EXPECT().
					FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "new-balance").
					Return(nil, pkg.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())).
					Times(1)

				// Get default balance fails
				balanceRepo.EXPECT().
					FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "default").
					Return(nil, pkg.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())).
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
			name: "repository error on create returns 500",
			payload: &mmodel.CreateAdditionalBalance{
				Key:            "test-key",
				AllowSending:   boolPtr(true),
				AllowReceiving: boolPtr(true),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, orgID, ledgerID, accountID uuid.UUID) {
				// Check if balance with key already exists - returns not found
				balanceRepo.EXPECT().
					FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "test-key").
					Return(nil, pkg.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())).
					Times(1)

				// Get default balance succeeds
				balanceRepo.EXPECT().
					FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "default").
					Return(&mmodel.Balance{
						ID:             uuid.New().String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						AccountID:      accountID.String(),
						Alias:          "@user1",
						Key:            "default",
						AssetCode:      "USD",
						AccountType:    "deposit",
					}, nil).
					Times(1)

				// Create fails with internal error
				balanceRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
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
			accountID := uuid.New()

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			tt.setupMocks(mockBalanceRepo, orgID, ledgerID, accountID)

			cmdUC := &command.UseCase{
				BalanceRepo: mockBalanceRepo,
			}
			handler := &BalanceHandler{
				Command: cmdUC,
			}

			app := fiber.New()
			app.Post("/test/:organization_id/:ledger_id/accounts/:account_id/balances",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("account_id", accountID)
					return c.Next()
				},
				// Simulate WithBody middleware by calling handler directly with parsed payload
				func(c *fiber.Ctx) error {
					return handler.CreateAdditionalBalance(tt.payload, c)
				},
			)

			// Act
			req := httptest.NewRequest("POST",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/accounts/"+accountID.String()+"/balances",
				nil)
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

// boolPtr is a helper function to create a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}

func TestBalanceHandler_GetBalanceAtTimestamp(t *testing.T) {
	tests := []struct {
		name           string
		date           string
		setupMocks     func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, balanceID uuid.UUID, date time.Time)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with balance at date",
			date: "2024-01-15 10:30:00",
			setupMocks: func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, balanceID uuid.UUID, date time.Time) {
				accountID := uuid.New()
				available := decimal.NewFromInt(5000)
				onHold := decimal.NewFromInt(500)
				version := int64(10)
				balanceCreatedAt := date.Add(-24 * time.Hour) // Balance created 1 day before the query date

				// First check current balance exists
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, balanceID).
					Return(&mmodel.Balance{
						ID:             balanceID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						AccountID:      accountID.String(),
						Alias:          "@user1",
						Key:            "default",
						AssetCode:      "USD",
						AccountType:    "deposit",
						CreatedAt:      balanceCreatedAt,
					}, nil).
					Times(1)

				// Then find last operation before date
				operationRepo.EXPECT().
					FindLastOperationBeforeTimestamp(gomock.Any(), orgID, ledgerID, balanceID, gomock.Any()).
					Return(&operation.Operation{
						ID:           uuid.New().String(),
						AccountID:    accountID.String(),
						BalanceKey:   "default",
						AssetCode:    "USD",
						BalanceAfter: operation.Balance{
							Available: &available,
							OnHold:    &onHold,
							Version:   &version,
						},
						CreatedAt: date.Add(-time.Hour),
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "balance should have id field")
				assert.Contains(t, result, "assetCode", "balance should have assetCode field")
				assert.Contains(t, result, "available", "balance should have available field")
				assert.Equal(t, "USD", result["assetCode"])

				// Verify createdAt is present and not the zero value
				assert.Contains(t, result, "createdAt", "balance should have createdAt field")
				createdAt, ok := result["createdAt"].(string)
				assert.True(t, ok, "createdAt should be a string")
				assert.NotEqual(t, "0001-01-01T00:00:00Z", createdAt, "createdAt should not be zero value")
				assert.NotEmpty(t, createdAt, "createdAt should not be empty")
			},
		},
		{
			name: "missing date returns 400",
			date: "",
			setupMocks: func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, balanceID uuid.UUID, date time.Time) {
				// No mocks needed - validation happens before repository calls
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrMissingFieldsInRequest.Error(), errResp["code"])
			},
		},
		{
			name: "invalid date format returns 400",
			date: "not-a-date",
			setupMocks: func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, balanceID uuid.UUID, date time.Time) {
				// No mocks needed - validation happens before repository calls
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrInvalidDatetimeFormat.Error(), errResp["code"])
			},
		},
		{
			name: "future timestamp returns 400",
			date: "2099-01-15 10:30:00",
			setupMocks: func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, balanceID uuid.UUID, date time.Time) {
				// No mocks needed - service validates timestamp before any repository calls
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrInvalidTimestamp.Error(), errResp["code"])
			},
		},
		{
			name: "balance not found returns 404",
			date: "2024-01-15 10:30:00",
			setupMocks: func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, balanceID uuid.UUID, date time.Time) {
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, balanceID).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrEntityNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "no balance data at date returns 404",
			date: "2024-01-15 10:30:00",
			setupMocks: func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, balanceID uuid.UUID, date time.Time) {
				// Balance exists but was created AFTER the query date
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, balanceID).
					Return(&mmodel.Balance{
						ID:             balanceID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						AccountID:      uuid.New().String(),
						Alias:          "@user1",
						Key:            "default",
						AssetCode:      "USD",
						CreatedAt:      date.Add(24 * time.Hour), // Balance created AFTER query date
					}, nil).
					Times(1)

				// No operation found before date (implementation checks this before CreatedAt)
				operationRepo.EXPECT().
					FindLastOperationBeforeTimestamp(gomock.Any(), orgID, ledgerID, balanceID, gomock.Any()).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrNoBalanceDataAtTimestamp.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			date: "2024-01-15 10:30:00",
			setupMocks: func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, balanceID uuid.UUID, date time.Time) {
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, balanceID).
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
			balanceID := uuid.New()

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockOperationRepo := operation.NewMockRepository(ctrl)

			var date time.Time
			if tt.date != "" {
				var err error
				date, _, err = libCommons.ParseDateTime(tt.date, false)
				if err != nil {
					date = time.Time{}
				}
			}

			tt.setupMocks(mockBalanceRepo, mockOperationRepo, orgID, ledgerID, balanceID, date)

			uc := &query.UseCase{
				BalanceRepo:   mockBalanceRepo,
				OperationRepo: mockOperationRepo,
			}
			handler := &BalanceHandler{Query: uc}

			app := fiber.New()
			app.Get("/test/:organization_id/:ledger_id/balances/:balance_id/history",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("balance_id", balanceID)
					return c.Next()
				},
				handler.GetBalanceAtTimestamp,
			)

			// Act
			testURL := "/test/" + orgID.String() + "/" + ledgerID.String() + "/balances/" + balanceID.String() + "/history"
			if tt.date != "" {
				testURL += "?date=" + url.QueryEscape(tt.date)
			}
			req := httptest.NewRequest("GET", testURL, nil)
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

func TestBalanceHandler_GetAccountBalancesAtTimestamp(t *testing.T) {
	tests := []struct {
		name           string
		date           string
		setupMocks     func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, accountID uuid.UUID, date time.Time)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with balances at date with valid createdAt",
			date: "2024-01-15 10:30:00",
			setupMocks: func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, accountID uuid.UUID, date time.Time) {
				balanceID := uuid.New()
				balanceCreatedAt := date.Add(-24 * time.Hour)
				updatedAt := date.Add(-time.Hour)

				balanceRepo.EXPECT().
					ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, date).
					Return([]*mmodel.Balance{
						{
							ID:             balanceID.String(),
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							AccountID:      accountID.String(),
							Alias:          "@user1",
							Key:            "default",
							AssetCode:      "USD",
							AccountType:    "deposit",
							Available:      decimal.NewFromInt(5000),
							OnHold:         decimal.NewFromInt(500),
							Version:        10,
							CreatedAt:      balanceCreatedAt,
							UpdatedAt:      updatedAt,
						},
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result []map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)
				require.Len(t, result, 1, "should have one balance")

				balance := result[0]
				assert.Contains(t, balance, "id", "balance should have id field")
				assert.Contains(t, balance, "assetCode", "balance should have assetCode field")
				assert.Equal(t, "USD", balance["assetCode"])

				// Verify createdAt is present and not the zero value
				assert.Contains(t, balance, "createdAt", "balance should have createdAt field")
				createdAt, ok := balance["createdAt"].(string)
				assert.True(t, ok, "createdAt should be a string")
				assert.NotEqual(t, "0001-01-01T00:00:00Z", createdAt, "createdAt should not be zero value")
				assert.NotEmpty(t, createdAt, "createdAt should not be empty")
			},
		},
		{
			name: "missing date returns 400",
			date: "",
			setupMocks: func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, accountID uuid.UUID, date time.Time) {
				// No mocks needed - validation happens before repository calls
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrMissingFieldsInRequest.Error(), errResp["code"])
			},
		},
		{
			name: "invalid date format returns 400",
			date: "not-a-date",
			setupMocks: func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, accountID uuid.UUID, date time.Time) {
				// No mocks needed - validation happens before repository calls
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrInvalidDatetimeFormat.Error(), errResp["code"])
			},
		},
		{
			name: "future timestamp returns 400",
			date: "2099-01-15 10:30:00",
			setupMocks: func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, accountID uuid.UUID, date time.Time) {
				// No mocks needed - service validates timestamp before repository calls
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrInvalidTimestamp.Error(), errResp["code"])
			},
		},
		{
			name: "no balance data at date returns 404",
			date: "2024-01-15 10:30:00",
			setupMocks: func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, accountID uuid.UUID, date time.Time) {
				balanceRepo.EXPECT().
					ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, date).
					Return([]*mmodel.Balance{}, nil).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Equal(t, cn.ErrNoBalanceDataAtTimestamp.Error(), errResp["code"])
			},
		},
		{
			name: "balance repository error returns 500",
			date: "2024-01-15 10:30:00",
			setupMocks: func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, accountID uuid.UUID, date time.Time) {
				balanceRepo.EXPECT().
					ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, date).
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

				assert.Contains(t, errResp, "code", "error response should contain code field")
				assert.Contains(t, errResp, "message", "error response should contain message field")
			},
		},
		{
			name: "success with multiple balances returns all balances",
			date: "2024-01-15 10:30:00",
			setupMocks: func(balanceRepo *balance.MockRepository, operationRepo *operation.MockRepository, orgID, ledgerID, accountID uuid.UUID, date time.Time) {
				balanceID1 := uuid.New()
				balanceID2 := uuid.New()
				balanceCreatedAt := date.Add(-24 * time.Hour)
				updatedAt := date.Add(-time.Hour)

				balanceRepo.EXPECT().
					ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, date).
					Return([]*mmodel.Balance{
						{
							ID:             balanceID1.String(),
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							AccountID:      accountID.String(),
							Alias:          "@user1",
							Key:            "default",
							AssetCode:      "USD",
							AccountType:    "deposit",
							Available:      decimal.NewFromInt(5000),
							OnHold:         decimal.NewFromInt(500),
							Version:        10,
							CreatedAt:      balanceCreatedAt,
							UpdatedAt:      updatedAt,
						},
						{
							ID:             balanceID2.String(),
							OrganizationID: orgID.String(),
							LedgerID:       ledgerID.String(),
							AccountID:      accountID.String(),
							Alias:          "@user1",
							Key:            "default",
							AssetCode:      "BRL",
							AccountType:    "deposit",
							Available:      decimal.NewFromInt(3000),
							OnHold:         decimal.NewFromInt(500),
							Version:        10,
							CreatedAt:      balanceCreatedAt,
							UpdatedAt:      updatedAt,
						},
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result []map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)
				require.Len(t, result, 2, "should have two balances")

				// Verify both balances are present
				assetCodes := make([]string, 0, 2)
				for _, balance := range result {
					assetCodes = append(assetCodes, balance["assetCode"].(string))
				}
				assert.Contains(t, assetCodes, "USD", "should contain USD balance")
				assert.Contains(t, assetCodes, "BRL", "should contain BRL balance")
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
			accountID := uuid.New()

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockOperationRepo := operation.NewMockRepository(ctrl)

			var date time.Time
			if tt.date != "" {
				var err error
				date, _, err = libCommons.ParseDateTime(tt.date, false)
				if err != nil {
					date = time.Time{}
				}
			}

			tt.setupMocks(mockBalanceRepo, mockOperationRepo, orgID, ledgerID, accountID, date)

			uc := &query.UseCase{
				BalanceRepo:   mockBalanceRepo,
				OperationRepo: mockOperationRepo,
			}
			handler := &BalanceHandler{Query: uc}

			app := fiber.New()
			app.Get("/test/:organization_id/:ledger_id/accounts/:account_id/balances/history",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					c.Locals("account_id", accountID)
					return c.Next()
				},
				handler.GetAccountBalancesAtTimestamp,
			)

			// Act
			testURL := "/test/" + orgID.String() + "/" + ledgerID.String() + "/accounts/" + accountID.String() + "/balances/history"
			if tt.date != "" {
				testURL += "?date=" + url.QueryEscape(tt.date)
			}
			req := httptest.NewRequest("GET", testURL, nil)
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
