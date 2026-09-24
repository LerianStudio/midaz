// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package contextutil

import (
	"context"
	"strings"
	"unicode/utf8"
)

// IntegrationIdentity identifies a verified producer and its configured asset
// namespace. It is separate from the administrative user/API-key principal.
// Neither field may be read from a transaction body or an unverified header.
type IntegrationIdentity struct {
	ID             string
	AssetNamespace string
}

type integrationIdentityKey struct{}

// Valid checks canonical values; the transport registry additionally enforces
// the configured namespace byte bound. The ID bound matches policy storage.
func (i IntegrationIdentity) Valid() bool {
	for _, value := range []string{i.ID, i.AssetNamespace} {
		if value == "" || !utf8.ValidString(value) || strings.TrimSpace(value) != value || strings.ContainsRune(value, 0) {
			return false
		}
	}

	return len(i.ID) <= 256
}

// WithIntegrationIdentity is for authenticated transport adapters only. It
// preserves tenant/pool/cancellation values already attached to the context.
func WithIntegrationIdentity(ctx context.Context, identity IntegrationIdentity) context.Context {
	return context.WithValue(ctx, integrationIdentityKey{}, identity)
}

// GetIntegrationIdentity never falls back to a user principal or tenant header.
func GetIntegrationIdentity(ctx context.Context) (IntegrationIdentity, bool) {
	if ctx == nil {
		return IntegrationIdentity{}, false
	}

	identity, ok := ctx.Value(integrationIdentityKey{}).(IntegrationIdentity)
	if !ok || !identity.Valid() {
		return IntegrationIdentity{}, false
	}

	return identity, true
}
