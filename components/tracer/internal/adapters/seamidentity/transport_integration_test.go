// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package seamidentity_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	grpcin "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in"
	httpin "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamidentity"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
)

// Real loopback TLS handshakes prove that both adapters read the certificate
// actually verified by the server, rather than a manually injected context.
func TestProducerIdentityMTLS(t *testing.T) {
	for _, tc := range []struct {
		name    string
		uris    []string
		allowed bool
	}{
		{"registered producer", []string{producerURI}, true},
		{"trusted CA but unknown producer", []string{producerURI + "-other"}, false},
		{"trusted CA but no URI", nil, false},
		{"ambiguous certificate", []string{producerURI, producerURI + "-other"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serverTLS, clientTLS := identityTLSConfigs(t, tc.uris)
			resolver, err := seamidentity.NewResolver(bindings(), 256)
			require.NoError(t, err)
			t.Run("HTTP", func(t *testing.T) { checkHTTPIdentity(t, resolver, serverTLS.Clone(), clientTLS.Clone(), tc.allowed) })
			t.Run("gRPC", func(t *testing.T) { checkGRPCIdentity(t, resolver, serverTLS.Clone(), clientTLS.Clone(), tc.allowed) })
		})
	}
}

func identityTLSConfigs(t *testing.T, uris []string) (*tls.Config, *tls.Config) {
	t.Helper()
	fixture := testutil.GenerateMTLSFixture(t, uris...)
	server, err := tls.X509KeyPair(fixture.ServerCertPEM, fixture.ServerKeyPEM)
	require.NoError(t, err)
	client, err := tls.X509KeyPair(fixture.ClientCertPEM, fixture.ClientKeyPEM)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(fixture.CACertPEM))
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{server}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool},
		&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{client}, RootCAs: pool, ServerName: "localhost"}
}

func identityListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func assertCapturedIdentity(t *testing.T, identities <-chan contextutil.IntegrationIdentity, allowed bool) {
	t.Helper()
	if !allowed {
		require.Empty(t, identities)
		return
	}
	select {
	case identity := <-identities:
		require.Equal(t, contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "assets"}, identity)
	default:
		t.Fatal("authorized handler did not receive the verified identity")
	}
}

func checkHTTPIdentity(t *testing.T, resolver *seamidentity.Resolver, serverTLS, clientTLS *tls.Config, allowed bool) {
	t.Helper()
	identities := make(chan contextutil.IntegrationIdentity, 1)
	app := fiber.New()
	app.Post("/v1/reservations", httpin.NewReservationIdentityMiddleware(resolver), func(c fiber.Ctx) error {
		identity, _ := contextutil.GetIntegrationIdentity(c.Context())
		identities <- identity
		return c.SendStatus(http.StatusNoContent)
	})
	listener := identityListener(t)
	done := make(chan error, 1)
	go func() {
		done <- app.Listener(tls.NewListener(listener, serverTLS), fiber.ListenConfig{DisableStartupMessage: true})
	}()
	t.Cleanup(func() { require.NoError(t, app.Shutdown()); require.NoError(t, <-done) })
	transport := &http.Transport{TLSClientConfig: clientTLS}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://"+listener.Addr().String()+"/v1/reservations", nil)
	require.NoError(t, err)
	req.Header.Set("X-Integration-Id", "forged")
	req.Header.Set("X-Asset-Namespace", "forged")
	req.Header.Set("X-Forwarded-Client-Cert", producerURI)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	want := http.StatusForbidden
	if allowed {
		want = http.StatusNoContent
	}
	require.Equal(t, want, resp.StatusCode)
	assertCapturedIdentity(t, identities, allowed)
}

func checkGRPCIdentity(t *testing.T, resolver *seamidentity.Resolver, serverTLS, clientTLS *tls.Config, allowed bool) {
	t.Helper()
	identities := make(chan contextutil.IntegrationIdentity, 1)
	capture := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		identity, _ := contextutil.GetIntegrationIdentity(ctx)
		identities <- identity
		return next(ctx, req)
	}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)), grpc.ChainUnaryInterceptor(grpcin.IdentityUnaryInterceptor(resolver), capture))
	healthv1.RegisterHealthServer(server, health.NewServer())
	listener := identityListener(t)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); require.NoError(t, <-done) })
	client, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("x-integration-id", "forged", "x-asset-namespace", "forged", "x-forwarded-client-cert", producerURI))
	_, err = healthv1.NewHealthClient(client).Check(ctx, &healthv1.HealthCheckRequest{})
	want := codes.PermissionDenied
	if allowed {
		want = codes.OK
	}
	require.Equal(t, want, status.Code(err))
	assertCapturedIdentity(t, identities, allowed)
}
