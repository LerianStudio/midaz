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
// rotating service identities to the same integration and asset namespace.
type Binding struct {
	URI            string
	IntegrationID  string
	AssetNamespace string
}

// Resolver holds an immutable allowlist shared by HTTP and gRPC adapters.
type Resolver struct {
	identities map[string]contextutil.IntegrationIdentity
}

// NewResolver rejects conflicting ownership rather than selecting an arbitrary
// namespace. Trust roots and certificate verification belong to the TLS listener.
func NewResolver(bindings []Binding, maxNamespaceBytes int) (*Resolver, error) {
	if len(bindings) == 0 || maxNamespaceBytes <= 0 {
		return nil, constant.ErrContextPolicyUnavailable
	}

	resolver := &Resolver{identities: make(map[string]contextutil.IntegrationIdentity, len(bindings))}
	namespaces := make(map[string]string)
	integrations := make(map[string]string)

	for _, binding := range bindings {
		identity := contextutil.IntegrationIdentity{ID: binding.IntegrationID, AssetNamespace: binding.AssetNamespace}
		if !identity.Valid() || len(identity.AssetNamespace) > maxNamespaceBytes || !validURI(binding.URI) {
			return nil, constant.ErrContextPolicyUnavailable
		}

		if _, exists := resolver.identities[binding.URI]; exists {
			return nil, constant.ErrContextPolicyUnavailable
		}

		if owner, exists := namespaces[identity.AssetNamespace]; exists && owner != identity.ID {
			return nil, constant.ErrContextPolicyUnavailable
		}

		if namespace, exists := integrations[identity.ID]; exists && namespace != identity.AssetNamespace {
			return nil, constant.ErrContextPolicyUnavailable
		}

		resolver.identities[binding.URI] = identity
		namespaces[identity.AssetNamespace] = identity.ID
		integrations[identity.ID] = identity.AssetNamespace
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
