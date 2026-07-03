// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import "context"

// authHostResolver resolves a service host, returning the fallback verbatim when
// discovery is disabled or fails. *libsd.Manager satisfies this contract.
type authHostResolver interface {
	Resolve(ctx context.Context, name, fallback string) (string, error)
}

// resolveAuthHost picks the plugin-auth host to feed into the auth client.
// It only resolves when auth is enabled (no point resolving a downstream we
// won't call) and always degrades to the static host on resolve error so a
// discovery outage never fails boot.
func resolveAuthHost(ctx context.Context, r authHostResolver, authEnabled bool, staticHost string) string {
	if !authEnabled {
		return staticHost
	}

	if resolved, err := r.Resolve(ctx, "plugin-auth", staticHost); err == nil {
		return resolved
	}

	return staticHost
}
