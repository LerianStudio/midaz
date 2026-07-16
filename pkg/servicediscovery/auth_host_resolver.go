// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"context"
	"time"
)

// authServiceName is the registry name plugin-auth is resolved by, and the
// service label attached to the resolve metric.
const authServiceName = "plugin-auth"

// sinceFn measures elapsed time for the resolve-duration metric. It is a package
// var so tests can freeze it deterministically (mirrors the hostnameFn seam);
// production uses time.Since.
var sinceFn = time.Since

// AuthHostResolver resolves a service to a preferred, scheme-complete URL,
// returning the fallback verbatim when discovery is disabled or fails.
// *libsd.Manager satisfies this contract via ResolvePreferredURL.
type AuthHostResolver interface {
	ResolvePreferredURL(ctx context.Context, name, fallback string) (string, error)
}

// ResolveAuthHost picks the plugin-auth host to feed into the auth client.
// It only resolves when auth is enabled (no point resolving a downstream we
// won't call) and always degrades to the static host on resolve error so a
// discovery outage never fails boot.
//
// It resolves the SD_PREFER_VIEW view via ResolvePreferredURL, which returns a
// scheme-complete URL. It passes an EMPTY fallback so the return distinguishes a
// discovery-resolved URL from a fell-back one: on a miss, disabled discovery, or
// unavailable view the resolver returns an error rather than swallowing it into
// the fallback, letting this function label the resolve outcome
// (resolved/fallback/error) for the metric. On success the resolved URL is
// returned verbatim; on error it degrades to the static host verbatim.
func ResolveAuthHost(ctx context.Context, r AuthHostResolver, authEnabled bool, staticHost string, recorder MetricsRecorder) string {
	if !authEnabled {
		return staticHost
	}

	start := time.Now()
	resolved, err := r.ResolvePreferredURL(ctx, authServiceName, "")
	durationMs := sinceFn(start).Milliseconds()

	var (
		authHost string
		result   string
	)

	switch {
	case err == nil:
		result = ResultResolved
		authHost = resolved
	case staticHost != "":
		result = ResultFallback
		authHost = staticHost
	default:
		result = ResultError
		authHost = staticHost
	}

	orNop(recorder).ResolveResult(ctx, authServiceName, result, durationMs)

	return authHost
}
