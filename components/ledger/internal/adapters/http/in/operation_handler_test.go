// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	txMongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaOperationApp mirrors buildHumaBalanceApp: problem.Install before any
// huma.Register, the Huma API over a /v1 group, an auth shim standing in for
// auth.Authorize("midaz","operations",verb) + tenant PostAuthMiddlewares, and
// http.ParseUUIDPathParameters("operation") + RegisterOperationRoutes.
//
// MUST-NOT-PARALLELIZE (same rationale as the asset/balance harness):
// libProblem.Install() swaps the process-global huma.NewError hook and Huma
// validation uses process-global sync.Pools. Keep these sequential.
func buildHumaOperationApp(t *testing.T, handler *OperationHandler, authOK bool) *fiber.App {
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

	parse := pkgHTTP.ParseUUIDPathParameters("operation")
	base := "/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations"
	apiV1.Get(base, parse)
	apiV1.Get(base+"/:operation_id", parse)
	// PATCH is on the transaction path (money-write leg), not the account path.
	apiV1.Patch("/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id", parse)

	RegisterOperationRoutes(hAPI, handler, routeOpSuffixV1)

	return f
}

func TestGetAllOperationsByAccount_Success(t *testing.T) {
	// NOT parallel: buildHumaOperationApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	// Default (non-metadata) path: empty operations returns BEFORE the mongodb
	// metadata overlay, so no TransactionMetadataRepo mock is needed.
	opRepo := operation.NewMockRepository(ctrl)
	opRepo.EXPECT().
		FindAllByAccount(gomock.Any(), orgID, ledgerID, accountID, gomock.Any(), gomock.Any()).
		Return([]*operation.Operation{}, libHTTP.CursorPagination{}, nil).Times(1)

	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: opRepo}}

	app := buildHumaOperationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations?limit=10", nil)
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

func TestGetAllOperationsByAccount_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	// No repo expectations: a rejected auth must never reach the service.
	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: operation.NewMockRepository(ctrl)}}

	app := buildHumaOperationApp(t, handler, false)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestGetAllOperationsByAccount_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ValidateParameters rejects limit=abc with
	// the canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: operation.NewMockRepository(ctrl)}}

	app := buildHumaOperationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations?limit=abc", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestGetAllOperationsByAccount_BadAccountUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ParseUUIDPathParameters rejects the bad
	// account_id with the canonical 0065 / 400 before Huma.
	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: operation.NewMockRepository(ctrl)}}

	app := buildHumaOperationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/not-a-uuid/operations", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestGetOperationByAccount_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	operationID := uuid.Must(libCommons.GenerateUUIDv7())

	opRepo := operation.NewMockRepository(ctrl)
	metaRepo := txMongodb.NewMockRepository(ctrl)

	opRepo.EXPECT().FindByAccount(gomock.Any(), orgID, ledgerID, accountID, operationID).
		Return(&operation.Operation{ID: operationID.String(), AccountID: accountID.String()}, nil).Times(1)
	metaRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityOperation, operationID.String()).Return(nil, nil).Times(1)

	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: opRepo, TransactionMetadataRepo: metaRepo}}

	app := buildHumaOperationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations/"+operationID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, operationID.String(), got["id"])
}

func TestUpdateOperation_BadOperationUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	txID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ParseUUIDPathParameters rejects the bad
	// operation_id with the canonical 0065 / 400 before Huma.
	handler := &OperationHandler{}

	app := buildHumaOperationApp(t, handler, true)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/operations/not-a-uuid"
	req := httptest.NewRequest(http.MethodPatch, url, strings.NewReader(`{"description":"x"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 on PATCH — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestUpdateOperation_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	txID := uuid.Must(libCommons.GenerateUUIDv7())
	operationID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: DecodeAndValidate rejects the malformed body
	// with the canonical 400 BEFORE the command — no native Huma 422.
	handler := &OperationHandler{}

	app := buildHumaOperationApp(t, handler, true)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/operations/" + operationID.String()
	req := httptest.NewRequest(http.MethodPatch, url, strings.NewReader(`{not-json`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed PATCH body stays canonical 400 — no native Huma 422")
}

func TestUpdateOperation_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	txID := uuid.Must(libCommons.GenerateUUIDv7())
	operationID := uuid.Must(libCommons.GenerateUUIDv7())

	// No repo expectations: a rejected auth must never reach the service.
	handler := &OperationHandler{}

	app := buildHumaOperationApp(t, handler, false)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/operations/" + operationID.String()
	req := httptest.NewRequest(http.MethodPatch, url, strings.NewReader(`{"description":"x"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestGetOperationByAccount_BadOperationUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ParseUUIDPathParameters rejects the bad
	// operation_id with the canonical 0065 / 400 before Huma.
	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: operation.NewMockRepository(ctrl)}}

	app := buildHumaOperationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations/not-a-uuid", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestGetAllOperationsByAccount_MetadataFilter_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	opID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	// Metadata branch: query.GetAllMetadataOperations resolves the filter through
	// FindList first, then overlays the matched data onto FindAllByAccount rows.
	opRepo := operation.NewMockRepository(ctrl)
	metaRepo := txMongodb.NewMockRepository(ctrl)

	metaRepo.EXPECT().FindList(gomock.Any(), constant.EntityOperation, gomock.Any()).
		Return([]*txMongodb.Metadata{{
			EntityID:   opID,
			EntityName: constant.EntityOperation,
			Data:       map[string]any{"key": "value"},
		}}, nil).Times(1)

	opRepo.EXPECT().FindAllByAccount(gomock.Any(), orgID, ledgerID, accountID, gomock.Any(), gomock.Any()).
		Return([]*operation.Operation{{ID: opID, AccountID: accountID.String()}}, libHTTP.CursorPagination{}, nil).Times(1)

	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: opRepo, TransactionMetadataRepo: metaRepo}}

	app := buildHumaOperationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations?metadata.key=value", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))

	items, ok := got["items"].([]any)
	require.True(t, ok, "items should be an array")
	require.Len(t, items, 1)

	first, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, opID, first["id"])
	assert.Contains(t, first, "metadata")
}

func TestGetAllOperationsByAccount_MetadataFilter_NotFound(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	// A nil metadata list short-circuits to the canonical 0069 / 404 before the
	// operation repository is touched.
	opRepo := operation.NewMockRepository(ctrl)
	metaRepo := txMongodb.NewMockRepository(ctrl)

	metaRepo.EXPECT().FindList(gomock.Any(), constant.EntityOperation, gomock.Any()).
		Return(nil, nil).Times(1)

	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: opRepo, TransactionMetadataRepo: metaRepo}}

	app := buildHumaOperationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations?metadata.key=value", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrNoOperationsFound.Error(), got["code"])
}

func TestGetAllOperationsByAccount_ServiceError_500(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	opRepo := operation.NewMockRepository(ctrl)
	opRepo.EXPECT().FindAllByAccount(gomock.Any(), orgID, ledgerID, accountID, gomock.Any(), gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, pkg.InternalServerError{
			Code:    "0046",
			Title:   "Internal Server Error",
			Message: "Database connection failed",
		}).Times(1)

	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: opRepo}}

	app := buildHumaOperationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "0046", got["code"])
	assert.Contains(t, got, "message")
}

func TestGetOperationByAccount_NotFound_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	operationID := uuid.Must(libCommons.GenerateUUIDv7())

	opRepo := operation.NewMockRepository(ctrl)
	opRepo.EXPECT().FindByAccount(gomock.Any(), orgID, ledgerID, accountID, operationID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrNoOperationsFound, constant.EntityOperation)).Times(1)

	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: opRepo}}

	app := buildHumaOperationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations/"+operationID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrNoOperationsFound.Error(), got["code"])
}

func TestUpdateOperation_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	txID := uuid.Must(libCommons.GenerateUUIDv7())
	operationID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	opRepo := operation.NewMockRepository(ctrl)
	metaRepo := txMongodb.NewMockRepository(ctrl)

	updated := &operation.Operation{
		ID:             operationID.String(),
		TransactionID:  txID.String(),
		Description:    "Updated operation description",
		AccountID:      accountID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
	}

	opRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, txID, operationID, gomock.Any()).
		Return(updated, nil).Times(1)
	opRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, txID, operationID).
		Return(updated, nil).Times(1)

	// Both the command's metadata upsert and the query re-read hit FindByEntity.
	metaRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityOperation, operationID.String()).
		Return(&txMongodb.Metadata{
			EntityID:   operationID.String(),
			EntityName: constant.EntityOperation,
			Data:       map[string]any{"reason": "Purchase refund"},
		}, nil).AnyTimes()
	metaRepo.EXPECT().Update(gomock.Any(), constant.EntityOperation, operationID.String(), gomock.Any()).
		Return(nil).AnyTimes()

	handler := &OperationHandler{
		Command: &command.UseCase{OperationRepo: opRepo, TransactionMetadataRepo: metaRepo},
		Query:   &query.UseCase{OperationRepo: opRepo, TransactionMetadataRepo: metaRepo},
	}

	app := buildHumaOperationApp(t, handler, true)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/operations/" + operationID.String()
	req := httptest.NewRequest(http.MethodPatch, url, strings.NewReader(`{"description":"Updated operation description","metadata":{"reason":"Purchase refund"}}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, operationID.String(), got["id"])
	assert.Equal(t, "Updated operation description", got["description"])

	metadata, ok := got["metadata"].(map[string]any)
	require.True(t, ok, "metadata should be an object")
	assert.Equal(t, "Purchase refund", metadata["reason"])
}

func TestUpdateOperation_CommandNotFound_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	txID := uuid.Must(libCommons.GenerateUUIDv7())
	operationID := uuid.Must(libCommons.GenerateUUIDv7())

	opRepo := operation.NewMockRepository(ctrl)
	metaRepo := txMongodb.NewMockRepository(ctrl)

	opRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, txID, operationID, gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrNoOperationsFound, constant.EntityOperation)).Times(1)

	handler := &OperationHandler{
		Command: &command.UseCase{OperationRepo: opRepo, TransactionMetadataRepo: metaRepo},
		Query:   &query.UseCase{OperationRepo: opRepo, TransactionMetadataRepo: metaRepo},
	}

	app := buildHumaOperationApp(t, handler, true)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/operations/" + operationID.String()
	req := httptest.NewRequest(http.MethodPatch, url, strings.NewReader(`{"description":"Updated description"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrNoOperationsFound.Error(), got["code"])
}

func TestUpdateOperation_QueryError_500(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	txID := uuid.Must(libCommons.GenerateUUIDv7())
	operationID := uuid.Must(libCommons.GenerateUUIDv7())

	opRepo := operation.NewMockRepository(ctrl)
	metaRepo := txMongodb.NewMockRepository(ctrl)

	// The command succeeds; the re-read fails, so the request surfaces the
	// technical 500 rather than a partially-updated body.
	opRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, txID, operationID, gomock.Any()).
		Return(&operation.Operation{ID: operationID.String()}, nil).Times(1)
	opRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, txID, operationID).
		Return(nil, pkg.InternalServerError{
			Code:    "0046",
			Title:   "Internal Server Error",
			Message: "Database connection failed",
		}).Times(1)

	metaRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityOperation, operationID.String()).
		Return(nil, nil).AnyTimes()
	metaRepo.EXPECT().Update(gomock.Any(), constant.EntityOperation, operationID.String(), gomock.Any()).
		Return(nil).AnyTimes()

	handler := &OperationHandler{
		Command: &command.UseCase{OperationRepo: opRepo, TransactionMetadataRepo: metaRepo},
		Query:   &query.UseCase{OperationRepo: opRepo, TransactionMetadataRepo: metaRepo},
	}

	app := buildHumaOperationApp(t, handler, true)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/operations/" + operationID.String()
	req := httptest.NewRequest(http.MethodPatch, url, strings.NewReader(`{"description":"Updated description"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "0046", got["code"])
	assert.Contains(t, got, "message")
}
