// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package tracer

// End-to-end proof that the multi-tenant reservation seam keeps tenant A's
// reservation state invisible to tenant B across the secured gRPC + mutual-TLS
// transport. It is the Task 2.2.1 capstone for Phase 2 (trusted x-tenant-id
// propagation + tenant-scoped resolution on the reservation surface).
//
// The flow under test:
//
//   - The REAL ledger *TracerGRPCClient drives a Reserve under tenant A's
//     trusted x-tenant-id over a REAL mutual-TLS handshake (the same
//     credentials.NewTLS seam the composition root wires in mtls mode). The
//     tenant rides the client's production propagation path: it is read from the
//     request ctx via tmcore.GetTenantIDContext and appended to outgoing metadata
//     by the client's tenantUnaryInterceptor — no hand-rolled header here.
//   - The reconstructed tracer runs a faithful LOCAL equivalent of the tracer's
//     server-side resolution: a gRPC interceptor that mirrors the production
//     in.TenantUnaryInterceptor + seamtenant.Resolver (MT mode) line for line —
//     read the trusted x-tenant-id from incoming metadata under the same wire key
//     ("x-tenant-id"), reject a missing/invalid key with codes.InvalidArgument
//     (never default), validate via the SAME lib-commons tmcore.IsValidTenantID,
//     and bind the per-tenant pool + tenant id into ctx via the SAME
//     tmcore.ContextWithPG / ContextWithTenantID the resolver uses. The
//     reconstruction is forced by Go's internal/ rule: components/tracer/internal/
//     (the production interceptor and resolver) is walled off from this ledger
//     test package, exactly as the Phase 1 seam_mtls_integration_test.go documents
//     for the server adapter. The wire key and tmcore binding calls are the real
//     ones, so the contract under test is the production one. The handler routes
//     its reservation state through the bound tenant id (tmcore.GetTenantIDContext),
//     so a read under a different tenant key resolves a DIFFERENT store and cannot
//     see tenant A's reservation.
//   - A subsequent ConfirmByTransaction under tenant B's key MUST NOT find tenant
//     A's reservation: the seam routes B to B's (empty) store. And a request that
//     forwards NO tenant key is rejected with codes.InvalidArgument before the
//     handler runs — the seam never falls back to a default/wrong pool.
//
// HONEST REACH — what this proves and what it does NOT:
//
//   - PROVEN, no Docker: the isolation MECHANISM on the seam end-to-end. The
//     trusted tenant key flows ledger -> tracer over the real mTLS+gRPC
//     transport (real ledger client, real handshake); the resolution path routes
//     each tenant to its OWN pool (asserted: the dbresolver.DB bound for A differs
//     from B's, and the handler-visible tenant id matches the caller's key); a
//     missing key is rejected, never defaulted. Tenant A's reservation is
//     invisible to tenant B at the tenant-resolution layer. The resolution logic
//     is a local mirror of the production interceptor/resolver (internal/ wall),
//     using the real tmcore wire key + binding calls.
//   - NOT PROVEN here: PostgreSQL row-level / connection-level isolation. That
//     requires real per-tenant databases and is Docker/testcontainer-gated (the
//     tracer's own integration suite owns it). This test uses an in-memory store
//     keyed by the resolver-bound tenant id plus distinct sqlmock-backed pools, so
//     it does not exercise a live DB and makes no claim of DB-level isolation.
//
// No Docker is required: loopback sockets, a deterministic cert fixture (shared
// generateSeamMTLSFixture, fixed 2020->2100 validity, no time.Now), and an
// in-memory tenant store make the test hermetic. It is tagged integration
// because it binds a real socket and performs a real TLS handshake.

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// The two trusted tenant keys. Fixed literals (no uuid.New), valid per
// tmcore.IsValidTenantID. tenant A reserves; tenant B must not see it.
const (
	isolationTenantA = "tenant-a"
	isolationTenantB = "tenant-b"
)

// isolationTenantMetadataKey is the gRPC metadata key the tracer reads the
// trusted tenant id from. It MUST equal seamtenant.MetadataKey on the tracer and
// the ledger client's tenantMetadataKey (both "x-tenant-id"); the internal/ wall
// blocks referencing the tracer constant directly, so it is reasserted here and
// the cross-binary contract is held by the equality assertion in the propagation
// tests (grpc_client_test.go) plus the tracer's tenant_interceptor_test.go.
const isolationTenantMetadataKey = "x-tenant-id"

// TestSeamMTLSTenantIsolation drives the real ledger gRPC client over a real
// mutual-TLS handshake against the reconstructed tracer running the PRODUCTION
// tenant interceptor + resolver, and proves tenant B cannot observe tenant A's
// reservation made over the same secured seam.
func TestSeamMTLSTenantIsolation(t *testing.T) {
	fixture := generateSeamMTLSFixture(t)
	serverTLS := serverMTLSConfig(t, fixture)
	clientTLS := clientMTLSConfig(t, fixture)

	store := newTenantReservationStore()

	// pools hands each tenant its OWN *dbresolver.DB so pool identity per tenant
	// is observable. A nil pool would make the resolver a no-op, so each tenant
	// gets a real (sqlmock-backed) pool; the boundary the test asserts is the
	// per-tenant ROUTING, not query execution.
	poolByTenant := map[string]dbresolver.DB{
		isolationTenantA: newStubPool(t),
		isolationTenantB: newStubPool(t),
	}

	handler := &isolationReservationServer{store: store}
	addr := startIsolationGRPCServer(t, serverTLS, poolByTenant, handler)

	client, err := NewTracerGRPCClient(addr,
		WithGRPCOperationTimeout(5*time.Second),
		WithGRPCDialOptions(grpc.WithTransportCredentials(credentials.NewTLS(clientTLS))))
	require.NoError(t, err)

	t.Cleanup(func() { _ = client.Close() })

	t.Run("tenant A reserves over the secured seam", func(t *testing.T) {
		ctxA := tmcore.ContextWithTenantID(context.Background(), isolationTenantA)
		ctxA, cancel := context.WithTimeout(ctxA, 5*time.Second)
		defer cancel()

		result, err := client.Reserve(ctxA, ReserveRequest{TransactionID: fixedTransactionID})
		require.NoError(t, err, "tenant A reserve must complete over mTLS")
		require.NotNil(t, result)
		require.False(t, result.Denied)
		require.Equal(t, []uuid.UUID{fixedReservationID}, result.ReservationIDs)
	})

	t.Run("tenant A's reservation is visible to tenant A", func(t *testing.T) {
		require.True(t, store.has(isolationTenantA, fixedTransactionID.String()),
			"the resolver must have routed tenant A's reserve into tenant A's store")
	})

	t.Run("tenant B cannot see tenant A's reservation via ConfirmByTransaction", func(t *testing.T) {
		ctxB := tmcore.ContextWithTenantID(context.Background(), isolationTenantB)
		ctxB, cancel := context.WithTimeout(ctxB, 5*time.Second)
		defer cancel()

		// ConfirmByTransaction routes to tenant B's store, which has nothing for
		// this transaction. The handler signals "not found for this tenant" as a
		// codes.NotFound — proving B's view excludes A's reservation rather than
		// silently confirming A's state under B's key.
		err := client.ConfirmByTransaction(ctxB, fixedTransactionID)
		require.Error(t, err, "tenant B must not be able to confirm tenant A's transaction")
		require.Equal(t, codes.NotFound, status.Code(err),
			"the seam must report B's view as empty, not confirm A's reservation")

		// Belt-and-suspenders: A's reservation is untouched in A's store and
		// absent from B's store.
		require.True(t, store.has(isolationTenantA, fixedTransactionID.String()),
			"tenant A's reservation must remain after tenant B's failed confirm")
		require.False(t, store.has(isolationTenantB, fixedTransactionID.String()),
			"tenant B's store must never contain tenant A's reservation")
	})

	t.Run("the resolver bound a distinct pool per tenant", func(t *testing.T) {
		// The handler captured the *dbresolver.DB the resolver bound on each call.
		// Distinct identities prove the seam routed A and B to different pools —
		// the resolution-layer isolation invariant.
		require.NotNil(t, handler.boundPool(isolationTenantA))
		require.NotNil(t, handler.boundPool(isolationTenantB))
		require.NotSame(t, handler.boundPool(isolationTenantA), handler.boundPool(isolationTenantB),
			"each tenant must resolve to its own pool")
		require.Same(t, poolByTenant[isolationTenantA], handler.boundPool(isolationTenantA),
			"tenant A must resolve to tenant A's pool")
		require.Same(t, poolByTenant[isolationTenantB], handler.boundPool(isolationTenantB),
			"tenant B must resolve to tenant B's pool")
	})

	t.Run("a request without a tenant key is rejected, not defaulted", func(t *testing.T) {
		// No tmcore tenant on the ctx -> the client appends no metadata -> the
		// server interceptor sees no key -> InvalidArgument before the handler.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := client.Reserve(ctx, ReserveRequest{TransactionID: fixedTransactionID})
		require.Error(t, err, "a tenantless reservation over the seam must be rejected under MT")
		require.Equal(t, codes.InvalidArgument, status.Code(err),
			"a missing tenant key must fail clean, never resolve a default pool")
	})
}

// isolationReservationServer is the reconstructed tracer reservation service for
// the isolation test. It does NOT trust the request body for tenant scoping: it
// reads the tenant id the interceptor bound into ctx and routes all state
// through that tenant's slice of the in-memory store. This is the server-side
// stand-in for tenant-scoped repositories, with the resolution path mirroring
// the production interceptor + resolver.
type isolationReservationServer struct {
	reservationv1.UnimplementedReservationServiceServer

	store *tenantReservationStore

	mu    sync.Mutex
	bound map[string]dbresolver.DB
}

// Reserve records the reservation under the resolver-bound tenant's store. It
// fails if no tenant was bound, so a resolution miss can never write to a
// default store.
func (s *isolationReservationServer) Reserve(ctx context.Context, req *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		return nil, status.Error(codes.Internal, "no tenant bound on the reserve handler ctx")
	}

	s.recordBoundPool(ctx, tenantID)
	s.store.add(tenantID, req.GetTransactionId())

	return &reservationv1.ReserveResult{
		TransactionId:  req.GetTransactionId(),
		Denied:         false,
		ReservationIds: []string{fixedReservationID.String()},
	}, nil
}

// ConfirmByTransaction confirms only within the resolver-bound tenant's store.
// A transaction absent from THIS tenant's view yields codes.NotFound — the
// visibility boundary the isolation test asserts.
func (s *isolationReservationServer) ConfirmByTransaction(ctx context.Context, req *reservationv1.ConfirmByTransactionRequest) (*reservationv1.ConfirmByTransactionResponse, error) {
	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		return nil, status.Error(codes.Internal, "no tenant bound on the confirm handler ctx")
	}

	s.recordBoundPool(ctx, tenantID)

	if !s.store.has(tenantID, req.GetTransactionId()) {
		return nil, status.Error(codes.NotFound, "transaction not found for this tenant")
	}

	return &reservationv1.ConfirmByTransactionResponse{}, nil
}

// recordBoundPool captures the *dbresolver.DB the resolver bound into ctx so the
// test can assert per-tenant pool identity. The handler reads it the same way a
// repository would, via tmcore.GetPGContext.
func (s *isolationReservationServer) recordBoundPool(ctx context.Context, tenantID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.bound == nil {
		s.bound = make(map[string]dbresolver.DB)
	}

	s.bound[tenantID] = tmcore.GetPGContext(ctx)
}

// boundPool returns the pool the resolver bound for tenantID on the last call.
func (s *isolationReservationServer) boundPool(tenantID string) dbresolver.DB {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.bound[tenantID]
}

// tenantReservationStore is an in-memory, per-tenant reservation set keyed by
// tenant id then transaction id. It stands in for tenant-scoped persistence: a
// read under one tenant's key cannot observe another tenant's entries.
type tenantReservationStore struct {
	mu     sync.Mutex
	byTena map[string]map[string]struct{}
}

func newTenantReservationStore() *tenantReservationStore {
	return &tenantReservationStore{byTena: make(map[string]map[string]struct{})}
}

func (s *tenantReservationStore) add(tenantID, transactionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.byTena[tenantID] == nil {
		s.byTena[tenantID] = make(map[string]struct{})
	}

	s.byTena[tenantID][transactionID] = struct{}{}
}

func (s *tenantReservationStore) has(tenantID, transactionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.byTena[tenantID][transactionID]

	return ok
}

// startIsolationGRPCServer stands up a gRPC server secured by serverTLS
// (RequireAndVerifyClientCert) with the reconstructed tenant interceptor chained
// in front of the reservation handler, registers the handler, and returns its
// loopback host:port. Stopped on cleanup.
func startIsolationGRPCServer(t *testing.T, serverTLS *tls.Config, poolByTenant map[string]dbresolver.DB, handler reservationv1.ReservationServiceServer) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(serverTLS)),
		grpc.ChainUnaryInterceptor(reconstructedTenantInterceptor(poolByTenant)),
	)
	reservationv1.RegisterReservationServiceServer(server, handler)

	go func() { _ = server.Serve(listener) }()

	t.Cleanup(server.Stop)

	return listener.Addr().String()
}

// reconstructedTenantInterceptor mirrors the tracer's production
// in.TenantUnaryInterceptor + seamtenant.Resolver (multi-tenant mode), which the
// internal/ wall blocks importing here. Line-for-line equivalent:
//   - read the trusted tenant id from incoming metadata under the same wire key;
//   - a missing/empty/invalid key fails with codes.InvalidArgument and NEVER
//     resolves a default/wrong pool (the hard cross-tenant-isolation invariant);
//   - validation uses the real lib-commons tmcore.IsValidTenantID;
//   - the resolved pool + tenant id are bound into ctx via the real
//     tmcore.ContextWithPG / ContextWithTenantID, exactly as the resolver does.
func reconstructedTenantInterceptor(poolByTenant map[string]dbresolver.DB) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		tenantID := tenantIDFromIncomingMetadata(ctx)
		if tenantID == "" || !tmcore.IsValidTenantID(tenantID) {
			return nil, status.Error(codes.InvalidArgument, "reservation tenant required")
		}

		db, ok := poolByTenant[tenantID]
		if !ok {
			return nil, status.Error(codes.Internal, "no pool for tenant")
		}

		ctx = tmcore.ContextWithTenantID(ctx, tenantID)
		ctx = tmcore.ContextWithPG(ctx, db)

		return handler(ctx, req)
	}
}

// tenantIDFromIncomingMetadata reads the trusted tenant id from incoming gRPC
// metadata under isolationTenantMetadataKey, mirroring the tracer's
// tenantIDFromMetadata. Empty when absent.
func tenantIDFromIncomingMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	values := md.Get(isolationTenantMetadataKey)
	if len(values) == 0 {
		return ""
	}

	return values[0]
}

// newStubPool builds a real dbresolver.DB over a sqlmock connection, mirroring
// the tracer seamtenant resolver_test pattern. The pool is never queried here;
// it exists so the resolver binds a non-nil, per-tenant-distinct pool the test
// can assert identity on.
func newStubPool(t *testing.T) dbresolver.DB {
	t.Helper()

	sqlDB, _, err := sqlmock.New()
	require.NoError(t, err)

	t.Cleanup(func() { _ = sqlDB.Close() })

	return dbresolver.New(dbresolver.WithPrimaryDBs(sqlDB))
}
