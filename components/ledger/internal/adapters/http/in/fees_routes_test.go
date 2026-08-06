// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"sort"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func TestFeeChainPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		specPath string
		expected string
	}{
		{
			name:     "organization scope",
			specPath: "/organizations/{organization_id}",
			expected: "/organizations/:organization_id",
		},
		{
			name:     "consecutive parameters keep their own names",
			specPath: "/organizations/{organization_id}/ledgers/{ledger_id}",
			expected: "/organizations/:organization_id/ledgers/:ledger_id",
		},
		{
			name:     "literal-only path is unchanged",
			specPath: "/billing/calculate",
			expected: "/billing/calculate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, feeChainPath(tt.specPath))
		})
	}
}

// TestFeesRoutesParameterNamesAgree pins the property the spec-vs-routes gate cannot
// see. That gate collapses every parameter to a positional token, so it reads a chain
// declaring ":ledger" and a contract declaring "{ledger_id}" as the same route. The
// mismatch has a runtime cost: ParseUUIDPathParameters keys on the name the FIBER route
// declares and parses only the names constant.UUIDPathParameters lists, so a parameter
// spelled off-list reaches the handler as an unvalidated string while the request still
// routes and the chain still runs.
//
// Both halves are asserted per path: the two surfaces name their parameters identically,
// and every name they use is one the UUID validator recognizes.
func TestFeesRoutesParameterNamesAgree(t *testing.T) {
	// NOT parallel: huma registration mutates process-global state.
	app := fiber.New()

	libProblem.Install()
	group := app.Group("/v1")
	api := openapi.New(app, group, openapi.Config{Title: "ledger-fees-params", Version: "test", Servers: []string{"/v1"}})
	pkgHTTP.InstallLedgerSchemaNamer(api)

	RegisterFeesRoutesToApp(group, api, &middleware.AuthClient{Enabled: false},
		&PackageHandler{}, &FeeHandler{}, &BillingPackageHandler{}, &BillingCalculateHandler{}, nil)

	// Both surfaces land in the Fiber router — the guard chain directly, the Huma
	// terminal through the adapter — so a path structure that carries two different
	// parameter spellings shows up as two entries here. Keying by canonical structure
	// and collecting every spelling seen is what makes the disagreement visible.
	//
	// The key set comes from the document this registrar just wrote, so no fee path can
	// be silently left out of the sweep. The API is built fresh above and only the fee
	// registrar writes to it, so every path in it is a fee path.
	spellings := make(map[string]map[string][]string)

	for path := range api.OpenAPI().Paths {
		spellings[canonicalizePath("/v1"+path)] = make(map[string][]string)
	}

	require.Len(t, spellings, 6, "the v1 fee surface publishes six distinct path structures")

	for _, r := range app.GetRoutes() {
		path := canonicalizePath(r.Path)
		if seen, ok := spellings[path]; ok {
			seen[strings.Join(r.Params, ",")] = r.Params
		}
	}

	for path, seen := range spellings {
		t.Run(strings.TrimPrefix(path, "/"), func(t *testing.T) {
			names := make([]string, 0, len(seen))
			for joined := range seen {
				names = append(names, joined)
			}

			sort.Strings(names)

			require.Lenf(t, names, 1,
				"%s: the guard chain and the contract must name their parameters identically, saw %v",
				path, names)

			for _, name := range seen[names[0]] {
				assert.Containsf(t, constant.UUIDPathParameters, name,
					"%s: parameter %q is not UUID-validated by ParseUUIDPathParameters", path, name)
			}
		})
	}
}
