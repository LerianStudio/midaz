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

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"

	libHTTP "github.com/LerianStudio/lib-commons/v6/commons/net/http"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaTransactionRouteApp mounts the five transaction-route Huma operations on
// a /v1 group, faithfully mirroring the production wiring (see buildHumaAssetApp for
// the full rationale). auth resource is "transaction-routes" under the "midaz"
// appName (protectedMidaz in routes.go); the auth shim stands in for
// auth.Authorize("midaz","transaction-routes",verb) + tenant PostAuthMiddlewares.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError
// hook and Huma validation uses process-global sync.Pools; concurrent builds/requests
// cross-contaminate. These tests are sub-second; keep them sequential.
func buildHumaTransactionRouteApp(t *testing.T, handler *TransactionRouteHandler, authOK bool) *fiber.App {
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

	parse := pkgHTTP.ParseUUIDPathParameters("transaction_route")
	base := "/organizations/:organization_id/ledgers/:ledger_id/transaction-routes"
	apiV1.Post(base, parse)
	apiV1.Get(base, parse)
	apiV1.Get(base+"/:transaction_route_id", parse)
	apiV1.Patch(base+"/:transaction_route_id", parse)
	apiV1.Delete(base+"/:transaction_route_id", parse)

	RegisterTransactionRouteRoutes(hAPI, handler, routeOpSuffixV1)

	return f
}

func TestCreateTransactionRoute_AuthPreserved(t *testing.T) {
	// NOT parallel: buildHumaTransactionRouteApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

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

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestCreateTransactionRoute_ValidationError_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

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

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "imperative validation stays 400 — no native Huma 422")
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.NotEmpty(t, got["code"], "canonical code present")
	assert.NotContains(t, got, "status", "the v1 envelope carries no status member")
}

func TestCreateTransactionRoute_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

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

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed body stays 400 — no 500, no native 422")
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), got["code"], "malformed-body code preserved (0094)")
	assert.NotContains(t, got, "status", "the v1 envelope carries no status member")
}

func TestGetTransactionRouteByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	id := uuid.Must(libCommons.GenerateUUIDv7())

	trRepo := transactionroute.NewMockRepository(ctrl)
	metaRepo := mongodb.NewMockRepository(ctrl)

	trRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, id).
		Return(&mmodel.TransactionRoute{ID: id, OrganizationID: orgID, LedgerID: ledgerID, Title: "Settlement"}, nil).Times(1)
	metaRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransactionRoute, id.String()).Return(nil, nil).Times(1)

	handler := &TransactionRouteHandler{Query: &query.UseCase{TransactionRouteRepo: trRepo, TransactionMetadataRepo: metaRepo}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes/"+id.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestGetTransactionRouteByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ParseUUIDPathParameters rejects the bad id
	// with the canonical 0065 / 400 before Huma.
	handler := &TransactionRouteHandler{Query: &query.UseCase{
		TransactionRouteRepo:    transactionroute.NewMockRepository(ctrl),
		TransactionMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes/not-a-uuid", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestGetAllTransactionRoutes_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	trRepo := transactionroute.NewMockRepository(ctrl)
	// nil slice -> query use case skips the metadata FindList join (empty page).
	trRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, nil).Times(1)

	handler := &TransactionRouteHandler{Query: &query.UseCase{TransactionRouteRepo: trRepo}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes?limit=10", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestGetAllTransactionRoutes_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ValidateParameters rejects limit=abc with the
	// canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler := &TransactionRouteHandler{Query: &query.UseCase{TransactionRouteRepo: transactionroute.NewMockRepository(ctrl)}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transaction-routes?limit=abc", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestDeleteTransactionRoute_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	id := uuid.Must(libCommons.GenerateUUIDv7())

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
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

//
// The five exported fiber.Ctx terminals on TransactionRouteHandler were deleted
// with the Huma migration; the branches their tests covered in the shared cores
// are exercised here through the live Huma transport instead.

func trPath(orgID, ledgerID uuid.UUID, suffix string) string {
	return "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transaction-routes" + suffix
}

func TestCreateTransactionRoute_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	op1 := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")
	op2 := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b")

	trRepo := transactionroute.NewMockRepository(ctrl)
	orRepo := operationroute.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)

	orRepo.EXPECT().FindByIDs(gomock.Any(), orgID, ledgerID, []uuid.UUID{op1, op2}).
		Return([]*mmodel.OperationRoute{
			{ID: op1, OperationType: "source", Title: "Source Route"},
			{ID: op2, OperationType: "destination", Title: "Destination Route"},
		}, nil).Times(1)
	trRepo.EXPECT().Create(gomock.Any(), orgID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ any, oID, lID uuid.UUID, tr *mmodel.TransactionRoute) (*mmodel.TransactionRoute, error) {
			tr.OrganizationID = oID
			tr.LedgerID = lID
			return tr, nil
		}).Times(1)
	metadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	// The accounting-route cache write is best-effort; the core logs and continues.
	redisRepo.EXPECT().SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()

	handler := &TransactionRouteHandler{Command: &command.UseCase{
		TransactionRouteRepo:    trRepo,
		OperationRouteRepo:      orRepo,
		TransactionMetadataRepo: metadataRepo,
		TransactionRedisRepo:    redisRepo,
	}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	body := `{"title":"Payment Settlement","description":"Route for payment settlement transactions","operationRoutes":["` +
		op1.String() + `","` + op2.String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, trPath(orgID, ledgerID, ""), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", string(respBody))
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Payment Settlement", got["title"])
}

func TestCreateTransactionRoute_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. createTransactionRoute's
	// command-error branch, before the cache write and the metric.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	op1 := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")

	orRepo := operationroute.NewMockRepository(ctrl)
	orRepo.EXPECT().FindByIDs(gomock.Any(), orgID, ledgerID, []uuid.UUID{op1}).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityTransactionRoute)).Times(1)

	handler := &TransactionRouteHandler{Command: &command.UseCase{
		TransactionRouteRepo: transactionroute.NewMockRepository(ctrl),
		OperationRouteRepo:   orRepo,
	}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	body := `{"title":"Payment Settlement","description":"d","operationRoutes":["` + op1.String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, trPath(orgID, ledgerID, ""), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "body: %s", string(respBody))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestGetTransactionRouteByID_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. getTransactionRouteByID's error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	id := uuid.Must(libCommons.GenerateUUIDv7())

	trRepo := transactionroute.NewMockRepository(ctrl)
	trRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, id).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityTransactionRoute)).Times(1)

	handler := &TransactionRouteHandler{Query: &query.UseCase{TransactionRouteRepo: trRepo}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, trPath(orgID, ledgerID, "/"+id.String()), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestUpdateTransactionRoute_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	id := uuid.Must(libCommons.GenerateUUIDv7())

	trRepo := transactionroute.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)

	trRepo.EXPECT().FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), gomock.Any()).
		Return(map[uuid.UUID][]uuid.UUID{}, nil).AnyTimes()
	trRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, id, gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.TransactionRoute{ID: id, OrganizationID: orgID, LedgerID: ledgerID, Title: "Renamed Route"}, nil).Times(1)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	metadataRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	redisRepo.EXPECT().SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()

	handler := &TransactionRouteHandler{Command: &command.UseCase{
		TransactionRouteRepo:    trRepo,
		TransactionMetadataRepo: metadataRepo,
		TransactionRedisRepo:    redisRepo,
	}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPatch, trPath(orgID, ledgerID, "/"+id.String()), bytes.NewBufferString(`{"title":"Renamed Route"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Renamed Route", got["title"])
}

func TestUpdateTransactionRoute_NotFound_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. updateTransactionRoute's error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	id := uuid.Must(libCommons.GenerateUUIDv7())

	trRepo := transactionroute.NewMockRepository(ctrl)
	trRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, id, gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityTransactionRoute)).Times(1)

	handler := &TransactionRouteHandler{Command: &command.UseCase{TransactionRouteRepo: trRepo}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPatch, trPath(orgID, ledgerID, "/"+id.String()), bytes.NewBufferString(`{"title":"Renamed Route"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDeleteTransactionRoute_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. deleteTransactionRouteByID's
	// command-error branch, before the cache delete.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	id := uuid.Must(libCommons.GenerateUUIDv7())

	trRepo := transactionroute.NewMockRepository(ctrl)
	trRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, id).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityTransactionRoute)).Times(1)

	handler := &TransactionRouteHandler{Command: &command.UseCase{
		TransactionRouteRepo: trRepo,
		TransactionRedisRepo: redis.NewMockRedisRepository(ctrl),
	}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, trPath(orgID, ledgerID, "/"+id.String()), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetAllTransactionRoutes_MetadataFilter(t *testing.T) {
	// NOT parallel: process-global huma state. getAllTransactionRoutes' metadata
	// branch, which returns its own cursor envelope.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	id := uuid.Must(libCommons.GenerateUUIDv7())

	trRepo := transactionroute.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	metadataRepo.EXPECT().FindList(gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*mongodb.Metadata{{EntityID: id.String(), Data: map[string]any{"tier": "premium"}}}, nil).Times(1)
	trRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return([]*mmodel.TransactionRoute{{ID: id, OrganizationID: orgID, LedgerID: ledgerID, Title: "Premium"}},
			libHTTP.CursorPagination{}, nil).Times(1)
	trRepo.EXPECT().FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), gomock.Any()).
		Return(map[uuid.UUID][]uuid.UUID{}, nil).AnyTimes()

	handler := &TransactionRouteHandler{Query: &query.UseCase{
		TransactionRouteRepo:    trRepo,
		TransactionMetadataRepo: metadataRepo,
	}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, trPath(orgID, ledgerID, "?metadata.tier=premium"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
}

func TestGetAllTransactionRoutes_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. The plain query-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	trRepo := transactionroute.NewMockRepository(ctrl)
	trRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityTransactionRoute)).Times(1)

	handler := &TransactionRouteHandler{Query: &query.UseCase{TransactionRouteRepo: trRepo}}

	app := buildHumaTransactionRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, trPath(orgID, ledgerID, ""), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
