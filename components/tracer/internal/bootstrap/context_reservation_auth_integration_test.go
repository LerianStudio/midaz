// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package bootstrap

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libAuth "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	grpcin "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	contractpb "github.com/LerianStudio/midaz/v4/pkg/tracercontract/protobuf"
)

const (
	// sharedDeployClientID is an operator-configured client id: multi-tenant
	// HTTP reservations never consult one.
	sharedDeployClientID = "ledger-m2m-client"
	// sharedDeployDeniedSub is the application subject plugin-auth refuses.
	sharedDeployDeniedSub   = "unauthorized-application"
	sharedDeployCertURI     = "spiffe://example.test/service/ledger"
	sharedDeployUnmappedURI = "spiffe://example.test/service/unknown"

	// tenantAssociated is listed active for the ledger and has a tracer pool;
	// tenantNotLedger has a tracer pool but is absent from the ledger list;
	// tenantSuspended is listed for the ledger as suspended; tenantNoTracer is
	// listed active for the ledger but has no tracer pool.
	tenantAssociated = "tenant-a"
	tenantNotLedger  = "tenant-b"
	tenantSuspended  = "tenant-c"
	tenantNoTracer   = "tenant-d"

	sharedDeployLegacyReserveBody = `{
  "transactionId": "11111111-1111-4111-8111-111111111111",
  "requestId": "22222222-2222-4222-8222-222222222222",
  "amount": "2500.125",
  "asset": "BRL",
  "account": {"accountId": "33333333-3333-4333-8333-333333333333"},
  "transactionType": "PIX",
  "transactionTimestamp": "2026-09-24T12:00:00Z",
  "longLived": true
}`
)

// TestContextReservationAuthSharedDeploy boots, in one process, the HTTP and
// gRPC listeners of a shared multi-tenant deploy in TRACER_TLS_MODE=mtls, wired
// through the bootstrap's own TLS builders, producer roster loader and
// tenant-manager clients. The collaborators outside the process are httptest
// fakes: plugin-auth authorizing the route guard of every route, reservations
// included, and the tenant-manager answering tenant associations and the tracer
// pool config. The Tracer never verifies a token signature — plugin-auth
// decides it — so the test tokens are well formed but signed with a throwaway
// key. The tracer pool points at a
// PostgreSQL wire fake, so the production pool manager resolves tenant pools
// without Docker. Admission and completion are mocks: the test covers who may
// reach them, not what they do.
//
// HTTP reservations identify the ledger by the platform claims the
// tenant-manager writes onto the per-tenant M2M application it provisions, and
// take the tenant from that token; gRPC identifies it by its client
// certificate and takes the tenant from the header. Both require the tenant's
// active ledger association.
//
// Cases run in order on the shared fixture. The tenant-manager outage boots a
// deploy of its own, so no active-tenant list fetched by an earlier case can
// mask it.
func TestContextReservationAuthSharedDeploy(t *testing.T) {
	// Not parallel: t.Setenv. The PostgreSQL wire fake speaks plaintext.
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	deploy := startSharedDeploy(t)

	t.Run("health answers without a client certificate", func(t *testing.T) {
		response := deploy.httpCall(t, http.MethodGet, "/health", "", nil, nil)
		require.Equal(t, http.StatusOK, response.status, response.body)
	})

	t.Run("user token on a user route passes the guard", func(t *testing.T) {
		before := deploy.pluginAuth.calls("limits", "post")
		response := deploy.httpCall(t, http.MethodPost, "/v1/limits", deploy.userToken(t), nil, []byte(`{}`))
		require.NotContains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, response.status, response.body)
		require.Equal(t, http.StatusBadRequest, response.status, "the guard admitted the request and the handler rejected its body: %s", response.body)
		require.Equal(t, before+1, deploy.pluginAuth.calls("limits", "post"), "plugin-auth decided the user route")
	})

	t.Run("user token on the reservation route is forbidden before the body is read", func(t *testing.T) {
		token := deploy.userToken(t)
		before := deploy.tenantManager.listCalls()
		response := deploy.httpCall(t, http.MethodPost, "/v1/reservations", token, tenantHeader(tenantAssociated), []byte(sharedDeployLegacyReserveBody))
		requireErrorCode(t, response, http.StatusForbidden, "0043")
		require.NotContains(t, response.body, token, "the rejection never echoes the token")
		require.Equal(t, before, deploy.tenantManager.listCalls(), "a non-application token is refused before any tenant lookup")
	})

	t.Run("reservation route without a token is unauthorized", func(t *testing.T) {
		before := deploy.pluginAuth.calls("reservations", "post")
		response := deploy.httpCall(t, http.MethodPost, "/v1/reservations", "", tenantHeader(tenantAssociated), deploy.reserveJSON)
		requireErrorCode(t, response, http.StatusUnauthorized, "0042")
		require.Equal(t, before, deploy.pluginAuth.calls("reservations", "post"), "a missing token never reaches plugin-auth")
	})

	t.Run("producer token plugin-auth refuses is forbidden", func(t *testing.T) {
		token := deploy.tenantToken(t, tenantAssociated, func(c jwt.MapClaims) { c["sub"] = sharedDeployDeniedSub })
		before := deploy.tenantManager.listCalls()
		response := deploy.httpCall(t, http.MethodPost, "/v1/reservations", token, nil, deploy.reserveJSON)
		requireErrorCode(t, response, http.StatusForbidden, "0043")
		require.NotContains(t, response.body, token, "the rejection never echoes the token")
		require.Equal(t, before, deploy.tenantManager.listCalls(), "a refused token is rejected before any tenant lookup")
	})

	t.Run("tenant-manager application token for an associated tenant reaches the handler", func(t *testing.T) {
		before := deploy.pluginAuth.calls("reservations", "post")
		deploy.expectAdmission(t, tenantAssociated, producerauth.ViaToken)
		response := deploy.httpCall(t, http.MethodPost, "/v1/reservations", deploy.tenantToken(t, tenantAssociated, nil), nil, deploy.reserveJSON)
		require.Equal(t, http.StatusCreated, response.status, response.body)
		require.Equal(t, before+1, deploy.pluginAuth.calls("reservations", "post"), "plugin-auth decided tracer/reservations:post")
	})

	t.Run("a requested tenant equal to the token tenant is accepted", func(t *testing.T) {
		deploy.expectAdmission(t, tenantAssociated, producerauth.ViaToken)
		response := deploy.httpCall(t, http.MethodPost, "/v1/reservations", deploy.tenantToken(t, tenantAssociated, nil), tenantHeader(tenantAssociated), deploy.reserveJSON)
		require.Equal(t, http.StatusCreated, response.status, response.body)
	})

	t.Run("a requested tenant other than the token tenant is forbidden", func(t *testing.T) {
		before := deploy.tenantManager.listCalls()
		response := deploy.httpCall(t, http.MethodPost, "/v1/reservations", deploy.tenantToken(t, tenantAssociated, nil), tenantHeader(tenantNotLedger), deploy.reserveJSON)
		requireErrorCode(t, response, http.StatusForbidden, "0043")
		require.Equal(t, before, deploy.tenantManager.listCalls(), "the mismatch is refused before any tenant lookup")
		require.Zero(t, deploy.tenantManager.calls(tenantNotLedger, trcConstant.ApplicationName), "the header never selects a pool")
	})

	t.Run("tokens without the tenant-manager platform claims are forbidden", func(t *testing.T) {
		for name, token := range map[string]string{
			"isInternal missing":     deploy.tenantToken(t, tenantAssociated, func(c jwt.MapClaims) { delete(c, "isInternal") }),
			"isInternal false":       deploy.tenantToken(t, tenantAssociated, func(c jwt.MapClaims) { c["isInternal"] = "false" }),
			"another source service": deploy.tenantToken(t, tenantAssociated, func(c jwt.MapClaims) { c["sourceService"] = "fees" }),
			"flowker source service": deploy.tenantToken(t, tenantAssociated, func(c jwt.MapClaims) { c["sourceService"] = "flowker" }),
			"tenantId missing":       deploy.tenantToken(t, tenantAssociated, func(c jwt.MapClaims) { delete(c, "tenantId") }),
			"operator-roster token":  deploy.applicationToken(t, "ledger-application", sharedDeployClientID),
		} {
			t.Run(name, func(t *testing.T) {
				response := deploy.httpCall(t, http.MethodPost, "/v1/reservations", token, tenantHeader(tenantAssociated), deploy.reserveJSON)
				requireErrorCode(t, response, http.StatusForbidden, "0043")
				require.NotContains(t, response.body, token, "the rejection never echoes the token")
			})
		}
	})

	t.Run("token tenant without the ledger association is forbidden", func(t *testing.T) {
		response := deploy.httpCall(t, http.MethodPost, "/v1/reservations", deploy.tenantToken(t, tenantNotLedger, nil), nil, deploy.reserveJSON)
		requireErrorCode(t, response, http.StatusForbidden, "0043")
		require.Zero(t, deploy.tenantManager.calls(tenantNotLedger, trcConstant.ApplicationName), "a denied tenant never resolves a tracer pool")
	})

	t.Run("token tenant suspended for the ledger is forbidden", func(t *testing.T) {
		response := deploy.httpCall(t, http.MethodPost, "/v1/reservations", deploy.tenantToken(t, tenantSuspended, nil), nil, deploy.reserveJSON)
		requireErrorCode(t, response, http.StatusForbidden, "0043")
		require.Zero(t, deploy.tenantManager.calls(tenantSuspended, trcConstant.ApplicationName), "a suspended tenant never resolves a tracer pool")
	})

	t.Run("token tenant associated with the ledger but without a tracer pool is forbidden", func(t *testing.T) {
		response := deploy.httpCall(t, http.MethodPost, "/v1/reservations", deploy.tenantToken(t, tenantNoTracer, nil), nil, deploy.reserveJSON)
		requireErrorCode(t, response, http.StatusForbidden, "0043")
		require.Equal(t, 1, deploy.tenantManager.calls(tenantNoTracer, trcConstant.ApplicationName), "the lib-commons tenant middleware asked for the pool")
	})

	t.Run("plugin-auth outage is an availability failure on HTTP", func(t *testing.T) {
		deploy.pluginAuth.down.Store(true)
		t.Cleanup(func() { deploy.pluginAuth.down.Store(false) })

		// Neither a denial nor the policy-configuration code 0537: the ledger
		// reads an unrecognized 503 as tracer unavailability, so its fail
		// posture decides.
		response := deploy.httpCall(t, http.MethodPost, "/v1/reservations", deploy.tenantToken(t, tenantAssociated, nil), nil, deploy.reserveJSON)
		requireErrorCode(t, response, http.StatusServiceUnavailable, "0525")
	})

	t.Run("gRPC without a client certificate fails the handshake", func(t *testing.T) {
		_, err := deploy.grpcReserve(t, nil, tenantAssociated)
		require.Equal(t, codes.Unavailable, status.Code(err), "err=%v", err)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		dialer := &tls.Dialer{Config: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: deploy.serverRoots, ServerName: "localhost", NextProtos: []string{"h2"}}}
		conn, err := dialer.DialContext(ctx, "tcp", deploy.grpcAddr)
		if err == nil {
			// TLS 1.3 reports the missing certificate on the first read, which
			// is bounded by the same deadline as the dial.
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.NoError(t, conn.SetReadDeadline(deadline))
			_, err = conn.Read(make([]byte, 1))
			_ = conn.Close()
		}
		require.ErrorContains(t, err, "certificate required")
	})

	t.Run("gRPC with the mapped certificate admits an associated tenant", func(t *testing.T) {
		deploy.expectAdmission(t, tenantAssociated, producerauth.ViaCert)
		response, err := deploy.grpcReserve(t, &deploy.mappedClient, tenantAssociated)
		require.NoError(t, err)
		require.Equal(t, string(tracercontract.DecisionAllow), response.GetDecision())
	})

	t.Run("gRPC with an unmapped certificate is permission denied", func(t *testing.T) {
		_, err := deploy.grpcReserve(t, &deploy.unmappedClient, tenantAssociated)
		require.Equal(t, codes.PermissionDenied, status.Code(err), "err=%v", err)
	})

	t.Run("tenant-manager outage before any tenant list is an availability failure on both transports", func(t *testing.T) {
		cold := startSharedDeploy(t)
		cold.tenantManager.down.Store(true)

		// Without a fetched list the first call refreshes it and meets the
		// failing tenant-manager; the calls inside the failure backoff that
		// follows answer without a list call. None of them is a denial, nor
		// the policy-configuration code 0537, which the ledger treats as
		// deterministic and blocks on even under a fail-open posture.
		_, err := cold.grpcReserve(t, &cold.mappedClient, tenantAssociated)
		require.Equal(t, codes.Unavailable, status.Code(err), "err=%v", err)
		require.Equal(t, "0161", status.Convert(err).Message(), "gRPC tenant-manager failure")
		require.Equal(t, 1, cold.tenantManager.listOutages(), "the gRPC call reached the tenant-manager")

		for range 2 {
			response := cold.httpCall(t, http.MethodPost, "/v1/reservations", cold.tenantToken(t, tenantAssociated, nil), nil, cold.reserveJSON)
			requireErrorCode(t, response, http.StatusServiceUnavailable, "0161")
		}

		_, err = cold.grpcReserve(t, &cold.mappedClient, tenantAssociated)
		require.Equal(t, codes.Unavailable, status.Code(err), "err=%v", err)
		require.Equal(t, "0161", status.Convert(err).Message(), "gRPC inside the failure backoff")
		require.Equal(t, 1, cold.tenantManager.listOutages(), "the failure backoff answered both transports without reaching the tenant-manager")
		require.Zero(t, cold.tenantManager.calls(tenantAssociated, trcConstant.ApplicationName), "an unauthorized tenant never resolves a tracer pool")
	})
}

type sharedDeploy struct {
	tenantManager  *tenantManagerFake
	pluginAuth     *pluginAuthFake
	admission      *mocks.MockContextReserveAdmitter
	request        tracercontract.ReserveRequest
	reserveJSON    []byte
	bounds         tracercontract.Limits
	httpURL        string
	httpClient     *http.Client
	grpcAddr       string
	serverRoots    *x509.CertPool
	mappedClient   tls.Certificate
	unmappedClient tls.Certificate
}

func startSharedDeploy(t *testing.T) *sharedDeploy {
	t.Helper()

	pg := testutil.StartFakePostgres(t)
	tenantManager := startTenantManagerFake(t, pg.Port())
	pluginAuth := startPluginAuthFake(t)

	mapped := testutil.GenerateMTLSFixture(t, sharedDeployCertURI)
	unmapped := testutil.GenerateMTLSFixture(t, sharedDeployUnmappedURI)
	cfg := sharedDeployConfig(t, mapped, unmapped, pluginAuth.server.URL, tenantManager.server.URL)
	logger := libLog.NewNop()

	reservation, err := loadContextReservationConfig(cfg, logger)
	require.NoError(t, err)
	require.False(t, reservation.unverifiedProducers, "the shared deploy authorizes reservation callers through plugin-auth")

	tmOptions, err := buildTMClientOptions(cfg, logger)
	require.NoError(t, err)
	tmClient, err := tmclient.NewClient(cfg.MultiTenantURL, logger, tmOptions...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tmClient.Close() })

	pgManager := tmpostgres.NewManager(tmClient, cfg.ApplicationName, buildPgManagerOptions(cfg, logger)...)
	t.Cleanup(func() { _ = pgManager.Close(context.Background()) })

	tenantAssociations := newActiveTenantSets(tmClient, activeTenantSetConfig{
		TTL:          time.Duration(cfg.MultiTenantCacheTTLSec) * time.Second,
		FetchTimeout: time.Duration(cfg.MultiTenantTimeout) * time.Second,
		Logger:       logger,
	}, producerauth.ServiceLedger)
	authz := producerauth.NewTenantAuthorizer(tenantAssociations.Lookup, true)

	ctrl := gomock.NewController(t)
	admission := mocks.NewMockContextReserveAdmitter(ctrl)
	completion := mocks.NewMockContextReserveCompleter(ctrl)
	completionByID := mocks.NewMockContextReserveIDCompleter(ctrl)
	bounds := reservation.evaluation.CEL.Limits

	handler, err := in.NewContextReservationHandler(admission, completion, completionByID, bounds, reservation.maxBodyBytes, reservation.admission.Plan.MaxReservations)
	require.NoError(t, err)

	telemetry := &libOtel.Telemetry{TelemetryConfig: libOtel.TelemetryConfig{Logger: logger}}
	app, err := in.NewRoutes(in.RoutesDeps{
		ContextReservation:                    handler,
		ContextReservationProducers:           reservation.producers,
		ContextReservationUnverifiedProducers: reservation.unverifiedProducers,
		ContextReservationTenants:             authz,
		Logger:                                logger,
		Telemetry:                             telemetry,
		HealthChecker:                         &in.HealthChecker{},
		Cfg:                                   &in.RouteConfig{},
		RuleService:                           in.NewMockRuleService(ctrl),
		LimitService:                          in.NewMockLimitService(ctrl),
		ValidationService:                     mocks.NewMockValidationService(ctrl),
		TransactionValidationService:          mocks.NewMockTransactionValidationService(ctrl),
		AuditEventService:                     in.NewMockAuditEventService(ctrl),
		Guard: middleware.NewAuthGuard(middleware.AuthGuardConfig{PluginAuthEnabled: cfg.PluginAuthEnabled, AppName: trcConstant.ApplicationName},
			libAuth.NewAuthClient(cfg.PluginAuthAddress, cfg.PluginAuthEnabled, logger)),
		Clock:              clock.New(),
		MultiTenantEnabled: true,
		PgManager:          pgManager,
	})
	require.NoError(t, err)

	httpTLS, err := buildHTTPTLSConfig(cfg)
	require.NoError(t, err)
	httpServer, err := NewHTTPServer(cfg, app, httpTLS, logger, telemetry)
	require.NoError(t, err)

	httpDone := make(chan error, 1)
	go func() { httpDone <- httpServer.Run(nil) }()
	t.Cleanup(func() {
		require.NoError(t, app.Shutdown())
		require.NoError(t, <-httpDone)
	})
	waitForListener(t, cfg.ServerAddress)

	grpcService, err := grpcin.NewContextReservationServer(admission, completion, completionByID, grpcin.ContextReservationConfig{Bounds: bounds, MaxBodyBytes: reservation.maxBodyBytes, MaxReservations: reservation.admission.Plan.MaxReservations})
	require.NoError(t, err)
	grpcTLS, err := buildGRPCTLSConfig(cfg)
	require.NoError(t, err)
	interceptor := grpcin.ContextReservationUnaryInterceptor(reservation.producers, authz, seamtenant.NewResolver(pgManager, cfg.MultiTenantEnabled))
	grpcServer, err := NewGRPCServer(cfg.TracerGRPCPort, grpcService, grpcTLS, interceptor, logger, telemetry, grpc.MaxRecvMsgSize(reservation.maxBodyBytes))
	require.NoError(t, err)

	grpcListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	grpcDone := make(chan error, 1)
	go func() { grpcDone <- grpcServer.server.Serve(grpcListener) }()
	t.Cleanup(func() {
		grpcServer.server.Stop()
		require.NoError(t, <-grpcDone)
	})

	raw, err := os.ReadFile("../../../../pkg/tracercontract/testdata/reserve_request.json")
	require.NoError(t, err)
	request, err := tracercontract.DecodeReserveJSON(t.Context(), raw, reservation.maxBodyBytes, bounds)
	require.NoError(t, err)

	serverRoots := x509.NewCertPool()
	require.True(t, serverRoots.AppendCertsFromPEM(mapped.CACertPEM))
	mappedClient, err := tls.X509KeyPair(mapped.ClientCertPEM, mapped.ClientKeyPEM)
	require.NoError(t, err)
	unmappedClient, err := tls.X509KeyPair(unmapped.ClientCertPEM, unmapped.ClientKeyPEM)
	require.NoError(t, err)

	// The HTTP client presents no certificate: HTTP callers authenticate with
	// tokens, and the listener must not ask for one.
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: serverRoots, ServerName: "localhost"}}
	t.Cleanup(transport.CloseIdleConnections)

	return &sharedDeploy{
		tenantManager: tenantManager, pluginAuth: pluginAuth, admission: admission,
		request: request, reserveJSON: raw, bounds: bounds,
		httpURL: "https://" + cfg.ServerAddress, httpClient: &http.Client{Transport: transport, Timeout: 10 * time.Second},
		grpcAddr: grpcListener.Addr().String(), serverRoots: serverRoots, mappedClient: mappedClient, unmappedClient: unmappedClient,
	}
}

// sharedDeployConfig is a multi-tenant mtls deploy whose route guard
// authorizes through plugin-auth. Both client CAs are trusted, so the unmapped
// certificate is refused by the producer registry rather than by the TLS
// handshake.
func sharedDeployConfig(t *testing.T, mapped, unmapped testutil.MTLSFixture, pluginAuthURL, tenantManagerURL string) *Config {
	t.Helper()

	dir := t.TempDir()
	certFile := filepath.Join(dir, "server-cert.pem")
	keyFile := filepath.Join(dir, "server-key.pem")
	caFile := filepath.Join(dir, "client-ca.pem")

	require.NoError(t, os.WriteFile(certFile, mapped.ServerCertPEM, 0o600))
	require.NoError(t, os.WriteFile(keyFile, mapped.ServerKeyPEM, 0o600))
	require.NoError(t, os.WriteFile(caFile, append(append([]byte{}, mapped.CACertPEM...), unmapped.CACertPEM...), 0o600))

	cfg := validContextReservationConfig()
	cfg.DeploymentMode = "byoc"
	cfg.PluginAuthEnabled = true
	cfg.PluginAuthAddress = pluginAuthURL
	// Multi-tenant HTTP reservations consult no client id, so the roster maps
	// the gRPC certificate only.
	cfg.TracerPlatformProducers = `[{"service":"ledger","certUri":"` + sharedDeployCertURI + `"}]`
	cfg.TracerTLSMode = tlsModeMTLS
	cfg.TracerTLSCertFile = certFile
	cfg.TracerTLSKeyFile = keyFile
	cfg.TracerTLSClientCAFile = caFile
	cfg.TracerGRPCPort = "127.0.0.1:0"
	cfg.ServerAddress = freeLoopbackAddress(t)
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = tenantManagerURL
	cfg.MultiTenantAllowInsecureHTTP = true
	cfg.MultiTenantServiceAPIKey = "shared-deploy-test-key"
	// Two tenant-manager failures open the breaker.
	cfg.MultiTenantCircuitBreakerThreshold = 2
	cfg.MultiTenantCircuitBreakerTimeoutSec = 60
	cfg.MultiTenantCacheTTLSec = 60
	ApplyMultiTenantDefaults(cfg)

	return cfg
}

// tenantToken is the access token of the per-tenant M2M application the
// tenant-manager provisions for the ledger in tenantID: a random client id
// and the platform attributes only the tenant-manager can write. mutate may
// alter its claims.
func (d *sharedDeploy) tenantToken(t *testing.T, tenantID string, mutate func(jwt.MapClaims)) string {
	t.Helper()

	claims := jwt.MapClaims{
		"type": "application", "sub": "admin/ledger-m2m-tracer-" + tenantID, "azp": "c0ffee-" + tenantID, "owner": "admin",
		"name": "ledger-m2m-tracer-" + tenantID, "tenantId": tenantID, "tenantSlug": tenantID, "isInternal": "true", "sourceService": producerauth.ServiceLedger,
	}
	if mutate != nil {
		mutate(claims)
	}

	return d.sign(t, claims)
}

func (d *sharedDeploy) applicationToken(t *testing.T, sub, clientID string) string {
	t.Helper()

	return d.sign(t, jwt.MapClaims{"type": "application", "sub": sub, "azp": clientID})
}

// userToken is a normal-user token plugin-auth grants, so only its type
// separates it from a producer token.
func (d *sharedDeploy) userToken(t *testing.T) string {
	t.Helper()

	return d.sign(t, jwt.MapClaims{"type": "normal-user", "owner": "lerian", "sub": "operator", "tenantId": tenantAssociated})
}

// sign issues a well-formed token under a throwaway key: the Tracer reads its
// claims only after plugin-auth authorized it and never checks the signature.
func (d *sharedDeploy) sign(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	claims["iat"] = float64(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC).Unix())
	claims["exp"] = float64(time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC).Unix())

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("shared-deploy-unverified-signature"))
	require.NoError(t, err)

	return signed
}

// expectAdmission expects exactly one admission carrying the authorized ledger
// producer and the tenant's own pool.
func (d *sharedDeploy) expectAdmission(t *testing.T, tenantID, via string) {
	t.Helper()

	d.admission.EXPECT().Execute(gomock.Any(), d.request).DoAndReturn(func(ctx context.Context, got tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
		producer, ok := producerauth.ProducerFromContext(ctx)
		require.True(t, ok)
		require.Equal(t, producerauth.Producer{Service: producerauth.ServiceLedger, Via: via}, producer)

		identity, ok := contextutil.GetIntegrationIdentity(ctx)
		require.True(t, ok)
		require.Equal(t, producerauth.ServiceLedger, identity.ID)
		require.Equal(t, tenantID, tmcore.GetTenantIDContext(ctx))
		require.NotNil(t, tmcore.GetPGContext(ctx), "the tenant's tracer pool is bound before admission")

		return &tracercontract.ReserveResult{
			ContractRevision: got.ContractRevision, TransactionID: got.TransactionID, EvaluationID: testutil.MustDeterministicUUID(91351),
			Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated},
			ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied},
		}, nil
	})
}

type httpResult struct {
	status int
	body   string
}

func (d *sharedDeploy) httpCall(t *testing.T, method, path, token string, header http.Header, body []byte) httpResult {
	t.Helper()

	call, err := http.NewRequestWithContext(t.Context(), method, d.httpURL+path, bytes.NewReader(body))
	require.NoError(t, err)

	for name, values := range header {
		call.Header[name] = values
	}

	if body != nil {
		call.Header.Set("Content-Type", "application/json")
	}

	if token != "" {
		call.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := d.httpClient.Do(call)
	require.NoError(t, err)

	defer func() { _ = response.Body.Close() }()

	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return httpResult{status: response.StatusCode, body: string(raw)}
}

// grpcReserve calls Reserve presenting cert, or no certificate when nil.
func (d *sharedDeploy) grpcReserve(t *testing.T, cert *tls.Certificate, tenantID string) (*reservationv1.ReserveResult, error) {
	t.Helper()

	clientTLS := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: d.serverRoots, ServerName: "localhost"}
	if cert != nil {
		clientTLS.Certificates = []tls.Certificate{*cert}
	}

	conn, err := grpc.NewClient(d.grpcAddr, grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
	require.NoError(t, err)

	defer func() { _ = conn.Close() }()

	wire, err := contractpb.EncodeReserve(t.Context(), d.request, d.bounds)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(seamtenant.MetadataKey, tenantID))

	return reservationv1.NewReservationServiceClient(conn).Reserve(ctx, wire)
}

func tenantHeader(tenantID string) http.Header {
	return http.Header{seamtenant.HeaderName: []string{tenantID}}
}

func requireErrorCode(t *testing.T, response httpResult, wantStatus int, wantCode string) {
	t.Helper()

	require.Equal(t, wantStatus, response.status, response.body)

	var document struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal([]byte(response.body), &document), response.body)
	require.Equal(t, wantCode, document.Code, response.body)
}

func freeLoopbackAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	return address
}

func waitForListener(t *testing.T, address string) {
	t.Helper()

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err != nil {
			return false
		}

		_ = conn.Close()

		return true
	}, 5*time.Second, 20*time.Millisecond, "HTTP listener did not start on %s", address)
}

// tenantManagerFake answers GET /v1/tenants/active?service=ledger with the
// ledger's tenant list, and GET
// /v1/tenants/{tenant}/associations/tracer/connections with a tracer pool.
type tenantManagerFake struct {
	server  *httptest.Server
	down    atomic.Bool
	mu      sync.Mutex
	counts  map[string]int
	outages int
	lists   int
}

func startTenantManagerFake(t *testing.T, pgPort int) *tenantManagerFake {
	t.Helper()

	fake := &tenantManagerFake{counts: map[string]int{}}
	tracerConfig := func(tenantID string) map[string]any {
		return map[string]any{
			"id": tenantID, "tenantSlug": tenantID, "service": trcConstant.ApplicationName, "status": "active", "isolationMode": "isolated",
			"databases": map[string]any{trcConstant.ModuleName: map[string]any{"postgresql": map[string]any{
				"host": "127.0.0.1", "port": pgPort, "database": "tracer_" + strings.ReplaceAll(tenantID, "-", "_"), "username": "tracer", "password": "tracer",
			}}},
		}
	}

	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fake.down.Load() {
			fake.mu.Lock()
			fake.outages++
			fake.mu.Unlock()

			http.Error(w, "tenant-manager unavailable", http.StatusServiceUnavailable)

			return
		}

		if r.URL.Path == "/v1/tenants/active" {
			if r.URL.Query().Get("service") != producerauth.ServiceLedger {
				writeJSON(w, http.StatusOK, []any{})
				return
			}

			fake.mu.Lock()
			fake.lists++
			fake.mu.Unlock()

			writeJSON(w, http.StatusOK, []map[string]string{
				{"id": tenantAssociated, "name": tenantAssociated, "status": "active"},
				{"id": tenantSuspended, "name": tenantSuspended, "status": "suspended"},
				{"id": tenantNoTracer, "name": tenantNoTracer, "status": "active"},
			})

			return
		}

		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 6 || parts[0] != "v1" || parts[1] != "tenants" || parts[3] != "associations" || parts[5] != "connections" {
			http.NotFound(w, r)
			return
		}

		tenantID, service := parts[2], parts[4]

		fake.mu.Lock()
		fake.counts[tenantID+"/"+service]++
		fake.mu.Unlock()

		if service == trcConstant.ApplicationName && (tenantID == tenantAssociated || tenantID == tenantNotLedger || tenantID == tenantSuspended) {
			writeJSON(w, http.StatusOK, tracerConfig(tenantID))
			return
		}

		http.NotFound(w, r)
	}))
	t.Cleanup(fake.server.Close)

	return fake
}

// calls counts the connection requests for tenantID and service.
func (f *tenantManagerFake) calls(tenantID, service string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.counts[tenantID+"/"+service]
}

// listCalls counts the ledger active-tenant list requests answered while up.
func (f *tenantManagerFake) listCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.lists
}

// listOutages counts the requests answered while the fake was down.
func (f *tenantManagerFake) listOutages() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.outages
}

// pluginAuthFake grants every authorization request except for a token whose
// sub is sharedDeployDeniedSub, answers 503 while down, and counts the
// decisions by resource and action.
type pluginAuthFake struct {
	server *httptest.Server
	down   atomic.Bool
	mu     sync.Mutex
	counts map[string]int
}

func startPluginAuthFake(t *testing.T) *pluginAuthFake {
	t.Helper()

	fake := &pluginAuthFake{counts: map[string]int{}}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/health":
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/authorize":
			if fake.down.Load() {
				http.Error(w, "plugin-auth unavailable", http.StatusServiceUnavailable)
				return
			}

			var decision struct {
				Resource string `json:"resource"`
				Action   string `json:"action"`
			}
			if err := json.NewDecoder(r.Body).Decode(&decision); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			fake.mu.Lock()
			fake.counts[decision.Resource+"/"+decision.Action]++
			fake.mu.Unlock()

			claims := jwt.MapClaims{}
			_, _, err := jwt.NewParser().ParseUnverified(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), &claims)
			writeJSON(w, http.StatusOK, map[string]any{"authorized": err == nil && claims["sub"] != sharedDeployDeniedSub})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fake.server.Close)

	return fake
}

func (f *pluginAuthFake) calls(resource, action string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.counts[resource+"/"+action]
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	document, err := json.Marshal(body)
	if err != nil {
		http.Error(w, "encode fake response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if _, err := w.Write(document); err != nil {
		return
	}
}
