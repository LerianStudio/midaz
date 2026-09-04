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

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"

	libConstants "github.com/LerianStudio/lib-commons/v7/commons/constants"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	holderrepo "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	instrumentrepo "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaHolderApp mounts the five holder Huma operations on a /v2 group,
// faithfully mirroring the production wiring in unified-server.go: problem.Install()
// runs before any huma.Register, the Huma API is built with openapi.New over a /v2
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
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV2 := f.Group("/v2")

	apiV2.Use(func(c fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV2, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v2"}})

	parse := pkgHTTP.ParseUUIDPathParameters("holder")
	base := "/organizations/:organization_id/holders"
	apiV2.Post(base, parse)
	apiV2.Get(base, parse)
	apiV2.Get(base+"/:id", parse)
	apiV2.Patch(base+"/:id", parse)
	apiV2.Delete(base+"/:id", parse)

	RegisterHolderRoutes(hAPI, handler, v2OpSuffix)

	return f
}

func newHolderHandler(t *testing.T, ctrl *gomock.Controller) (*HolderHandler, *holderrepo.MockRepository) {
	t.Helper()

	repo := holderrepo.NewMockRepository(ctrl)
	handler := &HolderHandler{Service: &services.UseCase{HolderRepo: repo}}

	return handler, repo
}

func TestCreateHolder_Success(t *testing.T) {
	// NOT parallel: buildHumaHolderApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		Create(gomock.Any(), orgID.String(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, h *mmodel.Holder) (*mmodel.Holder, error) {
			h.CreatedAt = fixedTestTime
			h.UpdatedAt = fixedTestTime
			return h, nil
		}).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"type": "NATURAL_PERSON", "name": "John Doe", "document": "91315026015"})
	req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/holders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestCreateHolder_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	// No repo expectations: a rejected auth must never reach the service.
	handler, _ := newHolderHandler(t, ctrl)

	app := buildHumaHolderApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"type": "NATURAL_PERSON", "name": "John Doe", "document": "91315026015"})
	req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/holders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestCreateHolder_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	// Malformed JSON -> DecodeAndValidate returns 0094; HumaProblem projects it to
	// problem+json at 400 (not a native Huma 422). Service never reached.
	handler, _ := newHolderHandler(t, ctrl)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/holders", bytes.NewReader([]byte("{not valid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestGetHolderByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	name := "John Doe"
	htype := "NATURAL_PERSON"

	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		Find(gomock.Any(), orgID.String(), holderID, false).
		Return(&mmodel.Holder{ID: &holderID, Name: &name, Type: &htype}, nil).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestGetHolderByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ParseUUIDPathParameters rejects the bad id
	// with the canonical 0065 / 400 before Huma.
	handler, _ := newHolderHandler(t, ctrl)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/holders/not-a-uuid", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestUpdateHolder_MergePatch_NullFieldRemoved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

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
	req := httptest.NewRequest(http.MethodPatch, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
}

func TestDeleteHolder_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

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

	req := httptest.NewRequest(http.MethodDelete, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestGetAllHolders_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		FindAll(gomock.Any(), orgID.String(), gomock.Any(), false).
		Return([]*mmodel.Holder{}, nil).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/holders?limit=10&page=1", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

// buildHumaHolderAccountsApp mounts the holder-accounts Huma operation on a /v2
// group, mirroring buildHumaHolderApp with the accounts route + reader handler.
func buildHumaHolderAccountsApp(t *testing.T, handler *HolderAccountsHandler) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV2 := f.Group("/v2")

	parse := pkgHTTP.ParseUUIDPathParameters("holder")
	apiV2.Get("/organizations/:organization_id/holders/:id/accounts", parse)

	hAPI := openapi.New(f, apiV2, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v2"}})

	RegisterHolderAccountsRoutes(hAPI, handler, v2OpSuffix)

	return f
}

func TestGetAccountsByHolder_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	reader := &stubHolderAccountsReader{accounts: []*mmodel.Account{{ID: uuid.Must(libCommons.GenerateUUIDv7()).String(), Name: "Wallet"}}}
	handler := &HolderAccountsHandler{Reader: reader}

	app := buildHumaHolderAccountsApp(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/accounts?limit=10&page=1", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	// The core must scope the filter to the path holder ID.
	require.NotNil(t, reader.gotHolderFilter)
	assert.Equal(t, holderID.String(), *reader.gotHolderFilter)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	items, ok := got["items"].([]any)
	require.True(t, ok)
	assert.Len(t, items, 1)
}

func TestGetAllHolders_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ValidateParameters rejects limit=abc with the
	// canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler, _ := newHolderHandler(t, ctrl)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/holders?limit=abc", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

// holderInternalServerError is the canonical 500 a CRM repository returns when
// its backing store fails; the shells must project it unchanged through HumaProblem.
func holderInternalServerError() pkg.InternalServerError {
	return pkg.InternalServerError{
		Code:    "0046",
		Title:   "Internal Server Error",
		Message: "Database connection failed",
	}
}

func TestCreateHolder_MissingRequiredField_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	// "document" is required: DecodeAndValidate rejects before the service.
	handler, _ := newHolderHandler(t, ctrl)

	app := buildHumaHolderApp(t, handler, true)

	body := []byte(`{"type":"NATURAL_PERSON","name":"John Doe"}`)
	req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/holders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "missing required field stays 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrMissingFieldsInRequest.Error(), got["code"])
}

func TestCreateHolder_ServiceError_500(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		Create(gomock.Any(), orgID.String(), gomock.Any()).
		Return(nil, holderInternalServerError()).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"type": "NATURAL_PERSON", "name": "John Doe", "document": "91315026015"})
	req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/holders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "0046", got["code"])
	assert.Contains(t, got, "detail")
}

func TestCreateHolder_IdempotentReplay(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	name := "John Doe"
	document := "91315026015"
	holderType := "NATURAL_PERSON"

	// The Mongo create MUST run exactly once across the two identical requests;
	// the second is served from the idempotency slot.
	repo := holderrepo.NewMockRepository(ctrl)
	repo.EXPECT().
		Create(gomock.Any(), orgID.String(), gomock.Any()).
		Return(&mmodel.Holder{ID: &holderID, Name: &name, Document: &document, Type: &holderType}, nil).
		Times(1)

	handler := &HolderHandler{Service: &services.UseCase{
		HolderRepo:  repo,
		Idempotency: newFakeCRMIdempotencyRepo(),
	}}

	app := buildHumaHolderApp(t, handler, true)

	body := `{"type":"NATURAL_PERSON","name":"John Doe","document":"91315026015"}`

	doRequest := func() (int, string, []byte) {
		req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/holders", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(libConstants.IdempotencyKey, "holder-key-1")

		resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		return resp.StatusCode, resp.Header.Get(libConstants.IdempotencyReplayed), respBody
	}

	status1, replayed1, body1 := doRequest()
	assert.Equal(t, http.StatusCreated, status1)
	assert.Equal(t, "false", replayed1, "fresh create is not a replay")

	var first map[string]any
	require.NoError(t, json.Unmarshal(body1, &first), "body: %s", string(body1))

	status2, replayed2, body2 := doRequest()
	assert.Equal(t, http.StatusCreated, status2)
	assert.Equal(t, "true", replayed2, "identical retry replays the original entity")

	var second map[string]any
	require.NoError(t, json.Unmarshal(body2, &second), "body: %s", string(body2))

	assert.Equal(t, first["id"], second["id"])
	assert.Equal(t, first["name"], second["name"])
}

func TestGetHolderByID_IncludeDeleted(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	name := "John Doe"
	htype := "NATURAL_PERSON"
	deletedAt := time.Unix(1700000000, 0).UTC()

	// include_deleted=true must reach the repository as the boolean flag.
	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		Find(gomock.Any(), orgID.String(), holderID, true).
		Return(&mmodel.Holder{ID: &holderID, Name: &name, Type: &htype, DeletedAt: &deletedAt}, nil).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String()+"?include_deleted=true", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "deletedAt")
}

func TestGetHolderByID_NotFound_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		Find(gomock.Any(), orgID.String(), holderID, false).
		Return(nil, pkg.ValidateBusinessError(constant.ErrHolderNotFound, constant.EntityHolder)).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrHolderNotFound.Error(), got["code"])
}

func TestGetHolderByID_ServiceError_500(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		Find(gomock.Any(), orgID.String(), holderID, false).
		Return(nil, holderInternalServerError()).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "0046", got["code"])
	assert.Contains(t, got, "detail")
}

func TestUpdateHolder_NotFound_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		Update(gomock.Any(), orgID.String(), holderID, gomock.Any(), gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrHolderNotFound, constant.EntityHolder)).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPatch, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String(), bytes.NewReader([]byte(`{"name":"Jane Doe"}`)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrHolderNotFound.Error(), got["code"])
}

func TestUpdateHolder_ServiceError_500(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		Update(gomock.Any(), orgID.String(), holderID, gomock.Any(), gomock.Any()).
		Return(nil, holderInternalServerError()).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPatch, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String(), bytes.NewReader([]byte(`{"name":"Jane Doe"}`)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "0046", got["code"])
	assert.Contains(t, got, "detail")
}

// newHolderDeleteHandler wires the two guards the delete flow runs before the
// Mongo delete: linked instruments (InstrumentRepo.Count) and owned accounts
// (LedgerAccounts.CountAccountsByHolder, stubbed to none).
func newHolderDeleteHandler(t *testing.T, ctrl *gomock.Controller) (*HolderHandler, *holderrepo.MockRepository, *instrumentrepo.MockRepository) {
	t.Helper()

	holderRepo := holderrepo.NewMockRepository(ctrl)
	instrumentRepo := instrumentrepo.NewMockRepository(ctrl)

	handler := &HolderHandler{Service: &services.UseCase{
		InstrumentRepo: instrumentRepo,
		HolderRepo:     holderRepo,
		LedgerAccounts: stubInstrumentLedgerAccountReader{},
	}}

	return handler, holderRepo, instrumentRepo
}

func TestDeleteHolder_HardDelete_204(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	// hard_delete=true must reach the repository as the boolean flag.
	handler, holderRepo, instrumentRepo := newHolderDeleteHandler(t, ctrl)
	instrumentRepo.EXPECT().Count(gomock.Any(), orgID.String(), holderID).Return(int64(0), nil).Times(1)
	holderRepo.EXPECT().Delete(gomock.Any(), orgID.String(), holderID, true).Return(nil).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String()+"?hard_delete=true", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestDeleteHolder_HasInstruments_422(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	// A linked instrument blocks the delete before the Mongo delete is reached.
	handler, _, instrumentRepo := newHolderDeleteHandler(t, ctrl)
	instrumentRepo.EXPECT().Count(gomock.Any(), orgID.String(), holderID).Return(int64(1), nil).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrHolderHasInstruments.Error(), got["code"])
}

func TestDeleteHolder_NotFound_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	handler, holderRepo, instrumentRepo := newHolderDeleteHandler(t, ctrl)
	instrumentRepo.EXPECT().Count(gomock.Any(), orgID.String(), holderID).Return(int64(0), nil).Times(1)
	holderRepo.EXPECT().
		Delete(gomock.Any(), orgID.String(), holderID, false).
		Return(pkg.ValidateBusinessError(constant.ErrHolderNotFound, constant.EntityHolder)).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrHolderNotFound.Error(), got["code"])
}

func TestDeleteHolder_ServiceError_500(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	handler, holderRepo, instrumentRepo := newHolderDeleteHandler(t, ctrl)
	instrumentRepo.EXPECT().Count(gomock.Any(), orgID.String(), holderID).Return(int64(0), nil).Times(1)
	holderRepo.EXPECT().
		Delete(gomock.Any(), orgID.String(), holderID, false).
		Return(holderInternalServerError()).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "0046", got["code"])
	assert.Contains(t, got, "detail")
}

func TestGetAllHolders_IncludeDeleted(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	name := "John Doe"
	htype := "NATURAL_PERSON"
	deletedAt := time.Unix(1700000000, 0).UTC()

	// include_deleted=true must reach the repository as the boolean flag.
	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		FindAll(gomock.Any(), orgID.String(), gomock.Any(), true).
		Return([]*mmodel.Holder{{ID: &holderID, Name: &name, Type: &htype, DeletedAt: &deletedAt}}, nil).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/holders?include_deleted=true", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))

	items, ok := got["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)

	first, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, first, "deletedAt")
}

func TestGetAllHolders_ServiceError_500(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	handler, repo := newHolderHandler(t, ctrl)
	repo.EXPECT().
		FindAll(gomock.Any(), orgID.String(), gomock.Any(), false).
		Return(nil, holderInternalServerError()).Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/holders", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "0046", got["code"])
	assert.Contains(t, got, "detail")
}

func TestGetAccountsByHolder_EmptyList(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	reader := &stubHolderAccountsReader{accounts: []*mmodel.Account{}}
	handler := &HolderAccountsHandler{Reader: reader}

	app := buildHumaHolderAccountsApp(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/accounts", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Equal(t, orgID.String(), reader.gotOrganizationID)
	assert.Equal(t, holderID, reader.gotHolderID)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "items")
}

func TestGetAccountsByHolder_NotFound_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	reader := &stubHolderAccountsReader{err: pkg.ValidateBusinessError(constant.ErrNoAccountsFound, constant.EntityAccount)}
	handler := &HolderAccountsHandler{Reader: reader}

	app := buildHumaHolderAccountsApp(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/accounts", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrNoAccountsFound.Error(), got["code"])
}

func TestGetAccountsByHolder_ReaderError_500(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	reader := &stubHolderAccountsReader{err: holderInternalServerError()}
	handler := &HolderAccountsHandler{Reader: reader}

	app := buildHumaHolderAccountsApp(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/accounts", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "0046", got["code"])
	assert.Contains(t, got, "detail")
}

func TestGetAccountsByHolder_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	// Reader must never be reached: ValidateParameters rejects limit=abc.
	reader := &stubHolderAccountsReader{}
	handler := &HolderAccountsHandler{Reader: reader}

	app := buildHumaHolderAccountsApp(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/accounts?limit=abc", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")
	assert.Nil(t, reader.gotHolderFilter, "reader must not be reached")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

// stubHolderAccountsReader is a hand-written stub for HolderAccountsReader. It
// captures the org ID, holder ID and holder filter the handler forwards, and
// returns a canned account slice or error.
type stubHolderAccountsReader struct {
	accounts []*mmodel.Account
	err      error

	gotOrganizationID string
	gotHolderID       uuid.UUID
	gotHolderFilter   *string
	gotLedgerID       *string
}

func (s *stubHolderAccountsReader) ListAccountsByHolder(_ context.Context, organizationID string, holderID uuid.UUID, filter pkgHTTP.QueryHeader) ([]*mmodel.Account, error) {
	s.gotOrganizationID = organizationID
	s.gotHolderID = holderID
	s.gotHolderFilter = filter.HolderID
	s.gotLedgerID = filter.LedgerID

	return s.accounts, s.err
}

// TestGetAccountsByHolder_LedgerIDFilter_PassedThrough pins the ledger_id query
// parameter reaching the reader: the listing is org-wide, so ledger_id is the
// only way a caller narrows it, and a transport that drops it silently widens
// every narrowed request.
func TestGetAccountsByHolder_LedgerIDFilter_PassedThrough(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	reader := &stubHolderAccountsReader{accounts: []*mmodel.Account{}}
	handler := &HolderAccountsHandler{Reader: reader}

	app := buildHumaHolderAccountsApp(t, handler)

	req := httptest.NewRequest(http.MethodGet,
		"/v2/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/accounts?limit=10&page=1&ledger_id="+ledgerID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))

	require.NotNil(t, reader.gotLedgerID, "ledger_id must reach the reader")
	assert.Equal(t, ledgerID.String(), *reader.gotLedgerID)
}

// TestGetAccountsByHolder_NoLedgerID_ReaderGetsNil pins the absent case: with no
// ledger_id the reader must see nil, which is what makes the listing org-wide.
func TestGetAccountsByHolder_NoLedgerID_ReaderGetsNil(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	reader := &stubHolderAccountsReader{accounts: []*mmodel.Account{}}
	handler := &HolderAccountsHandler{Reader: reader}

	app := buildHumaHolderAccountsApp(t, handler)

	req := httptest.NewRequest(http.MethodGet,
		"/v2/organizations/"+orgID.String()+"/holders/"+holderID.String()+"/accounts?limit=10&page=1", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))

	assert.Nil(t, reader.gotLedgerID, "an absent ledger_id must reach the reader as nil, not an empty string")
}
