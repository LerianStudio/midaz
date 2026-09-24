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

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamidentity"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
)

func TestIdentityInterceptorIgnoresForgedMetadata(t *testing.T) {
	t.Parallel()
	uri, err := url.Parse("spiffe://example.test/service/producer")
	require.NoError(t, err)
	resolver, err := seamidentity.NewResolver([]seamidentity.Binding{{URI: uri.String(), IntegrationID: "producer", AssetNamespace: "assets"}}, 256)
	require.NoError(t, err)
	_, plaintextAuth, err := insecure.NewCredentials().ServerHandshake(nil)
	require.NoError(t, err)
	leaf := &x509.Certificate{Raw: []byte("leaf"), URIs: []*url.URL{uri}}
	for _, tc := range []struct {
		name string
		auth credentials.AuthInfo
		want codes.Code
	}{
		{"missing auth", nil, codes.PermissionDenied},
		{"plaintext", plaintextAuth, codes.PermissionDenied},
		{"unverified cert", credentials.TLSInfo{State: tls.ConnectionState{HandshakeComplete: true, PeerCertificates: []*x509.Certificate{leaf}}}, codes.PermissionDenied},
		{"verified cert", credentials.TLSInfo{State: tls.ConnectionState{HandshakeComplete: true, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}}, codes.OK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-integration-id", "forged", "x-asset-namespace", "forged", "x-forwarded-client-cert", uri.String()))
			ctx = tmcore.ContextWithTenantID(ctx, "tenant-a")
			ctx = peer.NewContext(ctx, &peer.Peer{AuthInfo: tc.auth})
			called := false
			_, err := IdentityUnaryInterceptor(resolver)(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
				called = true
				identity, ok := contextutil.GetIntegrationIdentity(ctx)
				require.True(t, ok)
				require.Equal(t, contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "assets"}, identity)
				require.Equal(t, "tenant-a", tmcore.GetTenantIDContext(ctx))
				return nil, nil
			})
			require.Equal(t, tc.want, status.Code(err))
			require.Equal(t, tc.want == codes.OK, called)
		})
	}
}
