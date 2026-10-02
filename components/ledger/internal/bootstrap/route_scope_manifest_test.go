// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	ledgerembed "github.com/LerianStudio/midaz/v4/components/ledger"
)

// Pinned shape of the authorized surface, counted through the real router: how many
// endpoints send both dimensions, the organization alone, or nothing. A route that
// gains or loses a dimension moves one of these, so a change to what the ledger tells
// the authorization service has to show up in review.
const (
	scopedRoutesBothDimensions   = 165
	scopedRoutesOrganizationOnly = 37
	scopedRoutesNoDimension      = 18
)

// manifestScopeDimensions parses the scope section of the embedded manifest — the
// bytes the ledger publishes and wires — and fails when it is absent or invalid.
func manifestScopeDimensions(t *testing.T) []declaration.DeclarationDimension {
	t.Helper()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(ledgerembed.MidazManifest, &manifest), "the embedded manifest must parse")
	require.NoError(t, manifest.Validate(), "the embedded manifest must validate")
	require.NotNil(t, manifest.Scope, "the embedded manifest must declare a scope section")
	require.NotEmpty(t, manifest.Scope.Dimensions, "the manifest scope must declare at least one dimension")

	return manifest.Scope.Dimensions
}

// routePathParams returns the parameter names a Fiber raw path carries, in order.
func routePathParams(rawPath string) []string {
	params := make([]string, 0)

	for _, segment := range strings.Split(rawPath, "/") {
		if name, isParam := strings.CutPrefix(segment, ":"); isParam {
			params = append(params, name)
		}
	}

	return params
}

// scopeProbeValue is the value a probe URL carries for a dimension parameter. Each
// dimension gets its OWN value, so an attribute sent under the wrong name — the ledger
// id as the organization — is a visible mismatch instead of two equal strings.
func scopeProbeValue(index int) string {
	return "00000000-0000-0000-0000-0000000000" + string(rune('a'+index)) + "1"
}

// scopedRouteURL turns a raw path into a requestable URL: dimension parameters carry
// their own value, every other parameter the substitute concreteRouteURL uses, so the
// URL matches exactly the routes requireUnambiguousProbeURLs already proved it does.
func scopedRouteURL(rawPath string, values map[string]string) string {
	segments := strings.Split(rawPath, "/")

	for i, segment := range segments {
		name, isParam := strings.CutPrefix(segment, ":")
		if !isParam {
			continue
		}

		if value, isDimension := values[name]; isDimension {
			segments[i] = value

			continue
		}

		segments[i] = concreteRouteURL(segment)
	}

	return strings.Join(segments, "/")
}

// expectedScopeAttributes is the attribute map the authorization service must receive
// for a raw path: every manifest dimension whose parameter is a whole segment of it,
// under the dimension's name. Nil when the path carries none — the route then sends no
// attributes at all.
func expectedScopeAttributes(rawPath string, dims []declaration.DeclarationDimension, values map[string]string) map[string]string {
	params := make(map[string]struct{})
	for _, name := range routePathParams(rawPath) {
		params[name] = struct{}{}
	}

	var want map[string]string

	for _, dim := range dims {
		if _, carried := params[dim.Param]; !carried {
			continue
		}

		if want == nil {
			want = make(map[string]string, len(dims))
		}

		want[dim.Name] = values[dim.Param]
	}

	return want
}

// scopeShape names the set of dimensions an attribute map carries, in catalog order,
// so the per-shape counts read as "organizationId+ledgerId" rather than a size.
func scopeShape(attributes map[string]string) string {
	names := make([]string, 0, len(attributes))
	for _, name := range []string{"organizationId", "ledgerId"} {
		if _, sent := attributes[name]; sent {
			names = append(names, name)
		}
	}

	return strings.Join(names, "+")
}

// scopeRecorder stands in for the Access Manager. It records the attributes of every
// authorization call and always denies, so the chain stops at the 403 and no terminal
// runs.
type scopeRecorder struct {
	calls      int
	attributes map[string]string
}

func newScopeRecorder(t *testing.T) (*scopeRecorder, *httptest.Server) {
	t.Helper()

	recorder := &scopeRecorder{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close per request so no keep-alive goroutine reaches the package goleak check.
		w.Header().Set("Connection", "close")

		var body struct {
			Attributes map[string]string `json:"attributes"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("scope recorder: decode authorization body: %v", err)
		}

		recorder.calls++
		recorder.attributes = body.Attributes

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if _, err := w.Write([]byte(`{"authorized":false}`)); err != nil {
			t.Errorf("scope recorder: write response: %v", err)
		}
	}))

	t.Cleanup(server.Close)

	return recorder, server
}

// scopeProbeToken is a parseable user token: it gets the request past token parsing
// and into the authorization round-trip, which is the only place attributes leave.
func scopeProbeToken(t *testing.T) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type":  "normal-user",
		"owner": "scope-probe-org",
		"sub":   "scope-probe-user",
	}).SignedString([]byte("scope-probe-secret"))
	require.NoError(t, err)

	return signed
}

// TestManifestScope_MatchesTheRouteParameters ties the manifest's scope catalog to the
// parameters the routes actually spell. The two drift apart silently in both
// directions:
//
//   - a dimension whose parameter no route uses is a catalog entry nothing ever sends;
//   - a route that spells a dimension's slot differently (":id" right under
//     /organizations, or the attribute name ":organizationId" as a parameter) derives
//     nothing, sends no identifier, and a partner confined to that instance is refused
//     on the instance it owns.
func TestManifestScope_MatchesTheRouteParameters(t *testing.T) {
	unsetDocsGate(t)

	dims := manifestScopeDimensions(t)

	server := buildFullSurfaceServer(t)

	routeRows := collectRouteRows(t, server.app)
	require.GreaterOrEqual(t, len(routeRows), routeTableMinRows, "the harness must mount the full surface")

	groups := groupRouteRows(routeRows)

	byCollection := make(map[string]declaration.DeclarationDimension, len(dims))
	byName := make(map[string]declaration.DeclarationDimension, len(dims))
	used := make(map[string]int, len(dims))

	for _, dim := range dims {
		byCollection[dim.Collection] = dim
		byName[dim.Name] = dim
	}

	misspelled := make([]string, 0)

	for _, group := range groups {
		rawPath := group.rows[0].path
		segments := strings.Split(rawPath, "/")

		for i, segment := range segments {
			name, isParam := strings.CutPrefix(segment, ":")
			if !isParam {
				continue
			}

			for _, dim := range dims {
				if name == dim.Param {
					used[dim.Param]++
				}
			}

			if dim, isAttributeName := byName[name]; isAttributeName && name != dim.Param {
				misspelled = append(misspelled, group.display()+": parameter :"+name+" is the attribute name of "+
					dim.Name+"; the manifest reads it from :"+dim.Param)
			}

			if i == 0 {
				continue
			}

			if dim, inSlot := byCollection[segments[i-1]]; inSlot && name != dim.Param {
				misspelled = append(misspelled, group.display()+": parameter :"+name+" sits under /"+dim.Collection+
					" but the manifest reads "+dim.Name+" from :"+dim.Param)
			}
		}
	}

	sort.Strings(misspelled)

	assert.Emptyf(t, misspelled,
		"these routes spell a dimension's parameter in a way the manifest does not read, so they send no identifier "+
			"for it:\n%s", strings.Join(misspelled, "\n"))

	for _, dim := range dims {
		assert.Positivef(t, used[dim.Param],
			"manifest dimension %s reads :%s, which no route carries: the catalog declares a dimension nothing sends",
			dim.Name, dim.Param)
	}
}

// TestManifestScope_EveryProtectedRouteSendsItsDimensions drives every endpoint the
// unified server registers through its real guard chain — with the auth client wired
// the way the boot wires it — and asserts, per endpoint, the attributes that reached
// the authorization service: each manifest dimension whose parameter the path carries,
// under its name, with the value the request carried for it.
//
// The dimension values DIVERGE on purpose. With one shared value, a route that sent the
// ledger id as the organization would pass.
func TestManifestScope_EveryProtectedRouteSendsItsDimensions(t *testing.T) {
	unsetDocsGate(t)

	dims := manifestScopeDimensions(t)

	values := make(map[string]string, len(dims))
	for i, dim := range dims {
		values[dim.Param] = scopeProbeValue(i)
	}

	recorder, authz := newScopeRecorder(t)

	auth := &middleware.AuthClient{Enabled: true, Address: authz.URL}
	require.NoError(t, wireAuthScope(auth), "the boot scope wiring must accept the embedded manifest")

	server := buildFullSurfaceServerWithAuth(t, auth)

	groups := groupRouteRows(collectRouteRows(t, server.app))
	requireUnambiguousProbeURLs(t, groups)

	token := scopeProbeToken(t)
	shape := make(map[string]int)

	for _, group := range groups {
		if unguardedPublicRoutes[group.key] {
			continue
		}

		rawPath := group.rows[0].path
		want := expectedScopeAttributes(rawPath, dims, values)
		shape[scopeShape(want)]++

		t.Run(group.display(), func(t *testing.T) {
			before := recorder.calls
			recorder.attributes = nil

			req := httptest.NewRequest(group.rows[0].method, scopedRouteURL(rawPath, values), nil)
			req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

			resp, err := server.app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			require.Equalf(t, fiber.StatusForbidden, resp.StatusCode,
				"%s must reach the authorization service and be denied", group.display())
			require.Equalf(t, before+1, recorder.calls,
				"%s must make exactly one authorization call", group.display())

			assert.Equalf(t, want, recorder.attributes,
				"%s sent the wrong instance identifiers to the authorization service", group.display())
		})
	}

	assert.Equal(t, map[string]int{
		"organizationId+ledgerId": scopedRoutesBothDimensions,
		"organizationId":          scopedRoutesOrganizationOnly,
		"":                        scopedRoutesNoDimension,
	}, shape, "endpoints per set of dimensions sent")

	total := 0
	for _, n := range shape {
		total += n
	}

	assert.Equal(t, len(groups)-unguardedPublicRouteCount, total,
		"every endpoint outside the public carve-out must have been driven")
}
