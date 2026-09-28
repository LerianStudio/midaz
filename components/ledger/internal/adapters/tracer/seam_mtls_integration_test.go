// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package tracer

// End-to-end proof that the single-tenant reservation seam works over TLS on
// BOTH transports the ledger speaks. gRPC (TRACER_TRANSPORT=grpc) identifies the
// ledger by its client certificate: with a CA-signed cert the client drives
// reserve->confirm to success, without one the connection is rejected at the
// TLS layer. REST (TRACER_TRANSPORT=rest) identifies the ledger by its M2M
// bearer token: the client certificate is optional and the server ignores it.
//
// What is REAL on each side:
//
//   - LEDGER (the side under test): the production *ContextGRPCClient and
//     *ContextHTTPClient, constructed through their real transport-credential seams
//     (WithGRPCDialOptions(credentials.NewTLS(...)) and WithTLSConfig(...)) with
//     the same client *tls.Config the composition root's buildSeamClientTLSConfig
//     produces in mtls mode — client cert presented, tracer server cert verified
//     against the CA, ServerName pinned. So the real ledger mTLS dial path runs.
//   - TRACER (reconstructed): a grpc.NewServer with
//     grpc.Creds(credentials.NewTLS(serverTLS)) requiring a verified client
//     cert, and an http.Server that requests no client cert and requires a
//     bearer token. Go's internal/ rule walls components/tracer/internal/...
//     (the real gRPC server adapter, REST handler, and testutil mTLS fixture)
//     off from this ledger test package, so the server side and the cert
//     fixture are reconstructed here from importable pieces. The load-bearing
//     half of THIS test is the transport security, which is real on both ends:
//     a real TLS handshake over a real loopback socket.
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
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// fixedReservationID is the deterministic id the reconstructed tracer returns,
// so the round-trip assertions stay exact.
var fixedReservationID = uuid.MustParse("22222222-2222-2222-2222-222222222222")

// seamBearerToken is the M2M token the REST subtests present.
const seamBearerToken = "seam-token"

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

		request, config := contextClientFixture(t)
		request.TransactionID = fixedTransactionID
		coordinated := &ContextGRPCClient{transport: client, config: config}
		result, err := coordinated.Reserve(ctx, request)
		require.NoError(t, err, "CA-signed client must complete the Reserve RPC over mTLS")
		require.NotNil(t, result)
		require.Equal(t, tracercontract.DecisionAllow, result.Decision)
		require.Equal(t, fixedTransactionID, result.TransactionID)
		require.Equal(t, []uuid.UUID{fixedReservationID}, result.ReservationIDs)

		completion, err := coordinated.ConfirmByTransaction(ctx, fixedTransactionID)
		require.NoError(t, err, "confirm over the secured seam must succeed")
		require.Equal(t, "CONFIRMED", completion.Status)
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

		request, config := contextClientFixture(t)
		request.TransactionID = fixedTransactionID
		coordinated := &ContextGRPCClient{transport: client, config: config}
		_, err = coordinated.Reserve(ctx, request)
		require.Error(t, err, "client without a verified cert must be rejected at the TLS layer")
	})

	for name, restTLS := range map[string]*tls.Config{
		"with a client cert":    clientTLS,
		"without a client cert": serverOnlyTLSConfig(fixture),
	} {
		t.Run("REST reserve->confirm succeeds over TLS "+name, func(t *testing.T) {
			request, config := contextClientFixture(t)
			request.TransactionID = fixedTransactionID
			baseURL := startRESTSeamServer(t, fixture, config)

			client, err := NewContextHTTPClient(baseURL, config, staticTokens(seamBearerToken), WithOperationTimeout(5*time.Second), WithTLSConfig(restTLS))
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result, err := client.Reserve(ctx, request)
			require.NoError(t, err, "the bearer-authenticated client must complete the reserve POST")
			require.Equal(t, tracercontract.DecisionAllow, result.Decision)
			require.Equal(t, []uuid.UUID{fixedReservationID}, result.ReservationIDs)

			completion, err := client.ConfirmByTransaction(ctx, fixedTransactionID)
			require.NoError(t, err, "confirm over the REST seam must succeed")
			require.Equal(t, "CONFIRMED", completion.Status)
		})
	}

	t.Run("REST reserve with a rejected bearer token is a token unavailability", func(t *testing.T) {
		request, config := contextClientFixture(t)
		request.TransactionID = fixedTransactionID
		baseURL := startRESTSeamServer(t, fixture, config)

		client, err := NewContextHTTPClient(baseURL, config, staticTokens("wrong-token"), WithOperationTimeout(5*time.Second), WithTLSConfig(clientTLS))
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err = client.Reserve(ctx, request)
		require.ErrorIs(t, err, ErrTracerUnavailable, "a 401 falls under the ledger's fail posture")
		require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
	})

	t.Run("gRPC reserve->release succeeds with a CA-signed client cert", func(t *testing.T) {
		addr := startGRPCSeamServer(t, serverTLS)

		client, err := NewTracerGRPCClient(addr,
			WithGRPCOperationTimeout(5*time.Second),
			WithGRPCDialOptions(grpc.WithTransportCredentials(credentials.NewTLS(clientTLS))))
		require.NoError(t, err)

		t.Cleanup(func() { _ = client.Close() })

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		request, config := contextClientFixture(t)
		request.TransactionID = fixedTransactionID
		coordinated := &ContextGRPCClient{transport: client, config: config}
		_, err = coordinated.Reserve(ctx, request)
		require.NoError(t, err)

		completion, err := coordinated.ReleaseByTransaction(ctx, fixedTransactionID)
		require.NoError(t, err, "release over the secured seam must succeed")
		require.Equal(t, "RELEASED", completion.Status)
	})

	t.Run("REST reserve->release succeeds over TLS", func(t *testing.T) {
		request, config := contextClientFixture(t)
		request.TransactionID = fixedTransactionID
		baseURL := startRESTSeamServer(t, fixture, config)

		client, err := NewContextHTTPClient(baseURL, config, staticTokens(seamBearerToken), WithOperationTimeout(5*time.Second), WithTLSConfig(clientTLS))
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err = client.Reserve(ctx, request)
		require.NoError(t, err)

		completion, err := client.ReleaseByTransaction(ctx, fixedTransactionID)
		require.NoError(t, err, "release over the REST seam must succeed")
		require.Equal(t, "RELEASED", completion.Status)
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
		ContractRevision: tracercontract.ReserveContractRevision, TransactionId: req.GetTransactionId(),
		EvaluationId: "33333333-3333-4333-8333-333333333333", Decision: string(tracercontract.DecisionAllow),
		Controls:       &reservationv1.ReserveControls{Rules: string(tracercontract.RulesEvaluated), Limits: string(tracercontract.LimitsEvaluated)},
		ReservationIds: []string{fixedReservationID.String()}, Reasons: []string{string(tracercontract.ReasonLimitsSatisfied)},
	}, nil
}

func (reserveSeamServer) ConfirmByTransaction(_ context.Context, req *reservationv1.ConfirmByTransactionRequest) (*reservationv1.ConfirmByTransactionResponse, error) {
	evaluation := "33333333-3333-4333-8333-333333333333"

	return &reservationv1.ConfirmByTransactionResponse{
		ContractRevision: tracercontract.ReserveContractRevision, TransactionId: req.GetTransactionId(),
		Status: "CONFIRMED", Flipped: 1, EvaluationId: &evaluation,
	}, nil
}

func (reserveSeamServer) ReleaseByTransaction(_ context.Context, req *reservationv1.ReleaseByTransactionRequest) (*reservationv1.ReleaseByTransactionResponse, error) {
	evaluation := "33333333-3333-4333-8333-333333333333"

	return &reservationv1.ReleaseByTransactionResponse{
		ContractRevision: tracercontract.ReserveContractRevision, TransactionId: req.GetTransactionId(),
		Status: "RELEASED", Flipped: 1, EvaluationId: &evaluation,
	}, nil
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

// startRESTSeamServer runs an https server that requests no client cert and
// requires the bearer token, serving the contextual reserve and by-transaction
// confirm and release routes. It returns the https base URL. Stopped on cleanup.
func startRESTSeamServer(t *testing.T, fixture seamMTLSFixture, config ContextClientConfig) string {
	t.Helper()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(AuthorizationHeader) != "Bearer "+seamBearerToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/v1/reservations":
			request, err := tracercontract.DecodeReserveJSON(r.Context(), raw, config.MaxBodyBytes, config.Bounds)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			result := contextResultFixture(request)
			result.ReservationIDs = []uuid.UUID{fixedReservationID}

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(result)
		case strings.HasSuffix(r.URL.Path, "/confirm"), strings.HasSuffix(r.URL.Path, "/release"):
			evaluation := uuid.MustParse("33333333-3333-4333-8333-333333333333")

			status := "CONFIRMED"
			if strings.HasSuffix(r.URL.Path, "/release") {
				status = "RELEASED"
			}

			_ = json.NewEncoder(w).Encode(tracercontract.TransactionCompletionResult{
				ContractRevision: tracercontract.ReserveContractRevision, TransactionID: fixedTransactionID,
				Status: status, Flipped: 1, EvaluationID: &evaluation,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	serverCert, err := tls.X509KeyPair(fixture.serverCertPEM, fixture.serverKeyPEM)
	require.NoError(t, err)

	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{serverCert}, ClientAuth: tls.NoClientCert}
	server.StartTLS()
	t.Cleanup(server.Close)

	return server.URL
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
