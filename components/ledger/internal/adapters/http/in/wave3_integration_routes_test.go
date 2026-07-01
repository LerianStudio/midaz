// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

// mountWave3Routes wires the three Wave-3 (additive) Huma-migrated registrars —
// CRM (holders/instruments/holder-accounts/encryption/audit), fees/billing, and
// composition — on a /v1 group, mirroring the production humaMount seam:
// problem.Install() before any huma.Register, the shared Huma API built with
// openapi.New over the /v1 group, and each RegisterXxxRoutesToApp attaching the
// Fiber auth+tenant middleware chain (as middleware only) plus the Huma terminals
// on that group.
//
// Every conditional handler (hah/eh/auditHandler) is passed NON-nil here so the
// FULL surface mounts; the nil-guard conditionality is exercised separately by
// TestWave3RoutesRespectNilGuards.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError
// hook and Huma validation uses process-global sync.Pools.
func mountWave3Routes(app *fiber.App, auth *middleware.AuthClient) huma.API {
	libProblem.Install()
	apiV1 := app.Group("/v1")
	hAPI := openapi.New(app, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})
	pkgHTTP.InstallLedgerSchemaNamer(hAPI)

	RegisterCRMRoutesToApp(apiV1, hAPI, auth,
		&HolderHandler{}, &InstrumentHandler{}, &HolderAccountsHandler{},
		&EncryptionHandler{}, &AuditHandler{}, nil)
	RegisterFeesRoutesToApp(apiV1, hAPI, auth,
		&PackageHandler{}, &FeeHandler{}, &BillingPackageHandler{}, &BillingCalculateHandler{}, nil)
	RegisterCompositionRoutesToApp(apiV1, hAPI, auth, &CompositionHandler{}, nil)

	return hAPI
}

const (
	wave3Org       = "/v1/organizations/:organization_id"
	wave3OrgLedger = wave3Org + "/ledgers/:ledger_id"
)

// wave3FullRoutes is the byte-for-byte route surface the three Wave-3 registrars
// mount when every conditional handler is present. Paths + methods are preserved
// from the pre-Huma inline Fiber routes; only the transport changed.
var wave3FullRoutes = []string{
	// CRM holders (5)
	"POST:" + wave3Org + "/holders",
	"GET:" + wave3Org + "/holders/:id",
	"PATCH:" + wave3Org + "/holders/:id",
	"DELETE:" + wave3Org + "/holders/:id",
	"GET:" + wave3Org + "/holders",
	// CRM holder-accounts (1, conditional on hah)
	"GET:" + wave3Org + "/holders/:id/accounts",
	// CRM instruments (6)
	"GET:" + wave3Org + "/instruments",
	"POST:" + wave3Org + "/holders/:holder_id/instruments",
	"GET:" + wave3Org + "/holders/:holder_id/instruments/:instrument_id",
	"PATCH:" + wave3Org + "/holders/:holder_id/instruments/:instrument_id",
	"DELETE:" + wave3Org + "/holders/:holder_id/instruments/:instrument_id",
	"DELETE:" + wave3Org + "/holders/:holder_id/instruments/:instrument_id/related-parties/:related_party_id",
	// CRM encryption (2, conditional on eh)
	"POST:" + wave3Org + "/encryption/provision",
	"GET:" + wave3Org + "/encryption/status",
	// CRM audit (1, conditional on auditHandler)
	"GET:" + wave3Org + "/protection/audit",
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
	apiV1 := app.Group("/v1")
	hAPI := openapi.New(app, apiV1, openapi.Config{Title: "ledger-nilguard", Version: "test", Servers: []string{"/v1"}})
	pkgHTTP.InstallLedgerSchemaNamer(hAPI)

	// hah, eh, auditHandler all nil -> holder-accounts + encryption + audit absent.
	RegisterCRMRoutesToApp(apiV1, hAPI, auth,
		&HolderHandler{}, &InstrumentHandler{}, nil, nil, nil, nil)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	// Base holder/instrument routes MUST still mount.
	assert.True(t, routeSet["POST:"+wave3Org+"/holders"], "holders POST must mount unconditionally")
	assert.True(t, routeSet["GET:"+wave3Org+"/instruments"], "instruments GET must mount unconditionally")

	// Conditional routes MUST be absent.
	assert.False(t, routeSet["GET:"+wave3Org+"/holders/:id/accounts"],
		"holder-accounts route must NOT mount when hah is nil")
	assert.False(t, routeSet["POST:"+wave3Org+"/encryption/provision"],
		"encryption provision route must NOT mount when eh is nil")
	assert.False(t, routeSet["GET:"+wave3Org+"/encryption/status"],
		"encryption status route must NOT mount when eh is nil")
	assert.False(t, routeSet["GET:"+wave3Org+"/protection/audit"],
		"audit route must NOT mount when auditHandler is nil")
}

// TestRegisterFeesRoutesToApp_DoesNotMountFeeCalculate asserts POST /v1/fees stays
// unmounted after migration: in the unified binary fees run in-process via the
// transaction seam, so only the dry-run POST /v1/.../estimates is exposed.
func TestRegisterFeesRoutesToApp_DoesNotMountFeeCalculate(t *testing.T) {
	// NOT parallel: mutates process-global huma state.
	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	mountWave3Routes(app, auth)

	for _, r := range app.GetRoutes() {
		assert.NotEqualf(t, fiber.MethodPost+":/v1/fees", r.Method+":"+r.Path,
			"POST /v1/fees must NOT be mounted — fees run in-process via the seam")
	}
}
