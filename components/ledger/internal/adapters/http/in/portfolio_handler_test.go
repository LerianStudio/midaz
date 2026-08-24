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

	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaPortfolioApp mounts the six portfolio Huma operations on a /v1 group,
// mirroring the production wiring (asset exemplar's buildHumaAssetApp):
// problem.Install() runs before any huma.Register, the Huma API is built with
// openapi.New over a /v1 group, an auth shim stands in for
// auth.Authorize("midaz","portfolios",verb) + tenant PostAuthMiddlewares, and
// http.ParseUUIDPathParameters("portfolio") + RegisterPortfolioRoutes attach the
// chain.
//
// MUST-NOT-PARALLELIZE (same rationale as the asset exemplar): libProblem.Install()
// swaps the process-global huma.NewError hook and Huma validation uses
// process-global sync.Pools — concurrent builds/requests cross-contaminate.
//
// authOK=false makes the shim reject with the canonical 401 envelope so the
// auth-preserved contract is testable without a live lib-auth server.
func buildHumaPortfolioApp(t *testing.T, handler *PortfolioHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()
	pkgHTTP.InstallHumaFrameworkErrors()

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

	// Mirror the production chain: ParseUUIDPathParameters as a Fiber middleware
	// (no terminal handler) before the Huma terminal on each portfolio route.
	parse := pkgHTTP.ParseUUIDPathParameters("portfolio")
	base := "/organizations/:organization_id/ledgers/:ledger_id/portfolios"
	apiV1.Post(base, parse)
	apiV1.Patch(base+"/:id", parse)
	apiV1.Get(base, parse)
	apiV1.Get(base+"/:id", parse)
	apiV1.Delete(base+"/:id", parse)
	apiV1.Head(base+"/metrics/count", parse)

	RegisterPortfolioRoutes(hAPI, handler, routeOpSuffixV1)

	return f
}

func TestCreatePortfolio_Success(t *testing.T) {
	// NOT parallel: buildHumaPortfolioApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	portfolioRepo := portfolio.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	portfolioRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, p *mmodel.Portfolio) (*mmodel.Portfolio, error) {
			p.CreatedAt = fixedTestTime
			p.UpdatedAt = fixedTestTime
			return p, nil
		}).Times(1)
	// The shared body pipeline (DecodeAndValidate -> parseMetadata) initializes
	// Metadata to a non-nil empty map when the body carries no "metadata" key, so
	// CreateOnboardingMetadata persists it — faithful to the Fiber WithBody path.
	metadataRepo.EXPECT().Create(gomock.Any(), constant.EntityPortfolio, gomock.Any()).Return(nil).Times(1)

	handler := &PortfolioHandler{Command: &command.UseCase{
		PortfolioRepo:          portfolioRepo,
		OnboardingMetadataRepo: metadataRepo,
	}}

	app := buildHumaPortfolioApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "My Portfolio"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")
	assert.NotContains(t, string(respBody), "$ref")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "My Portfolio", got["name"])
	assert.Equal(t, orgID.String(), got["organizationId"], "tenant org captured from path")
	assert.Equal(t, ledgerID.String(), got["ledgerId"], "tenant ledger captured from path")
}

func TestCreatePortfolio_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// No repo expectations: a rejected auth must never reach the service.
	handler := &PortfolioHandler{Command: &command.UseCase{
		PortfolioRepo:          portfolio.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaPortfolioApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"name": "My Portfolio"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestCreatePortfolio_ValidationError_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Missing required "name" -> imperative ValidateStruct -> canonical 400.
	handler := &PortfolioHandler{Command: &command.UseCase{
		PortfolioRepo:          portfolio.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaPortfolioApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"entityId": "abc"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios", bytes.NewReader(body))
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

func TestCreatePortfolio_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &PortfolioHandler{Command: &command.UseCase{
		PortfolioRepo:          portfolio.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaPortfolioApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios", bytes.NewReader([]byte("{not valid json")))
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

func TestGetPortfolioByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	portfolioID := uuid.Must(libCommons.GenerateUUIDv7())

	portfolioRepo := portfolio.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	portfolioRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, portfolioID).
		Return(&mmodel.Portfolio{ID: portfolioID.String(), Name: "My Portfolio", OrganizationID: orgID.String(), LedgerID: ledgerID.String()}, nil).Times(1)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityPortfolio, portfolioID.String()).Return(nil, nil).Times(1)

	handler := &PortfolioHandler{Query: &query.UseCase{PortfolioRepo: portfolioRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaPortfolioApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios/"+portfolioID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "My Portfolio", got["name"])
	assert.Equal(t, portfolioID.String(), got["id"])
}

func TestGetPortfolioByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// ParseUUIDPathParameters rejects the bad id with 0065 / 400 before Huma.
	handler := &PortfolioHandler{Query: &query.UseCase{
		PortfolioRepo:          portfolio.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaPortfolioApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios/not-a-uuid", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestGetPortfolioByID_NotFound_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	portfolioID := uuid.Must(libCommons.GenerateUUIDv7())

	portfolioRepo := portfolio.NewMockRepository(ctrl)
	// Repo returns the sentinel not-found -> canonical 404, never a native 422.
	portfolioRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, portfolioID).
		Return(nil, services.ErrDatabaseItemNotFound).Times(1)

	handler := &PortfolioHandler{Query: &query.UseCase{PortfolioRepo: portfolioRepo}}

	app := buildHumaPortfolioApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios/"+portfolioID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "not-found stays canonical 404")
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.NotContains(t, got, "status", "the v1 envelope carries no status member")
}

func TestGetAllPortfolios_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	portfolioRepo := portfolio.NewMockRepository(ctrl)
	portfolioRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).Return([]*mmodel.Portfolio{}, nil).Times(1)

	handler := &PortfolioHandler{Query: &query.UseCase{PortfolioRepo: portfolioRepo}}

	app := buildHumaPortfolioApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios?limit=10&page=1", nil)
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

func TestGetAllPortfolios_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// ValidateParameters rejects limit=abc with canonical 400, NOT a native 422.
	handler := &PortfolioHandler{Query: &query.UseCase{PortfolioRepo: portfolio.NewMockRepository(ctrl)}}

	app := buildHumaPortfolioApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios?limit=abc", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestUpdatePortfolio_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	portfolioID := uuid.Must(libCommons.GenerateUUIDv7())

	portfolioRepo := portfolio.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	portfolioRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, portfolioID, gomock.Any()).
		Return(&mmodel.Portfolio{ID: portfolioID.String(), Name: "Renamed", OrganizationID: orgID.String(), LedgerID: ledgerID.String()}, nil).Times(1)
	// Body carries no "metadata" key -> parseMetadata sets a non-nil empty map, so
	// UpdateOnboardingMetadata runs its non-nil branch: FindByEntity (no existing
	// row) then Update — faithful to the Fiber WithBody path.
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityPortfolio, portfolioID.String()).Return(nil, nil).Times(1)
	metadataRepo.EXPECT().Update(gomock.Any(), constant.EntityPortfolio, portfolioID.String(), gomock.Any()).Return(nil).Times(1)

	handler := &PortfolioHandler{Command: &command.UseCase{
		PortfolioRepo:          portfolioRepo,
		OnboardingMetadataRepo: metadataRepo,
	}}

	app := buildHumaPortfolioApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Renamed"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios/"+portfolioID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Renamed", got["name"])
	assert.Equal(t, portfolioID.String(), got["id"])
}

func TestDeletePortfolio_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	portfolioID := uuid.Must(libCommons.GenerateUUIDv7())

	portfolioRepo := portfolio.NewMockRepository(ctrl)
	portfolioRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, portfolioID).Return(nil).Times(1)

	handler := &PortfolioHandler{Command: &command.UseCase{PortfolioRepo: portfolioRepo}}

	app := buildHumaPortfolioApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios/"+portfolioID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestCountPortfolios_204WithHeader(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	portfolioRepo := portfolio.NewMockRepository(ctrl)
	portfolioRepo.EXPECT().Count(gomock.Any(), orgID, ledgerID).Return(int64(7), nil).Times(1)

	handler := &PortfolioHandler{Query: &query.UseCase{PortfolioRepo: portfolioRepo}}

	app := buildHumaPortfolioApp(t, handler, true)

	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/portfolios/metrics/count", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "7", resp.Header.Get(constant.XTotalCount), "X-Total-Count header must carry the count")
	assert.Empty(t, respBody, "HEAD count must have an empty body")
	assert.Equal(t, "0", resp.Header.Get("Content-Length"), "HEAD 204 must set Content-Length: 0")
}

//
// The six exported fiber.Ctx terminals on PortfolioHandler were deleted with the
// Huma migration; the branches their tests covered in the shared cores are
// exercised here through the live Huma transport instead.

func portfoliosPath(orgID, ledgerID uuid.UUID, suffix string) string {
	return "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/portfolios" + suffix
}

func TestCreatePortfolio_ServiceError_Canonical4xx(t *testing.T) {
	// NOT parallel: process-global huma state. createPortfolio's command-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	portfolioRepo := portfolio.NewMockRepository(ctrl)
	portfolioRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPortfolio)).Times(1)

	handler := &PortfolioHandler{Command: &command.UseCase{PortfolioRepo: portfolioRepo}}

	app := buildHumaPortfolioApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Test Portfolio", "entityId": uuid.Must(libCommons.GenerateUUIDv7()).String()})
	req := httptest.NewRequest(http.MethodPost, portfoliosPath(orgID, ledgerID, ""), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.GreaterOrEqual(t, resp.StatusCode, http.StatusBadRequest)
	assert.Less(t, resp.StatusCode, http.StatusInternalServerError)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestGetAllPortfolios_MetadataFilter(t *testing.T) {
	// NOT parallel: process-global huma state. getAllPortfolios' metadata branch
	// filters in memory after a plain FindAll.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	p1 := uuid.Must(libCommons.GenerateUUIDv7()).String()

	portfolioRepo := portfolio.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	metadataRepo.EXPECT().FindList(gomock.Any(), constant.EntityPortfolio, gomock.Any()).
		Return([]*mongodb.Metadata{{EntityID: p1, Data: map[string]any{"tier": "premium"}}}, nil).Times(1)
	portfolioRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return([]*mmodel.Portfolio{{ID: p1, OrganizationID: orgID.String(), LedgerID: ledgerID.String(), Name: "Premium One"}}, nil).Times(1)

	handler := &PortfolioHandler{Query: &query.UseCase{PortfolioRepo: portfolioRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaPortfolioApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, portfoliosPath(orgID, ledgerID, "?metadata.tier=premium"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "items")
}

func TestGetAllPortfolios_MetadataFilter_NoMatch_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. The metadata branch's error path.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	metadataRepo := mongodb.NewMockRepository(ctrl)
	metadataRepo.EXPECT().FindList(gomock.Any(), constant.EntityPortfolio, gomock.Any()).Return(nil, nil).Times(1)

	handler := &PortfolioHandler{Query: &query.UseCase{
		PortfolioRepo:          portfolio.NewMockRepository(ctrl),
		OnboardingMetadataRepo: metadataRepo,
	}}

	app := buildHumaPortfolioApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, portfoliosPath(orgID, ledgerID, "?metadata.tier=nonexistent"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetAllPortfolios_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. getAllPortfolios' plain query-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	portfolioRepo := portfolio.NewMockRepository(ctrl)
	portfolioRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPortfolio)).Times(1)

	handler := &PortfolioHandler{Query: &query.UseCase{PortfolioRepo: portfolioRepo}}

	app := buildHumaPortfolioApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, portfoliosPath(orgID, ledgerID, ""), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestUpdatePortfolio_NotFound_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. updatePortfolio's command-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	portfolioID := uuid.Must(libCommons.GenerateUUIDv7())

	portfolioRepo := portfolio.NewMockRepository(ctrl)
	portfolioRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, portfolioID, gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPortfolio)).Times(1)

	handler := &PortfolioHandler{Command: &command.UseCase{PortfolioRepo: portfolioRepo}}

	app := buildHumaPortfolioApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Updated Portfolio Name"})
	req := httptest.NewRequest(http.MethodPatch, portfoliosPath(orgID, ledgerID, "/"+portfolioID.String()), bytes.NewReader(body))
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

func TestDeletePortfolio_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. deletePortfolio's command-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	portfolioID := uuid.Must(libCommons.GenerateUUIDv7())

	portfolioRepo := portfolio.NewMockRepository(ctrl)
	portfolioRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, portfolioID).
		Return(pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPortfolio)).Times(1)

	handler := &PortfolioHandler{Command: &command.UseCase{PortfolioRepo: portfolioRepo}}

	app := buildHumaPortfolioApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, portfoliosPath(orgID, ledgerID, "/"+portfolioID.String()), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestCountPortfolios_ServiceError(t *testing.T) {
	// NOT parallel: process-global huma state. countPortfolios' query-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	portfolioRepo := portfolio.NewMockRepository(ctrl)
	portfolioRepo.EXPECT().Count(gomock.Any(), orgID, ledgerID).
		Return(int64(0), pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPortfolio)).Times(1)

	handler := &PortfolioHandler{Query: &query.UseCase{PortfolioRepo: portfolioRepo}}

	app := buildHumaPortfolioApp(t, handler, true)

	req := httptest.NewRequest(http.MethodHead, portfoliosPath(orgID, ledgerID, "/metrics/count"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Empty(t, resp.Header.Get(constant.XTotalCount), "a failed count must not advertise a total")
}
