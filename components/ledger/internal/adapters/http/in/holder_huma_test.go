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

	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	holderrepo "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	instrumentrepo "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaHolderApp mounts the five holder Huma operations on a /v1 group,
// faithfully mirroring the production wiring in unified-server.go: problem.Install()
// runs before any huma.Register, the Huma API is built with openapi.New over a /v1
// group, an auth-shim middleware stands in for auth.Authorize("midaz","holders",verb)
// + tenant PostAuthMiddlewares, and http.ParseUUIDPathParameters("holder") +
// RegisterHolderRoutes attach the chain.
//
// MUST-NOT-PARALLELIZE (same rationale as the asset exemplar's buildHumaAssetApp):
// libProblem.Install() swaps the process-global huma.NewError hook and Huma
// validation uses process-global sync.Pools — concurrent builds/requests
// cross-contaminate. These tests are sub-second; keep them sequential.
func buildHumaHolderApp(t *testing.T, handler *HolderHandler, authOK bool) *fiber.App {
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

	parse := pkgHTTP.ParseUUIDPathParameters("holder")
	base := "/organizations/:organization_id/holders"
	apiV1.Post(base, parse)
	apiV1.Get(base, parse)
	apiV1.Get(base+"/:id", parse)
	apiV1.Patch(base+"/:id", parse)
	apiV1.Delete(base+"/:id", parse)

	RegisterHolderRoutes(hAPI, handler)

	return f
}

func newHolderHandler(t *testing.T, ctrl *gomock.Controller) (*HolderHandler, *holderrepo.MockRepository) {
	t.Helper()

	repo := holderrepo.NewMockRepository(ctrl)
	handler := &HolderHandler{Service: &services.UseCase{HolderRepo: repo}}

	return handler, repo
}

func TestHuma_CreateHolder_Success(t *testing.T) {
	// NOT parallel: buildHumaHolderApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		Create(gomock.Any(), orgID.String(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, h *mmodel.Holder) (*mmodel.Holder, error) {
			h.CreatedAt = time.Now()
			h.UpdatedAt = time.Now()
			return h, nil
		}).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"type": "NATURAL_PERSON", "name": "John Doe", "document": "91315026015"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/holders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, "false", resp.Header.Get("X-Idempotency-Replayed"), "fresh create is not a replay")
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "John Doe", got["name"])
	assert.Equal(t, "NATURAL_PERSON", got["type"])
}

func TestHuma_CreateHolder_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	// No repo expectations: a rejected auth must never reach the service.
	handler, _ := newHolderHandler(t, ctrl)

	app := buildHumaHolderApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"type": "NATURAL_PERSON", "name": "John Doe", "document": "91315026015"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/holders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestHuma_CreateHolder_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	// Malformed JSON -> DecodeAndValidate returns 0094; HumaProblem projects it to
	// problem+json at 400 (not a native Huma 422). Service never reached.
	handler, _ := newHolderHandler(t, ctrl)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/holders", bytes.NewReader([]byte("{not valid json")))
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

func TestHuma_GetHolderByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	holderID := uuid.New()

	name := "John Doe"
	htype := "NATURAL_PERSON"

	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		Find(gomock.Any(), orgID.String(), holderID, false).
		Return(&mmodel.Holder{ID: &holderID, Name: &name, Type: &htype}, nil).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/holders/"+holderID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "John Doe", got["name"])
	assert.Equal(t, holderID.String(), got["id"])
}

func TestHuma_GetHolderByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	// Service must never be reached: ParseUUIDPathParameters rejects the bad id
	// with the canonical 0065 / 400 before Huma.
	handler, _ := newHolderHandler(t, ctrl)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/holders/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestHuma_UpdateHolder_MergePatch_NullFieldRemoved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	holderID := uuid.New()

	// The PATCH sends "externalId": null. The Huma shell must derive
	// fieldsToRemove=["externalId"] via FindNilFields and pass it to Update, exactly
	// as the Fiber patchRemove local does — this is the merge-patch landmine.
	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		Update(gomock.Any(), orgID.String(), holderID, gomock.Any(), gomock.Cond(func(x any) bool {
			fields, ok := x.([]string)
			if !ok {
				return false
			}
			for _, f := range fields {
				if f == "externalId" {
					return true
				}
			}
			return false
		})).
		DoAndReturn(func(_ context.Context, _ string, id uuid.UUID, h *mmodel.Holder, _ []string) (*mmodel.Holder, error) {
			h.ID = &id
			return h, nil
		}).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	body := []byte(`{"name":"Jane","externalId":null}`)
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/holders/"+holderID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
}

func TestHuma_DeleteHolder_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	holderID := uuid.New()

	// The delete flow guards on linked instruments (InstrumentRepo.Count) and owned
	// accounts (LedgerAccounts.CountAccountsByHolder) before deleting; the stubs report
	// none, mirroring the Fiber delete test's wiring.
	holderRepo := holderrepo.NewMockRepository(ctrl)
	instrumentRepo := instrumentrepo.NewMockRepository(ctrl)
	instrumentRepo.EXPECT().Count(gomock.Any(), orgID.String(), holderID).Return(int64(0), nil).Times(1)
	holderRepo.EXPECT().Delete(gomock.Any(), orgID.String(), holderID, false).Return(nil).Times(1)

	handler := &HolderHandler{Service: &services.UseCase{
		InstrumentRepo: instrumentRepo,
		HolderRepo:     holderRepo,
		LedgerAccounts: stubInstrumentLedgerAccountReader{},
	}}

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/holders/"+holderID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestHuma_GetAllHolders_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		FindAll(gomock.Any(), orgID.String(), gomock.Any(), false).
		Return([]*mmodel.Holder{}, nil).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/holders?limit=10&page=1", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "items")
	assert.EqualValues(t, 10, got["limit"])
}

// buildHumaHolderAccountsApp mounts the holder-accounts Huma operation on a /v1
// group, mirroring buildHumaHolderApp with the accounts route + reader handler.
func buildHumaHolderAccountsApp(t *testing.T, handler *HolderAccountsHandler) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV1 := f.Group("/v1")

	parse := pkgHTTP.ParseUUIDPathParameters("holder")
	apiV1.Get("/organizations/:organization_id/holders/:id/accounts", parse)

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	RegisterHolderAccountsRoutes(hAPI, handler)

	return f
}

func TestHuma_GetAccountsByHolder_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	holderID := uuid.New()

	reader := &stubHolderAccountsReader{accounts: []*mmodel.Account{{ID: uuid.New().String(), Name: "Wallet"}}}
	handler := &HolderAccountsHandler{Reader: reader}

	app := buildHumaHolderAccountsApp(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/accounts?limit=10&page=1", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	// The core must scope the filter to the path holder ID (mirrors the Fiber wrapper).
	require.NotNil(t, reader.gotHolderFilter)
	assert.Equal(t, holderID.String(), *reader.gotHolderFilter)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	items, ok := got["items"].([]any)
	require.True(t, ok)
	assert.Len(t, items, 1)
}

func TestHuma_GetAllHolders_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	// Service must never be reached: ValidateParameters rejects limit=abc with the
	// canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler, _ := newHolderHandler(t, ctrl)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/holders?limit=abc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}
