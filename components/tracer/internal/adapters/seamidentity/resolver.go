// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package seamidentity maps verified service certificates to configured producer
// identities. It does not trust forwarded certificate headers or request bodies.
package seamidentity

import (
	"context"
	"crypto/tls"
	"net/url"
	"strings"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Binding is operator-owned configuration, never a reservation DTO. URI must
// exactly match the client leaf's sole URI SAN. Multiple bindings may map
// rotating service identities to the same integration.
type Binding struct {
	URI           string    `json:"uri"`
	IntegrationID string    `json:"integrationId"`
	Purposes      []Purpose `json:"purposes"`
}

// Purpose names the access a binding grants. Every binding must declare it
// explicitly so a future purpose cannot be granted by omission.
type Purpose string

const PurposeReserve Purpose = "reserve"

// Resolver holds an immutable allowlist shared by HTTP and gRPC adapters.
type Resolver struct {
	identities map[string]contextutil.IntegrationIdentity
}

// NewResolver rejects duplicate or malformed bindings rather than selecting an
// arbitrary identity. Trust roots and certificate verification belong to the
// TLS listener.
func NewResolver(bindings []Binding) (*Resolver, error) {
	if len(bindings) == 0 {
		return nil, constant.ErrContextPolicyUnavailable
	}

	resolver := &Resolver{identities: make(map[string]contextutil.IntegrationIdentity, len(bindings))}

	for _, binding := range bindings {
		if len(binding.Purposes) != 1 || binding.Purposes[0] != PurposeReserve {
			return nil, constant.ErrContextPolicyUnavailable
		}

		identity := contextutil.IntegrationIdentity{ID: binding.IntegrationID}
		if !identity.Valid() || !validURI(binding.URI) {
			return nil, constant.ErrContextPolicyUnavailable
		}

		if _, exists := resolver.identities[binding.URI]; exists {
			return nil, constant.ErrContextPolicyUnavailable
		}

		resolver.identities[binding.URI] = identity
	}

	return resolver, nil
}

func validURI(value string) bool {
	uri, err := url.Parse(value)

	return err == nil && uri.IsAbs() && uri.Host != "" && uri.User == nil && uri.Opaque == "" &&
		uri.RawQuery == "" && !uri.ForceQuery && uri.Fragment == "" && !strings.ContainsAny(value, "* \t\r\n") && uri.String() == value
}

// ResolveTLS accepts only the verified leaf of a completed native TLS handshake.
// A trusted CA alone does not grant producer access. Common names, DNS SANs,
// certificate chains without a peer match, and ambiguous URI SANs are rejected.
// Mesh-terminated plaintext is deliberately unsupported: a separate verified
// workload-identity adapter is required before enabling context Reserve there.
func (r *Resolver) ResolveTLS(ctx context.Context, state *tls.ConnectionState) (contextutil.IntegrationIdentity, error) {
	if err := ctx.Err(); err != nil {
		return contextutil.IntegrationIdentity{}, err
	}

	if r == nil || len(r.identities) == 0 {
		return contextutil.IntegrationIdentity{}, constant.ErrContextPolicyUnavailable
	}

	if state == nil || !state.HandshakeComplete || len(state.PeerCertificates) == 0 ||
		state.PeerCertificates[0] == nil || len(state.VerifiedChains) == 0 || len(state.VerifiedChains[0]) == 0 ||
		state.VerifiedChains[0][0] == nil || !state.PeerCertificates[0].Equal(state.VerifiedChains[0][0]) {
		return contextutil.IntegrationIdentity{}, constant.ErrInsufficientPrivileges
	}

	leaf := state.VerifiedChains[0][0]
	if len(leaf.URIs) != 1 || leaf.URIs[0] == nil {
		return contextutil.IntegrationIdentity{}, constant.ErrInsufficientPrivileges
	}

	identity, ok := r.identities[leaf.URIs[0].String()]
	if !ok {
		return contextutil.IntegrationIdentity{}, constant.ErrInsufficientPrivileges
	}

	return identity, nil
}
