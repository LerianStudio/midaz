// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"

	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libHTTP "github.com/LerianStudio/lib-commons/v6/commons/net/http"
	"github.com/shopspring/decimal"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
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
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	// Mirror production: the ledger registers ErrorEnvelope on the app root, so
	// /v1 serves the v3 envelope. Without it these assertions lock a shape no
	// deployed ledger returns.
	f.Use(ledgerMiddleware.ErrorEnvelope())

	apiV1 := f.Group("/v1")

	apiV1.Use(func(c fiber.Ctx) error {
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

	RegisterBalanceRoutes(hAPI, handler, v1OpSuffix)

	return f
}

func TestGetBalancesByAlias_EmptyItems(t *testing.T) {
	// NOT parallel: buildHumaBalanceApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Empty result returns BEFORE the Redis overlay, so no TransactionRedisRepo mock
	// is needed. GetBalancesByAlias must still emit the 200 Pagination envelope with
	// an empty (non-nil) items list.
	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().ListByAliases(gomock.Any(), orgID, ledgerID, []string{"@person1"}).Return([]*mmodel.Balance{}, nil).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/alias/@person1/balances", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestGetBalancesByAlias_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// No repo expectations: a rejected auth must never reach the service.
	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balance.NewMockRepository(ctrl)}}

	app := buildHumaBalanceApp(t, handler, false)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/alias/@person1/balances", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestDeleteBalance_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state. Write-op (MONEY-adjacent) — transport
	// only; the command core is untouched. Zero-funds balance so the delete succeeds.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(&mmodel.Balance{ID: balanceID.String()}, nil).Times(1)
	balanceRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, balanceID).Return(nil).Times(1)

	// The delete path plants and evicts balance delete markers via the honored-lock
	// guard. Lenient expectations keep this a transport-level test.
	redisRepo := redis.NewMockRedisRepository(ctrl)
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	handler := &BalanceHandler{Command: &command.UseCase{BalanceRepo: balanceRepo, TransactionRedisRepo: redisRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/balances/"+balanceID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestDeleteBalance_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state. ParseUUIDPathParameters rejects the
	// bad balance_id with the canonical 0065 / 400 before Huma; service never reached.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BalanceHandler{Command: &command.UseCase{BalanceRepo: balance.NewMockRepository(ctrl)}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/balances/not-a-uuid", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestGetBalanceAtTimestamp_MissingDate_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state. The date query param carries NO
	// validation tag (no native 422); the imperative missing-date guard in the core
	// yields the canonical 400 (ErrMissingRequiredQueryParameter). Service never
	// reaches the repo.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balance.NewMockRepository(ctrl)}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/balances/"+balanceID.String()+"/history", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "missing date query stays canonical 400")
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrMissingRequiredQueryParameter.Error(), got["code"])
}

//
// The ten exported fiber.Ctx terminals on BalanceHandler were deleted with the
// Huma migration; the branches their tests covered in the shared cores are
// exercised here through the live Huma transport instead. The three write cores
// (update / create-additional / delete) are MONEY-adjacent: these are
// transport-level tests, the command use cases they call are untouched.

const balOrgLedger = "/v1/organizations/"

func balPath(orgID, ledgerID uuid.UUID, suffix string) string {
	return balOrgLedger + orgID.String() + "/ledgers/" + ledgerID.String() + suffix
}

func TestGetAllBalances_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// An empty page returns before the Redis overlay, so no redis mock is needed.
	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().ListAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return([]*mmodel.Balance{}, libHTTP.CursorPagination{}, nil).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/balances?limit=10"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestGetAllBalances_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state. ValidateParameters rejects limit=abc
	// with the canonical 400 — never a native Huma 422; service never reached.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balance.NewMockRepository(ctrl)}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/balances?limit=abc"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestGetAllBalances_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. getAllBalances' query-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().ListAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/balances"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestGetAllBalancesByAccountID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().ListAllByAccountID(gomock.Any(), orgID, ledgerID, accountID, gomock.Any()).
		Return([]*mmodel.Balance{}, libHTTP.CursorPagination{}, nil).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/accounts/"+accountID.String()+"/balances?limit=10"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "items")
}

func TestGetAllBalancesByAccountID_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balance.NewMockRepository(ctrl)}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/accounts/"+accountID.String()+"/balances?limit=abc"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestGetAllBalancesByAccountID_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().ListAllByAccountID(gomock.Any(), orgID, ledgerID, accountID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/accounts/"+accountID.String()+"/balances"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetBalanceByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)

	balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(&mmodel.Balance{
			ID:             balanceID.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
			Alias:          "@user1",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.NewFromInt(0),
		}, nil).Times(1)
	// The service overlays the freshest amounts from Redis.
	redisRepo.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo, TransactionRedisRepo: redisRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/balances/"+balanceID.String()), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, balanceID.String(), got["id"])
}

func TestGetBalanceByID_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. getBalanceByID's query-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/balances/"+balanceID.String()), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestUpdateBalance_Success(t *testing.T) {
	// NOT parallel: process-global huma state. MONEY-adjacent write — transport only.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)

	// The scope guard reads the current balance before updating.
	balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(&mmodel.Balance{ID: balanceID.String(), Alias: "@user1", Key: "default"}, nil).Times(1)
	balanceRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, balanceID, mmodel.UpdateBalance{
		AllowSending:   testutils.Ptr(false),
		AllowReceiving: testutils.Ptr(true),
	}).Return(&mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@user1",
		Key:            "default",
		AssetCode:      "USD",
		AllowSending:   false,
		AllowReceiving: true,
	}, nil).Times(1)
	redisRepo.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()

	handler := &BalanceHandler{Command: &command.UseCase{BalanceRepo: balanceRepo, TransactionRedisRepo: redisRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"allowSending": false, "allowReceiving": true})
	req := httptest.NewRequest(http.MethodPatch, balPath(orgID, ledgerID, "/balances/"+balanceID.String()), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, balanceID.String(), got["id"])
	assert.Equal(t, false, got["allowSending"])
	assert.Equal(t, true, got["allowReceiving"])
}

func TestUpdateBalance_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. updateBalance's command-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).Times(1)

	handler := &BalanceHandler{Command: &command.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"allowSending": false, "allowReceiving": true})
	req := httptest.NewRequest(http.MethodPatch, balPath(orgID, ledgerID, "/balances/"+balanceID.String()), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestCreateAdditionalBalance_Success(t *testing.T) {
	// NOT parallel: process-global huma state. MONEY-adjacent write — transport only.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	// The key must be free, then the default balance is copied for its properties.
	balanceRepo.EXPECT().FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "freeze-assets").
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).Times(1)
	balanceRepo.EXPECT().FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "default").
		Return(&mmodel.Balance{
			ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      accountID.String(),
			Alias:          "@user1",
			Key:            "default",
			AssetCode:      "USD",
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}, nil).Times(1)
	balanceRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) { return b, nil }).Times(1)

	// The new balance inherits the owning account's block state and has that
	// projection re-verified after the INSERT: one read before, one after. The
	// account is unblocked, so the pair is converged and no realign is issued.
	accountRepo := account.NewMockRepository(ctrl)
	accountRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, nil, accountID, mmodel.HolderOffV1).
		Return(&mmodel.Account{ID: accountID.String(), Type: "deposit"}, nil).
		Times(2)

	handler := &BalanceHandler{Command: &command.UseCase{AccountRepo: accountRepo, BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"key": "freeze-assets", "allowSending": false, "allowReceiving": true})
	req := httptest.NewRequest(http.MethodPost, balPath(orgID, ledgerID, "/accounts/"+accountID.String()+"/balances"), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "freeze-assets", got["key"])
}

func TestCreateAdditionalBalance_ServiceError_Canonical4xx(t *testing.T) {
	// NOT parallel: process-global huma state. createAdditionalBalance's
	// command-error branch: a key that already exists is a client error, not a 5xx.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "freeze-assets").
		Return(&mmodel.Balance{ID: uuid.Must(libCommons.GenerateUUIDv7()).String(), Key: "freeze-assets"}, nil).Times(1)

	handler := &BalanceHandler{Command: &command.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"key": "freeze-assets", "allowSending": false, "allowReceiving": true})
	req := httptest.NewRequest(http.MethodPost, balPath(orgID, ledgerID, "/accounts/"+accountID.String()+"/balances"), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.GreaterOrEqual(t, resp.StatusCode, http.StatusBadRequest)
	assert.Less(t, resp.StatusCode, http.StatusInternalServerError, "a duplicate balance key is a client error, never a 5xx")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestGetBalancesByAlias_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. getBalancesByAlias' query-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().ListByAliases(gomock.Any(), orgID, ledgerID, []string{"@person1"}).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/accounts/alias/@person1/balances"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetBalancesExternalByCode_EmptyItems(t *testing.T) {
	// NOT parallel: process-global huma state. The code path derives the external
	// alias (DefaultExternalAccountAliasPrefix + code) before the query.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().
		ListByAliases(gomock.Any(), orgID, ledgerID, []string{constant.DefaultExternalAccountAliasPrefix + "BRL"}).
		Return([]*mmodel.Balance{}, nil).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/accounts/external/BRL/balances"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "items")
	assert.EqualValues(t, 10, got["limit"])
}

func TestGetBalancesExternalByCode_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().
		ListByAliases(gomock.Any(), orgID, ledgerID, []string{constant.DefaultExternalAccountAliasPrefix + "BRL"}).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/accounts/external/BRL/balances"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetBalanceAtTimestamp_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	date := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	available := decimal.NewFromInt(5000)
	onHold := decimal.NewFromInt(500)
	version := int64(10)

	balanceRepo := balance.NewMockRepository(ctrl)
	operationRepo := operation.NewMockRepository(ctrl)

	balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(&mmodel.Balance{
			ID:             balanceID.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      accountID.String(),
			Alias:          "@user1",
			Key:            "default",
			AssetCode:      "USD",
			AccountType:    "deposit",
			CreatedAt:      date.Add(-24 * time.Hour),
		}, nil).Times(1)
	operationRepo.EXPECT().
		FindLastOperationBeforeTimestamp(gomock.Any(), orgID, ledgerID, accountID, balanceID, gomock.Any()).
		Return(&operation.Operation{
			ID:         uuid.Must(libCommons.GenerateUUIDv7()).String(),
			AccountID:  accountID.String(),
			BalanceKey: "default",
			AssetCode:  "USD",
			BalanceAfter: operation.Balance{
				Available: &available,
				OnHold:    &onHold,
				Version:   &version,
			},
			CreatedAt: date.Add(-time.Hour),
		}, nil).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/balances/"+balanceID.String()+"/history?date=2024-01-15%2010:30:00"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, balanceID.String(), got["id"])
}

func TestGetBalanceAtTimestamp_BadDateFormat_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state. parseBalanceHistoryDate rejects a
	// date with no time component; the service is never reached.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balance.NewMockRepository(ctrl)}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/balances/"+balanceID.String()+"/history?date=2024-01-15"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "a date without a time component stays canonical 400")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidDatetimeFormat.Error(), got["code"])
}

func TestGetBalanceAtTimestamp_UnparseableDate_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state. parseBalanceHistoryDate's parse-error
	// branch, distinct from the missing-time-component branch above.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balance.NewMockRepository(ctrl)}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/balances/"+balanceID.String()+"/history?date=not-a-date"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidDatetimeFormat.Error(), got["code"])
}

func TestGetBalanceAtTimestamp_ServiceError(t *testing.T) {
	// NOT parallel: process-global huma state. getBalanceAtTimestamp's query-error
	// branch (HandleSpanError — treated as technical).
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/balances/"+balanceID.String()+"/history?date=2024-01-15%2010:30:00"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetAccountBalancesAtTimestamp_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	date := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, date).
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
				CreatedAt:      date.Add(-24 * time.Hour),
				UpdatedAt:      date.Add(-time.Hour),
			},
		}, nil).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/accounts/"+accountID.String()+"/balances/history?date=2024-01-15%2010:30:00"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")
}

func TestGetAccountBalancesAtTimestamp_NoData_Canonical4xx(t *testing.T) {
	// NOT parallel: process-global huma state. An empty result at the requested
	// timestamp is its own canonical business error, not an empty 200.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	date := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, date).
		Return([]*mmodel.Balance{}, nil).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/accounts/"+accountID.String()+"/balances/history?date=2024-01-15%2010:30:00"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.GreaterOrEqual(t, resp.StatusCode, http.StatusBadRequest)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrNoBalanceDataAtTimestamp.Error(), got["code"])
}

func TestGetAccountBalancesAtTimestamp_ServiceError(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	date := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, date).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).Times(1)

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balanceRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/accounts/"+accountID.String()+"/balances/history?date=2024-01-15%2010:30:00"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDeleteBalance_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. deleteBalance's command-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).Times(1)

	redisRepo := redis.NewMockRedisRepository(ctrl)
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	handler := &BalanceHandler{Command: &command.UseCase{BalanceRepo: balanceRepo, TransactionRedisRepo: redisRepo}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, balPath(orgID, ledgerID, "/balances/"+balanceID.String()), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestGetAccountBalancesAtTimestamp_MissingDate_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state. The account-history op runs the same
	// parseBalanceHistoryDate guard as the balance-history op; the service is never
	// reached.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BalanceHandler{Query: &query.UseCase{BalanceRepo: balance.NewMockRepository(ctrl)}}

	app := buildHumaBalanceApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, balPath(orgID, ledgerID, "/accounts/"+accountID.String()+"/balances/history"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "missing date query stays canonical 400")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrMissingRequiredQueryParameter.Error(), got["code"])
}
