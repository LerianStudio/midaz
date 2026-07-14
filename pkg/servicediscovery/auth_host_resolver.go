// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"context"
	"strings"
	"time"
)

// authServiceName is the registry name plugin-auth is resolved by, and the
// service label attached to the resolve metric.
const authServiceName = "plugin-auth"

// sinceFn measures elapsed time for the resolve-duration metric. It is a package
// var so tests can freeze it deterministically (mirrors the hostnameFn seam);
// production uses time.Since.
var sinceFn = time.Since

// AuthHostResolver resolves a service host, returning the fallback verbatim when
// discovery is disabled or fails. *libsd.Manager satisfies this contract.
type AuthHostResolver interface {
	Resolve(ctx context.Context, name, fallback string) (string, error)
}

// ResolveAuthHost picks the plugin-auth host to feed into the auth client.
// It only resolves when auth is enabled (no point resolving a downstream we
// won't call) and always degrades to the static host on resolve error so a
// discovery outage never fails boot.
//
// It passes an EMPTY fallback to r.Resolve so the return distinguishes a
// consul-resolved host from a fell-back one: on error libsd returns the error
// rather than swallowing it into the fallback, letting this function label the
// resolve outcome (resolved/fallback/error) for the metric. The returned host is
// byte-identical to feeding the static host as the fallback: on success it is the
// same consul value scheme-normalized; on error it is the static host verbatim.
func ResolveAuthHost(ctx context.Context, r AuthHostResolver, authEnabled bool, staticHost string, recorder MetricsRecorder) string {
	if !authEnabled {
		return staticHost
	}

	start := time.Now()
	resolved, err := r.Resolve(ctx, authServiceName, "")
	durationMs := sinceFn(start).Milliseconds()

	var (
		authHost string
		result   string
	)

	switch {
	case err == nil:
		result = ResultResolved
		authHost = withFallbackScheme(resolved, staticHost)
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

// withFallbackScheme returns resolved unchanged if it already carries a scheme
// (contains "://"); otherwise, if staticHost carries a scheme, it prepends that
// scheme to resolved so the auth client can reach it over the intended protocol.
// Discovery returns a bare host:port; the static fallback carries the scheme.
func withFallbackScheme(resolved, staticHost string) string {
	if strings.Contains(resolved, "://") {
		return resolved
	}

	if i := strings.Index(staticHost, "://"); i != -1 {
		return staticHost[:i+len("://")] + resolved
	}

	return resolved
}
