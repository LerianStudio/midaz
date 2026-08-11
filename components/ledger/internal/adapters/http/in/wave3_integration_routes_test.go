// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// mountWave3Routes wires the three Wave-3 (additive) Huma-migrated registrars on the
// version groups they serve, mirroring the production humaMount seam: fees/billing and
// composition on /v1, and CRM (holders/instruments/holder-accounts/encryption/audit) on
// /v2, since CRM is v2-only in the unified binary. One shared Huma document backs both
// version groups (openapi.New over the root app + one huma.NewGroup per prefix), and
// each RegisterXxxRoutesToApp attaches the Fiber auth+tenant middleware chain (as
// middleware only) plus the Huma terminals on its group.
//
// Every conditional CRM handler (hah/eh/auditHandler) is passed NON-nil here so the
// FULL surface mounts; the nil-guard conditionality is exercised separately by
// TestWave3RoutesRespectNilGuards.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError
// hook and Huma validation uses process-global sync.Pools.
func mountWave3Routes(app *fiber.App, auth *middleware.AuthClient) huma.API {
	libProblem.Install()
	hAPI := openapi.New(app, app, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/"}})
	pkgHTTP.InstallLedgerSchemaNamer(hAPI)

	fiberV1 := app.Group("/v1")
	humaV1 := huma.NewGroup(hAPI, "/v1")
	RegisterFeesRoutesToApp(fiberV1, humaV1, auth,
		&PackageHandler{}, &FeeHandler{}, &BillingPackageHandler{}, &BillingCalculateHandler{}, nil)
	RegisterCompositionRoutesToApp(fiberV1, humaV1, auth, &CompositionHandler{}, nil)

	fiberV2 := app.Group("/v2")
	humaV2 := huma.NewGroup(hAPI, "/v2")
	RegisterCRMV2RoutesToApp(fiberV2, humaV2, auth,
		&HolderHandler{}, &InstrumentHandler{}, &HolderAccountsHandler{},
		&EncryptionHandler{}, &AuditHandler{}, nil)

	return hAPI
}

const (
	wave3Org       = "/v1/organizations/:organization_id"
	wave3OrgLedger = wave3Org + "/ledgers/:ledger_id"
	// CRM is served on /v2 only.
	wave3OrgV2 = "/v2/organizations/:organization_id"
)

// wave3FullRoutes is the byte-for-byte route surface the three Wave-3 registrars
// mount when every conditional handler is present. Paths + methods are preserved
// from the pre-Huma inline Fiber routes; only the transport changed. CRM sits on /v2,
// fees/billing and composition on /v1.
var wave3FullRoutes = []string{
	// CRM holders (5)
	"POST:" + wave3OrgV2 + "/holders",
	"GET:" + wave3OrgV2 + "/holders/:id",
	"PATCH:" + wave3OrgV2 + "/holders/:id",
	"DELETE:" + wave3OrgV2 + "/holders/:id",
	"GET:" + wave3OrgV2 + "/holders",
	// CRM holder-accounts (1, conditional on hah)
	"GET:" + wave3OrgV2 + "/holders/:id/accounts",
	// CRM instruments (6)
	"GET:" + wave3OrgV2 + "/instruments",
	"POST:" + wave3OrgV2 + "/holders/:holder_id/instruments",
	"GET:" + wave3OrgV2 + "/holders/:holder_id/instruments/:instrument_id",
	"PATCH:" + wave3OrgV2 + "/holders/:holder_id/instruments/:instrument_id",
	"DELETE:" + wave3OrgV2 + "/holders/:holder_id/instruments/:instrument_id",
	"DELETE:" + wave3OrgV2 + "/holders/:holder_id/instruments/:instrument_id/related-parties/:related_party_id",
	// CRM encryption (2, conditional on eh)
	"POST:" + wave3OrgV2 + "/encryption/provision",
	"GET:" + wave3OrgV2 + "/encryption/status",
	// CRM audit (1, conditional on auditHandler)
	"GET:" + wave3OrgV2 + "/protection/audit",
	// Fees packages (5)
	"POST:" + wave3Org + "/packages",
	"GET:" + wave3Org + "/packages",
	"GET:" + wave3Org + "/packages/:id",
	"PATCH:" + wave3Org + "/packages/:id",
	"DELETE:" + wave3Org + "/packages/:id",
	// Fees estimate (1)
	"POST:" + wave3Org + "/estimates",
	// Billing packages (5)
	"POST:" + wave3Org + "/billing-packages",
	"GET:" + wave3Org + "/billing-packages",
	"GET:" + wave3Org + "/billing-packages/:id",
	"PATCH:" + wave3Org + "/billing-packages/:id",
	"DELETE:" + wave3Org + "/billing-packages/:id",
	// Billing calculate (1)
	"POST:" + wave3Org + "/billing/calculate",
	// Composition (1)
	"POST:" + wave3OrgLedger + "/holders/:id/accounts",
}

// TestWave3RoutesMountedOnGroup asserts every Wave-3 migrated route is served on
// the /v1 group after the Fiber-inline -> Huma migration. A missing route means the
// auth-middleware attach or the Huma registration regressed.
func TestWave3RoutesMountedOnGroup(t *testing.T) {
	// NOT parallel: mountWave3Routes mutates process-global huma state.
	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	mountWave3Routes(app, auth)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	for _, w := range wave3FullRoutes {
		assert.Truef(t, routeSet[w], "expected mounted route %q", w)
	}
}

// TestWave3RoutesRespectNilGuards asserts the conditional CRM seams stay
// unregistered when their handler is nil, exactly as the pre-Huma inline
// `if hah/eh/auditHandler != nil` guards did: no holder-accounts route without a
// ledger account-query backing, no encryption/audit routes outside envelope mode.
func TestWave3RoutesRespectNilGuards(t *testing.T) {
	// NOT parallel: mutates process-global huma state.
	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	libProblem.Install()
	apiV2 := app.Group("/v2")
	hAPI := openapi.New(app, apiV2, openapi.Config{Title: "ledger-nilguard", Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(hAPI)

	// hah, eh, auditHandler all nil -> holder-accounts + encryption + audit absent.
	RegisterCRMV2RoutesToApp(apiV2, hAPI, auth,
		&HolderHandler{}, &InstrumentHandler{}, nil, nil, nil, nil)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	// Base holder/instrument routes MUST still mount.
	assert.True(t, routeSet["POST:"+wave3OrgV2+"/holders"], "holders POST must mount unconditionally")
	assert.True(t, routeSet["GET:"+wave3OrgV2+"/instruments"], "instruments GET must mount unconditionally")

	// Conditional routes MUST be absent.
	assert.False(t, routeSet["GET:"+wave3OrgV2+"/holders/:id/accounts"],
		"holder-accounts route must NOT mount when hah is nil")
	assert.False(t, routeSet["POST:"+wave3OrgV2+"/encryption/provision"],
		"encryption provision route must NOT mount when eh is nil")
	assert.False(t, routeSet["GET:"+wave3OrgV2+"/encryption/status"],
		"encryption status route must NOT mount when eh is nil")
	assert.False(t, routeSet["GET:"+wave3OrgV2+"/protection/audit"],
		"audit route must NOT mount when auditHandler is nil")
}

// TestRegisterFeesRoutesToApp_DoesNotMountFeeCalculate asserts POST /v1/fees stays
// unmounted after migration: in the unified binary fees run in-process via the
// transaction seam, so only the dry-run POST /v1/.../estimates is exposed.
//
// Absence is checked on BOTH surfaces the registrar writes to — the Fiber router
// (nothing is served) and the Huma document (nothing is published) — because a path
// published without a route, or served without being published, is a divergence in its
// own right. The ledger-scoped twin of this is
// TestRegisterFeesV2RoutesToApp_DoesNotMountFeeCalculate.
func TestRegisterFeesRoutesToApp_DoesNotMountFeeCalculate(t *testing.T) {
	// NOT parallel: mutates process-global huma state.
	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	hAPI := mountWave3Routes(app, auth)

	for _, r := range app.GetRoutes() {
		assert.NotEqualf(t, fiber.MethodPost+":/v1/fees", r.Method+":"+r.Path,
			"POST /v1/fees must NOT be mounted — fees run in-process via the seam")
		assert.NotEqualf(t, fiber.MethodPost+":"+wave3Org+"/fees", r.Method+":"+r.Path,
			"POST %s/fees must NOT be mounted — fees run in-process via the seam", wave3Org)
	}

	for path := range hAPI.OpenAPI().Paths {
		assert.NotContainsf(t, path, "/fees",
			"the v1 contract must not publish a fee-calculate path, found %q", path)
	}
}
