// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/bxcodec/dbresolver/v2"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	servicesMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

const seamChainAPIKey = "a-32-character-api-key-for-tests" // gitleaks:allow

// newHealthyAuthServer stands in for the Access Manager's health endpoint, so
// building an AuthClient does not dial a real host.
func newHealthyAuthServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte("healthy"))

			return
		}

		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	return server
}

// seamChainTenantA is a tenant in the dashless form the tenant-manager writes
// into the ledger client's name and tenantId claim; seamChainTenantADashed is
// the same tenant dashed.
const (
	seamChainTenantA       = "0193b0c4d2a87e4f9c1d2e3f4a5b6c7d"
	seamChainTenantADashed = "0193b0c4-d2a8-7e4f-9c1d-2e3f4a5b6c7d"
	seamChainLedgerClient  = "lerian/midaz-ledger"
)

// chainAccessManager stands in for the Access Manager: it grants
// tracer/reservations to the subjects in granted and counts authorizations.
type chainAccessManager struct {
	server     *httptest.Server
	authorizes atomic.Int64
	granted    map[string]bool
}

func newChainAccessManager(t *testing.T, granted ...string) *chainAccessManager {
	t.Helper()

	am := &chainAccessManager{granted: map[string]bool{}}
	for _, sub := range granted {
		am.granted[sub] = true
	}

	am.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte("healthy"))
		case "/v1/authorize":
			am.authorizes.Add(1)

			var body struct {
				Sub string `json:"sub"`
			}

			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !am.granted[body.Sub] {
				w.WriteHeader(http.StatusForbidden)

				return
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"authorized":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(am.server.Close)

	return am
}

func (am *chainAccessManager) client(t *testing.T) *authMiddleware.AuthClient {
	t.Helper()

	return authMiddleware.NewAuthClient(am.server.URL, true, testutil.NewMockLogger())
}

// recordingResolver is an active seam tenant resolver that records every
// tenant it is asked to resolve.
type recordingResolver struct {
	mu      sync.Mutex
	tenants []string
}

func (r *recordingResolver) resolver() *seamtenant.Resolver {
	return seamtenant.NewResolverWithPool(func(_ context.Context, tenantID string) (dbresolver.DB, error) {
		r.mu.Lock()
		r.tenants = append(r.tenants, tenantID)
		r.mu.Unlock()

		return nil, nil
	}, true)
}

func (r *recordingResolver) resolved() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.tenants...)
}

// handlerTenantServer answers ConfirmByTransaction with OK and records the
// tenant bound into the context it receives, so a call that reaches it proves
// the whole chain admitted it.
type handlerTenantServer struct {
	reservationv1.UnimplementedReservationServiceServer

	mu      sync.Mutex
	reached int
	tenant  string
}

func (h *handlerTenantServer) ConfirmByTransaction(ctx context.Context, _ *reservationv1.ConfirmByTransactionRequest) (*reservationv1.ConfirmByTransactionResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.reached++
	h.tenant = tmcore.GetTenantIDContext(ctx)

	return &reservationv1.ConfirmByTransactionResponse{}, nil
}

func (h *handlerTenantServer) snapshot() (int, string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.reached, h.tenant
}

// serveChain serves handler behind exactly the interceptors
// seamUnaryInterceptors returns for cfg, over an in-memory listener.
func serveChain(
	t *testing.T,
	cfg *Config,
	authClient *authMiddleware.AuthClient,
	resolver *seamtenant.Resolver,
	handler reservationv1.ReservationServiceServer,
) reservationv1.ReservationServiceClient {
	t.Helper()

	chain, err := seamUnaryInterceptors(cfg, authClient, resolver, nil)
	require.NoError(t, err)

	lis := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(grpc.ChainUnaryInterceptor(chain...))
	reservationv1.RegisterReservationServiceServer(server, handler)

	go func() { _ = server.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		server.Stop()
		_ = lis.Close()
	})

	return reservationv1.NewReservationServiceClient(conn)
}

func confirmWith(t *testing.T, client reservationv1.ReservationServiceClient, pairs ...string) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if len(pairs) > 0 {
		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
	}

	_, err := client.ConfirmByTransaction(ctx, &reservationv1.ConfirmByTransactionRequest{})

	return err
}

// chainToken signs claims with a throwaway key: the chain reads the claims
// unverified after the Access Manager authorized the token.
func chainToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("seam-chain-signing-key"))
	require.NoError(t, err)

	return "Bearer " + token
}

// ledgerClientClaims mirrors the tenant-manager's per-tenant ledger client of
// the tracer: name "ledger-m2m-tracer-{nameTenant}", tenantId claimTenant.
func ledgerClientClaims(nameTenant, claimTenant string) jwt.MapClaims {
	name := "ledger-m2m-tracer-" + nameTenant

	return jwt.MapClaims{"type": "application", "sub": "admin/" + name, "name": name, "owner": "admin", "tenantId": claimTenant}
}

// TestSeamUnaryInterceptors_TokenMultiTenantChain drives the chain bootstrap
// ships for token identity in multi-tenant mode: lib-auth authorizes first,
// the principal guard admits only the ledger client of the claim's tenant,
// and the tenant resolves from the claim alone, canonically, without any
// x-tenant-id. It mutates the process environment lib-auth reads, so it does
// not run in parallel.
func TestSeamUnaryInterceptors_TokenMultiTenantChain(t *testing.T) {
	t.Setenv("AUTH_M2M_INVERSION_ENABLED", "true")
	t.Setenv("AUTH_CACHE_TTL", "")
	t.Setenv("MULTI_TENANT_ENABLED", "true")

	ledger := ledgerClientClaims(seamChainTenantA, seamChainTenantADashed)
	other := ledgerClientClaims(seamChainTenantA, seamChainTenantA)
	other["name"] = "flowker-m2m-tracer-" + seamChainTenantA
	other["sub"] = "admin/flowker-m2m-tracer-" + seamChainTenantA

	am := newChainAccessManager(t, ledger["sub"].(string), other["sub"].(string))
	rec := &recordingResolver{}
	handler := &handlerTenantServer{}

	client := serveChain(t, &Config{PluginAuthEnabled: true, MultiTenantEnabled: true}, am.client(t), rec.resolver(), handler)

	t.Run("a claim-only ledger token resolves the canonical claim tenant", func(t *testing.T) {
		require.NoError(t, confirmWith(t, client, "authorization", chainToken(t, ledger)))

		require.Equal(t, []string{seamChainTenantA}, rec.resolved(), "the tenant is the claim's, canonicalized")

		reached, tenant := handler.snapshot()
		assert.Equal(t, 1, reached)
		assert.Equal(t, seamChainTenantA, tenant)
	})

	t.Run("no token is unauthenticated before any tenant resolves", func(t *testing.T) {
		before := len(rec.resolved())

		assert.Equal(t, codes.Unauthenticated, status.Code(confirmWith(t, client, "x-tenant-id", seamChainTenantA)))
		assert.Len(t, rec.resolved(), before)
	})

	t.Run("a granted non-ledger client is refused after authorization and before the tenant", func(t *testing.T) {
		before := len(rec.resolved())
		authorizes := am.authorizes.Load()

		assert.Equal(t, codes.PermissionDenied, status.Code(confirmWith(t, client, "authorization", chainToken(t, other))))
		assert.Equal(t, authorizes+1, am.authorizes.Load(), "lib-auth runs before the principal guard")
		assert.Len(t, rec.resolved(), before, "the principal guard runs before the tenant interceptor")
	})
}

// TestSeamUnaryInterceptors_TokenSingleTenantChain proves single-tenant token
// identity chains lib-auth and the allowlist guard and resolves no tenant.
func TestSeamUnaryInterceptors_TokenSingleTenantChain(t *testing.T) {
	t.Setenv("AUTH_M2M_INVERSION_ENABLED", "true")
	t.Setenv("AUTH_CACHE_TTL", "")
	t.Setenv("MULTI_TENANT_ENABLED", "")

	am := newChainAccessManager(t, seamChainLedgerClient, "lerian/other-app")
	handler := &handlerTenantServer{}

	client := serveChain(t,
		&Config{PluginAuthEnabled: true, APIKeyEnabled: true, APIKey: seamChainAPIKey, TracerSeamAllowedClients: seamChainLedgerClient},
		am.client(t), seamtenant.NewResolver(nil, false), handler)

	require.NoError(t, confirmWith(t, client, "authorization", chainToken(t, jwt.MapClaims{"type": "application", "sub": seamChainLedgerClient})))

	reached, tenant := handler.snapshot()
	assert.Equal(t, 1, reached)
	assert.Empty(t, tenant, "a single-tenant chain binds no tenant")

	assert.Equal(t, codes.PermissionDenied,
		status.Code(confirmWith(t, client, "authorization", chainToken(t, jwt.MapClaims{"type": "application", "sub": "lerian/other-app"}))),
		"the allowlist guard is chained")
	assert.Equal(t, codes.Unauthenticated, status.Code(confirmWith(t, client, "x-api-key", seamChainAPIKey)),
		"the token wins over the API key")
}

// TestSeamUnaryInterceptors_HeaderTenantChains proves the identities other
// than the token resolve the tenant from x-tenant-id, after the API key check
// when there is one.
func TestSeamUnaryInterceptors_HeaderTenantChains(t *testing.T) {
	t.Parallel()

	t.Run("API key with an active resolver", func(t *testing.T) {
		t.Parallel()

		rec := &recordingResolver{}
		handler := &handlerTenantServer{}
		client := serveChain(t, &Config{APIKeyEnabled: true, APIKey: seamChainAPIKey}, nil, rec.resolver(), handler)

		assert.Equal(t, codes.Unauthenticated, status.Code(confirmWith(t, client, "x-tenant-id", seamChainTenantA)))
		assert.Empty(t, rec.resolved(), "the API key is checked before the tenant resolves")

		require.NoError(t, confirmWith(t, client, "x-api-key", seamChainAPIKey, "x-tenant-id", seamChainTenantA))
		assert.Equal(t, []string{seamChainTenantA}, rec.resolved())
	})

	t.Run("API key single-tenant", func(t *testing.T) {
		t.Parallel()

		handler := &handlerTenantServer{}
		client := serveChain(t, &Config{APIKeyEnabled: true, APIKey: seamChainAPIKey}, nil, seamtenant.NewResolver(nil, false), handler)

		require.NoError(t, confirmWith(t, client, "x-api-key", seamChainAPIKey))

		_, tenant := handler.snapshot()
		assert.Empty(t, tenant)
	})

	t.Run("transport with an active resolver", func(t *testing.T) {
		t.Parallel()

		rec := &recordingResolver{}
		client := serveChain(t, &Config{TracerTLSMode: "mtls"}, nil, rec.resolver(), &handlerTenantServer{})

		require.NoError(t, confirmWith(t, client, "x-tenant-id", seamChainTenantA))
		assert.Equal(t, []string{seamChainTenantA}, rec.resolved())
	})

	t.Run("no identity single-tenant reaches the handler", func(t *testing.T) {
		t.Parallel()

		handler := &handlerTenantServer{}
		client := serveChain(t, &Config{}, nil, seamtenant.NewResolver(nil, false), handler)

		require.NoError(t, confirmWith(t, client))

		reached, _ := handler.snapshot()
		assert.Equal(t, 1, reached)
	})
}

// TestSeamUnaryInterceptors_TokenIdentityRequiresAnAuthorizingClient proves
// token identity never runs on a client lib-auth would pass every call
// through on, which would leave the principal guard reading unverified claims.
func TestSeamUnaryInterceptors_TokenIdentityRequiresAnAuthorizingClient(t *testing.T) {
	t.Parallel()

	for name, client := range map[string]*authMiddleware.AuthClient{
		"nil client":      nil,
		"disabled client": {Address: "http://access-manager.invalid", Enabled: false},
		"no address":      {Enabled: true},
		"blank address":   {Address: "   ", Enabled: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := seamUnaryInterceptors(&Config{PluginAuthEnabled: true}, client, seamtenant.NewResolver(nil, false), nil)
			require.ErrorIs(t, err, errSeamAuthClientNotAuthorizing)
		})
	}
}

// seamBufconn serves the gRPC server initGRPCServer builds for cfg over an
// in-memory listener and returns a reservation client. The service sits on
// doubles; a call that reaches the handler with an empty transaction id
// answers InvalidArgument before any port is touched.
func seamBufconn(t *testing.T, cfg *Config, authClient *authMiddleware.AuthClient) reservationv1.ReservationServiceClient {
	t.Helper()

	ctrl := gomock.NewController(t)
	clk := testutil.NewMockClock(time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC))

	svc, err := services.NewReservationService(
		pgdbMocks.NewMockTxBeginner(ctrl),
		servicesMocks.NewMockLimitResolver(ctrl),
		servicesMocks.NewMockReservationRepository(ctrl),
		servicesMocks.NewMockReservationAuditWriter(ctrl),
		servicesMocks.NewMockRuleEvaluator(ctrl),
		clk,
	)
	require.NoError(t, err)

	cfg.TracerGRPCPort = DefaultTracerGRPCPort

	grpcServer, err := initGRPCServer(cfg, svc, nil, nil, authClient, clk, testutil.NewMockLogger(), &libOtel.Telemetry{})
	require.NoError(t, err)

	lis := bufconn.Listen(1024 * 1024)

	go func() { _ = grpcServer.server.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		grpcServer.server.Stop()
		_ = lis.Close()
	})

	return reservationv1.NewReservationServiceClient(conn)
}

func reserveWith(t *testing.T, client reservationv1.ReservationServiceClient, pairs ...string) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if len(pairs) > 0 {
		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
	}

	_, err := client.Reserve(ctx, &reservationv1.ReserveRequest{})

	return err
}

func TestInitGRPCServer_TokenIdentity_RefusesACallWithoutToken(t *testing.T) {
	t.Parallel()

	authServer := newHealthyAuthServer(t)
	authClient := authMiddleware.NewAuthClient(authServer.URL, true, testutil.NewMockLogger())

	client := seamBufconn(t, &Config{
		PluginAuthEnabled:        true,
		TracerSeamAllowedClients: "lerian/midaz-ledger",
		APIKeyEnabled:            true,
		APIKey:                   seamChainAPIKey,
	}, authClient)

	t.Run("no authorization metadata is unauthenticated", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, codes.Unauthenticated, status.Code(reserveWith(t, client)))
	})

	t.Run("an API key is ignored under token identity", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, codes.Unauthenticated, status.Code(reserveWith(t, client, "x-api-key", seamChainAPIKey)))
	})
}

func TestInitGRPCServer_AuthDisabled_ReachesTheHandler(t *testing.T) {
	t.Parallel()

	authClient := authMiddleware.NewAuthClient("", false, testutil.NewMockLogger())

	client := seamBufconn(t, &Config{}, authClient)

	assert.Equal(t, codes.InvalidArgument, status.Code(reserveWith(t, client)),
		"with no identity configured the call reaches the reservation handler")
}

func TestInitGRPCServer_APIKeyIdentity(t *testing.T) {
	t.Parallel()

	client := seamBufconn(t, &Config{APIKeyEnabled: true, APIKey: seamChainAPIKey}, nil)

	t.Run("right key reaches the handler", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, codes.InvalidArgument, status.Code(reserveWith(t, client, "x-api-key", seamChainAPIKey)))
	})

	t.Run("wrong key is unauthenticated", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, codes.Unauthenticated, status.Code(reserveWith(t, client, "x-api-key", "not-the-key")))
	})

	t.Run("missing key is unauthenticated", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, codes.Unauthenticated, status.Code(reserveWith(t, client)))
	})
}
