// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

// specPath is the committed, served swagger contract. The unified ledger server
// mounts this exact instance over fiberSwagger (unified-server.go), so it is the
// single source of truth the mounted route surface must match.
const specPath = "../../../../api/swagger.json"

// excludedPaths is the LOCKED set of public infra routes registered outside the
// authz'd API surface. They are mounted by the unified server (or NewRouter) but
// are intentionally absent from the OpenAPI contract, so the diff gate drops them
// from the mounted side before comparison. The list is a const so it cannot grow
// silently: adding a route here is a reviewed, deliberate carve-out, never a
// convenient way to hide a genuine served-vs-mounted divergence.
//
// /health, /version, /readyz are public operational probes; /swagger and
// /swagger/* serve the UI + spec. Fiber's auto-registered HEAD twins for GET
// routes are dropped separately (see collectMountedRoutes).
var excludedPaths = map[string]bool{
	"/health":    true,
	"/version":   true,
	"/readyz":    true,
	"/swagger":   true,
	"/swagger/*": true,
}

// pathParam matches a single path-parameter segment in EITHER syntax: Fiber
// ":name" or OpenAPI "{name}". Both are collapsed to a positional "{}" token by
// canonicalizePath so the two surfaces compare on path STRUCTURE, not on the
// parameter LABEL.
//
// This is deliberate and load-bearing. An OpenAPI path template's identity is its
// sequence of literal and parameter segments; the parameter NAME is documentation
// metadata, not part of the path's identity ("/x/{id}" and "/x/{account_id}"
// address the same endpoint). The Fiber routes use generic labels (":id") while
// the swag @Router annotations use semantic ones ("{account_id}"); comparing on
// the label would flag ~18 false divergences that are the SAME route, and would
// pressure the published contract toward worse (generic) parameter docs purely to
// satisfy a test. Canonicalizing positions keeps the gate strict on what matters
// — a route added, removed, re-segmented, or method-changed still trips it,
// because that alters structure or method, never just a label.
var pathParam = regexp.MustCompile(`(:[^/]+|\{[^/]+\})`)

// canonicalizePath collapses every path-parameter segment (Fiber ":name" or
// OpenAPI "{name}") to a positional "{}" token, yielding a label-independent
// path-structure key shared by both the mounted and the served surface.
func canonicalizePath(p string) string {
	return pathParam.ReplaceAllString(p, "{}")
}

// buildUnifiedRouteSurface composes every route registrar the unified ledger
// server mounts (metadata + onboarding + transaction + crm + fees + composition)
// over a single Fiber app with zero-value handlers and a disabled auth client.
// Registration never invokes the handlers, so nil-backed handler structs are
// safe; this mirrors the registrar-composition pattern in fees_routes_test.go /
// routes_test.go. The composition GET holder-accounts route mounts only when the
// HolderAccountsHandler is non-nil, so we pass a non-nil one to match the served
// contract.
func buildUnifiedRouteSurface() *fiber.App {
	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	// Mirror the unified server's public infra routes so the exclusion list is
	// exercised exactly as in production (unified-server.go).
	app.Get("/health", func(c *fiber.Ctx) error { return nil })
	app.Get("/version", func(c *fiber.Ctx) error { return nil })
	app.Get("/readyz", func(c *fiber.Ctx) error { return nil })
	app.Get("/swagger", func(c *fiber.Ctx) error { return nil })
	app.Get("/swagger/*", func(c *fiber.Ctx) error { return nil })

	RegisterMetadataRoutesToApp(app, auth, &MetadataIndexHandler{}, nil)
	RegisterOnboardingRoutesToApp(app, auth,
		&AccountHandler{}, &PortfolioHandler{}, &LedgerHandler{}, &AssetHandler{},
		&OrganizationHandler{}, &SegmentHandler{}, &AccountTypeHandler{}, nil)
	RegisterTransactionRoutesToApp(app, auth,
		&TransactionHandler{}, &OperationHandler{}, &AssetRateHandler{},
		&BalanceHandler{}, &OperationRouteHandler{}, &TransactionRouteHandler{}, nil)
	RegisterCRMRoutesToApp(app, auth,
		&HolderHandler{}, &InstrumentHandler{}, &HolderAccountsHandler{}, nil)
	RegisterFeesRoutesToApp(app, auth,
		&PackageHandler{}, &FeeHandler{}, &BillingPackageHandler{}, &BillingCalculateHandler{}, nil)
	RegisterCompositionRoutesToApp(app, auth, &CompositionHandler{}, nil)

	return app
}

// collectMountedRoutes returns the normalized "METHOD {path}" set the unified app
// mounts, minus the locked public-infra exclusions and Fiber's auto-registered
// HEAD twins. Fiber registers a HEAD route for every GET; those have no
// OpenAPI counterpart, so a HEAD whose path also has a GET is dropped. Explicit
// HEAD routes (the metrics/count endpoints) have NO sibling GET on the same path,
// so they survive and are compared.
func collectMountedRoutes(app *fiber.App) map[string]bool {
	getPaths := make(map[string]bool)

	for _, r := range app.GetRoutes() {
		if r.Method == fiber.MethodGet {
			getPaths[canonicalizePath(r.Path)] = true
		}
	}

	mounted := make(map[string]bool)

	for _, r := range app.GetRoutes() {
		path := canonicalizePath(r.Path)

		if excludedPaths[path] {
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

// collectSpecRoutes parses the committed swagger.json into the same
// "METHOD {path}" set, upper-casing the OpenAPI operation verbs to match Fiber's
// method constants.
func collectSpecRoutes(t *testing.T) map[string]bool {
	t.Helper()

	raw, err := os.ReadFile(specPath)
	require.NoError(t, err, "swagger.json must be readable at %s", specPath)

	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}

	require.NoError(t, json.Unmarshal(raw, &spec), "swagger.json must parse")
	require.NotEmpty(t, spec.Paths, "swagger.json must declare paths")

	specSet := make(map[string]bool)

	for path, ops := range spec.Paths {
		for verb := range ops {
			specSet[strings.ToUpper(verb)+" "+canonicalizePath(path)] = true
		}
	}

	return specSet
}

// TestContractSpecMatchesRoutes is the DC-3 spec-vs-routes diff gate: the routes
// the unified ledger binary mounts (normalized, minus locked public-infra routes)
// must be exactly the set of paths+methods the served swagger.json enumerates —
// in BOTH directions. A failure means served and mounted have diverged: either a
// route was added/removed without regenerating the spec, or the spec drifted from
// the mount. Do not weaken this assertion; reconcile the source of the mismatch.
func TestContractSpecMatchesRoutes(t *testing.T) {
	t.Parallel()

	mounted := collectMountedRoutes(buildUnifiedRouteSurface())
	spec := collectSpecRoutes(t)

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
		t.Errorf("served swagger.json and mounted routes diverged\n\n"+
			"MOUNTED but NOT in spec (%d):\n  %s\n\n"+
			"in SPEC but NOT mounted (%d):\n  %s\n\n"+
			"mounted total=%d  spec total=%d",
			len(mountedNotInSpec), strings.Join(mountedNotInSpec, "\n  "),
			len(specNotMounted), strings.Join(specNotMounted, "\n  "),
			len(mounted), len(spec))
	}
}
