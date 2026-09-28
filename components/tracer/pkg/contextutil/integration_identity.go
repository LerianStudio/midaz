// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package contextutil

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"
)

// IntegrationIdentity identifies a verified producer. It is separate from the
// administrative user/API-key principal and must never be read from a
// transaction body or an unverified header.
type IntegrationIdentity struct {
	ID string
}

type integrationIdentityKey struct{}

// maxIntegrationIDBytes matches the policy storage bound for integration IDs.
const maxIntegrationIDBytes = 256

// Valid checks that the ID is canonical: non-empty, valid UTF-8, free of
// surrounding whitespace and control characters, and within the storage bound.
func (i IntegrationIdentity) Valid() bool {
	return i.ID != "" && len(i.ID) <= maxIntegrationIDBytes && utf8.ValidString(i.ID) &&
		strings.TrimSpace(i.ID) == i.ID && !strings.ContainsFunc(i.ID, unicode.IsControl)
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
