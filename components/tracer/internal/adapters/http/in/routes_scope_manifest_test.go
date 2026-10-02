// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	tracerembed "github.com/LerianStudio/midaz/v4/components/tracer"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

// tracerScopedRoutesPerShape pins, per exact set of dimensions, how many tracer endpoints
// send it. A route that gains or loses a dimension moves one of these, so a change to
// what the tracer tells the authorization service has to show up in review.
var tracerScopedRoutesPerShape = map[string]int{
	"":             11,
	"ruleId":       6,
	"limitId":      7,
	"validationId": 1,
	"auditEventId": 2,
}

// tracerPublicRoutes is the LOCKED set of tracer routes mounted outside the authorized
// surface: probes and build metadata that must answer without a credential.
var tracerPublicRoutes = map[string]bool{
	"GET /health":  true,
	"GET /readyz":  true,
	"GET /metrics": true,
	"GET /version": true,
}

// tracerScopeDimensions parses the scope section of the embedded tracer manifest — the
// bytes the tracer publishes and wires — and fails when it is absent or invalid.
func tracerScopeDimensions(t *testing.T) []declaration.DeclarationDimension {
	t.Helper()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(tracerembed.TracerManifest, &manifest), "the embedded manifest must parse")
	require.NoError(t, manifest.Validate(), "the embedded manifest must validate")
	require.NotNil(t, manifest.Scope, "the embedded manifest must declare a scope section")
	require.NotEmpty(t, manifest.Scope.Dimensions, "the manifest scope must declare at least one dimension")

	return manifest.Scope.Dimensions
}

// tracerRoute is one endpoint the tracer app registers, by method and Fiber raw path.
type tracerRoute struct {
	method string
	path   string
}

func (r tracerRoute) display() string { return r.method + " " + r.path }

// tracerRoutes returns every distinct endpoint the app registers, sorted. Fiber holds the
// guard and the Huma terminal as two rows on one endpoint; they collapse here.
func tracerRoutes(app *fiber.App) []tracerRoute {
	seen := make(map[tracerRoute]bool)
	routes := make([]tracerRoute, 0)

	for _, r := range app.GetRoutes(true) {
		route := tracerRoute{method: r.Method, path: r.Path}
		if seen[route] {
			continue
		}

		seen[route] = true
		routes = append(routes, route)
	}

	sort.Slice(routes, func(i, j int) bool { return routes[i].display() < routes[j].display() })

	return routes
}

// tracerRouteParams returns the parameter names a Fiber raw path carries, in order.
func tracerRouteParams(rawPath string) []string {
	params := make([]string, 0)

	for _, segment := range strings.Split(rawPath, "/") {
		if name, isParam := strings.CutPrefix(segment, ":"); isParam {
			params = append(params, name)
		}
	}

	return params
}

// tracerRouteURL turns a raw path into a requestable URL, every parameter carrying the
// value values names for it.
func tracerRouteURL(rawPath string, values map[string]string) string {
	segments := strings.Split(rawPath, "/")

	for i, segment := range segments {
		if name, isParam := strings.CutPrefix(segment, ":"); isParam {
			segments[i] = values[name]
		}
	}

	return strings.Join(segments, "/")
}

// tracerScopeProbeToken is a parseable user token carrying the `sub` the guard requires,
// so the request reaches the authorization round-trip.
func tracerScopeProbeToken(t *testing.T) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type":  "normal-user",
		"owner": "scope-probe-org",
		"sub":   "scope-probe-user",
	}).SignedString([]byte("scope-probe-secret"))
	require.NoError(t, err)

	return signed
}

// tracerAuthzRecorder stands in for the Access Manager: it records the attributes of
// every authorization call and answers with the configured decision.
type tracerAuthzRecorder struct {
	mu         sync.Mutex
	calls      int
	attributes map[string]string
}

func newTracerAuthzRecorder(t *testing.T, authorized bool) (*tracerAuthzRecorder, *httptest.Server) {
	t.Helper()

	recorder := &tracerAuthzRecorder{}
	answer := `{"authorized":false}`

	if authorized {
		answer = `{"authorized":true}`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")

		var body struct {
			Attributes map[string]string `json:"attributes"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("authz recorder: decode authorization body: %v", err)
		}

		recorder.mu.Lock()
		recorder.calls++
		recorder.attributes = body.Attributes
		recorder.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		if _, err := w.Write([]byte(answer)); err != nil {
			t.Errorf("authz recorder: write response: %v", err)
		}
	}))

	t.Cleanup(server.Close)

	return recorder, server
}

func (r *tracerAuthzRecorder) snapshot() (int, map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.calls, r.attributes
}

// buildScopedTracerApp mounts the full tracer surface behind plugin auth, with the auth
// client wired from the embedded manifest the way the boot wires it.
func buildScopedTracerApp(t *testing.T, authzURL string) *fiber.App {
	t.Helper()

	authClient := &authMiddleware.AuthClient{Enabled: true, Address: authzURL}
	require.NoError(t, declaration.WireScope(authClient, tracerembed.TracerManifest),
		"the boot scope wiring must accept the embedded manifest")

	deps := newTestRouterDeps(t, middleware.AuthGuardConfig{PluginAuthEnabled: true, AppName: constant.ApplicationName})

	return deps.buildWithAuthClient(authClient)
}

// TestTracerManifestScope_RoutesAndDimensionsAgree ties the tracer manifest's scope
// catalog to the parameters its routes spell: no route spells a generic ":id", every
// parameter is a declared dimension, and every dimension is spelled by some route.
func TestTracerManifestScope_RoutesAndDimensionsAgree(t *testing.T) {
	dims := tracerScopeDimensions(t)

	_, authz := newTracerAuthzRecorder(t, false)
	app := buildScopedTracerApp(t, authz.URL)

	declared := make(map[string]bool, len(dims))
	for _, dim := range dims {
		declared[dim.Param] = true
	}

	used := make(map[string]int, len(dims))
	problems := make([]string, 0)

	for _, route := range tracerRoutes(app) {
		for _, param := range tracerRouteParams(route.path) {
			switch {
			case param == "id":
				problems = append(problems, route.display()+": generic :id; name it after its resource (:<resource>_id)")
			case !declared[param]:
				problems = append(problems, route.display()+": :"+param+" is not a manifest dimension")
			default:
				used[param]++
			}
		}
	}

	assert.Emptyf(t, problems, "route parameters the manifest does not read:\n%s", strings.Join(problems, "\n"))

	for _, dim := range dims {
		assert.Positivef(t, used[dim.Param],
			"manifest dimension %s reads :%s, which no route carries", dim.Name, dim.Param)
	}
}

// TestTracerManifestScope_EveryProtectedRouteSendsItsDimensions drives every protected
// tracer endpoint through its real guard and asserts the attributes that reached the
// authorization service: each dimension whose parameter the path carries, under its name,
// with the value the request carried. Values differ per dimension, so a value sent under
// the wrong name is a visible mismatch.
func TestTracerManifestScope_EveryProtectedRouteSendsItsDimensions(t *testing.T) {
	dims := tracerScopeDimensions(t)

	values := make(map[string]string, len(dims))
	byParam := make(map[string]string, len(dims))

	for i, dim := range dims {
		values[dim.Param] = "00000000-0000-0000-0000-0000000000" + string(rune('a'+i)) + "1"
		byParam[dim.Param] = dim.Name
	}

	recorder, authz := newTracerAuthzRecorder(t, false)
	app := buildScopedTracerApp(t, authz.URL)
	token := tracerScopeProbeToken(t)

	names := make([]string, 0, len(dims))
	for _, dim := range dims {
		names = append(names, dim.Name)
	}

	shape := make(map[string]int)

	for _, route := range tracerRoutes(app) {
		if tracerPublicRoutes[route.display()] {
			continue
		}

		var want map[string]string

		for _, param := range tracerRouteParams(route.path) {
			if want == nil {
				want = make(map[string]string)
			}

			want[byParam[param]] = values[param]
		}

		sent := make([]string, 0, len(want))

		for _, name := range names {
			if _, carried := want[name]; carried {
				sent = append(sent, name)
			}
		}

		shape[strings.Join(sent, "+")]++

		t.Run(route.display(), func(t *testing.T) {
			before, _ := recorder.snapshot()

			req := httptest.NewRequest(route.method, tracerRouteURL(route.path, values), nil)
			req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			calls, got := recorder.snapshot()

			require.Equalf(t, fiber.StatusForbidden, resp.StatusCode,
				"%s must reach the authorization service and be denied", route.display())
			require.Equalf(t, before+1, calls, "%s must make exactly one authorization call", route.display())

			assert.Equalf(t, want, got,
				"%s sent the wrong instance identifiers to the authorization service", route.display())
		})
	}

	assert.Equal(t, tracerScopedRoutesPerShape, shape, "endpoints per set of dimensions sent")
}

// TestTracerManifestScope_EveryDimensionIsValidated drives every protected tracer route
// once per dimension it carries, past an authorization service that allows everything,
// with that one value malformed: each must answer 400 naming the parameter, so the rename
// kept every entity id validated and the refusal names the parameter the route spells.
func TestTracerManifestScope_EveryDimensionIsValidated(t *testing.T) {
	dims := tracerScopeDimensions(t)

	values := make(map[string]string, len(dims))
	for _, dim := range dims {
		values[dim.Param] = "00000000-0000-0000-0000-000000000001"
	}

	_, authz := newTracerAuthzRecorder(t, true)
	app := buildScopedTracerApp(t, authz.URL)
	token := tracerScopeProbeToken(t)

	probed := 0

	for _, route := range tracerRoutes(app) {
		if tracerPublicRoutes[route.display()] {
			continue
		}

		for _, param := range tracerRouteParams(route.path) {
			probed++

			t.Run(route.display()+" :"+param, func(t *testing.T) {
				malformed := make(map[string]string, len(values))
				for k, v := range values {
					malformed[k] = v
				}

				malformed[param] = "not-a-uuid"

				// A write carries an empty JSON object so the body check passes and the
				// refusal can only come from the path.
				var payload io.Reader
				if route.method == fiber.MethodPatch || route.method == fiber.MethodPost {
					payload = strings.NewReader("{}")
				}

				req := httptest.NewRequest(route.method, tracerRouteURL(route.path, malformed), payload)
				req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
				req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

				resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
				require.NoError(t, err)

				defer func() { _ = resp.Body.Close() }()

				var body struct {
					Code    string `json:"code"`
					Type    string `json:"type"`
					Message string `json:"message"`
					Detail  string `json:"detail"`
				}

				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

				assert.Equalf(t, fiber.StatusBadRequest, resp.StatusCode,
					"%s must refuse a malformed :%s", route.display(), param)
				assert.Containsf(t, body.Message+body.Detail, param,
					"%s must name :%s in the refusal", route.display(), param)
			})
		}
	}

	assert.Positive(t, probed, "the probe must have driven at least one parameter")
}
