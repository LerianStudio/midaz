// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUnifiedServer_RegistersTenantTelemetryVariant guards the one line that
// turns the per-tenant HTTP metrics on. The behaviour itself is proven in
// pkg/net/http's wiring test, which builds its own app; nothing there observes
// what the ledger registers, so a revert to WithTelemetry would drop every
// tenant-labelled series with a fully green suite.
//
// The two variants are mutually exclusive: WithAuthenticatedTenantHTTPMetrics
// already records the standard HTTP telemetry, so registering both would double
// every observation of http.server.request.duration.
func TestUnifiedServer_RegistersTenantTelemetryVariant(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("unified-server.go")
	require.NoError(t, err)

	body := string(source)

	require.Contains(t, body, "WithAuthenticatedTenantHTTPMetrics(",
		"the unified server must register the per-tenant telemetry variant")
	require.False(t, strings.Contains(body, "tlMid.WithTelemetry("),
		"WithTelemetry must not be registered alongside the per-tenant variant: both record http.server.request.duration")

	// T11: probe paths must reach the middleware as excluded routes. Behaviour
	// is covered in pkg/net/http; this pins that the ledger actually passes the
	// list, which that test cannot observe.
	require.Contains(t, body, "WithAuthenticatedTenantHTTPMetrics(telemetry, skipTelemetryPaths...)",
		"the probe paths must be passed to the telemetry middleware as excluded routes")

	// Read the declaration rather than the file: every one of these literals
	// also appears at its app.Get mount, so a substring search over the source
	// would still pass with the path removed from the slice.
	require.ElementsMatch(t, []string{"/health", "/readyz", "/metrics"}, skipTelemetryPathsDecl(t),
		"skipTelemetryPaths must carry exactly the T11 probe paths")
}

// skipTelemetryPathsDecl parses unified-server.go and returns the string values
// of the skipTelemetryPaths declaration. Reading the value at runtime would not
// do: the variable is in this package, so the test would assert against whatever
// the source says and could never disagree with it.
func skipTelemetryPathsDecl(t *testing.T) []string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), "unified-server.go", nil, 0)
	require.NoError(t, err)

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok || len(valueSpec.Names) != 1 || valueSpec.Names[0].Name != "skipTelemetryPaths" {
				continue
			}

			require.Len(t, valueSpec.Values, 1)

			composite, ok := valueSpec.Values[0].(*ast.CompositeLit)
			require.Truef(t, ok, "skipTelemetryPaths must be a slice literal, got %T", valueSpec.Values[0])

			paths := make([]string, 0, len(composite.Elts))

			for _, elt := range composite.Elts {
				lit, ok := elt.(*ast.BasicLit)
				require.Truef(t, ok, "skipTelemetryPaths must hold string literals, got %T", elt)

				unquoted, err := strconv.Unquote(lit.Value)
				require.NoError(t, err)

				paths = append(paths, unquoted)
			}

			return paths
		}
	}

	t.Fatal("skipTelemetryPaths is not declared in unified-server.go")

	return nil
}
