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
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// feesV2Stubs bundles the four fake services the ledger-scoped surface delegates to,
// so a test can assert on whichever one its operation reaches.
type feesV2Stubs struct {
	pkgSvc     *stubPackageService
	feeSvc     *stubFeeService
	billingSvc *stubBillingPackageService
	calcSvc    *stubBillingCalculateService
}

// buildFeesV2App mounts the full ledger-scoped fee surface through the PRODUCTION
// registrar — the real guard chain (auth disabled, so it passes through, plus the real
// ParseUUIDPathParameters) and the real Huma terminals — so the tests below exercise
// the same wiring the unified server mounts.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError
// hook and Huma validation uses process-global sync.Pools.
func buildFeesV2App(t *testing.T) (*fiber.App, *feesV2Stubs) {
	t.Helper()

	return buildFeesV2AppWithOptions(t, nil)
}

// buildFeesV2AppWithOptions is buildFeesV2App with the fees route options under the
// caller's control, so a test can put a probe in the slot the production
// feesRouteOptions fills with the fee tenant chain.
func buildFeesV2AppWithOptions(t *testing.T, routeOptions *pkgHTTP.ProtectedRouteOptions) (*fiber.App, *feesV2Stubs) {
	t.Helper()

	stubs := &feesV2Stubs{
		pkgSvc:     &stubPackageService{},
		feeSvc:     &stubFeeService{},
		billingSvc: &stubBillingPackageService{},
		calcSvc:    &stubBillingCalculateService{},
	}

	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	libProblem.Install()

	apiV2 := app.Group("/v2")
	hAPI := openapi.New(app, apiV2, openapi.Config{Title: "ledger-fees-v2-behaviour", Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(hAPI)

	auth := &middleware.AuthClient{Enabled: false}

	RegisterPackageV2RoutesToApp(apiV2, hAPI, auth, &PackageHandler{Service: stubs.pkgSvc}, routeOptions)
	RegisterFeeEstimateV2RoutesToApp(apiV2, hAPI, auth, &FeeHandler{Service: stubs.feeSvc}, routeOptions)
	RegisterBillingPackageV2RoutesToApp(apiV2, hAPI, auth, &BillingPackageHandler{Service: stubs.billingSvc}, routeOptions)
	RegisterBillingCalculateV2RoutesToApp(apiV2, hAPI, auth, &BillingCalculateHandler{Service: stubs.calcSvc}, routeOptions)

	return app, stubs
}

// feeV2Path substitutes concrete identifiers into a Fiber route template. The
// organization and ledger labels do not contain the ":id" token, so the replacements
// cannot collide.
func feeV2Path(template string, orgID, ledgerID, id uuid.UUID) string {
	return strings.NewReplacer(
		":organization_id", orgID.String(),
		":ledger_id", ledgerID.String(),
		":id", id.String(),
	).Replace(template)
}

// driveFeeV2 issues a request against the mounted v2 surface and returns the status
// and the decoded body. Write verbs carry a body only when the caller supplies one.
func driveFeeV2(t *testing.T, app *fiber.App, method, url, body string) (int, map[string]any) {
	t.Helper()

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, url, nil)
	} else {
		req = httptest.NewRequest(method, url, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	decoded := make(map[string]any)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &decoded)
	}

	return resp.StatusCode, decoded
}

// driveFeeV2Probe drives one route of the ledger-scoped surface with placeholder
// identifiers and a minimal body, purely so the middleware in front of its terminal
// runs. The terminal's answer is irrelevant — the chain executes before it.
func driveFeeV2Probe(t *testing.T, app *fiber.App, method, template string) {
	t.Helper()

	url := feeV2Path(template, uuid.New(), uuid.New(), uuid.New())

	body := ""
	if method == http.MethodPost || method == http.MethodPatch {
		body = "{}"
	}

	driveFeeV2(t, app, method, url, body)
}

// createPackageV2JSON is validCreatePackageJSON with the ledger under the caller's
// control, so a test can make the body agree or disagree with the path.
func createPackageV2JSON(ledgerID string) string {
	return `{"feeGroupLabel":"Standard","ledgerId":"` + ledgerID + `","minimumAmount":"100.00","maximumAmount":"1000.00","enable":true,` +
		`"fees":{"f1":{"feeLabel":"Admin","referenceAmount":"afterFeesAmount","priority":2,"isDeductibleFrom":false,` +
		`"creditAccount":"conta_receita","calculationModel":{"applicationRule":"flatFee","calculations":[{"type":"flat","value":"50.00"}]}}}}`
}

// createBillingPackageV2JSON is validBillingPackageJSON with a caller-chosen ledger.
func createBillingPackageV2JSON(ledgerID string) string {
	return `{"label":"Monthly Volume","type":"volume","ledgerId":"` + ledgerID + `"}`
}

// estimateV2JSON is estimateBodyJSON with a caller-chosen ledger.
func estimateV2JSON(ledgerID string) string {
	return `{"packageId":"` + validLedgerUUID() + `","ledgerId":"` + ledgerID + `","transaction":{"send":` + validSendJSON() + `}}`
}

// TestFeesV2_BodyLedgerMustMatchPath pins the body-versus-path decision on the four
// operations whose body carries a ledger. The field stays required — the models are
// shared with the organization-scoped surface and with the in-process fee seam — so
// what the ledger-scoped surface adds is the refusal of a value that names a different
// ledger than the path. A matching value is accepted and reaches the service; a
// different one is refused before the service is touched.
func TestFeesV2_BodyLedgerMustMatchPath(t *testing.T) {
	orgID := uuid.New()
	pathLedger := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	otherLedger := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	tests := []struct {
		name     string
		method   string
		template string
		body     func(ledger string) string
		called   func(s *feesV2Stubs) bool
		okStatus int
	}{
		{
			name:     "create_package",
			method:   http.MethodPost,
			template: feesV2Scope + "/packages",
			body:     createPackageV2JSON,
			called:   func(s *feesV2Stubs) bool { return s.pkgSvc.createCalled },
			okStatus: http.StatusCreated,
		},
		{
			name:     "estimate_fee",
			method:   http.MethodPost,
			template: feesV2Scope + "/estimates",
			body:     estimateV2JSON,
			called:   func(s *feesV2Stubs) bool { return s.feeSvc.called },
			okStatus: http.StatusOK,
		},
		{
			name:     "create_billing_package",
			method:   http.MethodPost,
			template: feesV2Scope + "/billing-packages",
			body:     createBillingPackageV2JSON,
			called:   func(s *feesV2Stubs) bool { return s.billingSvc.createCalled },
			okStatus: http.StatusCreated,
		},
		{
			name:     "calculate_billing",
			method:   http.MethodPost,
			template: feesV2Scope + "/billing/calculate",
			body:     validBillingCalculateJSON,
			called:   func(s *feesV2Stubs) bool { return s.calcSvc.called },
			okStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_matching_body_ledger_is_accepted", func(t *testing.T) {
			// NOT parallel: huma registration mutates process-global state.
			app, stubs := buildFeesV2App(t)
			seedFeesV2Results(stubs)

			url := feeV2Path(tt.template, orgID, pathLedger, uuid.Nil)
			status, body := driveFeeV2(t, app, tt.method, url, tt.body(pathLedger.String()))

			assert.Equalf(t, tt.okStatus, status, "body: %v", body)
			assert.True(t, tt.called(stubs), "the service must be reached when the body agrees with the path")
		})

		t.Run(tt.name+"_conflicting_body_ledger_is_refused", func(t *testing.T) {
			// NOT parallel: huma registration mutates process-global state.
			app, stubs := buildFeesV2App(t)
			seedFeesV2Results(stubs)

			url := feeV2Path(tt.template, orgID, pathLedger, uuid.Nil)
			status, body := driveFeeV2(t, app, tt.method, url, tt.body(otherLedger.String()))

			assert.Equal(t, http.StatusBadRequest, status)
			assert.Equal(t, constant.ErrLedgerIDMismatch.Error(), body["code"],
				"a body naming another ledger must be refused with the mismatch code, got: %v", body)
			assert.False(t, tt.called(stubs),
				"MONEY-PATH: the service must not be reached when the body names another ledger")
		})
	}
}

// seedFeesV2Results gives every stub a non-nil success result, so an operation that is
// supposed to succeed is not turned into a 500 by a nil return.
func seedFeesV2Results(s *feesV2Stubs) {
	s.pkgSvc.createResult = &pack.Package{ID: uuid.New()}
	s.pkgSvc.getByIDResult = &pack.Package{ID: uuid.New()}
	s.feeSvc.result = &model.FeeEstimateResult{
		Transaction: model.FeeAdjustedTransaction{Metadata: map[string]any{}},
	}
	s.billingSvc.createResult = &model.BillingPackage{ID: uuid.New().String()}
	s.billingSvc.getByIDResult = &model.BillingPackage{ID: uuid.New().String()}
	s.billingSvc.updateResult = &model.BillingPackage{ID: uuid.New().String()}
	s.calcSvc.result = &model.BillingCalculateResponse{}
}

// TestFeesV2_ListsRefuseTheLedgerQueryParameter pins the decision on the query side.
//
// Neither listing validates its key set — the fee-package binder ignores keys it does
// not recognize and the billing binder reads the four it wants out of the map — so
// nothing would otherwise report a ledgerId on a path that already names the ledger,
// and a request for a DIFFERENT ledger would be answered with THIS one. The key is
// refused whatever it carries, including the empty value, which on the
// organization-scoped surface means "every ledger of the organization".
func TestFeesV2_ListsRefuseTheLedgerQueryParameter(t *testing.T) {
	orgID := uuid.New()
	pathLedger := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	otherLedger := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	lists := []struct {
		name     string
		template string
		reached  func(s *feesV2Stubs) bool
	}{
		{
			name:     "packages",
			template: feesV2Scope + "/packages",
			reached:  func(s *feesV2Stubs) bool { return s.pkgSvc.gotGetAllOrg != uuid.Nil },
		},
		{
			name:     "billing_packages",
			template: feesV2Scope + "/billing-packages",
			reached:  func(s *feesV2Stubs) bool { return s.billingSvc.gotGetAllOrg != uuid.Nil },
		},
	}

	queries := []struct {
		name  string
		query string
	}{
		{name: "naming_another_ledger", query: "?ledgerId=" + otherLedger.String()},
		{name: "empty_value", query: "?ledgerId="},
		{name: "restating_the_path_ledger", query: "?ledgerId=" + pathLedger.String()},
		{name: "different_casing", query: "?LedgerId=" + otherLedger.String()},
	}

	for _, list := range lists {
		for _, q := range queries {
			t.Run(list.name+"_"+q.name, func(t *testing.T) {
				// NOT parallel: huma registration mutates process-global state.
				app, stubs := buildFeesV2App(t)

				url := feeV2Path(list.template, orgID, pathLedger, uuid.Nil) + q.query
				status, body := driveFeeV2(t, app, http.MethodGet, url, "")

				assert.Equal(t, http.StatusBadRequest, status, "body: %v", body)
				assert.Equal(t, constant.ErrLedgerScopedQueryParameter.Error(), body["code"],
					"a ledgerId query on a ledger-scoped listing must be refused, got: %v", body)
				assert.False(t, list.reached(stubs),
					"MONEY-PATH: the listing must not run with a ledgerId query present")
			})
		}
	}
}

// TestFeesV2_ListsScopeToThePathLedger is the other direction of the decision above:
// with no ledgerId query the listing runs, and the ledger it asks the repository for is
// the one the path named — never the empty filter that means every ledger of the
// organization.
func TestFeesV2_ListsScopeToThePathLedger(t *testing.T) {
	orgID := uuid.New()
	pathLedger := uuid.MustParse("11111111-1111-4111-8111-111111111111")

	t.Run("packages", func(t *testing.T) {
		// NOT parallel: huma registration mutates process-global state.
		app, stubs := buildFeesV2App(t)

		url := feeV2Path(feesV2Scope+"/packages", orgID, pathLedger, uuid.Nil) + "?limit=5&page=2"
		status, body := driveFeeV2(t, app, http.MethodGet, url, "")

		assert.Equal(t, http.StatusOK, status, "body: %v", body)
		assert.Equal(t, orgID, stubs.pkgSvc.gotGetAllOrg)
		assert.Equal(t, pathLedger, stubs.pkgSvc.gotGetAllFilter.LedgerID,
			"MONEY-PATH: the listing must be pinned to the ledger the path named")
		assert.Equal(t, 5, stubs.pkgSvc.gotGetAllFilter.Limit, "the rest of the query is still honoured")
		assert.Equal(t, 2, stubs.pkgSvc.gotGetAllFilter.Page)
	})

	t.Run("billing_packages", func(t *testing.T) {
		// NOT parallel: huma registration mutates process-global state.
		app, stubs := buildFeesV2App(t)

		url := feeV2Path(feesV2Scope+"/billing-packages", orgID, pathLedger, uuid.Nil) + "?limit=5&page=2&type=volume"
		status, body := driveFeeV2(t, app, http.MethodGet, url, "")

		assert.Equal(t, http.StatusOK, status, "body: %v", body)
		assert.Equal(t, orgID, stubs.billingSvc.gotGetAllOrg)
		require.NotNil(t, stubs.billingSvc.gotGetAllLedger,
			"MONEY-PATH: a nil ledger would list every ledger of the organization")
		assert.Equal(t, pathLedger, *stubs.billingSvc.gotGetAllLedger)
		assert.Equal(t, "volume", stubs.billingSvc.gotGetAllType, "the rest of the query is still honoured")
		assert.Equal(t, 5, stubs.billingSvc.gotGetAllLimit)
		assert.Equal(t, 2, stubs.billingSvc.gotGetAllPage)
	})
}

// TestFeesV2_ByIDOperationsCarryThePathLedger pins that every by-ID read and write
// asks the service for the ledger the path named. The organization-scoped surface
// passes uuid.Nil there, which both repositories read as "any ledger of the
// organization"; a ledger-scoped route that also passed it would answer with a package
// the named ledger does not own.
func TestFeesV2_ByIDOperationsCarryThePathLedger(t *testing.T) {
	orgID := uuid.New()
	pathLedger := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	resourceID := uuid.New()

	tests := []struct {
		name     string
		method   string
		template string
		body     string
		got      func(s *feesV2Stubs) uuid.UUID
	}{
		{
			name:     "get_package",
			method:   http.MethodGet,
			template: feesV2Scope + "/packages/:id",
			got:      func(s *feesV2Stubs) uuid.UUID { return s.pkgSvc.gotGetByIDLedger },
		},
		{
			name:     "update_package",
			method:   http.MethodPatch,
			template: feesV2Scope + "/packages/:id",
			body:     `{"feeGroupLabel":"Renamed"}`,
			got:      func(s *feesV2Stubs) uuid.UUID { return s.pkgSvc.gotUpdateLedger },
		},
		{
			name:     "delete_package",
			method:   http.MethodDelete,
			template: feesV2Scope + "/packages/:id",
			got:      func(s *feesV2Stubs) uuid.UUID { return s.pkgSvc.gotDeleteLedger },
		},
		{
			name:     "get_billing_package",
			method:   http.MethodGet,
			template: feesV2Scope + "/billing-packages/:id",
			got:      func(s *feesV2Stubs) uuid.UUID { return s.billingSvc.gotGetByIDLedger },
		},
		{
			name:     "update_billing_package",
			method:   http.MethodPatch,
			template: feesV2Scope + "/billing-packages/:id",
			body:     `{"label":"Renamed"}`,
			got:      func(s *feesV2Stubs) uuid.UUID { return s.billingSvc.gotUpdateLedger },
		},
		{
			name:     "delete_billing_package",
			method:   http.MethodDelete,
			template: feesV2Scope + "/billing-packages/:id",
			got:      func(s *feesV2Stubs) uuid.UUID { return s.billingSvc.gotDeleteLedger },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// NOT parallel: huma registration mutates process-global state.
			app, stubs := buildFeesV2App(t)
			seedFeesV2Results(stubs)

			url := feeV2Path(tt.template, orgID, pathLedger, resourceID)
			status, body := driveFeeV2(t, app, tt.method, url, tt.body)

			require.Lessf(t, status, http.StatusBadRequest, "operation must succeed, body: %v", body)
			assert.Equal(t, pathLedger, tt.got(stubs),
				"MONEY-PATH: the service must be asked for the ledger the path named")
		})
	}
}

// TestFeesV2_NilLedgerInPathIsRefused pins the guard on the one path value that is a
// syntactically valid UUID and still means "no ledger" to both fee repositories. Carried
// inward it would turn every ledger-scoped operation back into an organization-scoped
// one, silently. No ledger is created with it, so refusing it turns nothing legitimate
// away.
func TestFeesV2_NilLedgerInPathIsRefused(t *testing.T) {
	orgID := uuid.New()
	resourceID := uuid.New()

	for _, route := range feesV2FullRoutes {
		method, template, ok := strings.Cut(route, ":")
		require.True(t, ok)

		t.Run(method+"_"+strings.TrimPrefix(template, "/v2/"), func(t *testing.T) {
			// NOT parallel: huma registration mutates process-global state.
			app, stubs := buildFeesV2App(t)
			seedFeesV2Results(stubs)

			body := ""
			if method == http.MethodPost || method == http.MethodPatch {
				body = "{}"
			}

			url := feeV2Path(template, orgID, uuid.Nil, resourceID)
			status, respBody := driveFeeV2(t, app, method, url, body)

			assert.Equal(t, http.StatusBadRequest, status, "body: %v", respBody)
			assert.Equal(t, constant.ErrInvalidPathParameter.Error(), respBody["code"],
				"the nil ledger must be refused as an invalid path parameter, got: %v", respBody)
			assert.False(t, feesV2AnyServiceReached(stubs),
				"MONEY-PATH: no service may be reached with the nil ledger")
		})
	}
}

// feesV2AnyServiceReached reports whether any of the four fakes was called.
func feesV2AnyServiceReached(s *feesV2Stubs) bool {
	return s.pkgSvc.createCalled || s.pkgSvc.updateCalled || s.pkgSvc.deleteCalled ||
		s.pkgSvc.gotGetAllOrg != uuid.Nil || s.pkgSvc.gotGetByIDID != uuid.Nil ||
		s.feeSvc.called ||
		s.billingSvc.createCalled || s.billingSvc.updateCalled || s.billingSvc.deleteCalled ||
		s.billingSvc.gotGetAllOrg != uuid.Nil || s.billingSvc.gotGetByIDID != uuid.Nil ||
		s.calcSvc.called
}

// TestFeesV2_CreateBillingPackageCanonicalisesTheBodyLedger pins that a billing
// package created through the ledger-scoped surface is reachable through it
// afterwards.
//
// The create guard admits the body ledger on parsed-UUID equality, so every
// spelling uuid.Parse accepts reaches the create. The stored ledger is a string,
// and every scoped read compares it against the canonical lowercase-hyphenated
// form the path resolves to — so a body spelled any other way would persist a
// value no scoped read, listing or billing calculation can match, and the package
// would be created and then be unreachable.
func TestFeesV2_CreateBillingPackageCanonicalisesTheBodyLedger(t *testing.T) {
	orgID := uuid.New()
	pathLedger := uuid.MustParse("018f3a2b-1111-4111-8111-111111111111")

	spellings := []struct {
		name string
		body string
	}{
		{name: "uppercase", body: strings.ToUpper(pathLedger.String())},
		{name: "braced", body: "{" + pathLedger.String() + "}"},
		{name: "unhyphenated", body: strings.ReplaceAll(pathLedger.String(), "-", "")},
	}

	for _, spelling := range spellings {
		t.Run(spelling.name, func(t *testing.T) {
			// NOT parallel: huma registration mutates process-global state.
			app, stubs := buildFeesV2App(t)
			seedFeesV2Results(stubs)

			createURL := feeV2Path(feesV2Scope+"/billing-packages", orgID, pathLedger, uuid.Nil)
			status, body := driveFeeV2(t, app, http.MethodPost, createURL, createBillingPackageV2JSON(spelling.body))

			require.Equalf(t, http.StatusCreated, status, "body: %v", body)
			require.NotNil(t, stubs.billingSvc.gotCreate)

			readURL := feeV2Path(feesV2Scope+"/billing-packages/:id", orgID, pathLedger, uuid.New())
			status, body = driveFeeV2(t, app, http.MethodGet, readURL, "")
			require.Equalf(t, http.StatusOK, status, "body: %v", body)

			assert.Equal(t, stubs.billingSvc.gotGetByIDLedger.String(), stubs.billingSvc.gotCreate.LedgerID,
				"MONEY-PATH: the ledger persisted by the create must be the one every scoped read filters on, "+
					"or the package is created and then unreachable")
		})
	}
}
