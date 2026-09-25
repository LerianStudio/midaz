// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package seamidentity_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamidentity"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const producerURI = "spiffe://example.test/service/producer"

func bindings() []seamidentity.Binding {
	return []seamidentity.Binding{{URI: producerURI, IntegrationID: "producer", AssetNamespace: "assets", Purposes: []seamidentity.Purpose{seamidentity.PurposeReserve}}}
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

func TestResolverRequiresVerifiedUnambiguousIdentity(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		state func(*testing.T) *tls.ConnectionState
	}{
		{"plaintext or mesh header", func(*testing.T) *tls.ConnectionState { return nil }},
		{"unverified certificate", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t, producerURI)
			s.VerifiedChains = nil
			return s
		}},
		{"unfinished handshake", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t, producerURI)
			s.HandshakeComplete = false
			return s
		}},
		{"different verified leaf", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t, producerURI)
			s.PeerCertificates = []*x509.Certificate{{Raw: []byte("other")}}
			return s
		}},
		{"no peer", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t, producerURI)
			s.PeerCertificates = nil
			return s
		}},
		{"empty chain", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t, producerURI)
			s.VerifiedChains = [][]*x509.Certificate{{}}
			return s
		}},
		{"unregistered identity", func(t *testing.T) *tls.ConnectionState { return verifiedState(t, producerURI+"-other") }},
		{"DNS and common name are not identity", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t)
			s.PeerCertificates[0].DNSNames = []string{producerURI}
			s.PeerCertificates[0].Subject.CommonName = producerURI
			return s
		}},
		{"multiple URIs", func(t *testing.T) *tls.ConnectionState { return verifiedState(t, producerURI, producerURI+"-other") }},
		{"nil URI", func(t *testing.T) *tls.ConnectionState {
			s := verifiedState(t)
			s.PeerCertificates[0].URIs = []*url.URL{nil}
			return s
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolver, err := seamidentity.NewResolver(bindings(), 256)
			require.NoError(t, err)
			identity, err := resolver.ResolveTLS(context.Background(), tc.state(t))
			require.ErrorIs(t, err, constant.ErrInsufficientPrivileges)
			require.Zero(t, identity)
		})
	}
}

func TestResolverCopiesConfigurationAndSupportsRotation(t *testing.T) {
	t.Parallel()
	config := bindings()
	config = append(config, seamidentity.Binding{URI: producerURI + "-rotated", IntegrationID: "producer", AssetNamespace: "assets", Purposes: []seamidentity.Purpose{seamidentity.PurposeReserve}})
	resolver, err := seamidentity.NewResolver(config, 256)
	require.NoError(t, err)
	config[0].IntegrationID = "attacker"
	for _, uri := range []string{producerURI, producerURI + "-rotated"} {
		identity, err := resolver.ResolveTLS(context.Background(), verifiedState(t, uri))
		require.NoError(t, err)
		require.Equal(t, contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "assets"}, identity)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = resolver.ResolveTLS(ctx, verifiedState(t, producerURI))
	require.ErrorIs(t, err, context.Canceled)
	var absent *seamidentity.Resolver
	_, err = absent.ResolveTLS(context.Background(), verifiedState(t, producerURI))
	require.ErrorIs(t, err, constant.ErrContextPolicyUnavailable)
}

func TestResolverRejectsAmbiguousConfiguration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		bindings []seamidentity.Binding
		maxText  int
	}{
		{"empty", nil, 256},
		{"missing bound", bindings(), 0},
		{"oversized namespace", bindings(), 2},
		{"duplicate URI", append(bindings(), bindings()...), 256},
		{"namespace shared across integrations", append(bindings(), seamidentity.Binding{URI: producerURI + "/other", IntegrationID: "other", AssetNamespace: "assets", Purposes: []seamidentity.Purpose{seamidentity.PurposeReserve}}), 256},
		{"integration with two namespaces", append(bindings(), seamidentity.Binding{URI: producerURI + "/other", IntegrationID: "producer", AssetNamespace: "other", Purposes: []seamidentity.Purpose{seamidentity.PurposeReserve}}), 256},
		{"relative URI", []seamidentity.Binding{{URI: "producer", IntegrationID: "producer", AssetNamespace: "assets", Purposes: []seamidentity.Purpose{seamidentity.PurposeReserve}}}, 256},
		{"wildcard URI", []seamidentity.Binding{{URI: "spiffe://*.test/producer", IntegrationID: "producer", AssetNamespace: "assets", Purposes: []seamidentity.Purpose{seamidentity.PurposeReserve}}}, 256},
		{"empty integration", []seamidentity.Binding{{URI: producerURI, AssetNamespace: "assets", Purposes: []seamidentity.Purpose{seamidentity.PurposeReserve}}}, 256},
		{"noncanonical namespace", []seamidentity.Binding{{URI: producerURI, IntegrationID: "producer", AssetNamespace: " assets", Purposes: []seamidentity.Purpose{seamidentity.PurposeReserve}}}, 256},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolver, err := seamidentity.NewResolver(tc.bindings, tc.maxText)
			require.ErrorIs(t, err, constant.ErrContextPolicyUnavailable)
			require.Nil(t, resolver)
		})
	}
}

func TestResolverSeparatesProducerPurposes(t *testing.T) {
	config := []seamidentity.Binding{
		{URI: producerURI, IntegrationID: "producer", AssetNamespace: "assets", Purposes: []seamidentity.Purpose{seamidentity.PurposeReserve}},
		{URI: producerURI + "-admin", IntegrationID: "producer", AssetNamespace: "assets", Purposes: []seamidentity.Purpose{seamidentity.PurposeAssetAdmin}},
	}
	resolver, err := seamidentity.NewResolver(config, 256)
	require.NoError(t, err)
	_, err = resolver.ResolveTLS(context.Background(), verifiedState(t, producerURI+"-admin"))
	require.ErrorIs(t, err, constant.ErrInsufficientPrivileges)
	_, err = resolver.ResolveTLSFor(context.Background(), verifiedState(t, producerURI), seamidentity.PurposeAssetAdmin)
	require.ErrorIs(t, err, constant.ErrInsufficientPrivileges)
	_, err = resolver.ResolveTLSFor(context.Background(), verifiedState(t, producerURI+"-admin"), seamidentity.PurposeAssetAdmin)
	require.NoError(t, err)
	for _, purposes := range [][]seamidentity.Purpose{nil, {"unknown"}, {seamidentity.PurposeReserve, seamidentity.PurposeReserve}} {
		config[0].Purposes = purposes
		_, err := seamidentity.NewResolver(config, 256)
		require.ErrorIs(t, err, constant.ErrContextPolicyUnavailable)
	}
}
