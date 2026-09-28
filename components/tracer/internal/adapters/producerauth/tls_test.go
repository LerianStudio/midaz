// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package producerauth_test

import (
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func certRegistry(t *testing.T) *producerauth.Registry {
	t.Helper()

	registry, err := producerauth.ParsePlatformProducers(
		`[{"service":"ledger","certUri":"` + ledgerCertURI + `"},{"service":"ledger","certUri":"` + ledgerCertURI + `-rotated"}]`,
	)
	require.NoError(t, err)

	return registry
}

func verifiedState(t *testing.T, identities ...string) *tls.ConnectionState {
	t.Helper()

	leaf := &x509.Certificate{Raw: []byte("verified-leaf")}

	for _, identity := range identities {
		uri, err := url.Parse(identity)
		require.NoError(t, err)

		leaf.URIs = append(leaf.URIs, uri)
	}

	return &tls.ConnectionState{HandshakeComplete: true, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
}

func TestByTLSRequiresVerifiedUnambiguousIdentity(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		state func(*testing.T) *tls.ConnectionState
	}{
		{"plaintext or mesh header", func(*testing.T) *tls.ConnectionState { return nil }},
		{"unverified certificate", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t, ledgerCertURI)
			s.VerifiedChains = nil

			return s
		}},
		{"unfinished handshake", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t, ledgerCertURI)
			s.HandshakeComplete = false

			return s
		}},
		{"different verified leaf", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t, ledgerCertURI)
			s.PeerCertificates = []*x509.Certificate{{Raw: []byte("other")}}

			return s
		}},
		{"nil peer leaf", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t, ledgerCertURI)
			s.PeerCertificates = []*x509.Certificate{nil}

			return s
		}},
		{"no peer", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t, ledgerCertURI)
			s.PeerCertificates = nil

			return s
		}},
		{"empty chain", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t, ledgerCertURI)
			s.VerifiedChains = [][]*x509.Certificate{{}}

			return s
		}},
		{"nil chain leaf", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t, ledgerCertURI)
			s.VerifiedChains = [][]*x509.Certificate{{nil}}

			return s
		}},
		{"unregistered identity", func(t *testing.T) *tls.ConnectionState { return verifiedState(t, ledgerCertURI+"-other") }},
		{"DNS and common name are not identity", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t)
			s.PeerCertificates[0].DNSNames = []string{ledgerCertURI}
			s.PeerCertificates[0].Subject.CommonName = ledgerCertURI

			return s
		}},
		{"multiple URIs", func(t *testing.T) *tls.ConnectionState {
			return verifiedState(t, ledgerCertURI, ledgerCertURI+"-rotated")
		}},
		{"nil URI", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t)
			s.PeerCertificates[0].URIs = []*url.URL{nil}

			return s
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			producer, err := certRegistry(t).ByTLS(tc.state(t))
			require.ErrorIs(t, err, constant.ErrInsufficientPrivileges)
			require.Zero(t, producer)
		})
	}
}

func TestByTLSResolvesMappedURIsIncludingRotation(t *testing.T) {
	t.Parallel()

	registry := certRegistry(t)

	for _, uri := range []string{ledgerCertURI, ledgerCertURI + "-rotated"} {
		producer, err := registry.ByTLS(verifiedState(t, uri))
		require.NoError(t, err)
		require.Equal(t, producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaCert}, producer)
	}
}

func TestByTLSWithoutCertificateMappingsIsUnavailable(t *testing.T) {
	t.Parallel()

	var absent *producerauth.Registry

	producer, err := absent.ByTLS(verifiedState(t, ledgerCertURI))
	require.ErrorIs(t, err, constant.ErrContextPolicyUnavailable)
	require.Zero(t, producer)

	tokenOnly, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","clientId":"` + ledgerClientID + `"}]`)
	require.NoError(t, err)

	producer, err = tokenOnly.ByTLS(verifiedState(t, ledgerCertURI))
	require.ErrorIs(t, err, constant.ErrContextPolicyUnavailable)
	require.Zero(t, producer)
}

func TestHasCertificateMappings(t *testing.T) {
	t.Parallel()

	var absent *producerauth.Registry

	require.False(t, absent.HasCertificateMappings())

	tokenOnly, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","clientId":"` + ledgerClientID + `"}]`)
	require.NoError(t, err)
	require.False(t, tokenOnly.HasCertificateMappings())
	require.True(t, certRegistry(t).HasCertificateMappings())
}
