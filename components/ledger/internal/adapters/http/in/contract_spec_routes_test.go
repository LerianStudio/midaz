// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildUnifiedHumaAPI composes the exact registrar set the unified ledger server
// mounts and returns BOTH the Fiber app and the shared huma.API (the latter feeds
// the offline OpenAPI 3.1 spec dump — see openapi_spec_dump_test.go). Registration
// never invokes the handlers, so nil-backed handler structs are safe; this mirrors
// the registrar-composition pattern in fees_routes_test.go / routes_test.go. Routes
// that mount only when their handler is non-nil — the composition GET holder-accounts
// route, and the encryption/audit routes (envelope mode only) — get non-nil
// zero-value handlers so the full surface matches the served contract.
func buildUnifiedHumaAPI() (*fiber.App, huma.API) {
	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	// Mirror the unified server's public infra routes (unified-server.go).
	app.Get("/health", func(c fiber.Ctx) error { return nil })
	app.Get("/version", func(c fiber.Ctx) error { return nil })
	app.Get("/readyz", func(c fiber.Ctx) error { return nil })

	RegisterMetadataRoutesToApp(app, auth, &MetadataIndexHandler{}, nil)
	RegisterOnboardingRoutesToApp(app, auth,
		&AccountHandler{}, &PortfolioHandler{}, &LedgerHandler{},
		&OrganizationHandler{}, &SegmentHandler{}, &AccountTypeHandler{}, nil)

	// Wave-1 Huma-migrated resources (organization, ledger, portfolio, segment,
	// account, account-type, metadata-index, asset, asset-rate) are mounted via the
	// same /v1 group + shared Huma API the unified server's humaMount uses. This block
	// mirrors the production humaMount closure in config.go.
	libProblem.Install()
	apiV1 := app.Group("/v1")
	humaAPI := openapi.New(app, apiV1, openapi.Config{Title: "Midaz Ledger API", Version: "4.0.0", Servers: []string{"/v1"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)
	RegisterOrganizationRoutesToApp(apiV1, humaAPI, auth, &OrganizationHandler{}, nil)
	RegisterLedgerRoutesToApp(apiV1, humaAPI, auth, &LedgerHandler{}, nil)
	RegisterPortfolioRoutesToApp(apiV1, humaAPI, auth, &PortfolioHandler{}, nil)
	RegisterSegmentRoutesToApp(apiV1, humaAPI, auth, &SegmentHandler{}, nil)
	RegisterAccountRoutesToApp(apiV1, humaAPI, auth, &AccountHandler{}, nil)
	RegisterAccountTypeRoutesToApp(apiV1, humaAPI, auth, &AccountTypeHandler{}, nil)
	RegisterMetadataIndexRoutesToApp(apiV1, humaAPI, auth, &MetadataIndexHandler{}, nil)
	RegisterAssetRoutesToApp(apiV1, humaAPI, auth, &AssetHandler{}, nil)
	RegisterAssetRateRoutesToApp(apiV1, humaAPI, auth, &AssetRateHandler{}, nil)

	// Wave-2 Huma-migrated resources (balance, operation-read, transaction-count,
	// operation-route, transaction-route) are mounted via the same /v1 group + shared
	// Huma API the unified server's humaMount uses. The operation PATCH (UpdateOperation,
	// Wave-4 money-write leg) is ALSO mounted by RegisterOperationRoutesToApp.
	RegisterBalanceRoutesToApp(apiV1, humaAPI, auth, &BalanceHandler{}, nil)
	RegisterOperationRoutesToApp(apiV1, humaAPI, auth, &OperationHandler{}, nil)
	RegisterCountTransactionRoutesToApp(apiV1, humaAPI, auth, &TransactionHandler{}, nil)
	RegisterOperationRouteRoutesToApp(apiV1, humaAPI, auth, &OperationRouteHandler{}, nil)
	RegisterTransactionRouteRoutesToApp(apiV1, humaAPI, auth, &TransactionRouteHandler{}, nil)

	// Wave-4 (MONEY-WRITE) Huma-migrated transaction ops (json/inflow/outflow/annotation/
	// block/unblock CREATE, commit/cancel/revert STATE, PATCH update, GET-by-id + list) are
	// mounted via the same /v1 group + shared Huma API the unified server's humaMount uses.
	RegisterTransactionHumaRoutesToApp(apiV1, humaAPI, auth, &TransactionHandler{}, nil)

	// Wave-3 (additive) Huma-migrated resources (CRM holders/instruments/holder-
	// accounts/encryption/audit, fees/billing, composition) are mounted via the same
	// /v1 group + shared Huma API the unified server's humaMount uses. The conditional
	// CRM handlers (holder-accounts, encryption, audit) get non-nil zero-value handlers
	// so the FULL surface is registered.
	RegisterCRMRoutesToApp(apiV1, humaAPI, auth,
		&HolderHandler{}, &InstrumentHandler{}, &HolderAccountsHandler{},
		&EncryptionHandler{}, &AuditHandler{}, nil)
	RegisterFeesRoutesToApp(apiV1, humaAPI, auth,
		&PackageHandler{}, &FeeHandler{}, &BillingPackageHandler{}, &BillingCalculateHandler{}, nil)
	RegisterCompositionRoutesToApp(apiV1, humaAPI, auth, &CompositionHandler{}, nil)

	return app, humaAPI
}

// buildUnifiedHumaAPIV2 composes the SECOND, INDEPENDENT Huma contract the unified
// ledger server mounts under /v2, returning BOTH the Fiber app and that v2 huma.API
// (the latter feeds the offline v2 OpenAPI 3.1 spec dump — see
// openapi_spec_dump_test.go). It mirrors the production humaMountV2 closure in
// config.go and the mountHumaContract scaffolding in unified-server.go: its own Fiber
// group, its own Huma document (own component registry, Servers ["/v2"]), and the v2
// registrars. Registration never invokes the handlers, so nil-backed handler structs
// are safe. As in buildUnifiedHumaAPI, the conditional CRM handlers (holder-accounts,
// encryption, audit) get non-nil zero-value handlers so the FULL surface matches the
// served contract.
func buildUnifiedHumaAPIV2() (*fiber.App, huma.API) {
	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	libProblem.Install()
	apiV2 := app.Group("/v2")
	humaAPIV2 := openapi.New(app, apiV2, openapi.Config{
		Title:       "Midaz Ledger API v2",
		Version:     "4.0.0",
		Description: "Midaz Ledger v2 API contract.",
		Servers:     []string{"/v2"},
	})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPIV2)

	RegisterTransactionV2RoutesToApp(apiV2, humaAPIV2, auth, &TransactionHandler{}, nil)
	RegisterCRMV2RoutesToApp(apiV2, humaAPIV2, auth,
		&HolderHandler{}, &InstrumentHandler{}, &HolderAccountsHandler{},
		&EncryptionHandler{}, &AuditHandler{}, nil)

	return app, humaAPIV2
}

// specPath is the committed, generated Huma OAS 3.1 dump for the ledger rail. It
// is the single source of truth the mounted route surface must match. The swaggo
// swagger.json this gate originally read against was deleted in the Huma migration
// (Epic 4.1); this dump — regenerated by TestOpenAPISpecDump, never hand-edited —
// replaces it.
const specPath = "../../../../api/openapi.huma.yaml"

// specServerPrefix is the base path the Huma spec carries in its `servers` block
// (openapi.New(..., Servers: []string{"/v1"})) rather than baked into each path
// key. The dump's `.paths` are therefore server-relative ("/organizations"), while
// Fiber's app.GetRoutes() reports the fully-mounted path ("/v1/organizations",
// mounted under app.Group("/v1")). collectSpecRoutes prepends this prefix so both
// surfaces compare on the same fully-qualified path.
const specServerPrefix = "/v1"

// specPathV2 is the committed, generated Huma OAS 3.1 dump for the SECOND,
// independent /v2 contract — the source of truth the /v2 mounted route surface must
// match, exactly as specPath is for /v1.
const specPathV2 = "../../../../api/openapi.v2.huma.yaml"

// specServerPrefixV2 is the /v2 counterpart of specServerPrefix: the v2 dump also
// carries its base path in `servers` and keeps its `.paths` server-relative, so the
// prefix is prepended the same way.
const specServerPrefixV2 = "/v2"

// excludedPaths is the LOCKED set of routes the unified server mounts under /v1 but
// that are intentionally absent from the v1 Huma contract, so the diff gate drops
// them from the mounted side before comparison. The list is a const so it cannot
// grow silently: adding a route here is a reviewed, deliberate carve-out, never a
// convenient way to hide a genuine served-vs-mounted divergence. Each entry is the
// CANONICALIZED path (positional "{}" params — see pathParam) and must be justified
// inline.
//
// It is passed explicitly to collectMountedRoutes rather than read globally, so each
// contract carries its own carve-outs and a /v1 exclusion can never suppress a /v2
// finding.
//
// No /swagger* entries: the swaggo UI + spec routes were retired with the migration.
var excludedPaths = map[string]bool{
	// Public operational probes, registered outside the authz'd API surface and
	// deliberately undocumented in the Huma contract (unified-server.go).
	canonicalizePath("/health"):  true,
	canonicalizePath("/version"): true,
	canonicalizePath("/readyz"):  true,
}

// excludedPathsV2 is the /v2 counterpart of excludedPaths, and is EMPTY: the /v2
// contract mounts no route that the v2 Huma dump omits. The public probes carved out
// of /v1 are mounted on the unified server outside any versioned group, so they are
// not part of the /v2 surface and must not be carved out here — an unjustified entry
// would blind the gate.
var excludedPathsV2 = map[string]bool{}

// pathParam matches a single path-parameter segment in EITHER syntax: Fiber
// ":name" or OpenAPI "{name}". Both are collapsed to a positional "{}" token by
// canonicalizePath so the two surfaces compare on path STRUCTURE, not on the
// parameter LABEL.
//
// This is deliberate and load-bearing. An OpenAPI path template's identity is its
// sequence of literal and parameter segments; the parameter NAME is documentation
// metadata, not part of the path's identity ("/x/{id}" and "/x/{account_id}"
// address the same endpoint). The Fiber routes use generic labels (":id") while the
// Huma operations use semantic ones ("{account_id}"); comparing on the label would
// flag divergences that are the SAME route, and would pressure the published
// contract toward worse (generic) parameter docs purely to satisfy a test.
// Canonicalizing positions keeps the gate strict on what matters — a route added,
// removed, re-segmented, or method-changed still trips it, because that alters
// structure or method, never just a label.
var pathParam = regexp.MustCompile(`(:[^/]+|\{[^/]+\})`)

// canonicalizePath collapses every path-parameter segment (Fiber ":name" or
// OpenAPI "{name}") to a positional "{}" token, yielding a label-independent
// path-structure key shared by both the mounted and the served surface.
func canonicalizePath(p string) string {
	return pathParam.ReplaceAllString(p, "{}")
}

// collectMountedRoutes returns the normalized "METHOD {path}" set the given app
// mounts, minus the caller's locked exclusions and Fiber's auto-registered HEAD
// twins. Fiber registers a HEAD route for every GET; those have no OpenAPI
// counterpart, so a HEAD whose path also has a GET is dropped. Explicit HEAD routes
// (the v1 metrics/count endpoints) have NO sibling GET on the same path, so they
// survive and are compared against the dump's `head` operations.
//
// excluded is per-contract: passing one contract's carve-outs never affects another.
func collectMountedRoutes(app *fiber.App, excluded map[string]bool) map[string]bool {
	getPaths := make(map[string]bool)

	for _, r := range app.GetRoutes() {
		if r.Method == fiber.MethodGet {
			getPaths[canonicalizePath(r.Path)] = true
		}
	}

	mounted := make(map[string]bool)

	for _, r := range app.GetRoutes() {
		path := canonicalizePath(r.Path)

		if excluded[path] {
			continue
		}

		// Drop Fiber's auto HEAD twin (a HEAD that shadows a GET on the same path).
		if r.Method == fiber.MethodHead && getPaths[path] {
			continue
		}

		mounted[r.Method+" "+path] = true
	}

	return mounted
}

// collectSpecRoutes parses the committed Huma OAS 3.1 dump at path into the same
// "METHOD {path}" set, prepending serverPrefix to each server-relative path key (see
// specServerPrefix) and upper-casing the OpenAPI operation verbs to match Fiber's
// method constants.
func collectSpecRoutes(t *testing.T, path, serverPrefix string) map[string]bool {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err, "Huma dump must be readable at %s", path)

	var spec struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}

	require.NoErrorf(t, yaml.Unmarshal(raw, &spec), "Huma dump %s must parse as YAML", path)
	require.NotEmptyf(t, spec.Paths, "Huma dump %s must declare paths", path)

	specSet := make(map[string]bool)

	for pathKey, ops := range spec.Paths {
		for verb := range ops {
			specSet[strings.ToUpper(verb)+" "+canonicalizePath(serverPrefix+pathKey)] = true
		}
	}

	return specSet
}

// assertContractMatchesRoutes fails when mounted and spec are not the same set, in
// BOTH directions, naming every route on either side of the difference. contract
// labels which dump is under test.
func assertContractMatchesRoutes(t *testing.T, contract string, mounted, spec map[string]bool) {
	t.Helper()

	var mountedNotInSpec, specNotMounted []string

	for r := range mounted {
		if !spec[r] {
			mountedNotInSpec = append(mountedNotInSpec, r)
		}
	}

	for r := range spec {
		if !mounted[r] {
			specNotMounted = append(specNotMounted, r)
		}
	}

	sort.Strings(mountedNotInSpec)
	sort.Strings(specNotMounted)

	if len(mountedNotInSpec) > 0 || len(specNotMounted) > 0 {
		t.Errorf("Huma dump %s and mounted routes diverged\n\n"+
			"MOUNTED but NOT in spec (%d):\n  %s\n\n"+
			"in SPEC but NOT mounted (%d):\n  %s\n\n"+
			"mounted total=%d  spec total=%d",
			contract,
			len(mountedNotInSpec), strings.Join(mountedNotInSpec, "\n  "),
			len(specNotMounted), strings.Join(specNotMounted, "\n  "),
			len(mounted), len(spec))
	}
}

// TestContractSpecMatchesRoutes is the DC-3 spec-vs-routes diff gate: the routes the
// unified ledger binary mounts (normalized, minus the locked exclusions) must be
// exactly the set of paths+methods the generated Huma dump enumerates — in BOTH
// directions. A failure means served and mounted have diverged: either a route was
// added/removed without regenerating the dump, or the dump drifted from the mount.
// Do not weaken this assertion; reconcile the source of the mismatch.
func TestContractSpecMatchesRoutes(t *testing.T) {
	t.Parallel()

	app, _ := buildUnifiedHumaAPI()

	assertContractMatchesRoutes(t, specPath,
		collectMountedRoutes(app, excludedPaths),
		collectSpecRoutes(t, specPath, specServerPrefix))
}

// TestContractSpecMatchesRoutesV2 is the same bidirectional diff gate over the /v2
// contract: the routes the unified ledger binary mounts under /v2 must be exactly the
// paths+methods the generated v2 Huma dump enumerates. A failure means served and
// mounted have diverged: either a route was added/removed without regenerating the
// dump, or the dump drifted from the mount. This is a distinct guarantee from the v2
// golden dump (TestOpenAPISpecDumpV2), which compares the serialized document against
// the Huma registrations and never reads the Fiber router.
//
// The comparison is over paths and methods only. It does not inspect the middleware
// chain on a mounted route, so it cannot by itself prove a route is authenticated.
// Do not weaken this assertion; reconcile the source of the mismatch.
func TestContractSpecMatchesRoutesV2(t *testing.T) {
	t.Parallel()

	app, _ := buildUnifiedHumaAPIV2()

	assertContractMatchesRoutes(t, specPathV2,
		collectMountedRoutes(app, excludedPathsV2),
		collectSpecRoutes(t, specPathV2, specServerPrefixV2))
}
