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
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Pinned shape of the authorized surface, counted through the real router: how many
// endpoints send both dimensions, the organization alone, or nothing. A route that
// gains or loses a dimension moves one of these, so a change to what the ledger tells
// the authorization service has to show up in review.
const (
	scopedRoutesBothDimensions   = 173
	scopedRoutesOrganizationOnly = 34
	scopedRoutesNoDimension      = 13
)

// scopedRoutesPerShape pins, per exact set of dimensions, how many endpoints send it. The
// three constants above are this map projected onto the organization and the ledger.
var scopedRoutesPerShape = map[string]int{
	"": scopedRoutesNoDimension,

	"organizationId":                                      21,
	"organizationId+holderId":                             3,
	"organizationId+holderId+instrumentId":                3,
	"organizationId+holderId+instrumentId+relatedPartyId": 1,
	"organizationId+operationRouteId":                     3,
	"organizationId+transactionRouteId":                   3,

	"organizationId+ledgerId":                                          60,
	"organizationId+ledgerId+accountId":                                35,
	"organizationId+ledgerId+accountId+balanceId":                      8,
	"organizationId+ledgerId+accountId+holderId":                       2,
	"organizationId+ledgerId+accountId+operationId":                    2,
	"organizationId+ledgerId+accountId+portfolioId+segmentId":          4,
	"organizationId+ledgerId+accountId+portfolioId+segmentId+holderId": 2,
	"organizationId+ledgerId+accountId+transactionId":                  10,
	"organizationId+ledgerId+accountId+transactionId+operationId":      2,
	"organizationId+ledgerId+accountTypeId":                            6,
	"organizationId+ledgerId+assetId":                                  6,
	"organizationId+ledgerId+billingPackageId":                         3,
	"organizationId+ledgerId+feeDebtId":                                1,
	"organizationId+ledgerId+holderId":                                 1,
	"organizationId+ledgerId+operationRouteId":                         6,
	"organizationId+ledgerId+packageId":                                3,
	"organizationId+ledgerId+portfolioId":                              6,
	"organizationId+ledgerId+portfolioId+segmentId+holderId":           2,
	"organizationId+ledgerId+segmentId":                                8,
	"organizationId+ledgerId+transactionRouteId":                       6,
}

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

// scopeShape names the set of dimensions an attribute map carries, in the order of
// names, so a per-shape count reads as "organizationId+ledgerId" rather than a size.
// Called with only the organization and ledger, it projects every route onto the two
// dimensions the ledger sent before the entity dimensions existed.
func scopeShape(attributes map[string]string, names []string) string {
	sent := make([]string, 0, len(attributes))
	for _, name := range names {
		if _, carried := attributes[name]; carried {
			sent = append(sent, name)
		}
	}

	return strings.Join(sent, "+")
}

// dimensionNames returns the manifest dimension names in catalog order.
func dimensionNames(dims []declaration.DeclarationDimension) []string {
	names := make([]string, 0, len(dims))
	for _, dim := range dims {
		names = append(names, dim.Name)
	}

	return names
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

// scopeProbePartnerToken is a parseable application token bound to a partner. Only a
// partner-bound credential has its scope read from the request body, so it is the one
// that drives the routes declaring body dimensions.
func scopeProbePartnerToken(t *testing.T) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type":    "application",
		"owner":   "scope-probe-org",
		"sub":     "scope-probe-org/scope-probe-app",
		"partner": "scope-probe-partner",
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
// A route that reads dimensions from its body is driven with a partner-bound
// credential, the only caller whose body is read for scope; every other route with a
// user credential.
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
	wireProbeAuthScope(t, auth)

	server := buildFullSurfaceServerWithAuth(t, auth)

	groups := groupRouteRows(collectRouteRows(t, server.app))
	requireUnambiguousProbeURLs(t, groups)

	userToken := scopeProbeToken(t)
	partnerToken := scopeProbePartnerToken(t)
	names := dimensionNames(dims)
	shape := make(map[string]int)
	fullShape := make(map[string]int)
	bodyProbes := bodyScopeProbes(t, dims, values)
	bodyProbed := 0

	for _, group := range groups {
		if unguardedPublicRoutes[group.key] {
			continue
		}

		rawPath := group.rows[0].path
		want := expectedScopeAttributes(rawPath, dims, values)

		// A route that reads dimensions from its body or its query is sent a request
		// naming them by a partner, and they join the ones its path derives.
		token := userToken

		probe, declared := bodyProbes[group.key]
		if declared {
			bodyProbed++
			token = partnerToken

			if want == nil {
				want = make(map[string]string, len(probe.bodyAttributes)+len(probe.queryAttributes))
			}

			for name, value := range probe.bodyAttributes {
				want[name] = value
			}

			for name, value := range probe.queryAttributes {
				want[name] = value
			}

			for name, value := range probe.resolvedAttributes {
				want[name] = value
			}
		}

		shape[scopeShape(want, []string{"organizationId", "ledgerId"})]++
		fullShape[scopeShape(want, names)]++

		t.Run(group.display(), func(t *testing.T) {
			before := recorder.calls
			recorder.attributes = nil

			req := httptest.NewRequest(group.rows[0].method, probe.url(scopedRouteURL(rawPath, values)), bodyReader(probe.body))
			if probe.readsBody() {
				req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
			}

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
	}, shape, "endpoints per organization/ledger projection of the dimensions sent")

	assert.Equal(t, scopedRoutesPerShape, fullShape, "endpoints per set of dimensions sent")
	assert.Equal(t, len(bodyProbes), bodyProbed, "every route that reads its scope from the body or the query must have been driven")

	total := 0
	for _, n := range shape {
		total += n
	}

	assert.Equal(t, len(groups)-unguardedPublicRouteCount, total,
		"every endpoint outside the public carve-out must have been driven")
}

// TestManifestScope_BodyRoutesSendOnlyPathDimensionsForANonPartner drives every route
// that declares body dimensions with credentials bound to no partner, carrying the same
// request a partner would send. The body is not read for scope: each makes exactly one
// authorization call carrying only the dimensions its path derives and the ones its
// query names, as it did before body dimensions existed.
func TestManifestScope_BodyRoutesSendOnlyPathDimensionsForANonPartner(t *testing.T) {
	unsetDocsGate(t)

	dims := manifestScopeDimensions(t)

	values := make(map[string]string, len(dims))
	for i, dim := range dims {
		values[dim.Param] = scopeProbeValue(i)
	}

	recorder, authz := newScopeRecorder(t)

	auth := &middleware.AuthClient{Enabled: true, Address: authz.URL}
	wireProbeAuthScope(t, auth)

	server := buildFullSurfaceServerWithAuth(t, auth)

	groups := groupRouteRows(collectRouteRows(t, server.app))
	requireUnambiguousProbeURLs(t, groups)

	bodyProbes := bodyScopeProbes(t, dims, values)
	require.NotEmpty(t, bodyProbes, "the manifest must declare the routes that read their scope from the body")

	applicationToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type":  "application",
		"owner": "scope-probe-org",
		"sub":   "scope-probe-org/scope-probe-app",
	}).SignedString([]byte("scope-probe-secret"))
	require.NoError(t, err)

	tokens := map[string]string{"user": scopeProbeToken(t), "application": applicationToken}
	probed := 0

	bodyRoutes := 0

	for _, probe := range bodyProbes {
		if probe.partnerOnly() {
			bodyRoutes++
		}
	}

	for _, group := range groups {
		probe, declared := bodyProbes[group.key]
		if !declared || !probe.partnerOnly() {
			continue
		}

		probed++

		rawPath := group.rows[0].path
		want := expectedScopeAttributes(rawPath, dims, values)

		for name, value := range probe.queryAttributes {
			if want == nil {
				want = make(map[string]string, len(probe.queryAttributes))
			}

			want[name] = value
		}

		for kind, token := range tokens {
			t.Run(group.display()+" "+kind, func(t *testing.T) {
				before := recorder.calls
				recorder.attributes = nil

				req := httptest.NewRequest(group.rows[0].method, probe.url(scopedRouteURL(rawPath, values)), bodyReader(probe.body))
				if probe.readsBody() {
					req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
				}

				req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

				resp, err := server.app.Test(req, fiber.TestConfig{Timeout: 0})
				require.NoError(t, err)

				defer func() { _ = resp.Body.Close() }()

				require.Equalf(t, fiber.StatusForbidden, resp.StatusCode,
					"%s must reach the authorization service and be denied", group.display())
				require.Equalf(t, before+1, recorder.calls,
					"%s must make exactly one authorization call for a non-partner", group.display())

				assert.Equalf(t, want, recorder.attributes,
					"%s must send a non-partner only the dimensions its path derives", group.display())
			})
		}
	}

	assert.Equal(t, bodyRoutes, probed, "every route that reads its scope from the body must have been driven")
}

// naturalKeyRouteParams is the LOCKED set of path parameters that are not entity ids and
// therefore carry no scope dimension: lookup keys (an account alias or external code, an
// asset code, an asset-rate external id) and metadata-index settings names. Every other
// parameter a route spells must be a manifest dimension.
var naturalKeyRouteParams = map[string]bool{
	"alias":       true,
	"code":        true,
	"asset_code":  true,
	"external_id": true,
	"entity_name": true,
	"index_key":   true,
}

// nonUUIDDimensionParams is the LOCKED set of dimension parameters whose values are not
// UUIDs, so ParseUUIDPathParameters must not validate them. A fee debt is keyed by its
// origin transaction and posting, not by a UUID.
var nonUUIDDimensionParams = map[string]bool{
	"fee_debt_id": true,
}

// routeParamsByName returns, for every parameter any route spells, the endpoints that
// spell it.
func routeParamsByName(groups []routeGroup) map[string][]routeGroup {
	byName := make(map[string][]routeGroup)

	for _, group := range groups {
		for _, name := range routePathParams(group.rows[0].path) {
			byName[name] = append(byName[name], group)
		}
	}

	return byName
}

// TestManifestScope_NoRouteSpellsAGenericID refuses a bare ":id" anywhere on the surface.
// A generic parameter cannot be a dimension — the manifest refuses one param standing
// for two collections — so the entity it names is invisible to partner scope.
func TestManifestScope_NoRouteSpellsAGenericID(t *testing.T) {
	unsetDocsGate(t)

	server := buildFullSurfaceServer(t)
	groups := groupRouteRows(collectRouteRows(t, server.app))

	generic := make([]string, 0)

	for _, group := range routeParamsByName(groups)["id"] {
		generic = append(generic, group.display())
	}

	assert.Emptyf(t, generic,
		"these routes spell a generic :id; name the parameter after its resource (:<resource>_id):\n%s",
		strings.Join(generic, "\n"))
}

// TestManifestScope_EveryEntityParameterHasADimension ties every path parameter to the
// manifest: an entity id must be a declared dimension, and the only parameters left
// undeclared are the locked natural keys. A new route that names an entity the manifest
// does not declare sends nothing for it, and a partner confined to that entity is never
// matched.
func TestManifestScope_EveryEntityParameterHasADimension(t *testing.T) {
	unsetDocsGate(t)

	dims := manifestScopeDimensions(t)

	declared := make(map[string]bool, len(dims))
	for _, dim := range dims {
		declared[dim.Param] = true

		assert.Falsef(t, naturalKeyRouteParams[dim.Param],
			"dimension %s reads :%s, which is locked as a natural key", dim.Name, dim.Param)
	}

	server := buildFullSurfaceServer(t)
	byName := routeParamsByName(groupRouteRows(collectRouteRows(t, server.app)))

	undeclared := make([]string, 0)

	for name, groups := range byName {
		if declared[name] || naturalKeyRouteParams[name] {
			continue
		}

		undeclared = append(undeclared, ":"+name+" on "+groups[0].display())
	}

	sort.Strings(undeclared)

	assert.Emptyf(t, undeclared,
		"these parameters are neither a manifest dimension nor a locked natural key:\n%s",
		strings.Join(undeclared, "\n"))

	for name := range naturalKeyRouteParams {
		assert.NotEmptyf(t, byName[name], "natural key :%s is locked but no route spells it", name)
	}
}

// TestManifestScope_EveryUUIDDimensionIsValidated proves the rename kept every entity id
// validated. ParseUUIDPathParameters validates only the names in
// constant.UUIDPathParameters and lets any other name through as a raw string, so a
// parameter renamed out of that list silently stops being checked.
//
// It asserts the list, and then drives every protected route once per UUID dimension it
// carries, past an authorization service that allows everything, with that one value
// malformed: each must answer 400 naming the parameter.
func TestManifestScope_EveryUUIDDimensionIsValidated(t *testing.T) {
	unsetDocsGate(t)

	dims := manifestScopeDimensions(t)

	uuidParams := make(map[string]bool, len(dims))

	for _, dim := range dims {
		if nonUUIDDimensionParams[dim.Param] {
			assert.NotContainsf(t, cn.UUIDPathParameters, dim.Param,
				"dimension %s reads :%s, whose values are not UUIDs", dim.Name, dim.Param)

			continue
		}

		uuidParams[dim.Param] = true

		assert.Containsf(t, cn.UUIDPathParameters, dim.Param,
			"dimension %s reads :%s, which ParseUUIDPathParameters does not validate", dim.Name, dim.Param)
	}

	authz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Connection", "close")
		w.Header().Set("Content-Type", "application/json")

		if _, err := w.Write([]byte(`{"authorized":true}`)); err != nil {
			t.Errorf("allow-all authorizer: write response: %v", err)
		}
	}))
	t.Cleanup(authz.Close)

	auth := &middleware.AuthClient{Enabled: true, Address: authz.URL}
	wireProbeAuthScope(t, auth)

	server := buildFullSurfaceServerWithAuth(t, auth)

	groups := groupRouteRows(collectRouteRows(t, server.app))
	requireUnambiguousProbeURLs(t, groups)

	token := scopeProbeToken(t)
	probed := 0
	probeValues := make(map[string]string, len(dims))
	for i, dim := range dims {
		probeValues[dim.Param] = scopeProbeValue(i)
	}

	bodyProbes := bodyScopeProbes(t, dims, probeValues)

	for _, group := range groups {
		if unguardedPublicRoutes[group.key] {
			continue
		}

		rawPath := group.rows[0].path

		for _, param := range routePathParams(rawPath) {
			if !uuidParams[param] {
				continue
			}

			probed++

			t.Run(group.display()+" :"+param, func(t *testing.T) {
				// A route that reads its scope from the body is sent the body a partner
				// would send, so the probe stays valid whoever the caller is.
				probe := bodyProbes[group.key]

				req := httptest.NewRequest(group.rows[0].method,
					probe.url(scopedRouteURL(rawPath, map[string]string{param: "not-a-uuid"})), bodyReader(probe.body))
				if probe.readsBody() {
					req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
				}

				req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

				resp, err := server.app.Test(req, fiber.TestConfig{Timeout: 0})
				require.NoError(t, err)

				defer func() { _ = resp.Body.Close() }()

				assert.Equalf(t, fiber.StatusBadRequest, resp.StatusCode,
					"%s must refuse a malformed :%s", group.display(), param)

				if group.rows[0].method == fiber.MethodHead {
					return // a HEAD answer carries no body to name the parameter in
				}

				// The v1 contract answers the legacy envelope (code, message), v2 the
				// problem document (type, detail); both carry the code and the name.
				var body struct {
					Code    string `json:"code"`
					Type    string `json:"type"`
					Message string `json:"message"`
					Detail  string `json:"detail"`
				}

				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

				assert.Truef(t, body.Code == cn.ErrInvalidPathParameter.Error() ||
					strings.HasSuffix(body.Type, "/"+cn.ErrInvalidPathParameter.Error()),
					"%s must answer the invalid-path-parameter code for :%s, got code %q type %q",
					group.display(), param, body.Code, body.Type)
				assert.Containsf(t, body.Message+body.Detail, " "+param+" ",
					"%s must name :%s in the refusal", group.display(), param)
			})
		}
	}

	assert.Positive(t, probed, "the probe must have driven at least one parameter")
}
