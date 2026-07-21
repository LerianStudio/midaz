// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package tracer

// End-to-end proof that the single-tenant reservation seam works over mutual
// TLS on BOTH transports the ledger speaks: gRPC (TRACER_TRANSPORT=grpc) and
// REST (TRACER_TRANSPORT=rest). It is the Task 1.3.3 capstone for Epic 1.3
// (Phase 1): with a CA-signed client cert the ledger client drives
// reserve->confirm to success; without one the connection is rejected at the
// TLS layer and the RPC never reaches the service.
//
// What is REAL on each side:
//
//   - LEDGER (the side under test): the production *TracerGRPCClient and
//     *TracerClient, constructed through their real transport-credential seams
//     (WithGRPCDialOptions(credentials.NewTLS(...)) and WithTLSConfig(...)) with
//     the same client *tls.Config the composition root's buildSeamClientTLSConfig
//     produces in mtls mode — client cert presented, tracer server cert verified
//     against the CA, ServerName pinned. So the real ledger mTLS dial path runs.
//   - TRACER (reconstructed): a grpc.NewServer with
//     grpc.Creds(credentials.NewTLS(serverTLS)) and an http.Server with the same
//     ClientAuth=RequireAndVerifyClientCert posture Task 1.3.1 wires on the
//     tracer. Go's internal/ rule walls components/tracer/internal/... (the real
//     gRPC server adapter, REST handler, and testutil mTLS fixture) off from this
//     ledger test package, exactly as documented in contract_test.go — so the
//     server side and the cert fixture are reconstructed here from importable
//     pieces. The load-bearing half of THIS test is the transport security, which
//     is real on both ends: a real TLS handshake over a real loopback socket.
//
// No Docker is required: loopback sockets plus a deterministic cert fixture
// (fixed 2020->2100 validity window, no time.Now) make the test hermetic. It is
// nonetheless tagged integration because it binds real sockets and performs real
// TLS handshakes, matching the tracer-side TestReservationMTLS.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// The deterministic ids the reconstructed tracer returns and the ledger client
// round-trips are the package-shared fixedTransactionID / fixedReservationID
// (client_test.go) — fixed literals, no uuid.New / time.Now — so the assertions
// stay exact and there is one source of truth for the seam's test ids.

// TestSeamMTLS drives the real ledger reservation clients over a real mutual-TLS
// handshake against a reconstructed tracer, on both transports, and proves an
// uncertified client cannot reach the seam.
func TestSeamMTLS(t *testing.T) {
	fixture := generateSeamMTLSFixture(t)

	serverTLS := serverMTLSConfig(t, fixture)
	clientTLS := clientMTLSConfig(t, fixture)

	t.Run("gRPC reserve->confirm succeeds with a CA-signed client cert", func(t *testing.T) {
		addr := startGRPCSeamServer(t, serverTLS)

		client, err := NewTracerGRPCClient(addr,
			WithGRPCOperationTimeout(5*time.Second),
			WithGRPCDialOptions(grpc.WithTransportCredentials(credentials.NewTLS(clientTLS))))
		require.NoError(t, err)

		t.Cleanup(func() { _ = client.Close() })

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := client.Reserve(ctx, ReserveRequest{TransactionID: fixedTransactionID})
		require.NoError(t, err, "CA-signed client must complete the Reserve RPC over mTLS")
		require.NotNil(t, result)
		require.False(t, result.Denied)
		require.Equal(t, fixedTransactionID, result.TransactionID)
		require.Equal(t, []uuid.UUID{fixedReservationID}, result.ReservationIDs)

		require.NoError(t, client.Confirm(ctx, fixedReservationID),
			"confirm over the secured seam must succeed")
	})

	t.Run("gRPC reserve is rejected without a valid client cert", func(t *testing.T) {
		addr := startGRPCSeamServer(t, serverTLS)

		// Verifies the server against the CA but presents NO client cert: the
		// server requires + verifies a client cert, so the handshake fails and
		// the RPC errors before reaching the service.
		client, err := NewTracerGRPCClient(addr,
			WithGRPCDialOptions(grpc.WithTransportCredentials(credentials.NewTLS(serverOnlyTLSConfig(fixture)))))
		require.NoError(t, err)

		t.Cleanup(func() { _ = client.Close() })

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err = client.Reserve(ctx, ReserveRequest{TransactionID: fixedTransactionID})
		require.Error(t, err, "client without a verified cert must be rejected at the TLS layer")
	})

	t.Run("REST reserve->confirm succeeds over mTLS", func(t *testing.T) {
		baseURL := startRESTSeamServer(t, serverTLS)

		client, err := NewTracerClient(baseURL, WithTLSConfig(clientTLS))
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := client.Reserve(ctx, ReserveRequest{
			TransactionID:        fixedTransactionID,
			RequestID:            fixedReservationID.String(),
			Amount:               "100",
			Currency:             "USD",
			Account:              ReserveAccount{AccountID: fixedReservationID.String()},
			TransactionTimestamp: "2020-01-02T00:00:00Z",
		})
		require.NoError(t, err, "CA-signed client must complete the reserve POST over mTLS")
		require.NotNil(t, result)
		require.False(t, result.Denied)
		require.Equal(t, []uuid.UUID{fixedReservationID}, result.ReservationIDs)

		require.NoError(t, client.Confirm(ctx, fixedReservationID),
			"confirm over the secured REST seam must succeed")
	})

	t.Run("REST reserve is rejected without a valid client cert", func(t *testing.T) {
		baseURL := startRESTSeamServer(t, serverTLS)

		client, err := NewTracerClient(baseURL, WithTLSConfig(serverOnlyTLSConfig(fixture)))
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err = client.Reserve(ctx, ReserveRequest{
			TransactionID:        fixedTransactionID,
			RequestID:            fixedReservationID.String(),
			Amount:               "100",
			Currency:             "USD",
			Account:              ReserveAccount{AccountID: fixedReservationID.String()},
			TransactionTimestamp: "2020-01-02T00:00:00Z",
		})
		require.Error(t, err, "client without a cert must be rejected by the mTLS REST seam")
	})
}

// reserveSeamServer is the reconstructed tracer gRPC reservation service. It
// returns deterministic ids so the ledger client's wire round-trip is asserted
// exactly; the business logic is exercised by the tracer's own suites, so this
// stub only needs to prove a verified RPC reaches the service.
type reserveSeamServer struct {
	reservationv1.UnimplementedReservationServiceServer
}

func (reserveSeamServer) Reserve(_ context.Context, req *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
	return &reservationv1.ReserveResult{
		TransactionId:  req.GetTransactionId(),
		Denied:         false,
		ReservationIds: []string{fixedReservationID.String()},
	}, nil
}

func (reserveSeamServer) ConfirmById(_ context.Context, _ *reservationv1.ConfirmByIdRequest) (*reservationv1.ConfirmByIdResponse, error) {
	return &reservationv1.ConfirmByIdResponse{}, nil
}

// startGRPCSeamServer stands up a gRPC server secured by serverTLS
// (RequireAndVerifyClientCert), registers the reconstructed reservation service,
// and returns its loopback host:port. Stopped on cleanup.
func startGRPCSeamServer(t *testing.T, serverTLS *tls.Config) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)))
	reservationv1.RegisterReservationServiceServer(server, reserveSeamServer{})

	go func() { _ = server.Serve(listener) }()

	t.Cleanup(server.Stop)

	return listener.Addr().String()
}

// startRESTSeamServer runs an http.Server with the same mTLS posture the tracer
// enforces (RequireAndVerifyClientCert) serving the reservation REST routes the
// ledger client calls (POST /v1/reservations and the per-id confirm path). It
// returns the https base URL. Stopped on cleanup.
func startRESTSeamServer(t *testing.T, serverTLS *tls.Config) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/reservations", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ReserveResult{
			TransactionID:  fixedTransactionID,
			Denied:         false,
			ReservationIDs: []uuid.UUID{fixedReservationID},
		})
	})
	// The per-id confirm path: any /v1/reservations/{id}/confirm returns 200,
	// matching the tracer's idempotent confirm contract.
	mux.HandleFunc("/v1/reservations/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/confirm") || strings.HasSuffix(r.URL.Path, "/release") {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	srv := &http.Server{
		Handler:           mux,
		TLSConfig:         serverTLS,
		ReadHeaderTimeout: 5 * time.Second,
	}

	tlsListener := tls.NewListener(listener, serverTLS)
	go func() { _ = srv.Serve(tlsListener) }()

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	})

	return "https://" + listener.Addr().String()
}

// serverMTLSConfig builds the tracer-side server *tls.Config: presents the
// server leaf and requires + verifies a client cert against the CA — the exact
// posture Task 1.3.1's buildSeamTLSConfig wires on the tracer in mtls mode.
func serverMTLSConfig(t *testing.T, fixture seamMTLSFixture) *tls.Config {
	t.Helper()

	serverCert, err := tls.X509KeyPair(fixture.serverCertPEM, fixture.serverKeyPEM)
	require.NoError(t, err)

	caPool := x509.NewCertPool()
	require.True(t, caPool.AppendCertsFromPEM(fixture.caCertPEM))

	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}
}

// clientMTLSConfig builds the ledger-side client *tls.Config: presents the
// client leaf and verifies the tracer's server leaf against the CA, ServerName
// pinned to "localhost" — the exact config buildSeamClientTLSConfig produces in
// mtls mode and threads into both transports.
func clientMTLSConfig(t *testing.T, fixture seamMTLSFixture) *tls.Config {
	t.Helper()

	clientCert, err := tls.X509KeyPair(fixture.clientCertPEM, fixture.clientKeyPEM)
	require.NoError(t, err)

	caPool := x509.NewCertPool()
	require.True(t, caPool.AppendCertsFromPEM(fixture.caCertPEM))

	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
		ServerName:   "localhost",
	}
}

// serverOnlyTLSConfig builds a client config that verifies the server but
// presents NO client certificate, modeling an uncertified caller that the
// RequireAndVerifyClientCert seam must reject.
func serverOnlyTLSConfig(fixture seamMTLSFixture) *tls.Config {
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(fixture.caCertPEM)

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    caPool,
		ServerName: "localhost",
	}
}

// seamMTLSFixture holds PEM-encoded cert material for the end-to-end handshake:
// a CA that signs both the server and the client leaf. The tracer's
// testutil.GenerateMTLSFixture is unreachable here (internal/ wall), so this is
// a local equivalent — same fixed validity window, no time.Now.
type seamMTLSFixture struct {
	caCertPEM     []byte
	serverCertPEM []byte
	serverKeyPEM  []byte
	clientCertPEM []byte
	clientKeyPEM  []byte
}

// seamFixtureNotBefore / seamFixtureNotAfter bound the validity of every fixture
// certificate. FIXED constants (no time.Now in tests), chosen to straddle any
// realistic test wall-clock so the certs are valid during the real TLS
// handshake crypto/tls runs against the system clock while staying deterministic.
var (
	seamFixtureNotBefore = time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	seamFixtureNotAfter  = time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)
)

// generateSeamMTLSFixture builds a self-signed CA plus a server leaf (with
// localhost/127.0.0.1 SANs) and a client leaf, both signed by it. ECDSA P-256
// keeps generation fast; the validity window is fixed.
func generateSeamMTLSFixture(t *testing.T) seamMTLSFixture {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "midaz-ledger-seam-e2e-ca"},
		NotBefore:             seamFixtureNotBefore,
		NotAfter:              seamFixtureNotAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	serverCertPEM, serverKeyPEM := signSeamLeaf(t, caCert, caKey, seamLeafSpec{
		commonName:  "tracer-seam-server",
		serial:      2,
		serverAuth:  true,
		dnsNames:    []string{"localhost"},
		ipAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	})

	clientCertPEM, clientKeyPEM := signSeamLeaf(t, caCert, caKey, seamLeafSpec{
		commonName: "ledger-seam-client",
		serial:     3,
		clientAuth: true,
	})

	return seamMTLSFixture{
		caCertPEM:     pemEncodeSeam("CERTIFICATE", caDER),
		serverCertPEM: serverCertPEM,
		serverKeyPEM:  serverKeyPEM,
		clientCertPEM: clientCertPEM,
		clientKeyPEM:  clientKeyPEM,
	}
}

type seamLeafSpec struct {
	commonName  string
	serial      int64
	serverAuth  bool
	clientAuth  bool
	dnsNames    []string
	ipAddresses []net.IP
}

// signSeamLeaf creates an ECDSA leaf signed by the given CA and returns its cert
// and key PEM blocks.
func signSeamLeaf(t *testing.T, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, spec seamLeafSpec) (certPEM, keyPEM []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(spec.serial),
		Subject:      pkix.Name{CommonName: spec.commonName},
		NotBefore:    seamFixtureNotBefore,
		NotAfter:     seamFixtureNotAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		DNSNames:     spec.dnsNames,
		IPAddresses:  spec.ipAddresses,
	}

	if spec.serverAuth {
		template.ExtKeyUsage = append(template.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
	}

	if spec.clientAuth {
		template.ExtKeyUsage = append(template.ExtKeyUsage, x509.ExtKeyUsageClientAuth)
	}

	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	return pemEncodeSeam("CERTIFICATE", der), pemEncodeSeam("EC PRIVATE KEY", keyDER)
}

// pemEncodeSeam wraps DER bytes in a PEM block of the given type.
func pemEncodeSeam(blockType string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
}
