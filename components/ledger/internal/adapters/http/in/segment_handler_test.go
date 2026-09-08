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

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaSegmentApp mirrors buildHumaAssetApp (the DE-RISK exemplar) for the six
// segment Huma operations: problem.Install() before any huma.Register, the Huma API
// built with openapi.New over a /v1 group, an auth-shim standing in for
// auth.Authorize("midaz","segments",verb) + tenant PostAuthMiddlewares, then
// ParseUUIDPathParameters("segment") + RegisterSegmentRoutes attach the chain.
//
// MUST-NOT-PARALLELIZE (same rationale as the asset exemplar): libProblem.Install()
// swaps the process-global huma.NewError hook and Huma validation uses process-global
// sync.Pools — concurrent builds/requests cross-contaminate. These tests are
// sub-second; keep them sequential.
//
// authOK=false makes the shim reject with the ledger's canonical 401 envelope so the
// auth-preserved contract is testable without a live lib-auth server.
func buildHumaSegmentApp(t *testing.T, handler *SegmentHandler, authOK bool) *fiber.App {
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

	// Mirror the production chain: ParseUUIDPathParameters runs as Fiber middleware
	// (no terminal) before the Huma terminal on each segment route. The path param is
	// :id, NOT :segment_id.
	parse := pkgHTTP.ParseUUIDPathParameters("segment")
	base := "/organizations/:organization_id/ledgers/:ledger_id/segments"
	apiV1.Post(base, parse)
	apiV1.Patch(base+"/:id", parse)
	apiV1.Get(base, parse)
	apiV1.Get(base+"/:id", parse)
	apiV1.Delete(base+"/:id", parse)
	apiV1.Head(base+"/metrics/count", parse)

	RegisterSegmentRoutes(hAPI, handler, v1OpSuffix)

	return f
}

func TestCreateSegment_Success(t *testing.T) {
	// NOT parallel: buildHumaSegmentApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	segmentRepo := segment.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	segmentRepo.EXPECT().ExistsByName(gomock.Any(), orgID, ledgerID, "Retail").Return(false, nil).Times(1)
	segmentRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, s *mmodel.Segment) (*mmodel.Segment, error) {
			s.CreatedAt = fixedTestTime
			s.UpdatedAt = fixedTestTime
			return s, nil
		}).Times(1)
	// The shared body pipeline (DecodeAndValidate -> parseMetadata) initializes
	// Metadata to a non-nil empty map when the body carries no "metadata" key, so
	// CreateOnboardingMetadata persists it — faithful to the production Fiber WithBody
	// path (mirrors the asset exemplar).
	metadataRepo.EXPECT().Create(gomock.Any(), constant.EntitySegment, gomock.Any()).Return(nil).Times(1)

	handler := &SegmentHandler{Command: &command.UseCase{
		SegmentRepo:            segmentRepo,
		OnboardingMetadataRepo: metadataRepo,
	}}

	app := buildHumaSegmentApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Retail"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments", bytes.NewReader(body))
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
	assert.Equal(t, "Retail", got["name"])
	assert.Equal(t, orgID.String(), got["organizationId"], "tenant path org must round-trip into the body")
	assert.Equal(t, ledgerID.String(), got["ledgerId"], "tenant path ledger must round-trip into the body")
}

func TestCreateSegment_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// No repo expectations: a rejected auth must never reach the service.
	handler := &SegmentHandler{Command: &command.UseCase{
		SegmentRepo:            segment.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaSegmentApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"name": "Retail"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestCreateSegment_ValidationError_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Missing required "name" -> imperative ValidateStruct -> canonical 400, service
	// never reached (no repo expectations).
	handler := &SegmentHandler{Command: &command.UseCase{
		SegmentRepo:            segment.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaSegmentApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"status": map[string]any{"code": "ACTIVE"}})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments", bytes.NewReader(body))
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

func TestCreateSegment_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Malformed JSON -> DecodeAndValidate returns a 0094; HumaProblem must project it
	// to problem+json at 400 (NOT the 500 fallback, NOT a native Huma 422). Service
	// never reached.
	handler := &SegmentHandler{Command: &command.UseCase{
		SegmentRepo:            segment.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaSegmentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments", bytes.NewReader([]byte("{not valid json")))
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

func TestGetSegmentByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	segmentID := uuid.Must(libCommons.GenerateUUIDv7())

	segmentRepo := segment.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	segmentRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, segmentID).
		Return(&mmodel.Segment{ID: segmentID.String(), Name: "Retail", OrganizationID: orgID.String(), LedgerID: ledgerID.String()}, nil).Times(1)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntitySegment, segmentID.String()).Return(nil, nil).Times(1)

	handler := &SegmentHandler{Query: &query.UseCase{SegmentRepo: segmentRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaSegmentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments/"+segmentID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Retail", got["name"])
	assert.Equal(t, segmentID.String(), got["id"])
}

func TestGetSegmentByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// ParseUUIDPathParameters rejects the bad id with 0065 / 400 before Huma; service
	// never reached.
	handler := &SegmentHandler{Query: &query.UseCase{
		SegmentRepo:            segment.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaSegmentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments/not-a-uuid", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestGetAllSegments_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	segmentRepo := segment.NewMockRepository(ctrl)
	segmentRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).Return([]*mmodel.Segment{}, nil).Times(1)

	handler := &SegmentHandler{Query: &query.UseCase{SegmentRepo: segmentRepo}}

	app := buildHumaSegmentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments?limit=10&page=1", nil)
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

func TestGetAllSegments_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// ValidateParameters rejects limit=abc with the canonical 400, NOT a native 422.
	handler := &SegmentHandler{Query: &query.UseCase{SegmentRepo: segment.NewMockRepository(ctrl)}}

	app := buildHumaSegmentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments?limit=abc", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestDeleteSegment_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	segmentID := uuid.Must(libCommons.GenerateUUIDv7())

	segmentRepo := segment.NewMockRepository(ctrl)
	segmentRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, segmentID).Return(nil).Times(1)

	handler := &SegmentHandler{Command: &command.UseCase{SegmentRepo: segmentRepo}}

	app := buildHumaSegmentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments/"+segmentID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestCountSegments_204WithHeader(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	segmentRepo := segment.NewMockRepository(ctrl)
	segmentRepo.EXPECT().Count(gomock.Any(), orgID, ledgerID).Return(int64(7), nil).Times(1)

	handler := &SegmentHandler{Query: &query.UseCase{SegmentRepo: segmentRepo}}

	app := buildHumaSegmentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/segments/metrics/count", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "7", resp.Header.Get(constant.XTotalCount), "X-Total-Count header must carry the count")
	assert.Empty(t, respBody, "HEAD count must have an empty body")
	assert.Equal(t, "0", resp.Header.Get("Content-Length"), "HEAD 204 must set Content-Length: 0 (parity with the Fiber NoContent path)")
}

//
// The six exported fiber.Ctx terminals on SegmentHandler were deleted with the Huma
// migration; the branches their tests covered in the shared cores are exercised
// here through the live Huma transport instead.

func segmentsPath(orgID, ledgerID uuid.UUID, suffix string) string {
	return "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/segments" + suffix
}

func TestCreateSegment_ServiceError_Canonical4xx(t *testing.T) {
	// NOT parallel: process-global huma state. createSegment's command-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	segmentRepo := segment.NewMockRepository(ctrl)
	segmentRepo.EXPECT().ExistsByName(gomock.Any(), orgID, ledgerID, "Test Segment").Return(false, nil).Times(1)
	segmentRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrDuplicateSegmentName, constant.EntitySegment, "Test Segment")).Times(1)

	handler := &SegmentHandler{Command: &command.UseCase{SegmentRepo: segmentRepo}}

	app := buildHumaSegmentApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Test Segment"})
	req := httptest.NewRequest(http.MethodPost, segmentsPath(orgID, ledgerID, ""), bytes.NewReader(body))
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

func TestGetSegmentByID_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. getSegmentByID's query-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	segmentID := uuid.Must(libCommons.GenerateUUIDv7())

	segmentRepo := segment.NewMockRepository(ctrl)
	segmentRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, segmentID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntitySegment)).Times(1)

	handler := &SegmentHandler{Query: &query.UseCase{SegmentRepo: segmentRepo}}

	app := buildHumaSegmentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, segmentsPath(orgID, ledgerID, "/"+segmentID.String()), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestGetAllSegments_MetadataFilter(t *testing.T) {
	// NOT parallel: process-global huma state. getAllSegments' metadata branch
	// resolves the matched IDs via FindByIDs, not FindAll.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	seg1 := uuid.Must(libCommons.GenerateUUIDv7()).String()

	segmentRepo := segment.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	metadataRepo.EXPECT().FindList(gomock.Any(), constant.EntitySegment, gomock.Any()).
		Return([]*mongodb.Metadata{{EntityID: seg1, Data: map[string]any{"tier": "premium"}}}, nil).Times(1)
	segmentRepo.EXPECT().FindByIDs(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return([]*mmodel.Segment{{ID: seg1, OrganizationID: orgID.String(), LedgerID: ledgerID.String(), Name: "Premium One"}}, nil).Times(1)

	handler := &SegmentHandler{Query: &query.UseCase{SegmentRepo: segmentRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaSegmentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, segmentsPath(orgID, ledgerID, "?metadata.tier=premium"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	items, ok := got["items"].([]any)
	require.True(t, ok, "items should be an array")
	assert.Len(t, items, 1)
}

func TestGetAllSegments_MetadataFilter_NoMatch_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. The metadata branch's error path.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	metadataRepo := mongodb.NewMockRepository(ctrl)
	metadataRepo.EXPECT().FindList(gomock.Any(), constant.EntitySegment, gomock.Any()).Return(nil, nil).Times(1)

	handler := &SegmentHandler{Query: &query.UseCase{
		SegmentRepo:            segment.NewMockRepository(ctrl),
		OnboardingMetadataRepo: metadataRepo,
	}}

	app := buildHumaSegmentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, segmentsPath(orgID, ledgerID, "?metadata.tier=nonexistent"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetAllSegments_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. getAllSegments' plain query-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	segmentRepo := segment.NewMockRepository(ctrl)
	segmentRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntitySegment)).Times(1)

	handler := &SegmentHandler{Query: &query.UseCase{SegmentRepo: segmentRepo}}

	app := buildHumaSegmentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, segmentsPath(orgID, ledgerID, ""), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestUpdateSegment_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	segmentID := uuid.Must(libCommons.GenerateUUIDv7())

	segmentRepo := segment.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	segmentRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, segmentID).
		Return(&mmodel.Segment{ID: segmentID.String(), Name: "Original Segment Name"}, nil).Times(1)
	segmentRepo.EXPECT().ExistsByName(gomock.Any(), orgID, ledgerID, "Updated Segment Name").Return(false, nil).Times(1)
	segmentRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, segmentID, gomock.Any()).
		Return(&mmodel.Segment{
			ID:             segmentID.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			Name:           "Updated Segment Name",
			Status:         mmodel.Status{Code: "ACTIVE"},
		}, nil).Times(1)
	metadataRepo.EXPECT().Update(gomock.Any(), constant.EntitySegment, segmentID.String(), gomock.Any()).Return(nil).AnyTimes()
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntitySegment, segmentID.String()).Return(nil, nil).AnyTimes()

	handler := &SegmentHandler{Command: &command.UseCase{SegmentRepo: segmentRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaSegmentApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Updated Segment Name"})
	req := httptest.NewRequest(http.MethodPatch, segmentsPath(orgID, ledgerID, "/"+segmentID.String()), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Updated Segment Name", got["name"])
	assert.Equal(t, segmentID.String(), got["id"])
}

func TestUpdateSegment_NotFound_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. updateSegment's command-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	segmentID := uuid.Must(libCommons.GenerateUUIDv7())

	segmentRepo := segment.NewMockRepository(ctrl)
	segmentRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, segmentID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntitySegment)).Times(1)

	handler := &SegmentHandler{Command: &command.UseCase{SegmentRepo: segmentRepo}}

	app := buildHumaSegmentApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Updated Segment Name"})
	req := httptest.NewRequest(http.MethodPatch, segmentsPath(orgID, ledgerID, "/"+segmentID.String()), bytes.NewReader(body))
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

func TestDeleteSegment_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. deleteSegment's command-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	segmentID := uuid.Must(libCommons.GenerateUUIDv7())

	segmentRepo := segment.NewMockRepository(ctrl)
	segmentRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, segmentID).
		Return(pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntitySegment)).Times(1)

	handler := &SegmentHandler{Command: &command.UseCase{SegmentRepo: segmentRepo}}

	app := buildHumaSegmentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, segmentsPath(orgID, ledgerID, "/"+segmentID.String()), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestCountSegments_ServiceError(t *testing.T) {
	// NOT parallel: process-global huma state. countSegments' query-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	segmentRepo := segment.NewMockRepository(ctrl)
	segmentRepo.EXPECT().Count(gomock.Any(), orgID, ledgerID).
		Return(int64(0), pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntitySegment)).Times(1)

	handler := &SegmentHandler{Query: &query.UseCase{SegmentRepo: segmentRepo}}

	app := buildHumaSegmentApp(t, handler, true)

	req := httptest.NewRequest(http.MethodHead, segmentsPath(orgID, ledgerID, "/metrics/count"), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Empty(t, resp.Header.Get(constant.XTotalCount), "a failed count must not advertise a total")
}
