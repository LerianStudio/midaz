// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const producerCertURI = "spiffe://example.test/service/ledger"

func certRegistry(t *testing.T) *producerauth.Registry {
	t.Helper()

	reg, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","certUri":"` + producerCertURI + `"}]`)
	require.NoError(t, err)

	return reg
}

func verifiedTLSInfo(t *testing.T, uris ...string) credentials.TLSInfo {
	t.Helper()

	parsed := make([]*url.URL, 0, len(uris))

	for _, raw := range uris {
		uri, err := url.Parse(raw)
		require.NoError(t, err)

		parsed = append(parsed, uri)
	}

	leaf := &x509.Certificate{Raw: []byte("leaf"), URIs: parsed}

	return credentials.TLSInfo{State: tls.ConnectionState{HandshakeComplete: true, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}}
}

func TestIdentityInterceptorIgnoresForgedMetadata(t *testing.T) {
	t.Parallel()

	reg := certRegistry(t)
	_, plaintextAuth, err := insecure.NewCredentials().ServerHandshake(nil)
	require.NoError(t, err)

	uri, err := url.Parse(producerCertURI)
	require.NoError(t, err)

	unverifiedLeaf := &x509.Certificate{Raw: []byte("leaf"), URIs: []*url.URL{uri}}

	for _, tc := range []struct {
		name string
		auth credentials.AuthInfo
		want codes.Code
	}{
		{"missing auth", nil, codes.PermissionDenied},
		{"plaintext", plaintextAuth, codes.PermissionDenied},
		{"unverified cert", credentials.TLSInfo{State: tls.ConnectionState{HandshakeComplete: true, PeerCertificates: []*x509.Certificate{unverifiedLeaf}}}, codes.PermissionDenied},
		{"cert without URI", verifiedTLSInfo(t), codes.PermissionDenied},
		{"cert with two URIs", verifiedTLSInfo(t, producerCertURI, "spiffe://example.test/service/other"), codes.PermissionDenied},
		{"unknown URI", verifiedTLSInfo(t, "spiffe://example.test/service/unknown"), codes.PermissionDenied},
		{"mapped URI", verifiedTLSInfo(t, producerCertURI), codes.OK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-integration-id", "forged", "x-forwarded-client-cert", producerCertURI))
			ctx = tmcore.ContextWithTenantID(ctx, "tenant-a")
			ctx = peer.NewContext(ctx, &peer.Peer{AuthInfo: tc.auth})

			called := false

			_, err := IdentityUnaryInterceptor(reg)(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
				called = true

				identity, ok := contextutil.GetIntegrationIdentity(ctx)
				require.True(t, ok)
				require.Equal(t, contextutil.IntegrationIdentity{ID: producerauth.ServiceLedger}, identity)

				producer, ok := producerauth.ProducerFromContext(ctx)
				require.True(t, ok)
				require.Equal(t, producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaCert}, producer)
				require.Equal(t, "tenant-a", tmcore.GetTenantIDContext(ctx))

				return nil, nil
			})
			require.Equal(t, tc.want, status.Code(err))
			require.Equal(t, tc.want == codes.OK, called)

			if tc.want == codes.PermissionDenied {
				require.Equal(t, constant.ErrInsufficientPrivileges.Error(), status.Convert(err).Message())
			}
		})
	}
}

func TestIdentityInterceptorRejectsUnboundCertificateForCompletion(t *testing.T) {
	t.Parallel()

	ctx := peer.NewContext(t.Context(), &peer.Peer{AuthInfo: verifiedTLSInfo(t, "spiffe://example.test/unbound")})

	_, err := IdentityUnaryInterceptor(certRegistry(t))(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/reservation.v1.ReservationService/ReleaseByTransaction"}, func(context.Context, any) (any, error) {
		t.Fatal("unbound certificate reached completion")

		return nil, nil
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestIdentityInterceptorWithoutCertificateMappingsIsUnavailable(t *testing.T) {
	t.Parallel()

	tokenOnly, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","clientId":"ledger-m2m"}]`)
	require.NoError(t, err)

	for name, reg := range map[string]*producerauth.Registry{"nil registry": nil, "token-only registry": tokenOnly} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := peer.NewContext(t.Context(), &peer.Peer{AuthInfo: verifiedTLSInfo(t, producerCertURI)})

			_, err := IdentityUnaryInterceptor(reg)(ctx, nil, &grpc.UnaryServerInfo{}, func(context.Context, any) (any, error) {
				t.Fatal("unresolvable producer reached the handler")

				return nil, nil
			})
			require.Equal(t, codes.Unavailable, status.Code(err))
			require.Equal(t, constant.ErrContextPolicyUnavailable.Error(), status.Convert(err).Message())
		})
	}
}
