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

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	txMongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaOperationApp mirrors buildHumaBalanceApp: problem.Install before any
// huma.Register, the Huma API over a /v1 group, an auth shim standing in for
// auth.Authorize("midaz","operations","get") + tenant PostAuthMiddlewares, and
// http.ParseUUIDPathParameters("operation") + RegisterOperationRoutesToApp.
//
// MUST-NOT-PARALLELIZE (same rationale as the asset/balance harness):
// libProblem.Install() swaps the process-global huma.NewError hook and Huma
// validation uses process-global sync.Pools. Keep these sequential.
func buildHumaOperationApp(t *testing.T, handler *OperationHandler, authOK bool) *fiber.App {
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

	parse := pkgHTTP.ParseUUIDPathParameters("operation")
	base := "/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations"
	apiV1.Get(base, parse)
	apiV1.Get(base+"/:operation_id", parse)

	RegisterOperationRoutesToApp(hAPI, handler)

	return f
}

func TestHuma_GetAllOperationsByAccount_Success(t *testing.T) {
	// NOT parallel: buildHumaOperationApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	// Default (non-metadata) path: empty operations returns BEFORE the mongodb
	// metadata overlay, so no TransactionMetadataRepo mock is needed.
	opRepo := operation.NewMockRepository(ctrl)
	opRepo.EXPECT().
		FindAllByAccount(gomock.Any(), orgID, ledgerID, accountID, gomock.Any(), gomock.Any()).
		Return([]*operation.Operation{}, libHTTP.CursorPagination{}, nil).Times(1)

	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: opRepo}}

	app := buildHumaOperationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations?limit=10", nil)
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

func TestHuma_GetAllOperationsByAccount_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	// No repo expectations: a rejected auth must never reach the service.
	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: operation.NewMockRepository(ctrl)}}

	app := buildHumaOperationApp(t, handler, false)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestHuma_GetAllOperationsByAccount_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	// Service must never be reached: ValidateParameters rejects limit=abc with
	// the canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: operation.NewMockRepository(ctrl)}}

	app := buildHumaOperationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations?limit=abc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestHuma_GetAllOperationsByAccount_BadAccountUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Service must never be reached: ParseUUIDPathParameters rejects the bad
	// account_id with the canonical 0065 / 400 before Huma.
	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: operation.NewMockRepository(ctrl)}}

	app := buildHumaOperationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/not-a-uuid/operations", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestHuma_GetOperationByAccount_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	operationID := uuid.New()

	opRepo := operation.NewMockRepository(ctrl)
	metaRepo := txMongodb.NewMockRepository(ctrl)

	opRepo.EXPECT().FindByAccount(gomock.Any(), orgID, ledgerID, accountID, operationID).
		Return(&operation.Operation{ID: operationID.String(), AccountID: accountID.String()}, nil).Times(1)
	metaRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityOperation, operationID.String()).Return(nil, nil).Times(1)

	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: opRepo, TransactionMetadataRepo: metaRepo}}

	app := buildHumaOperationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations/"+operationID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, operationID.String(), got["id"])
}

func TestHuma_GetOperationByAccount_BadOperationUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	// Service must never be reached: ParseUUIDPathParameters rejects the bad
	// operation_id with the canonical 0065 / 400 before Huma.
	handler := &OperationHandler{Query: &query.UseCase{OperationRepo: operation.NewMockRepository(ctrl)}}

	app := buildHumaOperationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/operations/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}
