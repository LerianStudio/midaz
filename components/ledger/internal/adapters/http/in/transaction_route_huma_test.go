// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaTransactionRouteApp mounts the five transaction-route Huma operations on
// a /v1 group, faithfully mirroring the production wiring (see buildHumaAssetApp for
// the full rationale). auth resource is "transaction-routes" under the "routing"
// appName (protectedRouting in routes.go); the auth shim stands in for
// auth.Authorize("routing","transaction-routes",verb) + tenant PostAuthMiddlewares.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError
// hook and Huma validation uses process-global sync.Pools; concurrent builds/requests
// cross-contaminate. These tests are sub-second; keep them sequential.
func buildHumaTransactionRouteApp(t *testing.T, handler *TransactionRouteHandler, authOK bool) *fiber.App {
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

	parse := pkgHTTP.ParseUUIDPathParameters("transaction_route")
	base := "/organizations/:organization_id/ledgers/:ledger_id/transaction-routes"
	apiV1.Post(base, parse)
	apiV1.Get(base, parse)
	apiV1.Get(base+"/:transaction_route_id", parse)
	apiV1.Patch(base+"/:transaction_route_id", parse)
	apiV1.Delete(base+"/:transaction_route_id", parse)

	RegisterTransactionRouteRoutesToApp(hAPI, handler)

	return f
}

func TestHuma_CreateTransactionRoute_AuthPreserved(t *testing.T) {
	// NOT parallel: buildHumaTransactionRouteApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// No repo expectations: a rejected auth must never reach the service.
	handler := &TransactionRouteHandler{Command: &command.UseCase{
		TransactionRouteRepo:    transactionroute.NewMockRepository(ctrl),
		TransactionMetadataRepo: mongodb.NewMockRepository(ctrl),
		TransactionRedisRepo:    redis.NewMockRedisRepository(ctrl),
	}}

	app := buildHumaTransactionRouteApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"title": "Settlement", "operationRoutes": []string{uuid.NewString()}})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestHuma_CreateTransactionRoute_ValidationError_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Missing required "title" -> imperative ValidateStruct -> canonical 400, service never reached.
	handler := &TransactionRouteHandler{Command: &command.UseCase{
		TransactionRouteRepo:    transactionroute.NewMockRepository(ctrl),
		TransactionMetadataRepo: mongodb.NewMockRepository(ctrl),
		TransactionRedisRepo:    redis.NewMockRedisRepository(ctrl),
	}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"description": "no title"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "imperative validation stays 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.NotEmpty(t, got["code"], "canonical code present")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"])
}

func TestHuma_CreateTransactionRoute_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Malformed JSON -> DecodeAndValidate returns 0094; HumaProblem must project it
	// to problem+json at 400 (NOT the 500 fallback, NOT a native 422). Service never reached.
	handler := &TransactionRouteHandler{Command: &command.UseCase{
		TransactionRouteRepo:    transactionroute.NewMockRepository(ctrl),
		TransactionMetadataRepo: mongodb.NewMockRepository(ctrl),
		TransactionRedisRepo:    redis.NewMockRedisRepository(ctrl),
	}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes", bytes.NewReader([]byte("{not valid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed body stays 400 — no 500, no native 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), got["code"], "malformed-body code preserved (0094)")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"])
}

func TestHuma_GetTransactionRouteByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	id := uuid.New()

	trRepo := transactionroute.NewMockRepository(ctrl)
	metaRepo := mongodb.NewMockRepository(ctrl)

	trRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, id).
		Return(&mmodel.TransactionRoute{ID: id, OrganizationID: orgID, LedgerID: ledgerID, Title: "Settlement"}, nil).Times(1)
	metaRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransactionRoute, id.String()).Return(nil, nil).Times(1)

	handler := &TransactionRouteHandler{Query: &query.UseCase{TransactionRouteRepo: trRepo, TransactionMetadataRepo: metaRepo}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes/"+id.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, id.String(), got["id"])
	assert.Equal(t, "Settlement", got["title"])
}

func TestHuma_GetTransactionRouteByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Service must never be reached: ParseUUIDPathParameters rejects the bad id
	// with the canonical 0065 / 400 before Huma.
	handler := &TransactionRouteHandler{Query: &query.UseCase{
		TransactionRouteRepo:    transactionroute.NewMockRepository(ctrl),
		TransactionMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestHuma_GetAllTransactionRoutes_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	trRepo := transactionroute.NewMockRepository(ctrl)
	// nil slice -> query use case skips the metadata FindList join (empty page).
	trRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, nil).Times(1)

	handler := &TransactionRouteHandler{Query: &query.UseCase{TransactionRouteRepo: trRepo}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes?limit=10", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "items")
	assert.EqualValues(t, 10, got["limit"])
}

func TestHuma_GetAllTransactionRoutes_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Service must never be reached: ValidateParameters rejects limit=abc with the
	// canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler := &TransactionRouteHandler{Query: &query.UseCase{TransactionRouteRepo: transactionroute.NewMockRepository(ctrl)}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes?limit=abc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestHuma_DeleteTransactionRoute_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	id := uuid.New()

	trRepo := transactionroute.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)

	// Command.DeleteTransactionRouteByID: FindByID then Delete; the wrapper then
	// clears the cache (Del). Cache failure is logged, never returned.
	trRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, id).
		Return(&mmodel.TransactionRoute{ID: id, OrganizationID: orgID, LedgerID: ledgerID, Title: "Settlement"}, nil).Times(1)
	trRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, id, gomock.Any()).Return(nil).Times(1)
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	handler := &TransactionRouteHandler{Command: &command.UseCase{
		TransactionRouteRepo: trRepo,
		TransactionRedisRepo: redisRepo,
	}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes/"+id.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}
