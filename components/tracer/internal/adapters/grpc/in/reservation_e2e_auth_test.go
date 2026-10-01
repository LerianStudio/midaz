// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

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
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	commandMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command/mocks"
	servicesMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// fakeAccessManager stands in for the Access Manager: /health answers
// "healthy", /v1/authorize grants the subjects in granted on
// tracer/reservations and refuses everyone else with 403, or answers 503 for
// every request while down is set. It counts /v1/authorize requests.
type fakeAccessManager struct {
	server     *httptest.Server
	authorizes atomic.Int64
	down       atomic.Bool

	mu      sync.Mutex
	granted map[string]bool
	seen    []authorizeBody
}

// authorizeBody is the /v1/authorize request body lib-auth sends.
type authorizeBody struct {
	Sub      string `json:"sub"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

func newFakeAccessManager(t *testing.T, granted ...string) *fakeAccessManager {
	t.Helper()

	am := &fakeAccessManager{granted: map[string]bool{}}
	for _, sub := range granted {
		am.granted[sub] = true
	}

	am.server = httptest.NewServer(http.HandlerFunc(am.serve))
	t.Cleanup(am.server.Close)

	return am
}

func (am *fakeAccessManager) serve(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/health":
		_, _ = w.Write([]byte("healthy"))
	case "/v1/authorize":
		am.authorizes.Add(1)

		if am.down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}

		var body authorizeBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		am.mu.Lock()
		am.seen = append(am.seen, body)
		allowed := am.granted[body.Sub] && body.Resource == SeamAuthResource
		am.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		if !allowed {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"code":"AUT-1003","title":"Forbidden","message":"insufficient privileges"}`))

			return
		}

		_, _ = w.Write([]byte(`{"authorized":true,"timestamp":"2026-10-01T12:00:00Z"}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (am *fakeAccessManager) lastAuthorize(t *testing.T) authorizeBody {
	t.Helper()

	am.mu.Lock()
	defer am.mu.Unlock()

	require.NotEmpty(t, am.seen)

	return am.seen[len(am.seen)-1]
}

// seamAuthE2E serves a real ReservationServer behind the token-identity chain
// bootstrap installs: lib-auth policy → principal guard → tenant from the
// token claim (only when the resolver is active).
type seamAuthE2E struct {
	client   reservationv1.ReservationServiceClient
	tenantMu sync.Mutex
	tenants  []string
	admitted []context.Context
}

func newSeamAuthE2E(t *testing.T, authClient *authMiddleware.AuthClient, multiTenant bool) *seamAuthE2E {
	t.Helper()

	e := &seamAuthE2E{}

	ctrl := gomock.NewController(t)
	clk := testutil.NewMockClock(reserveFixtureTime)

	svc, err := services.NewReservationService(
		pgdbMocks.NewMockTxBeginner(ctrl),
		servicesMocks.NewMockLimitResolver(ctrl),
		servicesMocks.NewMockReservationRepository(ctrl),
		servicesMocks.NewMockReservationAuditWriter(ctrl),
		servicesMocks.NewMockRuleEvaluator(ctrl),
		clk,
	)
	require.NoError(t, err)

	server, err := NewReservationServer(svc, clk)
	require.NoError(t, err)

	interceptors := []grpc.UnaryServerInterceptor{
		authMiddleware.NewGRPCAuthUnaryPolicy(authClient, SeamAuthPolicyConfig()),
		SeamPrincipalInterceptor(SeamPrincipalConfig{MultiTenant: multiTenant, AllowedClients: []string{seamTestClient}}),
	}

	if multiTenant {
		resolver := seamtenant.NewResolverWithPool(func(_ context.Context, tenantID string) (dbresolver.DB, error) {
			e.tenantMu.Lock()
			e.tenants = append(e.tenants, tenantID)
			e.tenantMu.Unlock()

			return stubPoolDB(t), nil
		}, true)

		interceptors = append(interceptors, TokenTenantUnaryInterceptor(resolver, nil))
	}

	interceptors = append(interceptors, func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		e.tenantMu.Lock()
		e.admitted = append(e.admitted, ctx)
		e.tenantMu.Unlock()

		return handler(ctx, req)
	})

	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptors...))
	reservationv1.RegisterReservationServiceServer(grpcServer, server)

	go func() { _ = grpcServer.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		grpcServer.Stop()
		_ = lis.Close()
	})

	e.client = reservationv1.NewReservationServiceClient(conn)

	return e
}

// confirm issues ConfirmByTransaction with an empty transaction id: a call
// the chain admits reaches the handler and answers InvalidArgument before any
// port is touched.
func (e *seamAuthE2E) confirm(t *testing.T, token string, pairs ...string) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if token != "" {
		pairs = append(pairs, "authorization", "Bearer "+token)
	}

	if len(pairs) > 0 {
		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
	}

	_, err := e.client.ConfirmByTransaction(ctx, &reservationv1.ConfirmByTransactionRequest{})

	return err
}

// lastAdmitted returns the context the handler received on the last call the
// whole chain admitted.
func (e *seamAuthE2E) lastAdmitted(t *testing.T) context.Context {
	t.Helper()

	e.tenantMu.Lock()
	defer e.tenantMu.Unlock()

	require.NotEmpty(t, e.admitted, "no call reached the handler")

	return e.admitted[len(e.admitted)-1]
}

func (e *seamAuthE2E) resolvedTenants() []string {
	e.tenantMu.Lock()
	defer e.tenantMu.Unlock()

	return append([]string(nil), e.tenants...)
}

// handlerReached is the status a call admitted by the whole chain answers.
const handlerReached = codes.InvalidArgument

// TestReservationSeamAuthE2E_SingleTenant drives the token chain end to end
// against a fake Access Manager. It mutates the process environment lib-auth
// reads, so it does not run in parallel.
func TestReservationSeamAuthE2E_SingleTenant(t *testing.T) {
	t.Setenv("AUTH_M2M_INVERSION_ENABLED", "true")
	t.Setenv("AUTH_CACHE_TTL", "")
	t.Setenv("MULTI_TENANT_ENABLED", "")

	am := newFakeAccessManager(t, seamTestClient, seamTestOtherClient, "lerian/alice")
	e := newSeamAuthE2E(t, authMiddleware.NewAuthClient(am.server.URL, true, testutil.NewMockLogger()), false)

	allowed := seamTestToken(t, applicationClaims(seamTestClient, ""))

	t.Run("allowlisted application token reaches the handler", func(t *testing.T) {
		require.Equal(t, handlerReached, status.Code(e.confirm(t, allowed)))

		got := am.lastAuthorize(t)
		assert.Equal(t, seamTestClient, got.Sub, "the application authorizes under its own sub")
		assert.Equal(t, "reservations", got.Resource)
		assert.Equal(t, "post", got.Action)
	})

	t.Run("user token refused by the Access Manager is permission denied", func(t *testing.T) {
		before := am.authorizes.Load()
		token := seamTestToken(t, jwt.MapClaims{"type": "normal-user", "sub": "bob", "owner": "lerian"})
		err := e.confirm(t, token)

		assert.Equal(t, codes.PermissionDenied, status.Code(err))
		assert.Equal(t, before+1, am.authorizes.Load(), "the refusal is the Access Manager's decision")
		assert.Equal(t, "lerian/bob", am.lastAuthorize(t).Sub, "the Access Manager saw the user's subject")
		assert.NotEqual(t, seamPrincipalRefusedMessage, status.Convert(err).Message(),
			"the call never reached the principal guard")
	})

	t.Run("user token granted by the Access Manager is still refused by the guard", func(t *testing.T) {
		token := seamTestToken(t, jwt.MapClaims{"type": "normal-user", "sub": "alice", "owner": "lerian"})
		err := e.confirm(t, token)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
		assert.Equal(t, seamPrincipalRefusedMessage, status.Convert(err).Message())
	})

	t.Run("application token outside the allowlist is permission denied", func(t *testing.T) {
		token := seamTestToken(t, applicationClaims(seamTestOtherClient, ""))
		err := e.confirm(t, token)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
		assert.Equal(t, seamPrincipalRefusedMessage, status.Convert(err).Message())
	})

	t.Run("missing token is unauthenticated without an authorize round trip", func(t *testing.T) {
		before := am.authorizes.Load()
		assert.Equal(t, codes.Unauthenticated, status.Code(e.confirm(t, "")))
		assert.Equal(t, before, am.authorizes.Load())
	})

	t.Run("Access Manager outage is unavailable", func(t *testing.T) {
		am.down.Store(true)
		defer am.down.Store(false)

		assert.Equal(t, codes.Unavailable, status.Code(e.confirm(t, allowed)))
	})

	t.Run("without a decision cache every call is authorized again", func(t *testing.T) {
		before := am.authorizes.Load()

		require.Equal(t, handlerReached, status.Code(e.confirm(t, allowed)))
		require.Equal(t, handlerReached, status.Code(e.confirm(t, allowed)))
		assert.Equal(t, before+2, am.authorizes.Load())
	})
}

// TestReservationSeamAuthE2E_DecisionCache proves AUTH_CACHE_TTL spares the
// Access Manager a round trip per reservation.
func TestReservationSeamAuthE2E_DecisionCache(t *testing.T) {
	t.Setenv("AUTH_M2M_INVERSION_ENABLED", "true")
	t.Setenv("AUTH_CACHE_TTL", "60s")
	t.Setenv("MULTI_TENANT_ENABLED", "")

	am := newFakeAccessManager(t, seamTestClient)
	e := newSeamAuthE2E(t, authMiddleware.NewAuthClient(am.server.URL, true, testutil.NewMockLogger()), false)

	allowed := seamTestToken(t, applicationClaims(seamTestClient, ""))

	require.Equal(t, handlerReached, status.Code(e.confirm(t, allowed)))
	require.Equal(t, handlerReached, status.Code(e.confirm(t, allowed)))
	assert.Equal(t, int64(1), am.authorizes.Load(), "the second allowed call is served from the decision cache")
}

// TestReservationSeamAuthE2E_MultiTenant proves the tenant comes from the
// token's tenantId claim, in its canonical form, and that only the ledger's
// client of that tenant is admitted even when other clients hold the grant.
func TestReservationSeamAuthE2E_MultiTenant(t *testing.T) {
	t.Setenv("AUTH_M2M_INVERSION_ENABLED", "true")
	t.Setenv("AUTH_CACHE_TTL", "")
	t.Setenv("MULTI_TENANT_ENABLED", "true")

	ledgerA := ledgerClaims(seamTenantA, seamTenantA)
	ledgerB := ledgerClaims(seamTenantB, seamTenantB)

	otherProduct := ledgerClaims(seamTenantA, seamTenantA)
	otherProduct["name"] = "flowker-m2m-tracer-" + seamTenantA
	otherProduct["sub"] = "admin/flowker-m2m-tracer-" + seamTenantA

	am := newFakeAccessManager(t, ledgerA["sub"].(string), ledgerB["sub"].(string), otherProduct["sub"].(string))
	e := newSeamAuthE2E(t, authMiddleware.NewAuthClient(am.server.URL, true, testutil.NewMockLogger()), true)

	withClaim := seamTestToken(t, ledgerA)

	t.Run("claim tenant without x-tenant-id", func(t *testing.T) {
		require.Equal(t, handlerReached, status.Code(e.confirm(t, withClaim)))
		assert.Equal(t, seamTenantA, lastOf(e.resolvedTenants()))
	})

	t.Run("claim tenant with an equal x-tenant-id", func(t *testing.T) {
		require.Equal(t, handlerReached, status.Code(e.confirm(t, withClaim, seamtenant.MetadataKey, seamTenantA)))
		assert.Equal(t, seamTenantA, lastOf(e.resolvedTenants()))
	})

	t.Run("a forged md-tenant-id is overwritten by the claim", func(t *testing.T) {
		require.Equal(t, handlerReached, status.Code(e.confirm(t, withClaim, TokenTenantMetadataKey, seamTenantB)))
		assert.Equal(t, seamTenantA, lastOf(e.resolvedTenants()))
	})

	t.Run("dashed claim with the dashed x-tenant-id resolves the canonical tenant", func(t *testing.T) {
		token := seamTestToken(t, ledgerClaims(seamTenantA, seamTenantADashed))

		require.Equal(t, handlerReached, status.Code(e.confirm(t, token, seamtenant.MetadataKey, seamTenantADashed)))
		assert.Equal(t, seamTenantA, lastOf(e.resolvedTenants()), "the tenant resolves in its canonical dashless form")
	})

	t.Run("another product's client holding the grant is permission denied", func(t *testing.T) {
		before := len(e.resolvedTenants())
		err := e.confirm(t, seamTestToken(t, otherProduct))

		assert.Equal(t, codes.PermissionDenied, status.Code(err))
		assert.Equal(t, seamPrincipalRefusedMessage, status.Convert(err).Message(), "the guard refused it after the grant passed")
		assert.Equal(t, otherProduct["sub"], am.lastAuthorize(t).Sub)
		assert.Len(t, e.resolvedTenants(), before, "a refused call resolves no tenant")
	})

	t.Run("the ledger client of tenant B with a tenant A claim is permission denied", func(t *testing.T) {
		before := len(e.resolvedTenants())
		crossed := ledgerClaims(seamTenantB, seamTenantA)
		crossed["sub"] = ledgerB["sub"]

		err := e.confirm(t, seamTestToken(t, crossed))
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
		assert.Equal(t, seamPrincipalRefusedMessage, status.Convert(err).Message())
		assert.Len(t, e.resolvedTenants(), before)
	})

	t.Run("token without tenantId is permission denied", func(t *testing.T) {
		before := len(e.resolvedTenants())
		token := seamTestToken(t, ledgerClaims(seamTenantA, ""))

		assert.Equal(t, codes.PermissionDenied, status.Code(e.confirm(t, token, seamtenant.MetadataKey, seamTenantA)))
		assert.Len(t, e.resolvedTenants(), before, "a refused call resolves no tenant")
	})

	t.Run("x-tenant-id that differs from the claim is permission denied", func(t *testing.T) {
		before := len(e.resolvedTenants())

		assert.Equal(t, codes.PermissionDenied, status.Code(e.confirm(t, withClaim, seamtenant.MetadataKey, seamTenantB)))
		assert.Len(t, e.resolvedTenants(), before)
	})
}

func lastOf(values []string) string {
	if len(values) == 0 {
		return ""
	}

	return values[len(values)-1]
}

// TestReservationSeamAuthE2E_AuditActorIsTheLedgerApplication proves a
// reservation admitted on the ledger's token is audited under the ledger's
// application (its sub and name claims), never the system actor.
func TestReservationSeamAuthE2E_AuditActorIsTheLedgerApplication(t *testing.T) {
	t.Setenv("AUTH_M2M_INVERSION_ENABLED", "true")
	t.Setenv("AUTH_CACHE_TTL", "")
	t.Setenv("MULTI_TENANT_ENABLED", "true")

	ledgerA := ledgerClaims(seamTenantA, seamTenantA)

	am := newFakeAccessManager(t, ledgerA["sub"].(string))
	e := newSeamAuthE2E(t, authMiddleware.NewAuthClient(am.server.URL, true, testutil.NewMockLogger()), true)

	require.Equal(t, handlerReached, status.Code(e.confirm(t, seamTestToken(t, ledgerA))))

	ctrl := gomock.NewController(t)
	repo := commandMocks.NewMockAuditEventRepository(ctrl)

	var recorded *model.AuditEvent

	repo.EXPECT().Insert(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, event *model.AuditEvent) error {
			recorded = event
			return nil
		})

	err := command.NewRecordAuditEventCommand(repo).RecordReservationEvent(
		e.lastAdmitted(t),
		model.AuditEventReservationConfirmed,
		model.AuditActionConfirm,
		testutil.MustDeterministicUUID(901),
		command.ReservationAuditContext{TransactionID: testutil.MustDeterministicUUID(902)},
	)
	require.NoError(t, err)
	require.NotNil(t, recorded)

	assert.Equal(t, model.ActorTypeUser, recorded.Actor.ActorType)
	assert.Equal(t, "admin/ledger-m2m-tracer-"+seamTenantA, recorded.Actor.ID, "the actor is the ledger application's sub")
	assert.Equal(t, "ledger-m2m-tracer-"+seamTenantA, recorded.Actor.Name, "the actor carries the application name")
}
