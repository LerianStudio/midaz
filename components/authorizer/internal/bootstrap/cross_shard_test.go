// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// Test sentinel errors for cross-shard test stubs.
var (
	errTestPeerUnavailable = errors.New("peer unavailable")
	errTestPublishFailed   = errors.New("publish failed")
	errTestRedpandaDown    = errors.New("redpanda broker unreachable")
)

type stubPeerClient struct {
	prepareResp *authorizerv1.PrepareAuthorizeResponse
	prepareErr  error
	commitResp  *authorizerv1.CommitPreparedResponse
	commitErr   error
	abortErr    error

	onPrepare func()

	mu         sync.Mutex
	abortCalls int
}

func peerAuthIncomingContext(t *testing.T, token, method string, req proto.Message) context.Context {
	t.Helper()

	ctx, err := withPeerAuth(context.Background(), token, method, req)
	require.NoError(t, err)

	md, _ := metadata.FromOutgoingContext(ctx)

	return metadata.NewIncomingContext(context.Background(), md)
}

func (s *stubPeerClient) Authorize(_ context.Context, _ *authorizerv1.AuthorizeRequest, _ ...grpc.CallOption) (*authorizerv1.AuthorizeResponse, error) {
	return &authorizerv1.AuthorizeResponse{}, nil
}

func (s *stubPeerClient) AuthorizeStream(_ context.Context, _ ...grpc.CallOption) (grpc.BidiStreamingClient[authorizerv1.AuthorizeRequest, authorizerv1.AuthorizeResponse], error) {
	return nil, nil
}

func (s *stubPeerClient) LoadBalances(_ context.Context, _ *authorizerv1.LoadBalancesRequest, _ ...grpc.CallOption) (*authorizerv1.LoadBalancesResponse, error) {
	return &authorizerv1.LoadBalancesResponse{}, nil
}

func (s *stubPeerClient) GetBalance(_ context.Context, _ *authorizerv1.GetBalanceRequest, _ ...grpc.CallOption) (*authorizerv1.GetBalanceResponse, error) {
	return &authorizerv1.GetBalanceResponse{}, nil
}

func (s *stubPeerClient) PublishBalanceOperations(_ context.Context, _ *authorizerv1.PublishBalanceOperationsRequest, _ ...grpc.CallOption) (*authorizerv1.PublishBalanceOperationsResponse, error) {
	return &authorizerv1.PublishBalanceOperationsResponse{}, nil
}

func (s *stubPeerClient) PrepareAuthorize(_ context.Context, _ *authorizerv1.AuthorizeRequest, _ ...grpc.CallOption) (*authorizerv1.PrepareAuthorizeResponse, error) {
	if s.onPrepare != nil {
		s.onPrepare()
	}

	if s.prepareErr != nil {
		return nil, s.prepareErr
	}

	if s.prepareResp != nil {
		return s.prepareResp, nil
	}

	return &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-peer"}, nil
}

func (s *stubPeerClient) CommitPrepared(_ context.Context, _ *authorizerv1.CommitPreparedRequest, _ ...grpc.CallOption) (*authorizerv1.CommitPreparedResponse, error) {
	if s.commitErr != nil {
		return nil, s.commitErr
	}

	if s.commitResp != nil {
		return s.commitResp, nil
	}

	return &authorizerv1.CommitPreparedResponse{Committed: true}, nil
}

func (s *stubPeerClient) AbortPrepared(_ context.Context, _ *authorizerv1.AbortPreparedRequest, _ ...grpc.CallOption) (*authorizerv1.AbortPreparedResponse, error) {
	s.mu.Lock()
	s.abortCalls++
	s.mu.Unlock()

	if s.abortErr != nil {
		return nil, s.abortErr
	}

	return &authorizerv1.AbortPreparedResponse{Aborted: true}, nil
}

func TestPrepareAuthorizeAllowsEmptyOperationsWithoutPanic(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	svc := &authorizerService{
		engine:         eng,
		peerAuthToken:  "peer-secret",
		peerNonceStore: newPeerNonceStore(defaultPeerAuthMaxSkew, 100000),
	}

	prepareReq := &authorizerv1.AuthorizeRequest{}
	ctx := peerAuthIncomingContext(t, "peer-secret", peerRPCMethodPrepareAuthorize, prepareReq)

	resp, err := svc.PrepareAuthorize(ctx, prepareReq)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.GetAuthorized())
	require.Empty(t, resp.GetPreparedTxId())
}

func TestPrepareAuthorizeRejectsMissingPeerToken(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	svc := &authorizerService{
		engine:        eng,
		peerAuthToken: "peer-secret",
	}

	_, err := svc.PrepareAuthorize(context.Background(), &authorizerv1.AuthorizeRequest{})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestCommitAndAbortPreparedValidateRequest(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	svc := &authorizerService{
		engine:         eng,
		peerAuthToken:  "peer-secret",
		peerNonceStore: newPeerNonceStore(defaultPeerAuthMaxSkew, 100000),
	}

	commitReq := &authorizerv1.CommitPreparedRequest{}
	authCommitCtx := peerAuthIncomingContext(t, "peer-secret", peerRPCMethodCommitPrepared, commitReq)

	_, err := svc.CommitPrepared(authCommitCtx, commitReq)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	abortReq := &authorizerv1.AbortPreparedRequest{}
	authAbortCtx := peerAuthIncomingContext(t, "peer-secret", peerRPCMethodAbortPrepared, abortReq)
	_, err = svc.AbortPrepared(authAbortCtx, abortReq)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestAuthorizeCrossShardRejectsRequestExceedingGlobalOperationLimit(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.ConfigureAuthorizationLimits(2, 10)
	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b-local",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	peerPrepareCalled := false
	peer := &stubPeerClient{
		onPrepare: func() {
			peerPrepareCalled = true
		},
	}

	svc := &authorizerService{
		engine:          eng,
		logger:          mustInitLogger(t),
		ownedShardStart: 0,
		ownedShardEnd:   3,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			clients:    []authorizerv1.BalanceAuthorizerClient{peer},
			shardStart: 4,
			shardEnd:   7,
		}},
	}

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-limit-ops",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1", AccountAlias: "@bob", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.CREDIT},
			{OperationAlias: "2", AccountAlias: "@remote", BalanceKey: "default", Amount: 50, Scale: 2, Operation: constant.CREDIT},
		},
	}

	shardOps := map[int][]*authorizerv1.BalanceOperation{
		1: {req.GetOperations()[0], req.GetOperations()[1]},
		6: {req.GetOperations()[2]},
	}

	resp, err := svc.authorizeCrossShard(context.Background(), req, shardOps)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.False(t, resp.GetAuthorized())
	require.Equal(t, engine.RejectionRequestTooLarge, resp.GetRejectionCode())
	require.Contains(t, resp.GetRejectionMessage(), "operations exceed allowed request limit")
	require.False(t, peerPrepareCalled)

	balance, ok := eng.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(1000), balance.Available)
	require.Equal(t, uint64(1), balance.Version)
}

func TestAuthorizeCrossShardRejectsRequestExceedingGlobalUniqueBalanceLimit(t *testing.T) {
	// The unique-balance limit is enforced per-participant (per PrepareAuthorize call),
	// not globally across all shards. A cross-shard request is split into per-participant
	// sub-requests, so the limit fires when a single participant's sub-request references
	// more unique balances than allowed.
	//
	// This test: limit=1, local participant has 2 ops for 2 different local balances.
	// Both balances are in the local shard range (shard 0-3), so the local prepare
	// sub-request has 2 unique balances, which exceeds the limit of 1.
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()
	// maxOps=10, maxUniqueBalances=1 — any sub-request with 2+ unique balances is rejected.
	eng.ConfigureAuthorizationLimits(10, 1)

	// Load both balances so they resolve (limit check happens after balance lookup).
	// Use AccountAlias values that hash to shards 0-3 (local range) via the router.
	// We use shard.NewRouter(8) so shards 0-3 are local. The router hashes aliases
	// deterministically, so we pre-set shardOps manually below to control the routing.
	eng.UpsertBalances([]*engine.Balance{
		{
			ID:             "b-alice",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     "default",
			AssetCode:      "USD",
			Available:      1000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
		{
			ID:             "b-bob",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@bob",
			BalanceKey:     "default",
			AssetCode:      "USD",
			Available:      1000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	peerPrepareCalled := false
	peer := &stubPeerClient{
		onPrepare: func() {
			peerPrepareCalled = true
		},
	}

	svc := &authorizerService{
		engine:          eng,
		logger:          mustInitLogger(t),
		ownedShardStart: 0,
		ownedShardEnd:   3,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			clients:    []authorizerv1.BalanceAuthorizerClient{peer},
			shardStart: 4,
			shardEnd:   7,
		}},
	}

	// Both ops are routed to local shards (0-3) so the local PrepareAuthorize
	// sub-request references 2 unique balances, exceeding the limit of 1.
	// We pick shard IDs 1 and 2 explicitly (both within local range 0-3).
	op0 := &authorizerv1.BalanceOperation{
		OperationAlias: "0", AccountAlias: "@alice", BalanceKey: "default",
		Amount: 100, Scale: 2, Operation: constant.DEBIT,
	}
	op1 := &authorizerv1.BalanceOperation{
		OperationAlias: "1", AccountAlias: "@bob", BalanceKey: "default",
		Amount: 100, Scale: 2, Operation: constant.CREDIT,
	}

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-limit-unique",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations:        []*authorizerv1.BalanceOperation{op0, op1},
	}

	// Both ops go to local shards (1 and 2) — the remote peer is not involved.
	// shardOps mirrors what engine.ResolveOperationShards would return if @alice
	// and @bob happened to route to different local shards.
	shardOps := map[int][]*authorizerv1.BalanceOperation{
		1: {op0},
		2: {op1},
	}

	resp, err := svc.authorizeCrossShard(context.Background(), req, shardOps)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.False(t, resp.GetAuthorized())
	require.Equal(t, engine.RejectionRequestTooLarge, resp.GetRejectionCode())
	require.Contains(t, resp.GetRejectionMessage(), "unique balances exceed allowed request limit")
	// Remote peer must not be contacted — the local prepare rejected first.
	require.False(t, peerPrepareCalled)
}

func TestAuthorizeCrossShardReturnsErrorOnPartialCommit(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b-local",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	pub := &capturingPublisher{}
	peer := &stubPeerClient{
		prepareResp: &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-peer-1"},
		commitErr:   errTestPeerUnavailable,
	}

	svc := &authorizerService{
		engine:          eng,
		pub:             pub,
		logger:          mustInitLogger(t),
		grpcAddr:        "authorizer-1:50051",
		ownedShardStart: 0,
		ownedShardEnd:   3,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			clients:    []authorizerv1.BalanceAuthorizerClient{peer},
			shardStart: 4,
			shardEnd:   7,
		}},
	}

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-cross-1",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1#@remote#default", AccountAlias: "@remote", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.CREDIT},
		},
	}

	shardOps := map[int][]*authorizerv1.BalanceOperation{
		1: {req.GetOperations()[0]},
		5: {req.GetOperations()[1]},
	}

	resp, err := svc.authorizeCrossShard(context.Background(), req, shardOps)
	require.Error(t, err)
	require.Nil(t, resp)
	// Partial-commit state returns FailedPrecondition (non-retryable) so the
	// transaction service maps it to ErrTransactionRequiresManualIntervention
	// rather than the retryable ServiceUnavailable path. Prior to this
	// change the code was Internal — see D4 scope item #6.
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Contains(t, err.Error(), "manual intervention required")

	peer.mu.Lock()
	abortCalls := peer.abortCalls
	peer.mu.Unlock()
	require.Equal(t, 0, abortCalls)

	statuses := make([]string, 0, len(pub.messages))
	for _, msg := range pub.messages {
		var intent commitIntent
		require.NoError(t, json.Unmarshal(msg.Payload, &intent))
		statuses = append(statuses, intent.Status)
	}

	require.Contains(t, statuses, commitIntentStatusPrepared)
	require.Contains(t, statuses, commitIntentStatusCommitted)
	require.NotContains(t, statuses, commitIntentStatusCompleted)
}

func TestAuthorizeCrossShardTreatsPeerCommitNotFoundAsManualIntervention(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b-local",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	pub := &capturingPublisher{}
	peer := &stubPeerClient{
		prepareResp: &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-peer-1"},
		commitErr:   status.Error(codes.NotFound, "prepared transaction not found"),
	}

	svc := &authorizerService{
		engine:          eng,
		pub:             pub,
		logger:          mustInitLogger(t),
		grpcAddr:        "authorizer-1:50051",
		ownedShardStart: 0,
		ownedShardEnd:   3,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			clients:    []authorizerv1.BalanceAuthorizerClient{peer},
			shardStart: 4,
			shardEnd:   7,
		}},
	}

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-cross-notfound-is-idempotent",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1#@remote#default", AccountAlias: "@remote", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.CREDIT},
		},
	}

	shardOps := map[int][]*authorizerv1.BalanceOperation{
		1: {req.GetOperations()[0]},
		5: {req.GetOperations()[1]},
	}

	// NotFound from a peer commit is now treated as ambiguous — the peer may have
	// lost prepared state (restart, expiry). The coordinator must NOT blindly assume
	// committed. Instead, the intent is flagged for manual intervention and the
	// response is FailedPrecondition so the transaction service maps it to a
	// non-retryable business error (D4 scope item #5 + #6).
	resp, err := svc.authorizeCrossShard(context.Background(), req, shardOps)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Contains(t, err.Error(), "manual intervention required")

	statuses := make([]string, 0, len(pub.messages))
	for _, msg := range pub.messages {
		var intent commitIntent
		require.NoError(t, json.Unmarshal(msg.Payload, &intent))
		statuses = append(statuses, intent.Status)
	}

	require.Contains(t, statuses, commitIntentStatusPrepared)
	require.Contains(t, statuses, commitIntentStatusManualIntervention)
	require.NotContains(t, statuses, commitIntentStatusCompleted)
}

func TestAuthorizeCrossShardCommitIntentPublishFailureAbortsPreparedParticipants(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b-local",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	peer := &stubPeerClient{
		prepareResp: &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-peer-1"},
	}

	svc := &authorizerService{
		engine:          eng,
		pub:             &capturingPublisher{err: errTestPublishFailed},
		logger:          mustInitLogger(t),
		grpcAddr:        "authorizer-1:50051",
		ownedShardStart: 0,
		ownedShardEnd:   3,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			clients:    []authorizerv1.BalanceAuthorizerClient{peer},
			shardStart: 4,
			shardEnd:   7,
		}},
	}

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-cross-intent-fail",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1#@remote#default", AccountAlias: "@remote", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.CREDIT},
		},
	}

	shardOps := map[int][]*authorizerv1.BalanceOperation{
		1: {req.GetOperations()[0]},
		5: {req.GetOperations()[1]},
	}

	resp, err := svc.authorizeCrossShard(context.Background(), req, shardOps)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "failed to write commit intent")

	peer.mu.Lock()
	abortCalls := peer.abortCalls
	peer.mu.Unlock()
	require.Equal(t, 1, abortCalls)
}

func TestAuthorizeCrossShardPrepareOrderIncludesLocalRangePosition(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b-local",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      10,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	lowPeer := &stubPeerClient{prepareResp: &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-low"}}
	highPeer := &stubPeerClient{prepareResp: &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-high"}}

	svc := &authorizerService{
		engine:          eng,
		pub:             &capturingPublisher{},
		logger:          mustInitLogger(t),
		grpcAddr:        "authorizer-mid:50051",
		ownedShardStart: 2,
		ownedShardEnd:   5,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{
			{addr: "authorizer-low:50051", clients: []authorizerv1.BalanceAuthorizerClient{lowPeer}, shardStart: 0, shardEnd: 1},
			{addr: "authorizer-high:50051", clients: []authorizerv1.BalanceAuthorizerClient{highPeer}, shardStart: 6, shardEnd: 7},
		},
	}

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-cross-order-local-middle",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@remote-low#default", AccountAlias: "@remote-low", BalanceKey: "default", Amount: 1, Scale: 2, Operation: constant.CREDIT},
			{OperationAlias: "1#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "2#@remote-high#default", AccountAlias: "@remote-high", BalanceKey: "default", Amount: 1, Scale: 2, Operation: constant.CREDIT},
		},
	}

	shardOps := map[int][]*authorizerv1.BalanceOperation{
		0: {req.GetOperations()[0]},
		2: {req.GetOperations()[1]},
		6: {req.GetOperations()[2]},
	}

	resp, err := svc.authorizeCrossShard(context.Background(), req, shardOps)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.False(t, resp.GetAuthorized())
	require.Equal(t, engine.RejectionInsufficientFunds, resp.GetRejectionCode())

	// Under parallel prepare fan-out, a participant either (a) ran to completion
	// and holds a prepared_tx_id — in which case it MUST be aborted — or (b)
	// was cancelled via groupCtx before its goroutine started — in which case
	// it never prepared and has nothing to abort. The safety invariant is:
	// no orphan prepared_tx_ids, so abortCalls ≤ 1 for each peer. The exact
	// count depends on the race between local engine rejection (synchronous,
	// fast) and peer goroutine scheduling. Both outcomes are correct.
	lowPeer.mu.Lock()
	lowAbortCalls := lowPeer.abortCalls
	lowPeer.mu.Unlock()
	require.LessOrEqual(t, lowAbortCalls, 1, "low peer must be aborted at most once")

	highPeer.mu.Lock()
	highAbortCalls := highPeer.abortCalls
	highPeer.mu.Unlock()
	require.LessOrEqual(t, highAbortCalls, 1, "high peer must be aborted at most once")
}

func TestAuthorizeCrossShardPrepareDeadlineAbortsWithBackgroundContext(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	lowPeer := &stubPeerClient{}
	highPeer := &stubPeerClient{}

	ctx, cancel := context.WithCancel(context.Background())
	lowPeer.onPrepare = cancel

	svc := &authorizerService{
		engine:          eng,
		pub:             &capturingPublisher{},
		logger:          mustInitLogger(t),
		grpcAddr:        "authorizer-mid:50051",
		ownedShardStart: 4,
		ownedShardEnd:   7,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{
			{addr: "authorizer-low:50051", clients: []authorizerv1.BalanceAuthorizerClient{lowPeer}, shardStart: 0, shardEnd: 1},
			{addr: "authorizer-high:50051", clients: []authorizerv1.BalanceAuthorizerClient{highPeer}, shardStart: 2, shardEnd: 3},
		},
	}

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-cross-deadline",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@remote-low#default", AccountAlias: "@remote-low", BalanceKey: "default", Amount: 1, Scale: 2, Operation: constant.CREDIT},
			{OperationAlias: "1#@remote-high#default", AccountAlias: "@remote-high", BalanceKey: "default", Amount: 1, Scale: 2, Operation: constant.CREDIT},
			{OperationAlias: "2#@local#default", AccountAlias: "@local", BalanceKey: "default", Amount: 1, Scale: 2, Operation: constant.CREDIT},
		},
	}

	shardOps := map[int][]*authorizerv1.BalanceOperation{
		0: {req.GetOperations()[0]},
		2: {req.GetOperations()[1]},
		4: {req.GetOperations()[2]},
	}

	resp, err := svc.authorizeCrossShard(ctx, req, shardOps)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, codes.DeadlineExceeded, status.Code(err))

	// Under parallel fan-out, low peer's onPrepare cancels the parent ctx
	// while participants run concurrently. The stub ignores ctx and returns
	// Authorized=true, so low is always aborted (it prepared). High may or
	// may not have started before cancellation propagated to the group ctx.
	// Safety invariant: no orphan prepared peers (abort ≤ 1 per peer).
	lowPeer.mu.Lock()
	lowAbortCalls := lowPeer.abortCalls
	lowPeer.mu.Unlock()
	require.Equal(t, 1, lowAbortCalls, "low peer must be aborted (it prepared before cancellation)")

	highPeer.mu.Lock()
	highAbortCalls := highPeer.abortCalls
	highPeer.mu.Unlock()
	require.LessOrEqual(t, highAbortCalls, 1, "high peer aborts at most once; 0 if ctx cancelled before its goroutine ran")
}

// TestAuthorizeCrossShardPreparesRemotePeersInShardOrder verifies that even
// though RPC dispatch is now parallel (prepare fan-out), the *result*
// ordering observed downstream (commit intent participants, abort iteration)
// remains deterministic by shard position. RPC dispatch order is not
// observable and must not be asserted.
func TestAuthorizeCrossShardPreparesRemotePeersInShardOrder(t *testing.T) {
	pub := &capturingPublisher{}

	peerA := &stubPeerClient{
		prepareResp: &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-a"},
	}
	peerB := &stubPeerClient{
		prepareResp: &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-b"},
	}

	svc := &authorizerService{
		pub:           pub,
		logger:        mustInitLogger(t),
		grpcAddr:      "authorizer-1:50051",
		peerAuthToken: "peer-secret",
		peers: []*peerClient{
			{addr: "authorizer-a:50051", clients: []authorizerv1.BalanceAuthorizerClient{peerA}, shardStart: 6, shardEnd: 7},
			{addr: "authorizer-b:50051", clients: []authorizerv1.BalanceAuthorizerClient{peerB}, shardStart: 4, shardEnd: 5},
		},
	}

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-cross-order",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@r1#default", AccountAlias: "@r1", BalanceKey: "default", Amount: 10, Scale: 2, Operation: constant.CREDIT},
			{OperationAlias: "1#@r2#default", AccountAlias: "@r2", BalanceKey: "default", Amount: 10, Scale: 2, Operation: constant.CREDIT},
		},
	}

	shardOps := map[int][]*authorizerv1.BalanceOperation{
		6: {req.GetOperations()[0]},
		4: {req.GetOperations()[1]},
	}

	resp, err := svc.authorizeCrossShard(context.Background(), req, shardOps)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.GetAuthorized())

	// The published commit intent's participant list must reflect shard-sorted
	// order (peer-b at shards 4-5 before peer-a at shards 6-7). RPC dispatch
	// parallelism does not affect this deterministic downstream ordering.
	pub.mu.Lock()
	require.NotEmpty(t, pub.messages, "at least one commit intent must be published")

	firstMsg := pub.messages[0]
	pub.mu.Unlock()

	var firstIntent commitIntent

	require.NoError(t, json.Unmarshal(firstMsg.Payload, &firstIntent))
	require.Len(t, firstIntent.Participants, 2)
	require.Equal(t, "ptx-b", firstIntent.Participants[0].PreparedTxID, "shard 4-5 peer-b must precede shard 6-7 peer-a")
	require.Equal(t, "ptx-a", firstIntent.Participants[1].PreparedTxID)
}

func TestAuthorizeCrossShardPrepareRejectionAbortsLocalPrepared(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b-local",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	peer := &stubPeerClient{
		prepareResp: &authorizerv1.PrepareAuthorizeResponse{
			Authorized:       false,
			RejectionCode:    engine.RejectionInsufficientFunds,
			RejectionMessage: "insufficient funds",
		},
	}

	svc := &authorizerService{
		engine:          eng,
		pub:             &capturingPublisher{},
		logger:          mustInitLogger(t),
		grpcAddr:        "authorizer-1:50051",
		ownedShardStart: 0,
		ownedShardEnd:   3,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			clients:    []authorizerv1.BalanceAuthorizerClient{peer},
			shardStart: 4,
			shardEnd:   7,
		}},
	}

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-cross-reject",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1#@remote#default", AccountAlias: "@remote", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.CREDIT},
		},
	}

	shardOps := map[int][]*authorizerv1.BalanceOperation{
		1: {req.GetOperations()[0]},
		5: {req.GetOperations()[1]},
	}

	resp, err := svc.authorizeCrossShard(context.Background(), req, shardOps)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.False(t, resp.GetAuthorized())
	require.Equal(t, engine.RejectionInsufficientFunds, resp.GetRejectionCode())

	localResp, localErr := eng.Authorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-after-reject",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	})
	require.NoError(t, localErr)
	require.True(t, localResp.GetAuthorized())
}

func TestIsLocalShardAndPeerForShardBoundaries(t *testing.T) {
	svc := &authorizerService{
		ownedShardStart: 2,
		ownedShardEnd:   4,
		peers: []*peerClient{
			{addr: "peer-a", shardStart: 0, shardEnd: 1},
			{addr: "peer-b", shardStart: 5, shardEnd: 7},
		},
	}

	require.False(t, svc.isLocalShard(1))
	require.True(t, svc.isLocalShard(2))
	require.True(t, svc.isLocalShard(4))
	require.False(t, svc.isLocalShard(5))

	require.Equal(t, "peer-a", svc.peerForShard(1).addr)
	require.Equal(t, "peer-b", svc.peerForShard(5).addr)
	require.Nil(t, svc.peerForShard(4))
}

func TestPeerClientPickClientRoundRobin(t *testing.T) {
	stub1 := &stubPeerClient{}
	stub2 := &stubPeerClient{}
	stub3 := &stubPeerClient{}

	peer := &peerClient{
		addr:    "test-peer:50051",
		clients: []authorizerv1.BalanceAuthorizerClient{stub1, stub2, stub3},
	}

	seen := make(map[authorizerv1.BalanceAuthorizerClient]int)

	for i := 0; i < 9; i++ {
		c := peer.pickClient()
		seen[c]++
	}

	require.Equal(t, 3, seen[stub1])
	require.Equal(t, 3, seen[stub2])
	require.Equal(t, 3, seen[stub3])
}

func TestAuthorizeCrossShardAsyncCommitIntentDebitsLocal(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b-local",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	pub := &capturingPublisher{}
	peer := &stubPeerClient{
		prepareResp: &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-peer-1"},
	}

	svc := &authorizerService{
		engine:            eng,
		pub:               pub,
		logger:            mustInitLogger(t),
		grpcAddr:          "authorizer-1:50051",
		ownedShardStart:   0,
		ownedShardEnd:     3,
		peerAuthToken:     "peer-secret",
		asyncCommitIntent: true,
		commitRPCDeadline: 10 * time.Second,
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			clients:    []authorizerv1.BalanceAuthorizerClient{peer},
			shardStart: 4,
			shardEnd:   7,
		}},
	}

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-async-commit",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1#@remote#default", AccountAlias: "@remote", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.CREDIT},
		},
	}

	shardOps := map[int][]*authorizerv1.BalanceOperation{
		1: {req.GetOperations()[0]},
		5: {req.GetOperations()[1]},
	}

	resp, err := svc.authorizeCrossShard(context.Background(), req, shardOps)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.GetAuthorized())

	// Verify balance was actually debited (local commit happened).
	balance, ok := eng.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(900), balance.Available)

	// Verify commit intent was published.
	require.NotEmpty(t, pub.messages, "expected at least one published commit intent")
}

func TestAuthorizeCrossShardAsyncCommitIntentPublishFailure(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b-local",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	// Publisher that always fails — simulates Redpanda being unreachable.
	pub := &capturingPublisher{err: errTestRedpandaDown}
	peer := &stubPeerClient{
		prepareResp: &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-peer-1"},
	}

	svc := &authorizerService{
		engine:            eng,
		pub:               pub,
		logger:            mustInitLogger(t),
		grpcAddr:          "authorizer-1:50051",
		ownedShardStart:   0,
		ownedShardEnd:     3,
		peerAuthToken:     "peer-secret",
		asyncCommitIntent: true,
		commitRPCDeadline: 5 * time.Second,
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			clients:    []authorizerv1.BalanceAuthorizerClient{peer},
			shardStart: 4,
			shardEnd:   7,
		}},
	}

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-async-publish-fail",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1#@remote#default", AccountAlias: "@remote", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.CREDIT},
		},
	}

	shardOps := map[int][]*authorizerv1.BalanceOperation{
		1: {req.GetOperations()[0]},
		5: {req.GetOperations()[1]},
	}

	resp, err := svc.authorizeCrossShard(context.Background(), req, shardOps)

	// The response must be a FailedPrecondition error (not Aborted), because
	// the local commit already happened — the transaction is partially
	// committed and the recovery mechanism needs to drive it to completion.
	// FailedPrecondition is deliberate (D4 scope item #6): it is mapped by
	// the transaction service to a non-retryable business error so naive
	// client retry does not double-spend.
	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Contains(t, err.Error(), "manual intervention required")

	// The local balance MUST have been debited. Even though the Redpanda
	// publish failed, the local commit happened first in the async path
	// (lines 388-431 of cross_shard.go). The balance lock was released and
	// the WAL entry was written.
	balance, ok := eng.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(900), balance.Available, "local balance should be debited even when publish fails")

	// Peer abort should NOT have been called — this is a partial-commit scenario,
	// not a clean abort. The peer was never committed (no publish gate passed),
	// but the local was committed, so we're in recovery territory.
	peer.mu.Lock()
	abortCalls := peer.abortCalls
	peer.mu.Unlock()
	require.Equal(t, 0, abortCalls, "peer abort should not be called after local commit succeeded")
}

// ─── B5: parallel prepare + parallel commit ──────────────────────────────.

// delayingPeerClient is a stubPeerClient specialisation that lets tests
// control per-call latency, observe in-flight RPC contexts, and verify that
// parallel dispatch actually happens.
type delayingPeerClient struct {
	stubPeerClient

	prepareDelay time.Duration
	commitDelay  time.Duration

	// prepareGate, if non-nil, blocks each PrepareAuthorize until closed or ctx
	// expires. Used by the first-rejection cancellation test to hold slow peers
	// so the fast rejecting peer can cancel them.
	prepareGate <-chan struct{}

	// prepareRejects, when true, returns an authorized=false response to
	// trigger short-circuit cancellation.
	prepareRejects bool

	// commitFailErr, when non-nil, is returned from CommitPrepared after the
	// commit delay elapses.
	commitFailErr error

	// preparedTxID overrides the default prepared_tx_id for this stub.
	preparedTxID string

	// Recorded contexts let tests assert cancellation propagated.
	ctxMu    sync.Mutex
	gotCtxes []context.Context
}

func (d *delayingPeerClient) PrepareAuthorize(ctx context.Context, _ *authorizerv1.AuthorizeRequest, _ ...grpc.CallOption) (*authorizerv1.PrepareAuthorizeResponse, error) {
	d.ctxMu.Lock()
	d.gotCtxes = append(d.gotCtxes, ctx)
	d.ctxMu.Unlock()

	if d.prepareGate != nil {
		select {
		case <-d.prepareGate:
		case <-ctx.Done():
			return nil, fmt.Errorf("delayingPeerClient prepare gate cancelled: %w", ctx.Err())
		}
	}

	if d.prepareDelay > 0 {
		select {
		case <-time.After(d.prepareDelay):
		case <-ctx.Done():
			return nil, fmt.Errorf("delayingPeerClient prepare delay cancelled: %w", ctx.Err())
		}
	}

	if d.prepareRejects {
		return &authorizerv1.PrepareAuthorizeResponse{
			Authorized:       false,
			RejectionCode:    "INSUFFICIENT_FUNDS",
			RejectionMessage: "test rejection",
		}, nil
	}

	id := d.preparedTxID
	if id == "" {
		id = "ptx-delaying"
	}

	return &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: id}, nil
}

func (d *delayingPeerClient) CommitPrepared(ctx context.Context, _ *authorizerv1.CommitPreparedRequest, _ ...grpc.CallOption) (*authorizerv1.CommitPreparedResponse, error) {
	if d.commitDelay > 0 {
		select {
		case <-time.After(d.commitDelay):
		case <-ctx.Done():
			return nil, fmt.Errorf("delayingPeerClient commit delay cancelled: %w", ctx.Err())
		}
	}

	if d.commitFailErr != nil {
		return nil, d.commitFailErr
	}

	return &authorizerv1.CommitPreparedResponse{Committed: true}, nil
}

// buildParallelPrepareHarness wires N delaying peers into an authorizerService
// and returns orderedPrepare closures whose RPC ctx is the one supplied by
// runPrepareSequence (the errgroup ctx). That plumbing is what makes this a
// valid test of parallel fan-out and cancellation propagation.
func buildParallelPrepareHarness(t *testing.T, peerClients []*delayingPeerClient) (*authorizerService, []orderedPrepare) {
	t.Helper()

	eng := engine.New(shard.NewRouter(64), wal.NewNoopWriter())

	t.Cleanup(func() { eng.Close() })

	peers := make([]*peerClient, 0, len(peerClients))

	prepares := make([]orderedPrepare, 0, len(peerClients))

	for i, pc := range peerClients {
		start := 8 + i*8
		end := start + 7

		peer := &peerClient{
			addr:       "authorizer-peer-" + pc.preparedTxID,
			clients:    []authorizerv1.BalanceAuthorizerClient{pc},
			shardStart: start,
			shardEnd:   end,
		}
		peers = append(peers, peer)

		currentPeer := peer
		currentPC := pc

		prepares = append(prepares, orderedPrepare{
			shardStart: peer.shardStart,
			shardEnd:   peer.shardEnd,
			peerAddr:   peer.addr,
			run: func(rpcCtx context.Context) prepareResult {
				resp, err := currentPC.PrepareAuthorize(rpcCtx, &authorizerv1.AuthorizeRequest{})
				r := prepareResult{peer: currentPeer, err: err}

				if resp != nil {
					r.txID = resp.GetPreparedTxId()
					r.resp = &authorizerv1.AuthorizeResponse{
						Authorized:       resp.GetAuthorized(),
						RejectionCode:    resp.GetRejectionCode(),
						RejectionMessage: resp.GetRejectionMessage(),
					}
				}

				return r
			},
		})
	}

	svc := &authorizerService{
		engine:          eng,
		logger:          mustInitLogger(t),
		grpcAddr:        "authorizer-coord:50051",
		ownedShardStart: 0,
		ownedShardEnd:   7,
		peerAuthToken:   "peer-secret",
		peers:           peers,
	}

	return svc, prepares
}

// TestRunPrepareSequence_Parallel_FanoutLatency verifies that total prepare
// latency is bounded by max(RTT), not sum(RTT). With 4 peers each sleeping
// 50ms, a sequential implementation would take ≥200ms; correct parallel
// fan-out must finish in well under 100ms (1.5x max RTT budget).
func TestRunPrepareSequence_Parallel_FanoutLatency(t *testing.T) {
	t.Parallel()

	const (
		peerCount = 4
		peerRTT   = 50 * time.Millisecond
		budget    = 100 * time.Millisecond
	)

	clients := make([]*delayingPeerClient, 0, peerCount)
	for i := 0; i < peerCount; i++ {
		clients = append(clients, &delayingPeerClient{
			preparedTxID: "ptx-" + string(rune('a'+i)),
			prepareDelay: peerRTT,
		})
	}

	svc, prepares := buildParallelPrepareHarness(t, clients)

	start := time.Now()

	results, err := svc.runPrepareSequence(context.Background(), &authorizerv1.AuthorizeRequest{TransactionId: "tx-fanout"}, prepares)

	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Len(t, results, peerCount)

	for _, r := range results {
		require.NoError(t, r.err)
		require.NotNil(t, r.resp)
		require.True(t, r.resp.GetAuthorized())
	}

	require.Lessf(t, elapsed, budget,
		"parallel prepare should finish in <%v (max RTT * 1.5 budget), got %v (sequential would be ≥%v)",
		budget, elapsed, peerRTT*peerCount)
}

// TestRunPrepareSequence_FirstRejectionCancelsOthers verifies that when one
// peer rejects, remaining in-flight peer RPCs observe ctx cancellation and
// return promptly rather than blocking on their artificial delay.
func TestRunPrepareSequence_FirstRejectionCancelsOthers(t *testing.T) {
	t.Parallel()

	// Slow peers are gated by a channel — they complete only if released
	// or cancelled. If cancellation works, they unblock via ctx.Done(); if
	// it doesn't, they each wait 2s and the test runs for >2s.
	slowGate := make(chan struct{}) // never closed

	rejecter := &delayingPeerClient{
		preparedTxID:   "ptx-reject",
		prepareRejects: true,
		// Small delay so the rejecter doesn't lose the race to goroutine
		// startup. Still orders of magnitude below the slow peers.
		prepareDelay: 10 * time.Millisecond,
	}

	slowA := &delayingPeerClient{
		preparedTxID: "ptx-slowA",
		prepareGate:  slowGate,
		prepareDelay: 2 * time.Second,
	}
	slowB := &delayingPeerClient{
		preparedTxID: "ptx-slowB",
		prepareGate:  slowGate,
		prepareDelay: 2 * time.Second,
	}

	svc, prepares := buildParallelPrepareHarness(t, []*delayingPeerClient{rejecter, slowA, slowB})

	// Parent ctx with a safety deadline above expected completion but well
	// below the slow-peer delay. If cancellation is broken, this deadline
	// fires and the test fails with a clear latency signal.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()

	results, err := svc.runPrepareSequence(ctx, &authorizerv1.AuthorizeRequest{TransactionId: "tx-reject"}, prepares)

	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotEmpty(t, results)

	// Must complete well under the slow-peer delay (2s) — meaning slow peer
	// ctx was cancelled, not allowed to run to completion.
	require.Less(t, elapsed, 400*time.Millisecond,
		"rejection should cancel in-flight slow peers; elapsed=%v", elapsed)

	// Slow peers that observed an RPC ctx must have seen it cancelled.
	for _, slow := range []*delayingPeerClient{slowA, slowB} {
		slow.ctxMu.Lock()
		ctxes := slow.gotCtxes
		slow.ctxMu.Unlock()

		for _, c := range ctxes {
			select {
			case <-c.Done():
				require.ErrorIs(t, c.Err(), context.Canceled,
					"slow peer ctx must be Canceled, got %v", c.Err())
			case <-time.After(100 * time.Millisecond):
				t.Fatalf("slow peer ctx was not cancelled after rejection")
			}
		}
	}
}

// TestCommitAllRemotePeers_ParallelContinueOnFailure verifies that commit
// fan-out (1) does not let a slow peer block a failing peer's error signal,
// (2) attempts every peer even on failure, and (3) returns the combined
// outcome (partial snapshots + failed=true).
func TestCommitAllRemotePeers_ParallelContinueOnFailure(t *testing.T) {
	t.Parallel()

	slowDelay := 200 * time.Millisecond

	slow := &delayingPeerClient{
		preparedTxID: "ptx-slow",
		commitDelay:  slowDelay,
	}
	failing := &delayingPeerClient{
		preparedTxID:  "ptx-fail",
		commitFailErr: errTestPeerUnavailable,
	}
	fast := &delayingPeerClient{
		preparedTxID: "ptx-fast",
	}

	eng := engine.New(shard.NewRouter(64), wal.NewNoopWriter())

	t.Cleanup(func() { eng.Close() })

	peerSlow := &peerClient{addr: "peer-slow", clients: []authorizerv1.BalanceAuthorizerClient{slow}, shardStart: 8, shardEnd: 15}
	peerFail := &peerClient{addr: "peer-fail", clients: []authorizerv1.BalanceAuthorizerClient{failing}, shardStart: 16, shardEnd: 23}
	peerFast := &peerClient{addr: "peer-fast", clients: []authorizerv1.BalanceAuthorizerClient{fast}, shardStart: 24, shardEnd: 31}

	svc := &authorizerService{
		engine:          eng,
		logger:          mustInitLogger(t),
		grpcAddr:        "authorizer-coord:50051",
		ownedShardStart: 0,
		ownedShardEnd:   7,
		peerAuthToken:   "peer-secret",
		peers:           []*peerClient{peerSlow, peerFail, peerFast},
	}

	results := []prepareResult{
		{txID: "ptx-slow", peer: peerSlow},
		{txID: "ptx-fail", peer: peerFail},
		{txID: "ptx-fast", peer: peerFast},
	}

	intent := &commitIntent{
		TransactionID: "tx-commit-parallel",
		Participants: []commitParticipant{
			{InstanceAddr: peerSlow.addr, PreparedTxID: "ptx-slow"},
			{InstanceAddr: peerFail.addr, PreparedTxID: "ptx-fail"},
			{InstanceAddr: peerFast.addr, PreparedTxID: "ptx-fast"},
		},
	}

	committedAny := false

	publishCount := 0
	publishCommittedStatus := func() { publishCount++ }

	start := time.Now()

	snapshots, anyFailed := svc.commitAllRemotePeers(
		context.Background(),
		results,
		"tx-commit-parallel",
		intent,
		&committedAny,
		publishCommittedStatus,
	)

	elapsed := time.Since(start)

	// (1) Failure propagates.
	require.True(t, anyFailed, "failing peer must propagate failed=true")

	// (2) Slow peer was awaited, not skipped. delayingPeerClient returns an
	// empty CommitPreparedResponse (no balances), so snapshots is expected
	// to be empty — we assert only on elapsed here.
	require.GreaterOrEqual(t, elapsed, slowDelay, "slow peer must have been awaited")

	require.Empty(t, snapshots, "stub peers return no balance snapshots; partial failure skips failing peer")

	// (3) Elapsed is bounded by slowest peer, not sum of peers.
	require.Less(t, elapsed, slowDelay+150*time.Millisecond,
		"commit fan-out must be bounded by slowest peer; elapsed=%v slowDelay=%v", elapsed, slowDelay)

	// (4) First-committed publish fired exactly once under concurrent completions.
	require.Equal(t, 1, publishCount, "publishCommittedStatus must fire once under parallel commits")
	require.True(t, committedAny, "committedAny must be set by guarded publish")

	// (5) Intent marks successful participants as committed, failing one as not.
	committedByID := map[string]bool{}

	for _, p := range intent.Participants {
		committedByID[p.PreparedTxID] = p.Committed
	}

	require.True(t, committedByID["ptx-slow"], "slow peer eventually committed")
	require.True(t, committedByID["ptx-fast"], "fast peer committed")
	require.False(t, committedByID["ptx-fail"], "failing peer must not be marked committed")
}

// TestSortPreparesDebitFirstAcrossShards covers D8 finding #2: within each
// participant's sub-request, DEBIT/ONHOLD must sort before CREDIT/RELEASE so
// the engine's funds-decrementing validation runs on the funds-decrementing
// legs first. The check is per-participant because each shard's engine
// validates its own ops independently.
func TestSortPreparesDebitFirstAcrossShards(t *testing.T) {
	t.Parallel()

	// Build three participants' worth of mixed operations. Within each, the
	// caller supplied CREDIT before DEBIT (or interleaved) — our sort must
	// bring all DEBIT/ONHOLD ops to the head while preserving the relative
	// order within each rank (stable sort).
	shardA := []*authorizerv1.BalanceOperation{
		{OperationAlias: "a-credit-1", AccountAlias: "@alice", Operation: constant.CREDIT},
		{OperationAlias: "a-debit-1", AccountAlias: "@alice", Operation: constant.DEBIT},
		{OperationAlias: "a-credit-2", AccountAlias: "@bob", Operation: constant.CREDIT},
		{OperationAlias: "a-onhold-1", AccountAlias: "@alice", Operation: constant.ONHOLD},
	}

	shardB := []*authorizerv1.BalanceOperation{
		{OperationAlias: "b-release-1", AccountAlias: "@carol", Operation: constant.RELEASE},
		{OperationAlias: "b-debit-1", AccountAlias: "@dan", Operation: constant.DEBIT},
	}

	shardC := []*authorizerv1.BalanceOperation{
		{OperationAlias: "c-credit-1", AccountAlias: "@eve", Operation: constant.CREDIT},
	}

	sortOperationsDebitFirst(shardA)
	sortOperationsDebitFirst(shardB)
	sortOperationsDebitFirst(shardC)

	// Shard A: DEBIT, ONHOLD first (rank 0), then CREDIT pair (rank 1).
	// Stable sort: DEBIT before ONHOLD (original relative order within rank 0
	// is a-debit-1 then a-onhold-1), and within rank 1 the two CREDIT ops
	// preserve caller order (a-credit-1 then a-credit-2).
	require.Equal(t, []string{"a-debit-1", "a-onhold-1", "a-credit-1", "a-credit-2"},
		opAliases(shardA),
		"shard A: DEBIT/ONHOLD must precede CREDIT; stable within rank",
	)

	// Shard B: DEBIT (rank 0) must come before RELEASE (rank 1).
	require.Equal(t, []string{"b-debit-1", "b-release-1"}, opAliases(shardB),
		"shard B: DEBIT must precede RELEASE",
	)

	// Shard C: single op is a no-op.
	require.Equal(t, []string{"c-credit-1"}, opAliases(shardC))
}

// TestSortPreparesDebitFirstCaseInsensitive exercises the mixed-case safeguard
// — callers are not contractually required to UPPER-CASE operations.
func TestSortPreparesDebitFirstCaseInsensitive(t *testing.T) {
	t.Parallel()

	ops := []*authorizerv1.BalanceOperation{
		{OperationAlias: "credit-lower", AccountAlias: "@a", Operation: "credit"},
		{OperationAlias: "debit-mixed", AccountAlias: "@b", Operation: "Debit"},
		{OperationAlias: "onhold-mixed", AccountAlias: "@c", Operation: "On_Hold"},
	}

	sortOperationsDebitFirst(ops)

	require.Equal(t, []string{"debit-mixed", "onhold-mixed", "credit-lower"},
		opAliases(ops),
		"lowercase/mixed-case ops must rank the same as UPPER-CASE",
	)
}

// TestSortPreparesDebitFirstUnknownOpsLast guards against a silent behaviour
// change if a new operation type is added without updating debitFirstOpRank.
// Unknown strings rank at 2 — they sort AFTER known ops, never before DEBIT.
func TestSortPreparesDebitFirstUnknownOpsLast(t *testing.T) {
	t.Parallel()

	ops := []*authorizerv1.BalanceOperation{
		{OperationAlias: "unknown-1", AccountAlias: "@a", Operation: "FROBNICATE"},
		{OperationAlias: "credit-1", AccountAlias: "@b", Operation: constant.CREDIT},
		{OperationAlias: "debit-1", AccountAlias: "@c", Operation: constant.DEBIT},
	}

	sortOperationsDebitFirst(ops)

	require.Equal(t, []string{"debit-1", "credit-1", "unknown-1"}, opAliases(ops),
		"unknown ops must sort last — DEBIT must never be displaced",
	)
}

func opAliases(ops []*authorizerv1.BalanceOperation) []string {
	out := make([]string, len(ops))
	for i, op := range ops {
		out[i] = op.GetOperationAlias()
	}

	return out
}

// TestAuthorizeCrossShard_SingleShardLocalBypassesIntent covers D8 finding #3:
// even though authorizeWithMetric already routes single-shard local ops to
// authorizeFastPath, authorizeCrossShard itself defends against a future
// internal caller (or test) that hands it a degenerate shardOps map. When
// len(shardOps) == 1 and that single shard is local, the full 2PC ceremony —
// and most importantly the Redpanda commit-intent publish — MUST be skipped.
func TestAuthorizeCrossShard_SingleShardLocalBypassesIntent(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b-local",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})
	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b-local-2",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@bob",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      0,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	pub := &capturingPublisher{}

	svc := &authorizerService{
		engine:          eng,
		pub:             pub,
		logger:          mustInitLogger(t),
		grpcAddr:        "authorizer-1:50051",
		ownedShardStart: 0,
		ownedShardEnd:   3,
		peerAuthToken:   "peer-secret",
		// Peers exist — we deliberately want authorizeCrossShard entered with a
		// single-shard-local map to exercise the defensive early-return.
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			clients:    []authorizerv1.BalanceAuthorizerClient{&stubPeerClient{}},
			shardStart: 4,
			shardEnd:   7,
		}},
	}

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-single-shard-local",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey, Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1#@bob#default", AccountAlias: "@bob", BalanceKey: constant.DefaultBalanceKey, Amount: 100, Scale: 2, Operation: constant.CREDIT},
		},
	}

	// Single-shard, local-owned (shard 2 is within [0..3]).
	shardOps := map[int][]*authorizerv1.BalanceOperation{
		2: req.GetOperations(),
	}

	resp, err := svc.authorizeCrossShard(context.Background(), req, shardOps)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.GetAuthorized(), "single-shard-local path must commit via fast path")

	// Critical invariant: NO commit intent publish, because the 2PC ceremony
	// was skipped entirely. A committed-intent message would prove we ran the
	// cross-shard flow and published a durable marker — which is exactly what
	// the fast path is meant to avoid.
	require.Empty(t, pub.messages,
		"single-shard-local bypass must not publish any commit intent (found %d messages)",
		len(pub.messages),
	)

	// Balance actually moved — confirms we didn't just return a rejection.
	alice, ok := eng.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(900), alice.Available)
}

// TestAuthorizeCrossShard_SingleShardRemoteStillUses2PC is the negative case:
// if the one shard in the map is NOT local, we must not short-circuit. The
// coordinator still has to prepare/commit on the owning peer.
func TestAuthorizeCrossShard_SingleShardRemoteStillUses2PC(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	pub := &capturingPublisher{}
	peer := &stubPeerClient{
		prepareResp: &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-peer-only"},
		commitResp:  &authorizerv1.CommitPreparedResponse{Committed: true},
	}

	svc := &authorizerService{
		engine:          eng,
		pub:             pub,
		logger:          mustInitLogger(t),
		grpcAddr:        "authorizer-1:50051",
		ownedShardStart: 0,
		ownedShardEnd:   3,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			clients:    []authorizerv1.BalanceAuthorizerClient{peer},
			shardStart: 4,
			shardEnd:   7,
		}},
	}

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-single-shard-remote",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@remote#default", AccountAlias: "@remote", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.CREDIT},
		},
	}

	shardOps := map[int][]*authorizerv1.BalanceOperation{
		6: req.GetOperations(), // shard 6 is remote (peer range 4..7)
	}

	resp, err := svc.authorizeCrossShard(context.Background(), req, shardOps)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.GetAuthorized())

	// Remote shard → full 2PC → commit intent publishes must have fired.
	require.NotEmpty(t, pub.messages,
		"single-shard-remote path must still publish commit intents",
	)
}

// TestLocalCommit_PublishesCommittedStatusAtomically covers D8 finding #5: the
// COMMITTED commit-intent MUST be published synchronously as part of
// runLocalCommitPhase, not after it returns. This is the atomicity invariant
// that ensures a coordinator crash between local commit success and COMMITTED
// publish cannot leave recovery with only a PREPARED intent. The test
// simulates "crash between phases" by asserting the COMMITTED message exists
// in the publisher BEFORE the remote-peer commit fans out.
func TestLocalCommit_PublishesCommittedStatusAtomically(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b-local",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	pub := &capturingPublisher{}

	// Peer that blocks on commit — simulates a coordinator "crash" between
	// local commit success and remote commit. Before the peer's commit
	// returns, the COMMITTED intent MUST already exist in the publisher so
	// recovery can distinguish drive-to-completion from abort.
	peerCommitGate := make(chan struct{})
	peer := &blockingCommitStubPeerClient{
		prepareResp: &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-peer-slow"},
		commitResp:  &authorizerv1.CommitPreparedResponse{Committed: true},
		commitGate:  peerCommitGate,
	}

	svc := &authorizerService{
		engine:          eng,
		pub:             pub,
		logger:          mustInitLogger(t),
		grpcAddr:        "authorizer-1:50051",
		ownedShardStart: 0,
		ownedShardEnd:   3,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			clients:    []authorizerv1.BalanceAuthorizerClient{peer},
			shardStart: 4,
			shardEnd:   7,
		}},
	}

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-atomic-committed",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey, Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1#@remote#default", AccountAlias: "@remote", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.CREDIT},
		},
	}

	shardOps := map[int][]*authorizerv1.BalanceOperation{
		1: {req.GetOperations()[0]}, // local
		5: {req.GetOperations()[1]}, // remote, commit blocked on gate
	}

	done := make(chan struct{})

	go func() {
		_, _ = svc.authorizeCrossShard(context.Background(), req, shardOps)

		close(done)
	}()

	// Wait until PREPARED and COMMITTED are both present before releasing the
	// peer commit. If the COMMITTED publish were "best-effort after local
	// commit returns", we'd see only PREPARED here — which is the exact
	// crash-window the fix closes.
	require.Eventually(t, func() bool {
		pub.mu.Lock()
		defer pub.mu.Unlock()

		hasCommitted := false

		for _, msg := range pub.messages {
			var intent commitIntent
			if err := json.Unmarshal(msg.Payload, &intent); err != nil {
				continue
			}

			if intent.Status == commitIntentStatusCommitted {
				hasCommitted = true
			}
		}

		return hasCommitted
	}, 2*time.Second, 10*time.Millisecond,
		"COMMITTED intent must be published before remote commit completes",
	)

	// Release the peer and let the full flow finish.
	close(peerCommitGate)
	<-done

	// Final invariant: PREPARED, COMMITTED, and COMPLETED are all present.
	statuses := make(map[string]int)

	for _, msg := range pub.messages {
		var intent commitIntent
		require.NoError(t, json.Unmarshal(msg.Payload, &intent))

		statuses[intent.Status]++
	}

	require.GreaterOrEqual(t, statuses[commitIntentStatusPrepared], 1)
	require.GreaterOrEqual(t, statuses[commitIntentStatusCommitted], 1)
	require.GreaterOrEqual(t, statuses[commitIntentStatusCompleted], 1)
}

// blockingCommitStubPeerClient is a stubPeerClient variant whose CommitPrepared
// blocks on a caller-provided channel. Used to assert ordering invariants
// between local commit → COMMITTED publish → remote commit.
type blockingCommitStubPeerClient struct {
	stubPeerClient

	prepareResp *authorizerv1.PrepareAuthorizeResponse
	commitResp  *authorizerv1.CommitPreparedResponse
	commitGate  chan struct{}
}

func (b *blockingCommitStubPeerClient) PrepareAuthorize(_ context.Context, _ *authorizerv1.AuthorizeRequest, _ ...grpc.CallOption) (*authorizerv1.PrepareAuthorizeResponse, error) {
	if b.prepareResp != nil {
		return b.prepareResp, nil
	}

	return &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-blocking"}, nil
}

func (b *blockingCommitStubPeerClient) CommitPrepared(ctx context.Context, _ *authorizerv1.CommitPreparedRequest, _ ...grpc.CallOption) (*authorizerv1.CommitPreparedResponse, error) {
	select {
	case <-b.commitGate:
	case <-ctx.Done():
		return nil, fmt.Errorf("blocking commit stub: %w", ctx.Err())
	}

	if b.commitResp != nil {
		return b.commitResp, nil
	}

	return &authorizerv1.CommitPreparedResponse{Committed: true}, nil
}
