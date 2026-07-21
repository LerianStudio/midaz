// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaBalanceApp mounts the ten balance Huma operations on a /v1 group,
// mirroring the production wiring (see buildHumaAssetApp for the full rationale +
// MUST-NOT-PARALLELIZE note). The Fiber ParseUUIDPathParameters("balance")
// middleware runs before each Huma terminal; alias/code path params are NOT in
// UUIDPathParameters, so they pass through raw (matching the by-alias/by-code
// Fiber handlers that read c.Params directly).
func buildHumaBalanceApp(t *testing.T, handler *BalanceHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV1 := f.Group("/v1")

	apiV1.Use(func(c *fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	parse := pkgHTTP.ParseUUIDPathParameters("balance")
	orgLedger := "/organizations/:organization_id/ledgers/:ledger_id"
	apiV1.Get(orgLedger+"/balances", parse)
	apiV1.Get(orgLedger+"/balances/:balance_id", parse)
	apiV1.Patch(orgLedger+"/balances/:balance_id", parse)
	apiV1.Delete(orgLedger+"/balances/:balance_id", parse)
	apiV1.Get(orgLedger+"/balances/:balance_id/history", parse)
	apiV1.Get(orgLedger+"/accounts/:account_id/balances", parse)
	apiV1.Post(orgLedger+"/accounts/:account_id/balances", parse)
	apiV1.Get(orgLedger+"/accounts/:account_id/balances/history", parse)
	apiV1.Get(orgLedger+"/accounts/alias/:alias/balances", parse)
	apiV1.Get(orgLedger+"/accounts/external/:code/balances", parse)

	RegisterBalanceRoutes(hAPI, handler)

	return f
}

func TestHuma_GetBalancesByAlias_EmptyItems(t *testing.T) {
	// NOT parallel: buildHumaBalanceApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Empty result returns BEFORE the Redis overlay, so no TransactionRedisRepo mock
	// is needed. GetBalancesByAlias must still emit the 200 Pagination envelope with
	// an empty (non-nil) items list.
	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().ListByAliases(gomock.Any(), orgID, ledgerID, []string{"@person1"}).Return([]*mmodel.Balance{}, nil).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/alias/@person1/balances", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "items")
	assert.EqualValues(t, 10, got["limit"])
}

func TestHuma_GetBalancesByAlias_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// No repo expectations: a rejected auth must never reach the service.
	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balance.NewMockRepository(ctrl)}}

	app := buildHumaBalanceApp(t, handler, false)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/alias/@person1/balances", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestHuma_DeleteBalance_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state. Write-op (MONEY-adjacent) — transport
	// only; the command core is untouched. Zero-funds balance so the delete succeeds.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	balanceID := uuid.New()

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(&mmodel.Balance{ID: balanceID.String()}, nil).Times(1)
	balanceRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, balanceID).Return(nil).Times(1)

	handler := &BalanceHandler{Command: &command.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/balances/"+balanceID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestHuma_DeleteBalance_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state. ParseUUIDPathParameters rejects the
	// bad balance_id with the canonical 0065 / 400 before Huma; service never reached.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	handler := &BalanceHandler{Command: &command.UseCase{BalanceRepo: balance.NewMockRepository(ctrl)}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/balances/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestHuma_GetBalanceAtTimestamp_MissingDate_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state. The date query param carries NO
	// validation tag (no native 422); the imperative missing-date guard in the core
	// yields the canonical 400 (ErrMissingRequiredQueryParameter). Service never
	// reaches the repo.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	balanceID := uuid.New()

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balance.NewMockRepository(ctrl)}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/balances/"+balanceID.String()+"/history", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "missing date query stays canonical 400")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrMissingRequiredQueryParameter.Error(), got["code"])
}
