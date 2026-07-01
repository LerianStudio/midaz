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
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaOperationRouteApp mounts the five operation-route Huma operations on a
// /v1 group, faithfully mirroring the production wiring (see buildHumaAssetApp for
// the full rationale). auth resource is "operation-routes" under the "routing"
// appName; the auth shim stands in for auth.Authorize("routing","operation-routes",
// verb) + tenant PostAuthMiddlewares.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError
// hook and Huma validation uses process-global sync.Pools; concurrent builds/requests
// cross-contaminate. These tests are sub-second; keep them sequential.
func buildHumaOperationRouteApp(t *testing.T, handler *OperationRouteHandler, authOK bool) *fiber.App {
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

	parse := pkgHTTP.ParseUUIDPathParameters("operation_route")
	base := "/organizations/:organization_id/ledgers/:ledger_id/operation-routes"
	apiV1.Post(base, parse)
	apiV1.Get(base, parse)
	apiV1.Get(base+"/:operation_route_id", parse)
	apiV1.Patch(base+"/:operation_route_id", parse)
	apiV1.Delete(base+"/:operation_route_id", parse)

	RegisterOperationRouteRoutesToApp(hAPI, handler)

	return f
}

func TestHuma_CreateOperationRoute_Success(t *testing.T) {
	// NOT parallel: buildHumaOperationRouteApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	orRepo := operationroute.NewMockRepository(ctrl)
	metaRepo := mongodb.NewMockRepository(ctrl)

	orRepo.EXPECT().Create(gomock.Any(), orgID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ any, oID, lID uuid.UUID, or *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
			or.ID = uuid.New()
			or.OrganizationID = oID
			or.LedgerID = lID
			or.CreatedAt = time.Now()
			or.UpdatedAt = time.Now()
			return or, nil
		}).Times(1)
	metaRepo.EXPECT().Create(gomock.Any(), constant.EntityOperationRoute, gomock.Any()).Return(nil).Times(1)

	handler := &OperationRouteHandler{Command: &command.UseCase{
		OperationRouteRepo:      orRepo,
		TransactionMetadataRepo: metaRepo,
	}}

	app := buildHumaOperationRouteApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"title": "Cashin Route", "operationType": "source"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/operation-routes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", string(respBody))
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Cashin Route", got["title"])
	assert.Equal(t, "source", got["operationType"])
}

func TestHuma_CreateOperationRoute_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// No repo expectations: a rejected auth must never reach the service.
	handler := &OperationRouteHandler{Command: &command.UseCase{
		OperationRouteRepo:      operationroute.NewMockRepository(ctrl),
		TransactionMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaOperationRouteApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"title": "Cashin Route", "operationType": "source"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/operation-routes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestHuma_CreateOperationRoute_UnknownAccountingEntryKey_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	// The accountingEntries unknown-key probe (operation_route.go create path) must
	// be reproduced in the Huma core against in.RawBody. "foobar" is not a valid
	// accountingEntries key -> canonical 400, service never reached.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	handler := &OperationRouteHandler{Command: &command.UseCase{
		OperationRouteRepo:      operationroute.NewMockRepository(ctrl),
		TransactionMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaOperationRouteApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{
		"title":         "Route",
		"operationType": "source",
		"accountingEntries": map[string]any{
			"foobar": map[string]any{"debit": map[string]any{"code": "x", "description": "y"}},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/operation-routes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "unknown accountingEntries key -> canonical 400; body: %s", string(respBody))
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
}

func TestHuma_GetOperationRouteByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	id := uuid.New()

	orRepo := operationroute.NewMockRepository(ctrl)
	metaRepo := mongodb.NewMockRepository(ctrl)

	orRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, id).
		Return(&mmodel.OperationRoute{ID: id, OrganizationID: orgID, LedgerID: ledgerID, Title: "Route", OperationType: "source"}, nil).Times(1)
	metaRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityOperationRoute, id.String()).Return(nil, nil).Times(1)

	handler := &OperationRouteHandler{Query: &query.UseCase{OperationRouteRepo: orRepo, TransactionMetadataRepo: metaRepo}}

	app := buildHumaOperationRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/operation-routes/"+id.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, id.String(), got["id"])
	assert.Equal(t, "Route", got["title"])
}

func TestHuma_GetOperationRouteByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	handler := &OperationRouteHandler{Query: &query.UseCase{
		OperationRouteRepo:      operationroute.NewMockRepository(ctrl),
		TransactionMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaOperationRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/operation-routes/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestHuma_DeleteOperationRoute_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	id := uuid.New()

	orRepo := operationroute.NewMockRepository(ctrl)
	// Command.DeleteOperationRouteByID checks for transaction-route links before deleting.
	orRepo.EXPECT().HasTransactionRouteLinks(gomock.Any(), orgID, ledgerID, id).Return(false, nil).Times(1)
	orRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, id).Return(nil).Times(1)

	handler := &OperationRouteHandler{Command: &command.UseCase{OperationRouteRepo: orRepo}}

	app := buildHumaOperationRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/operation-routes/"+id.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestHuma_GetAllOperationRoutes_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	orRepo := operationroute.NewMockRepository(ctrl)
	// nil slice -> query use case skips the metadata FindList join (empty page).
	orRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, nil).Times(1)

	handler := &OperationRouteHandler{Query: &query.UseCase{OperationRouteRepo: orRepo}}

	app := buildHumaOperationRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/operation-routes?limit=10", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "items")
	assert.EqualValues(t, 10, got["limit"])
}

func TestHuma_GetAllOperationRoutes_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	handler := &OperationRouteHandler{Query: &query.UseCase{OperationRouteRepo: operationroute.NewMockRepository(ctrl)}}

	app := buildHumaOperationRouteApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/operation-routes?limit=abc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

// TestHuma_UpdateOperationRoute_MergePatch is the LANDMINE test for RFC 7396 JSON
// Merge Patch. The distinction that would break SILENTLY if the Huma core fed
// typed-decoded bytes (which collapse absent vs null) instead of in.RawBody:
//
//   - overdraft key ABSENT inside accountingEntries  -> existing overdraft KEPT
//   - overdraft key explicitly null                  -> existing overdraft REMOVED
//
// The existing route carries direct + overdraft (operationType "source"). We capture
// the payload.AccountingEntriesRaw that reaches Command.UpdateOperationRoute and
// assert the raw bytes preserve (or omit) the "overdraft":null token — proving the
// Huma shell forwarded in.RawBody to the core byte-for-byte, exactly like the Fiber
// c.Body() path. Both cases fetch the existing route and validate the merged matrix.
func TestHuma_UpdateOperationRoute_MergePatch(t *testing.T) {
	directRubric := func() *mmodel.AccountingEntry {
		return &mmodel.AccountingEntry{Debit: &mmodel.AccountingRubric{Code: "D1", Description: "direct debit"}}
	}
	overdraftRubric := func() *mmodel.AccountingEntry {
		return &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "OD", Description: "overdraft debit"},
			Credit: &mmodel.AccountingRubric{Code: "OC", Description: "overdraft credit"},
		}
	}
	existing := func(orgID, ledgerID, id uuid.UUID) *mmodel.OperationRoute {
		return &mmodel.OperationRoute{
			ID: id, OrganizationID: orgID, LedgerID: ledgerID,
			Title: "Existing", OperationType: "source",
			AccountingEntries: &mmodel.AccountingEntries{
				Direct:    directRubric(),
				Overdraft: overdraftRubric(),
			},
		}
	}

	// direct entry sent on every PATCH (unchanged); only overdraft differs.
	directBody := map[string]any{
		"debit": map[string]any{"code": "D1", "description": "direct debit"},
	}

	tests := []struct {
		name              string
		accountingEntries map[string]any
		wantRawContains   string // token that must survive in AccountingEntriesRaw
	}{
		{
			name:              "overdraft ABSENT keeps existing overdraft",
			accountingEntries: map[string]any{"direct": directBody},
			wantRawContains:   "", // no "overdraft" token at all
		},
		{
			name:              "overdraft null removes existing overdraft",
			accountingEntries: map[string]any{"direct": directBody, "overdraft": nil},
			wantRawContains:   `"overdraft":null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// NOT parallel: process-global huma state.
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New()
			ledgerID := uuid.New()
			id := uuid.New()

			orRepo := operationroute.NewMockRepository(ctrl)
			metaRepo := mongodb.NewMockRepository(ctrl)

			// Both cases fetch existing (accountingEntries present as object -> raw probe fires).
			// FindByEntity fires twice: once for the fetch (Query.GetOperationRouteByID's
			// metadata join) and once for UpdateMetadata (DecodeAndValidate leaves Metadata a
			// non-nil empty map, so the nil-skip branch is not taken).
			orRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, id).Return(existing(orgID, ledgerID, id), nil).Times(1)
			metaRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityOperationRoute, id.String()).Return(nil, nil).Times(2)

			var capturedRaw string
			orRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, id, gomock.Any()).
				DoAndReturn(func(_ any, _, _, _ uuid.UUID, in *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
					capturedRaw = string(in.AccountingEntriesRaw)
					return &mmodel.OperationRoute{ID: id, OrganizationID: orgID, LedgerID: ledgerID, Title: "Existing", OperationType: "source"}, nil
				}).Times(1)

			metaRepo.EXPECT().Update(gomock.Any(), constant.EntityOperationRoute, id.String(), gomock.Any()).Return(nil).Times(1)

			handler := &OperationRouteHandler{
				Command: &command.UseCase{OperationRouteRepo: orRepo, TransactionMetadataRepo: metaRepo},
				Query:   &query.UseCase{OperationRouteRepo: orRepo, TransactionMetadataRepo: metaRepo},
			}

			app := buildHumaOperationRouteApp(t, handler, true)

			body, _ := json.Marshal(map[string]any{"accountingEntries": tt.accountingEntries})
			req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/operation-routes/"+id.String(), bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, _ := io.ReadAll(resp.Body)
			require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))

			// The raw bytes forwarded from in.RawBody must preserve the null distinction.
			if tt.wantRawContains == "" {
				assert.NotContains(t, capturedRaw, "overdraft", "absent overdraft must not appear in AccountingEntriesRaw")
			} else {
				assert.Contains(t, capturedRaw, tt.wantRawContains, "explicit null overdraft must survive byte-for-byte in AccountingEntriesRaw")
			}
		})
	}
}
