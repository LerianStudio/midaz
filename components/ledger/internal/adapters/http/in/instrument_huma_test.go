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

// buildHumaInstrumentApp mounts the six instrument Huma operations on a /v1 group,
// faithfully mirroring the production wiring in crm_routes.go/unified-server.go:
// problem.Install() runs before any huma.Register, the Huma API is built with
// openapi.New over a /v1 group, an auth-shim middleware stands in for
// auth.Authorize("midaz","instruments",verb) + tenant PostAuthMiddlewares, and the
// per-route ParseUUIDPathParameters ("instruments", except "related-parties" on the
// related-party delete) + RegisterInstrumentRoutes attach the chain.
//
// MUST-NOT-PARALLELIZE (same rationale as buildHumaAssetApp/buildHumaHolderApp):
// libProblem.Install() swaps the process-global huma.NewError hook and Huma
// validation uses process-global sync.Pools — concurrent builds/requests
// cross-contaminate. These tests are sub-second; keep them sequential.
func buildHumaInstrumentApp(t *testing.T, handler *InstrumentHandler, authOK bool) *fiber.App {
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

	parse := pkgHTTP.ParseUUIDPathParameters("instruments")
	parseRP := pkgHTTP.ParseUUIDPathParameters("related-parties")

	listPath := "/organizations/:organization_id/instruments"
	holderScoped := "/organizations/:organization_id/holders/:holder_id/instruments"
	idPath := holderScoped + "/:instrument_id"

	apiV1.Get(listPath, parse)
	apiV1.Post(holderScoped, parse)
	apiV1.Get(idPath, parse)
	apiV1.Patch(idPath, parse)
	apiV1.Delete(idPath, parse)
	apiV1.Delete(idPath+"/related-parties/:related_party_id", parseRP)

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	RegisterInstrumentRoutes(hAPI, handler)

	return f
}

func newInstrumentHandler(t *testing.T, ctrl *gomock.Controller) (*InstrumentHandler, *instrumentrepo.MockRepository) {
	t.Helper()

	repo := instrumentrepo.NewMockRepository(ctrl)
	handler := &InstrumentHandler{Service: &services.UseCase{InstrumentRepo: repo}}

	return handler, repo
}

func TestHuma_CreateInstrument_IdempotentReplay(t *testing.T) {
	// NOT parallel: buildHumaInstrumentApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	holderID := uuid.New()
	instrumentID := uuid.New()
	document := "12345678901"
	holderType := "individual"

	instrumentRepo := instrumentrepo.NewMockRepository(ctrl)
	holderRepo := holderrepo.NewMockRepository(ctrl)

	// GetHolderByID runs on every non-replay create; only the first request reaches
	// the create path, so Find is expected exactly once.
	holderRepo.EXPECT().
		Find(gomock.Any(), orgID.String(), holderID, false).
		Return(&mmodel.Holder{ID: &holderID, Document: &document, Type: &holderType}, nil).
		Times(1)

	// The Mongo create MUST run exactly once across the two identical requests.
	instrumentRepo.EXPECT().
		Create(gomock.Any(), orgID.String(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, a *mmodel.Instrument) (*mmodel.Instrument, error) {
			a.ID = &instrumentID
			return a, nil
		}).
		Times(1)

	handler := &InstrumentHandler{Service: &services.UseCase{
		InstrumentRepo: instrumentRepo,
		HolderRepo:     holderRepo,
		Idempotency:    newFakeCRMIdempotencyRepo(),
		LedgerAccounts: stubInstrumentLedgerAccountReader{ledgerExists: true, accountExists: true},
	}}

	app := buildHumaInstrumentApp(t, handler, true)

	body := `{"ledgerId":"00000000-0000-0000-0000-000000000001","accountId":"00000000-0000-0000-0000-000000000002"}`

	doRequest := func() (int, string, []byte) {
		req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/instruments", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Idempotency-Key", "instrument-key-1")

		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		respBody, _ := io.ReadAll(resp.Body)

		return resp.StatusCode, resp.Header.Get("X-Idempotency-Replayed"), respBody
	}

	status1, replayed1, body1 := doRequest()
	assert.Equal(t, http.StatusCreated, status1)
	assert.Equal(t, "false", replayed1, "fresh create is not a replay")
	assert.NotContains(t, string(body1), "$schema", "SchemaLinkTransformer must be zeroed")

	var first map[string]any
	require.NoError(t, json.Unmarshal(body1, &first), "body: %s", string(body1))

	status2, replayed2, body2 := doRequest()
	assert.Equal(t, http.StatusCreated, status2)
	assert.Equal(t, "true", replayed2, "identical retry replays the cached instrument")

	var second map[string]any
	require.NoError(t, json.Unmarshal(body2, &second), "body: %s", string(body2))

	assert.Equal(t, first["id"], second["id"])
	assert.Equal(t, first["ledgerId"], second["ledgerId"])
}

func TestHuma_CreateInstrument_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	holderID := uuid.New()

	// No repo expectations: a rejected auth must never reach the service.
	handler, _ := newInstrumentHandler(t, ctrl)

	app := buildHumaInstrumentApp(t, handler, false)

	body := `{"ledgerId":"00000000-0000-0000-0000-000000000001","accountId":"00000000-0000-0000-0000-000000000002"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/instruments", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestHuma_CreateInstrument_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	holderID := uuid.New()

	// Malformed JSON -> DecodeAndValidate returns 0094; HumaProblem projects it to
	// problem+json at 400 (not a native Huma 422). Service never reached.
	handler, _ := newInstrumentHandler(t, ctrl)

	app := buildHumaInstrumentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/instruments", bytes.NewReader([]byte("{not valid json")))
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

func TestHuma_GetInstrumentByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	holderID := uuid.New()
	instrumentID := uuid.New()

	handler, repo := newInstrumentHandler(t, ctrl)
	repo.EXPECT().
		Find(gomock.Any(), orgID.String(), holderID, instrumentID, false).
		Return(&mmodel.Instrument{ID: &instrumentID, HolderID: &holderID}, nil).Times(1)

	app := buildHumaInstrumentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/instruments/"+instrumentID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, instrumentID.String(), got["id"])
}

func TestHuma_GetInstrumentByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	holderID := uuid.New()

	// Service must never be reached: ParseUUIDPathParameters rejects the bad
	// instrument_id with the canonical 0065 / 400 before Huma.
	handler, _ := newInstrumentHandler(t, ctrl)

	app := buildHumaInstrumentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/instruments/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestHuma_UpdateInstrument_MergePatch_NullFieldRemoved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	holderID := uuid.New()
	instrumentID := uuid.New()

	// The PATCH sends "bankingDetails": null. The Huma shell must derive
	// fieldsToRemove=["bankingDetails"] via FindNilFields and pass it to Update,
	// exactly as the Fiber patchRemove local does — the merge-patch landmine.
	handler, repo := newInstrumentHandler(t, ctrl)
	repo.EXPECT().
		Update(gomock.Any(), orgID.String(), holderID, instrumentID, gomock.Any(), gomock.Cond(func(x any) bool {
			fields, ok := x.([]string)
			if !ok {
				return false
			}
			for _, f := range fields {
				if f == "bankingDetails" {
					return true
				}
			}
			return false
		})).
		DoAndReturn(func(_ context.Context, _ string, _, id uuid.UUID, a *mmodel.Instrument, _ []string) (*mmodel.Instrument, error) {
			a.ID = &id
			return a, nil
		}).Times(1)

	app := buildHumaInstrumentApp(t, handler, true)

	body := []byte(`{"metadata":{"k":"v"},"bankingDetails":null}`)
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/instruments/"+instrumentID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
}

func TestHuma_DeleteInstrument_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	holderID := uuid.New()
	instrumentID := uuid.New()

	handler, repo := newInstrumentHandler(t, ctrl)
	repo.EXPECT().Delete(gomock.Any(), orgID.String(), holderID, instrumentID, false).Return(nil).Times(1)

	app := buildHumaInstrumentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/instruments/"+instrumentID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestHuma_DeleteRelatedParty_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	holderID := uuid.New()
	instrumentID := uuid.New()
	relatedPartyID := uuid.New()

	handler, repo := newInstrumentHandler(t, ctrl)
	repo.EXPECT().
		DeleteRelatedParty(gomock.Any(), orgID.String(), holderID, instrumentID, relatedPartyID).
		Return(nil).Times(1)

	app := buildHumaInstrumentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/instruments/"+instrumentID.String()+"/related-parties/"+relatedPartyID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestHuma_GetAllInstruments_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	// GetAllInstruments is org-scoped; with no holder_id query filter the service
	// passes the zero-UUID holder filter through to FindAll.
	handler, repo := newInstrumentHandler(t, ctrl)
	repo.EXPECT().
		FindAll(gomock.Any(), orgID.String(), uuid.Nil, gomock.Any(), false).
		Return([]*mmodel.Instrument{}, nil).Times(1)

	app := buildHumaInstrumentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/instruments?limit=10&page=1", nil)
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

func TestHuma_GetAllInstruments_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	// Service must never be reached: ValidateParameters rejects limit=abc with the
	// canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler, _ := newInstrumentHandler(t, ctrl)

	app := buildHumaInstrumentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/instruments?limit=abc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}
