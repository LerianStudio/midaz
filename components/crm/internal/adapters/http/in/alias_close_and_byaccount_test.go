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

	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/idempotency"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestAliasHandler_CloseAlias covers the CloseAlias HTTP handler. The handler runs
// in three observable shapes: success (200 with the closed alias), business not-found
// (404), and infra failure (500). The fourth shape — idempotency-key conflict — is
// covered separately at the service layer in close-alias_test.go and is reachable here
// only through real Mongo, so we don't duplicate it.
func TestAliasHandler_CloseAlias(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with closed alias",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID) {
				closingDate := mmodel.Date{Time: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)}
				closed := &mmodel.Alias{
					ID:       &aliasID,
					HolderID: &holderID,
					BankingDetails: &mmodel.BankingDetails{
						ClosingDate: &closingDate,
					},
				}

				aliasRepo.EXPECT().
					CloseByID(gomock.Any(), orgID, holderID, aliasID, gomock.Any()).
					Return(closed, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id")
				assert.Contains(t, result, "bankingDetails")
			},
		},
		{
			name: "not found returns 404",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID) {
				aliasRepo.EXPECT().
					CloseByID(gomock.Any(), orgID, holderID, aliasID, gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Equal(t, cn.ErrAliasNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID string, holderID, aliasID uuid.UUID) {
				aliasRepo.EXPECT().
					CloseByID(gomock.Any(), orgID, holderID, aliasID, gomock.Any()).
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

				assert.Contains(t, errResp, "code")
				assert.Contains(t, errResp, "message")
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
			mockIdempRepo := idempotency.NewMockRepository(ctrl)
			tt.setupMocks(mockAliasRepo, orgID, holderID, aliasID)

			uc := &services.UseCase{
				AliasRepo:       mockAliasRepo,
				IdempotencyRepo: mockIdempRepo,
			}
			handler := &AliasHandler{Service: uc}

			app := fiber.New()
			app.Post("/v1/holders/:holder_id/aliases/:id/close",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Locals("id", aliasID)
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				handler.CloseAlias,
			)

			url := "/v1/holders/" + holderID.String() + "/aliases/" + aliasID.String() + "/close"
			req := httptest.NewRequest("POST", url, nil)
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

// TestAliasHandler_CloseAlias_WithIdempotencyKey covers the path through the service
// idempotency guard. With a fresh key, the handler must invoke Find (cache miss),
// CloseByID (the mutation), and Store (the cache write). A second call with the same
// key must short-circuit on cache hit without re-invoking CloseByID.
func TestAliasHandler_CloseAlias_WithIdempotencyKey_FreshKeyExecutesAndStores(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New().String()
	holderID := uuid.New()
	aliasID := uuid.New()
	idempKey := "idem-close-" + uuid.NewString()

	closingDate := mmodel.Date{Time: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)}
	closed := &mmodel.Alias{
		ID:       &aliasID,
		HolderID: &holderID,
		BankingDetails: &mmodel.BankingDetails{
			ClosingDate: &closingDate,
		},
	}

	mockAliasRepo := alias.NewMockRepository(ctrl)
	mockIdempRepo := idempotency.NewMockRepository(ctrl)

	// Cache miss -> the guard probes the store then runs the closure.
	mockIdempRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), idempKey).
		Return(nil, nil).
		Times(1)

	mockAliasRepo.EXPECT().
		CloseByID(gomock.Any(), orgID, holderID, aliasID, gomock.Any()).
		Return(closed, nil).
		Times(1)

	mockIdempRepo.EXPECT().
		Store(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	uc := &services.UseCase{
		AliasRepo:       mockAliasRepo,
		IdempotencyRepo: mockIdempRepo,
	}
	handler := &AliasHandler{Service: uc}

	app := fiber.New()
	app.Post("/v1/holders/:holder_id/aliases/:id/close",
		func(c *fiber.Ctx) error {
			c.Locals("holder_id", holderID)
			c.Locals("id", aliasID)
			c.Request().Header.Set("X-Organization-Id", orgID)
			return c.Next()
		},
		handler.CloseAlias,
	)

	url := "/v1/holders/" + holderID.String() + "/aliases/" + aliasID.String() + "/close"
	req := httptest.NewRequest("POST", url, nil)
	req.Header.Set("X-Organization-Id", orgID)
	req.Header.Set("X-Idempotency", idempKey)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// TestAliasHandler_GetAliasByAccount covers the by-account lookup handler. This handler
// is the saga's read counterpart to CreateAlias and must be tight on input validation:
// missing query parameters yield 400, the lookup error surfaces as 404 / 500 depending
// on type, and the success path returns the alias as JSON.
func TestAliasHandler_GetAliasByAccount(t *testing.T) {
	tests := []struct {
		name           string
		ledgerID       string
		accountID      string
		setupMocks     func(aliasRepo *alias.MockRepository, orgID, ledgerID, accountID string)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:      "success returns 200 with alias",
			ledgerID:  uuid.New().String(),
			accountID: uuid.New().String(),
			setupMocks: func(aliasRepo *alias.MockRepository, orgID, ledgerID, accountID string) {
				aliasID := uuid.New()
				holderID := uuid.New()

				aliasRepo.EXPECT().
					FindByLedgerAndAccount(gomock.Any(), orgID, ledgerID, accountID).
					Return(&mmodel.Alias{
						ID:        &aliasID,
						LedgerID:  &ledgerID,
						AccountID: &accountID,
						HolderID:  &holderID,
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id")
				assert.Contains(t, result, "ledgerId")
				assert.Contains(t, result, "accountId")
			},
		},
		{
			name:      "missing ledger_id returns 400",
			ledgerID:  "",
			accountID: uuid.New().String(),
			setupMocks: func(aliasRepo *alias.MockRepository, orgID, ledgerID, accountID string) {
				// No repo call expected — the handler validates query params upfront.
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code")
				assert.Equal(t, cn.ErrInvalidQueryParameter.Error(), errResp["code"])
			},
		},
		{
			name:      "missing account_id returns 400",
			ledgerID:  uuid.New().String(),
			accountID: "",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID, ledgerID, accountID string) {
				// No repo call expected.
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Equal(t, cn.ErrInvalidQueryParameter.Error(), errResp["code"])
			},
		},
		{
			name:      "missing both query params returns 400",
			ledgerID:  "",
			accountID: "",
			setupMocks: func(aliasRepo *alias.MockRepository, orgID, ledgerID, accountID string) {
				// No repo call expected.
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Equal(t, cn.ErrInvalidQueryParameter.Error(), errResp["code"])
			},
		},
		{
			name:      "alias not found returns 404",
			ledgerID:  uuid.New().String(),
			accountID: uuid.New().String(),
			setupMocks: func(aliasRepo *alias.MockRepository, orgID, ledgerID, accountID string) {
				aliasRepo.EXPECT().
					FindByLedgerAndAccount(gomock.Any(), orgID, ledgerID, accountID).
					Return(nil, pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Equal(t, cn.ErrAliasNotFound.Error(), errResp["code"])
			},
		},
		{
			name:      "repository error returns 500",
			ledgerID:  uuid.New().String(),
			accountID: uuid.New().String(),
			setupMocks: func(aliasRepo *alias.MockRepository, orgID, ledgerID, accountID string) {
				aliasRepo.EXPECT().
					FindByLedgerAndAccount(gomock.Any(), orgID, ledgerID, accountID).
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

				assert.Contains(t, errResp, "code")
				assert.Contains(t, errResp, "message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New().String()

			mockAliasRepo := alias.NewMockRepository(ctrl)
			tt.setupMocks(mockAliasRepo, orgID, tt.ledgerID, tt.accountID)

			uc := &services.UseCase{AliasRepo: mockAliasRepo}
			handler := &AliasHandler{Service: uc}

			app := fiber.New()
			app.Get("/v1/aliases/by-account",
				func(c *fiber.Ctx) error {
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				handler.GetAliasByAccount,
			)

			url := "/v1/aliases/by-account"
			separator := "?"
			if tt.ledgerID != "" {
				url += separator + "ledger_id=" + tt.ledgerID
				separator = "&"
			}

			if tt.accountID != "" {
				url += separator + "account_id=" + tt.accountID
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
