// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"io"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	tmmongo "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/postgres"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/gofiber/fiber/v3"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// attestationTenantID and attestationTenantSlug are the identity the probe token carries.
// Fixed rather than generated for the same reason probeUUID is: a value that varied between
// runs would vary nothing the assertions read.
var (
	attestationTenantID   = uuid.MustParse("6b1f7d24-2f3a-4c0e-9a11-8d5e4c3b2a10")
	attestationTenantSlug = "route-guard-tenant"
)

// tenantAttestationJWT builds an unsigned-but-parseable token carrying the claims
// pkgHTTP.MarkTrustedAuthAssertion reads via ParseUnverified: a UUID tenantId, the
// tenantSlug that becomes the attestation's display name, and a sub so the assertion
// does not fall back to its trusted-upstream sentinel.
//
// The signature is never verified on this path — MarkTrustedAuthAssertion runs BEHIND an
// auth middleware that has already validated the credential, and parses unverified by
// design — so the "none" algorithm is the honest spelling of a token this harness never
// asks anyone to trust.
func tenantAttestationJWT(t *testing.T) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub":        "route-guard-user",
		"tenantId":   attestationTenantID.String(),
		"tenantSlug": attestationTenantSlug,
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err, "failed to build the attestation probe token")

	return signed
}

// attestationObserver records the authenticated tenant present on the request context once
// the chain behind it has unwound. It is the harness stand-in for lib-observability's
// telemetry middleware, which reads the attestation at exactly this moment: AFTER c.Next()
// returns, because MarkTrustedAuthAssertion writes it into the request context mid-chain
// via c.SetContext.
//
// Reading before c.Next() would observe the context as it entered the chain and could never
// see an attestation, whoever wrote one.
type attestationObserver struct {
	mu     sync.Mutex
	ran    bool
	found  bool
	tenant libObservability.AuthenticatedTenant
}

// reset clears the record between probes so a route that attested nothing cannot inherit the
// previous route's observation. Valid only while the probe loop stays sequential.
func (o *attestationObserver) reset() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.ran = false
	o.found = false
	o.tenant = libObservability.AuthenticatedTenant{}
}

func (o *attestationObserver) snapshot() (ran, found bool, tenant libObservability.AuthenticatedTenant) {
	o.mu.Lock()
	defer o.mu.Unlock()

	return o.ran, o.found, o.tenant
}

// handler returns the observing middleware. It never short-circuits: the chain behind it must
// run in full, because the attestation it reads is written by a handler on that chain.
//
// The read sits in a DEFER rather than after the c.Next() call. Some terminals in this harness
// panic on their zero-value handler struct, and the app-level WithRecover boundary sits OUTSIDE
// this middleware, so a straight post-Next read would be skipped entirely on those routes
// and report "the chain never came back" where the honest answer is "here is what the context
// held when it left". The defer records on both the returning and the unwinding path.
func (o *attestationObserver) handler() fiber.Handler {
	return func(c fiber.Ctx) error {
		defer func() {
			tenant, found := libObservability.AuthenticatedTenantFromContext(c.Context())

			o.mu.Lock()
			o.ran = true
			o.found = found
			o.tenant = tenant
			o.mu.Unlock()
		}()

		return c.Next()
	}
}

// prependAttestationObserver rewrites every one of the seven route-scoped option sets so the
// observer runs ahead of the handlers the production builder put there, keeping those handlers
// in their production order behind it.
//
// It PREPENDS rather than replaces on purpose: replacing the list would delete the very
// MarkTrustedAuthAssertion whose reachability is under test, and the guard would then prove
// only that the harness can write its own context.
//
// It edits copies, never the production slices, so the setup's own instances stay intact for
// the pointer-identity assertions TestRouteOptionsBinding makes over the same builder.
func prependAttestationObserver(setup *unifiedRouteSetup, observer fiber.Handler) {
	wrap := func(opt *pkgHTTP.ProtectedRouteOptions) *pkgHTTP.ProtectedRouteOptions {
		if opt == nil {
			return &pkgHTTP.ProtectedRouteOptions{PostAuthMiddlewares: []fiber.Handler{observer}}
		}

		chain := make([]fiber.Handler, 0, len(opt.PostAuthMiddlewares)+1)
		chain = append(chain, observer)
		chain = append(chain, opt.PostAuthMiddlewares...)

		return &pkgHTTP.ProtectedRouteOptions{PostAuthMiddlewares: chain}
	}

	setup.onboardingRouteOptions = wrap(setup.onboardingRouteOptions)
	setup.transactionRouteOptions = wrap(setup.transactionRouteOptions)
	setup.ledgerRouteOptions = wrap(setup.ledgerRouteOptions)
	setup.crmRouteOptions = wrap(setup.crmRouteOptions)
	setup.feesRouteOptions = wrap(setup.feesRouteOptions)
	setup.compositionRouteOptions = wrap(setup.compositionRouteOptions)
	setup.holderAccountsRouteOptions = wrap(setup.holderAccountsRouteOptions)
}

// buildTenantAttestationServer mounts the full registered surface with the MULTI-TENANT route
// options the production builder produces, each fronted by the observer.
//
// The options come from buildUnifiedRouteSetup(&Config{MultiTenantEnabled: true}, ...) — the
// same call bootstrap makes — so the post-auth chains under probe are the production chains,
// MarkTrustedAuthAssertion included. Single-tenant mode would prove nothing here: that branch
// short-circuits to a zero-value setup whose seven options are nil, so no assertion middleware
// exists to be reachable.
//
// The tenant managers are ZERO-VALUE structs and the cache/loader are nil. buildUnifiedRouteSetup
// only requires them non-nil and stores them in middleware options; nothing dials. The tenant
// middleware behind the assertion therefore fails its own DB resolution and answers 4xx/5xx,
// which is irrelevant to this guard: the attestation is written ahead of that resolution and the
// observer reads it on the way out regardless of the status.
//
// Auth is DISABLED so the authorizer at chain position 0 passes the probe token through. That is
// the same trade TestRouteRoles makes, and it removes only the authorizer — never the post-auth
// middlewares, which is where the handler under test lives. Refusal by that authorizer is the
// separate property TestFullSurfaceRoutes_RejectTokenlessRequests pins with auth ENABLED.
func buildTenantAttestationServer(t *testing.T, observer fiber.Handler) *UnifiedServer {
	t.Helper()

	logger := newTestLogger()

	setup, err := buildUnifiedRouteSetup(
		&Config{MultiTenantEnabled: true}, logger,
		&tmpostgres.Manager{}, &tmpostgres.Manager{},
		&tmmongo.Manager{}, &tmmongo.Manager{}, &tmmongo.Manager{}, &tmmongo.Manager{},
		nil, nil,
	)
	require.NoError(t, err, "multi-tenant route setup must build")
	require.NotNil(t, setup, "multi-tenant route setup must be non-nil")

	prependAttestationObserver(setup, observer)

	auth := &middleware.AuthClient{Enabled: false}

	humaDeps := fullSurfaceHumaDeps(auth, setup)

	// The app-root streaming manifest route carries the onboarding options in production, so it
	// is mounted here too: leaving it off would drop a guarded route from the sweep while the
	// completeness count, being taken from this same server, stayed green.
	manifestHandler, manifestErr := BuildStreamingManifestHandler(&Config{})
	require.NoError(t, manifestErr, "streaming manifest handler must build for the attestation sweep")

	streamingManifestRegistrar := func(router fiber.Router) {
		httpin.RegisterStreamingManifestRouteToApp(router, auth, setup.onboardingRouteOptions, manifestHandler)
	}

	readyzHandler := NewReadyzHandler(ReadyzHandlerConfig{Logger: logger, Version: "test-version"})

	server := NewUnifiedServer(":0", "test-version", logger, &libOpentelemetry.Telemetry{}, readyzHandler,
		humaDeps.MountV1, humaDeps.MountV2, streamingManifestRegistrar)
	require.NotNil(t, server, "NewUnifiedServer should return a non-nil server")
	require.NotNil(t, server.app, "server should hold a Fiber app")

	return server
}

// TestFullSurfaceRoutes_AttestTenantIdentity asserts REACHABILITY of the tenant attestation over
// the whole registered surface: with the MULTI-TENANT route options production builds and a token
// carrying a UUID tenantId plus a tenantSlug, every route except the pinned public carve-outs must
// leave libObservability.AuthenticatedTenantFromContext answering with exactly that tenant by the
// time the chain has unwound.
//
// It is the complement of TestFullSurfaceRoutes_RejectTokenlessRequests, which states outright
// that it proves refusal and never reachability. Nothing else in this repository pins that
// pkgHTTP.MarkTrustedAuthAssertion is actually ON every protected chain: a registrar mounted with
// no ProtectedRouteOptions, or an option set built without authAssertion, would produce no
// attestation and therefore no per-tenant HTTP metrics for those routes, with every other gate in
// the package green. Removing authAssertion from any one of the seven option sets in
// buildUnifiedRouteSetup fails this test, naming the routes that lost it.
//
// What it does NOT prove:
//   - that the SINGLE-TENANT deployment attests anything. It does not: buildUnifiedRouteSetup
//     returns nil options there and no assertion middleware is mounted at all. That is the
//     product's current shape, not an oversight this guard can observe.
//   - that the authorizer ahead of the assertion validated the token. Auth is disabled here so
//     the probe reaches the assertion; the refusal property is the other test's.
//   - that the attestation reaches the terminal handler. The tenant middleware behind the
//     assertion cannot resolve a database in this harness, so most chains stop there. What is
//     pinned is that the identity is on the context when the OUTERMOST middleware — where
//     lib-observability's telemetry middleware sits — reads it, which is the only position the
//     per-tenant metrics are recorded from.
//   - equivalence with the production reader's position. The telemetry middleware is registered
//     app-wide BEHIND WithRecover, so a panicking terminal reaches it as a returned error and its
//     plain post-Next read still runs. This observer sits inside the route chain, ahead of the
//     terminal but inside that recovery boundary, so it reads from a defer instead. The identity
//     observed is the same; the unwinding path it survives is an artifact of where it sits.
//   - anything about the metric series themselves. pkg/net/http/tenant_metrics_wiring_test.go
//     pins those against a real meter.
func TestFullSurfaceRoutes_AttestTenantIdentity(t *testing.T) {
	// The docs surface is gated on OPENAPI_DOCS_ENABLED and this test asserts the default
	// deployment posture: the docs routes are not protected chains and would have no
	// attestation to find. unsetDocsGate uses t.Setenv, which precludes t.Parallel here and
	// in the subtests.
	unsetDocsGate(t)

	observer := &attestationObserver{}
	server := buildTenantAttestationServer(t, observer.handler())

	rows := collectRouteRows(t, server.app)
	require.GreaterOrEqualf(t, len(rows), routeTableMinRows,
		"route table holds %d rows, under the %d floor: the harness is not mounting the full surface, so passing here "+
			"would mean nothing", len(rows), routeTableMinRows)

	groups := groupRouteRows(rows)

	// Same attribution guard the tokenless sweep uses: a probe URL matching two raw paths would
	// credit one route's attestation to the other.
	requireUnambiguousProbeURLs(t, groups)

	token := tenantAttestationJWT(t)

	probed := 0

	for _, group := range groups {
		// The public operational probes are mounted outside the authorized surface on purpose
		// and are justified entry by entry in unguardedPublicRoutes. They carry no auth
		// middleware, so they can carry no attestation either.
		if unguardedPublicRoutes[group.key] {
			continue
		}

		probed++

		t.Run(group.display(), func(t *testing.T) {
			// Reset beside the read rather than once per harness, so an observation belongs to
			// the route that produced it. Valid only while these subtests stay sequential: a
			// t.Parallel() here would let siblings overwrite each other's record.
			observer.reset()

			req := httptest.NewRequest(group.rows[0].method, concreteRouteURL(group.rows[0].path), nil)
			req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

			resp, err := server.app.Test(req)
			require.NoError(t, err)

			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()

			ran, found, tenant := observer.snapshot()

			require.Truef(t, ran,
				"the observer never ran on %s: the route is mounted with no ProtectedRouteOptions at all, so nothing "+
					"post-auth executes on it and no tenant identity can be attested", group.display())

			require.Truef(t, found,
				"no authenticated tenant on the context after a request to %s carrying a UUID tenantId claim: "+
					"pkgHTTP.MarkTrustedAuthAssertion is not on this route's post-auth chain, so per-tenant HTTP "+
					"metrics are unlabelled for every request it serves", group.display())

			assert.Equalf(t, attestationTenantID, tenant.ID,
				"%s attested tenant id %s, expected %s", group.display(), tenant.ID, attestationTenantID)

			assert.Equalf(t, attestationTenantSlug, tenant.Name,
				"%s attested tenant name %q, expected %q: the tenantSlug claim is not reaching the attestation",
				group.display(), tenant.Name, attestationTenantSlug)
		})
	}

	// Counted against the PINNED carve-out size, not against len(unguardedPublicRoutes): both
	// sides of that comparison move with the map, so adding a key would subtract one from each
	// and disarm the very skip it authorizes.
	assert.Equalf(t, len(groups)-unguardedPublicRouteCount, probed,
		"%d of %d registered endpoints were probed, leaving more than the %d pinned carve-outs unprobed",
		probed, len(groups), unguardedPublicRouteCount)
}

// fullSurfaceHumaDeps assembles the mount deps for a full-surface probe: the given auth client
// and route setup, with every resource handler a non-nil zero-value struct so every route mounts,
// including the conditional CRM holder-accounts/encryption/audit ones. Route registration stores
// handler funcs and never calls them, so zero values are sufficient.
//
// It threads the setup through buildHumaMountDeps — the SAME mapper production uses — so a
// registrar or an option rebinding added there reaches every probe built on this helper.
func fullSurfaceHumaDeps(auth *middleware.AuthClient, setup *unifiedRouteSetup) httpin.HumaMountDeps {
	return buildHumaMountDeps(
		auth,
		&httpin.OrganizationHandler{}, &httpin.LedgerHandler{}, &httpin.PortfolioHandler{}, &httpin.SegmentHandler{},
		&httpin.AccountHandler{}, &httpin.AccountTypeHandler{}, &httpin.MetadataIndexHandler{}, &httpin.AssetHandler{},
		&httpin.AssetRateHandler{},
		&httpin.BalanceHandler{}, &httpin.OperationHandler{}, &httpin.OperationRouteHandler{}, &httpin.TransactionRouteHandler{},
		&httpin.TransactionHandler{},
		&httpin.HolderHandler{}, &httpin.InstrumentHandler{}, &httpin.HolderAccountsHandler{}, &httpin.EncryptionHandler{},
		&httpin.AuditHandler{},
		&httpin.PackageHandler{}, &httpin.FeeHandler{}, &httpin.BillingPackageHandler{}, &httpin.BillingCalculateHandler{},
		&httpin.CompositionHandler{},
		setup,
	)
}
