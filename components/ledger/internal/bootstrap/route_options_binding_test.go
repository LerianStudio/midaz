// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"flag"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tmmongo "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/postgres"
	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file pins the registrar -> ProtectedRouteOptions binding, the one relationship the
// route-table golden cannot capture: six of the seven route-scoped options carry exactly two
// post-auth handlers, so a positional swap moves neither path nor handler count. The crm and
// fees options both write the GENERIC tenant-context key over different Mongo managers, so
// swapping that pair resolves CRM holder PII against the fees tenant database with every other
// gate green.
//
// Two halves close the hole from both ends. TestRouteOptionsBinding pins role -> real instance
// by pointer identity at the composition seam (buildHumaMountDeps). TestRouteRoles pins route ->
// role behaviourally, by mounting the full surface with a distinct sentinel per role and
// recording which one each route runs.

// TestRouteOptionsBinding asserts that buildHumaMountDeps threads each route-scoped option to
// the correctly named field, by pointer identity. It exercises buildUnifiedRouteSetup in both
// modes: single-tenant returns a zero-value setup whose six options are nil (the product
// default), multi-tenant returns seven pairwise-distinct instances. A crm<->fees swap in the
// mapper fails the two named assertions here because those two instances are distinct pointers.
func TestRouteOptionsBinding(t *testing.T) {
	logger := newTestLogger()

	// Single-tenant: buildUnifiedRouteSetup short-circuits to a zero-value setup before it
	// looks at any manager, so nil managers are the correct inputs and all seven options are nil.
	stSetup, err := buildUnifiedRouteSetup(&Config{}, logger, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err, "single-tenant setup must not error")
	require.NotNil(t, stSetup, "single-tenant setup is a zero value, not nil")

	assert.Nil(t, stSetup.onboardingRouteOptions, "single-tenant onboarding option must be nil")
	assert.Nil(t, stSetup.transactionRouteOptions, "single-tenant transaction option must be nil")
	assert.Nil(t, stSetup.ledgerRouteOptions, "single-tenant ledger option must be nil")
	assert.Nil(t, stSetup.crmRouteOptions, "single-tenant crm option must be nil")
	assert.Nil(t, stSetup.feesRouteOptions, "single-tenant fees option must be nil")
	assert.Nil(t, stSetup.compositionRouteOptions, "single-tenant composition option must be nil")
	assert.Nil(t, stSetup.holderAccountsRouteOptions, "single-tenant holder-accounts option must be nil")

	stDeps := buildHumaMountDepsWithNilHandlers(stSetup)
	assert.Nil(t, stDeps.OnboardingOptions, "single-tenant deps onboarding option must be nil")
	assert.Nil(t, stDeps.CRMOptions, "single-tenant deps crm option must be nil")
	assert.Nil(t, stDeps.FeesOptions, "single-tenant deps fees option must be nil")
	assert.Nil(t, stDeps.HolderAccountsOptions, "single-tenant deps holder-accounts option must be nil")

	// Multi-tenant: non-nil managers make buildUnifiedRouteSetup build seven distinct options
	// drawn from five separate tenant middlewares. The managers are zero-value structs because
	// buildUnifiedRouteSetup only wires them into middleware options; nothing connects here.
	mtSetup, err := buildUnifiedRouteSetup(
		&Config{MultiTenantEnabled: true}, logger,
		&tmpostgres.Manager{}, &tmpostgres.Manager{},
		&tmmongo.Manager{}, &tmmongo.Manager{}, &tmmongo.Manager{}, &tmmongo.Manager{},
		nil, nil,
	)
	require.NoError(t, err, "multi-tenant setup must not error")
	require.NotNil(t, mtSetup, "multi-tenant setup must be non-nil")

	roleInstances := []struct {
		role string
		opt  *pkgHTTP.ProtectedRouteOptions
	}{
		{"onboarding", mtSetup.onboardingRouteOptions},
		{"transaction", mtSetup.transactionRouteOptions},
		{"ledger", mtSetup.ledgerRouteOptions},
		{"crm", mtSetup.crmRouteOptions},
		{"fees", mtSetup.feesRouteOptions},
		{"composition", mtSetup.compositionRouteOptions},
		{"holder-accounts", mtSetup.holderAccountsRouteOptions},
	}

	for _, ri := range roleInstances {
		require.NotNilf(t, ri.opt, "%s route options must be non-nil in multi-tenant mode", ri.role)
	}

	for i := 0; i < len(roleInstances); i++ {
		for j := i + 1; j < len(roleInstances); j++ {
			assert.NotSamef(t, roleInstances[i].opt, roleInstances[j].opt,
				"%s and %s route options must be distinct instances: a shared instance would let a swap go unnoticed",
				roleInstances[i].role, roleInstances[j].role)
		}
	}

	deps := buildHumaMountDepsWithNilHandlers(mtSetup)

	assert.Samef(t, mtSetup.onboardingRouteOptions, deps.OnboardingOptions,
		"onboarding option must bind to the onboarding route setup")
	assert.Samef(t, mtSetup.transactionRouteOptions, deps.TransactionOptions,
		"transaction option must bind to the transaction route setup")
	assert.Samef(t, mtSetup.ledgerRouteOptions, deps.LedgerOptions,
		"ledger option must bind to the ledger route setup")
	assert.Samef(t, mtSetup.crmRouteOptions, deps.CRMOptions,
		"CRM option must bind to the crm route setup, not the fees one: the crm and fees middlewares write the generic "+
			"tenant key over different Mongo managers, so this swap resolves CRM holder PII against the fees tenant database")
	assert.Samef(t, mtSetup.feesRouteOptions, deps.FeesOptions,
		"Fees option must bind to the fees route setup, not the crm one: swapping it against crm resolves fee lookups "+
			"against the CRM tenant database")
	assert.Samef(t, mtSetup.compositionRouteOptions, deps.CompositionOptions,
		"composition option must bind to the composition route setup")
	assert.Samef(t, mtSetup.holderAccountsRouteOptions, deps.HolderAccountsOptions,
		"holder-accounts option must bind to the holder-accounts route setup, not the crm one: the crm middleware binds "+
			"the CRM Mongo on the generic key and no onboarding PG, so this swap fails the listing's account read")
}

// buildHumaMountDepsWithNilHandlers exercises the mapper with the setup under test and nil
// handlers: the binding this test pins is the option pairing, not the handler wiring, and nil
// pointers make the call site read as one argument list of options rather than 24 of noise.
func buildHumaMountDepsWithNilHandlers(setup *unifiedRouteSetup) httpin.HumaMountDeps {
	return buildHumaMountDeps(
		&middleware.AuthClient{},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil,
		nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil,
		nil,
		setup,
	)
}

// updateRouteRoles rewrites the committed route-roles golden instead of asserting against it:
// `go test ./components/ledger/internal/bootstrap/ -run TestRouteRoles -update-route-roles`.
// It carries its own spelling for the same reason updateRouteTable does: a shared -update would
// couple this golden to an unrelated regeneration the moment either invocation widened scope.
var updateRouteRoles = flag.Bool("update-route-roles", false, "rewrite the committed route-roles golden")

// routeRolesGoldenPath is the committed serialization of route -> role over the whole registered
// surface: one line per group, carrying method, raw path and the role whose sentinel ran.
const routeRolesGoldenPath = "testdata/route_roles.golden"

// routeRolesGoldenHeader prefixes the golden so a reader knows what the third column means and how
// to regenerate. It is part of the compared bytes, so it cannot drift from the rows it describes.
const routeRolesGoldenHeader = `# Route -> role map: METHOD<TAB>RAW PATH<TAB>ROLE.
# ROLE is which of the seven route-scoped ProtectedRouteOptions a registered route runs, observed by
# threading a distinct sentinel post-auth handler per role and recording which one executed.
# It pins the registrar -> options pairing the route table cannot: swapping the crm and fees
# options moves neither path nor handler count, but flips the role recorded here.
#
# What this map does NOT pin: the authorize tuple (namespace/resource/action). Swapping namespace,
# resource or action moves neither path, count nor role, and no gate in this repository covers it.
# Generated — do not hand-edit. Regenerate with:
#   go test ./components/ledger/internal/bootstrap/ -run TestRouteRoles -update-route-roles
`

// crmPathSegments and feePathSegments name the raw-path segments unique to the crm and fee
// surfaces. They anchor the swap-of-consequence assertion independently of the golden, so a
// regeneration cannot bake a crm<->fees swap into the committed bytes.
var (
	crmPathSegments = map[string]bool{"holders": true, "instruments": true, "encryption": true, "protection": true}
	feePathSegments = map[string]bool{"packages": true, "billing-packages": true, "estimates": true, "billing": true}
)

func pathHasSegment(rawPath string, set map[string]bool) bool {
	for _, seg := range strings.Split(rawPath, "/") {
		if set[seg] {
			return true
		}
	}

	return false
}

// TestRouteRoles mounts the full registered surface with one distinct sentinel per role, then
// sweeps every route and records which role's sentinel ran. It pins the resulting route -> role
// map to a committed golden, guarded by probe-health assertions so an empty or swapped probe
// cannot regenerate clean.
func TestRouteRoles(t *testing.T) {
	// The docs surface is gated on OPENAPI_DOCS_ENABLED and this test asserts the default
	// deployment posture, so the variable must be genuinely absent. unsetDocsGate uses
	// t.Setenv, which precludes t.Parallel here.
	unsetDocsGate(t)

	observed, groups := probeRouteRoles(t)

	// Probe-health assertions run BEFORE the byte comparison and before -update-route-roles can
	// write, so a broken or empty probe cannot be baked into the golden.

	// 1. Every non-carve-out group ran a sentinel.
	for _, group := range groups {
		if unguardedPublicRoutes[group.key] {
			continue
		}

		_, ok := observed[group.key]
		assert.Truef(t, ok, "%s ran no role sentinel: it is mounted outside all seven route-scoped options, or its chain "+
			"never reached the post-auth handler", group.display())
	}

	// 2. All seven roles appear at least once, so no role's registrars silently vanished.
	rolesSeen := make(map[string]bool, 7)
	for _, role := range observed {
		rolesSeen[role] = true
	}

	for _, role := range []string{"onboarding", "ledger", "transaction", "crm", "fees", "composition", "holder-accounts"} {
		assert.Truef(t, rolesSeen[role], "no route ran the %q sentinel: that role's registrars are missing from the surface", role)
	}

	// 3. The swap of consequence, stated explicitly: no crm route runs the fees role and no fees
	// route runs the crm role. A crm<->fees option swap trips exactly this.
	for key, role := range observed {
		rawPath := strings.SplitN(key, "\t", 2)[1]

		if pathHasSegment(rawPath, crmPathSegments) {
			assert.NotEqualf(t, "fees", role, "%s is a CRM route running the fees role: the crm and fees route options are "+
				"swapped, so CRM holder PII resolves against the fees tenant database", strings.ReplaceAll(key, "\t", " "))
		}

		if pathHasSegment(rawPath, feePathSegments) {
			assert.NotEqualf(t, "crm", role, "%s is a fee route running the crm role: the crm and fees route options are "+
				"swapped", strings.ReplaceAll(key, "\t", " "))
		}
	}

	// 4. Observed count equals the guarded surface: every group minus the pinned public carve-outs.
	assert.Equalf(t, len(groups)-unguardedPublicRouteCount, len(observed),
		"%d routes ran a role sentinel, expected %d (%d groups minus %d public carve-outs)",
		len(observed), len(groups)-unguardedPublicRouteCount, len(groups), unguardedPublicRouteCount)

	got := serializeRouteRoles(observed)

	if *updateRouteRoles {
		// The probe-health assertions above are assert-based, so a failure marks the test and
		// execution reaches here. Writing then bakes a broken probe into the golden.
		if t.Failed() {
			return
		}

		require.NoError(t, os.MkdirAll(filepath.Dir(routeRolesGoldenPath), 0o755),
			"create golden directory for %s", routeRolesGoldenPath)
		require.NoError(t, os.WriteFile(routeRolesGoldenPath, []byte(got), 0o644),
			"write golden route roles %s", routeRolesGoldenPath)
		t.Logf("wrote golden route roles %s (%d routes)", routeRolesGoldenPath, len(observed))

		return
	}

	want, err := os.ReadFile(routeRolesGoldenPath)
	require.NoErrorf(t, err, "read golden route roles %s (run with -update-route-roles to generate)", routeRolesGoldenPath)
	require.Equalf(t, string(want), got,
		"route -> role map drifted from %s: a registrar received a different route-scoped option. The map does not pin the "+
			"authorize tuple, so an unmoved map is not evidence the namespace/resource/action is unchanged either",
		routeRolesGoldenPath)

	t.Logf("route roles match golden: %d routes", len(observed))
}

// probeRouteRoles mounts the full surface with seven distinct sentinel options and returns the
// observed route -> role map plus the route groups. Auth is DISABLED so the authorizer passes and
// the sentinel is the handler that answers; each sentinel records its role and short-circuits with
// 204 so no terminal runs.
func probeRouteRoles(t *testing.T) (map[string]string, []routeGroup) {
	t.Helper()

	var (
		mu       sync.Mutex
		lastRole string
	)

	sentinel := func(role string) *pkgHTTP.ProtectedRouteOptions {
		handler := func(c fiber.Ctx) error {
			mu.Lock()
			lastRole = role
			mu.Unlock()

			return c.SendStatus(fiber.StatusNoContent)
		}

		return &pkgHTTP.ProtectedRouteOptions{PostAuthMiddlewares: []fiber.Handler{handler}}
	}

	setup := &unifiedRouteSetup{
		onboardingRouteOptions:  sentinel("onboarding"),
		transactionRouteOptions: sentinel("transaction"),
		ledgerRouteOptions:      sentinel("ledger"),
		crmRouteOptions:         sentinel("crm"),
		feesRouteOptions:        sentinel("fees"),
		compositionRouteOptions: sentinel("composition"),

		holderAccountsRouteOptions: sentinel("holder-accounts"),
	}

	logger := newTestLogger()
	telemetry := &libOpentelemetry.Telemetry{}
	auth := &middleware.AuthClient{Enabled: false}

	// Handlers are non-nil zero-value structs so every route mounts, including the conditional
	// CRM holder-accounts/encryption/audit routes. Threading the options through buildHumaMountDeps
	// mounts through the SAME mapper production uses.
	humaDeps := buildHumaMountDeps(
		auth,
		&httpin.OrganizationHandler{}, &httpin.LedgerHandler{}, &httpin.PortfolioHandler{}, &httpin.SegmentHandler{},
		&httpin.AccountHandler{}, &httpin.AccountExceptionHandler{}, &httpin.AccountTypeHandler{}, &httpin.MetadataIndexHandler{}, &httpin.AssetHandler{},
		&httpin.AssetRateHandler{},
		&httpin.BalanceHandler{}, &httpin.OperationHandler{}, &httpin.OperationRouteHandler{}, &httpin.TransactionRouteHandler{},
		&httpin.TransactionHandler{},
		&httpin.HolderHandler{}, &httpin.InstrumentHandler{}, &httpin.HolderAccountsHandler{}, &httpin.EncryptionHandler{},
		&httpin.AuditHandler{},
		&httpin.PackageHandler{}, &httpin.FeeHandler{}, &httpin.BillingPackageHandler{}, &httpin.BillingCalculateHandler{},
		&httpin.CompositionHandler{},
		setup,
	)

	readyzHandler := NewReadyzHandler(ReadyzHandlerConfig{Logger: logger, Version: "test-version"})

	// The role map is scoped to the versioned Huma groups, so no RouteRegistrar is passed here.
	// The app-root streaming manifest route does carry onboardingRouteOptions in production,
	// so this harness leaves that binding unpinned.
	server := NewUnifiedServer(":0", "test-version", logger, telemetry, readyzHandler,
		humaDeps.MountV1, humaDeps.MountV2)
	require.NotNil(t, server, "NewUnifiedServer should return a non-nil server")
	require.NotNil(t, server.app, "server should hold a Fiber app")

	rows := collectRouteRows(t, server.app)
	require.GreaterOrEqualf(t, len(rows), routeTableMinRows,
		"route table holds %d rows, under the %d floor: the harness is not mounting the full surface, so the role map "+
			"would pin nothing", len(rows), routeTableMinRows)

	groups := groupRouteRows(rows)

	// Reuse the same attribution guard the tokenless sweep uses: a probe URL that matched two raw
	// paths would attribute a role to the wrong route.
	requireUnambiguousProbeURLs(t, groups)

	observed := make(map[string]string, len(groups))

	for _, group := range groups {
		if unguardedPublicRoutes[group.key] {
			continue
		}

		mu.Lock()
		lastRole = ""
		mu.Unlock()

		req := httptest.NewRequest(group.rows[0].method, concreteRouteURL(group.rows[0].path), nil)

		resp, err := server.app.Test(req)
		require.NoError(t, err)

		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		mu.Lock()
		role := lastRole
		mu.Unlock()

		if role != "" {
			observed[group.key] = role
		}
	}

	return observed, groups
}

// serializeRouteRoles renders the observed map as the golden's "METHOD\tpath\trole" lines under
// the golden header, sorted by key so the bytes are stable.
func serializeRouteRoles(observed map[string]string) string {
	lines := make([]string, 0, len(observed))
	for key, role := range observed {
		lines = append(lines, key+"\t"+role)
	}

	sort.Strings(lines)

	return routeRolesGoldenHeader + strings.Join(lines, "\n") + "\n"
}
