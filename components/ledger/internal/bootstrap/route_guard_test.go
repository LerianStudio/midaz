// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// probeUUID and probeSegment are the FIXED values substituted for path parameters when a
// raw path is turned into a requestable URL. Fixed, not generated: a value that varied
// between runs would vary which registered route a URL matches without varying anything
// the test asserts, and the repository forbids time.Now in tests for the same reason.
//
// Only syntactic routability matters here. Nothing in these requests reaches a parser or a
// repository: the assertion is that the request is refused, and refusal happens ahead of
// every handler that would look at a parameter's value.
const (
	probeUUID    = "00000000-0000-0000-0000-000000000001"
	probeSegment = "route-guard-probe"
)

// concreteRouteURL turns a Fiber raw path into a requestable URL. Id-shaped parameters get the
// UUID-shaped substitute and every other parameter gets one opaque segment that is not a
// literal segment of any registered path, so a substituted value can never make a URL match a
// route it was not derived from.
func concreteRouteURL(rawPath string) string {
	segments := strings.Split(rawPath, "/")

	for i, segment := range segments {
		name, isParam := strings.CutPrefix(segment, ":")
		if !isParam {
			continue
		}

		if name == "id" || strings.HasSuffix(name, "_id") {
			segments[i] = probeUUID

			continue
		}

		segments[i] = probeSegment
	}

	return strings.Join(segments, "/")
}

// rawPathMatchesURL reports whether a Fiber raw path would match a concrete URL, treating
// each ":name" segment as a wildcard over exactly one segment.
//
// It exists to police an attribution gap rather than to route anything: when two DIFFERENT raw
// paths on one method both match the URL derived from one of them, the response to that URL says
// nothing about which of the two was exercised, and a missing guard on the loser could hide
// behind a 401 from the winner.
//
// It is correct only while every parameter spans exactly one segment: segment-count equality plus
// literal equality is then the entire matching rule. Fiber's optional ("?"), plus ("+") and
// wildcard ("*") forms span zero or many, so requireUnambiguousProbeURLs asserts the surface
// registers none of them before consuming this function.
func rawPathMatchesURL(rawPath, url string) bool {
	pathSegments := strings.Split(rawPath, "/")
	urlSegments := strings.Split(url, "/")

	if len(pathSegments) != len(urlSegments) {
		return false
	}

	for i, segment := range pathSegments {
		if strings.HasPrefix(segment, ":") {
			continue
		}

		if segment != urlSegments[i] {
			return false
		}
	}

	return true
}

// requireUnambiguousProbeURLs fails when a probe URL matches more than one registered raw
// path on its own method, naming every colliding pair. See rawPathMatchesURL for why a
// collision voids the attribution the guard assertions depend on.
//
// It first enforces that function's precondition: single-segment parameters only.
func requireUnambiguousProbeURLs(t *testing.T, groups []routeGroup) {
	t.Helper()

	pathsByMethod := make(map[string][]string, len(groups))

	for _, group := range groups {
		require.Falsef(t, strings.ContainsAny(group.rows[0].path, "?+*"),
			"%s carries an optional or multi-segment parameter, which rawPathMatchesURL treats as one mandatory segment: "+
				"handle that form there before trusting this gate", group.display())

		pathsByMethod[group.rows[0].method] = append(pathsByMethod[group.rows[0].method], group.rows[0].path)
	}

	collisions := make([]string, 0)

	for _, group := range groups {
		method := group.rows[0].method
		rawPath := group.rows[0].path
		url := concreteRouteURL(rawPath)

		for _, candidate := range pathsByMethod[method] {
			if candidate == rawPath {
				continue
			}

			if rawPathMatchesURL(candidate, url) {
				collisions = append(collisions, method+" "+url+" matches both "+rawPath+" and "+candidate)
			}
		}
	}

	sort.Strings(collisions)

	require.Emptyf(t, collisions,
		"these probe URLs match more than one registered route, so a tokenless response cannot be attributed to the "+
			"route it was derived from and a missing guard on one of them would hide behind the other's refusal:\n%s",
		strings.Join(collisions, "\n"))
}

// TestFullSurfaceRoutes_RejectTokenlessRequests asserts BEHAVIOR over the whole registered
// surface: with auth enabled and no credentials presented, every route except the carved-out
// public probes must be answered 401, and the harness post-auth observer must never have run —
// so the refusal came from the FIRST handler on the chain, the authorizer, rather than from
// something further down that also answers 401 on a missing token.
//
// It proves REFUSAL, never REACHABILITY. A globally broken chain that answered 401 to every
// request on the surface would be green here, and so would a surface whose terminals are
// unreachable for reasons that have nothing to do with auth.
//
// It extends the same pattern the transaction v2 surface uses in
// components/ledger/internal/adapters/http/in, over the whole registered surface instead
// of four paths.
func TestFullSurfaceRoutes_RejectTokenlessRequests(t *testing.T) {
	// The docs surface is gated on OPENAPI_DOCS_ENABLED and this test asserts the default
	// deployment posture. unsetDocsGate uses t.Setenv, which precludes t.Parallel here and
	// in the subtests.
	unsetDocsGate(t)

	server := buildFullSurfaceServer(t)

	rows := collectRouteRows(t, server.app)
	require.GreaterOrEqualf(t, len(rows), routeTableMinRows,
		"route table holds %d rows, under the %d floor: the harness is not mounting the full surface, so passing here "+
			"would mean nothing", len(rows), routeTableMinRows)

	groups := groupRouteRows(rows)

	requireUnambiguousProbeURLs(t, groups)

	probed := 0

	for _, group := range groups {
		// The public operational probes are mounted outside the authorized surface on
		// purpose and are justified entry by entry in unguardedPublicRoutes. They are the
		// only routes allowed to answer a credential-less request.
		if unguardedPublicRoutes[group.key] {
			continue
		}

		probed++

		t.Run(group.display(), func(t *testing.T) {
			req := httptest.NewRequest(group.rows[0].method, concreteRouteURL(group.rows[0].path), nil)

			resp, err := server.app.Test(req)
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			assert.Equalf(t, fiber.StatusUnauthorized, resp.StatusCode,
				"a credential-less request to %s was answered %d instead of 401: the auth guard did not run ahead of "+
					"the terminal on this path", group.display(), resp.StatusCode)
		})
	}

	// Counted against the PINNED carve-out size, not against len(unguardedPublicRoutes): both
	// sides of that comparison move with the map, so adding a key would subtract one from each
	// and disarm the very skip it authorizes.
	assert.Equalf(t, len(groups)-unguardedPublicRouteCount, probed,
		"%d of %d registered endpoints were probed, leaving more than the %d pinned carve-outs unprobed",
		probed, len(groups), unguardedPublicRouteCount)

	assert.Falsef(t, fullSurfaceMarkerRan.Load(),
		"the harness post-auth observer ran during a credential-less request: a handler behind the authorizer answered, so "+
			"the 401s above are not attributable to the auth guard")
}
