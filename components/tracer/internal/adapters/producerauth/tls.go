// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package producerauth

import (
	"crypto/tls"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ByTLS resolves the producer from the verified leaf of a completed native TLS
// handshake. A trusted CA alone does not grant producer access: common names,
// DNS SANs, chains whose leaf is not the presented peer, and certificates with
// other than exactly one URI SAN are rejected with
// constant.ErrInsufficientPrivileges, as is a URI absent from the registry.
// A registry without certificate mappings reports
// constant.ErrContextPolicyUnavailable. Mesh-terminated plaintext carries no
// verifiable identity here and is rejected.
func (r *Registry) ByTLS(state *tls.ConnectionState) (Producer, error) {
	if r == nil || len(r.byCertURI) == 0 {
		return Producer{}, constant.ErrContextPolicyUnavailable
	}

	if state == nil || !state.HandshakeComplete || len(state.PeerCertificates) == 0 ||
		state.PeerCertificates[0] == nil || len(state.VerifiedChains) == 0 || len(state.VerifiedChains[0]) == 0 ||
		state.VerifiedChains[0][0] == nil || !state.PeerCertificates[0].Equal(state.VerifiedChains[0][0]) {
		return Producer{}, constant.ErrInsufficientPrivileges
	}

	leaf := state.VerifiedChains[0][0]
	if len(leaf.URIs) != 1 || leaf.URIs[0] == nil {
		return Producer{}, constant.ErrInsufficientPrivileges
	}

	service, ok := r.byCertURI[leaf.URIs[0].String()]
	if !ok {
		return Producer{}, constant.ErrInsufficientPrivileges
	}

	return Producer{Service: service, Via: ViaCert}, nil
}
