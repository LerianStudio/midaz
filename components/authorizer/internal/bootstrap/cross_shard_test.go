// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
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

func peerAuthIncomingContext(token, method string, req proto.Message) context.Context {
	ctx, err := withPeerAuth(context.Background(), token, method, req)
	if err != nil {
		panic(err)
	}
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
		engine:        eng,
		peerAuthToken: "peer-secret",
	}

	prepareReq := &authorizerv1.AuthorizeRequest{}
	ctx := peerAuthIncomingContext("peer-secret", peerRPCMethodPrepareAuthorize, prepareReq)

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
		engine:        eng,
		peerAuthToken: "peer-secret",
	}

	commitReq := &authorizerv1.CommitPreparedRequest{}
	authCommitCtx := peerAuthIncomingContext("peer-secret", peerRPCMethodCommitPrepared, commitReq)

	_, err := svc.CommitPrepared(authCommitCtx, commitReq)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	abortReq := &authorizerv1.AbortPreparedRequest{}
	authAbortCtx := peerAuthIncomingContext("peer-secret", peerRPCMethodAbortPrepared, abortReq)
	_, err = svc.AbortPrepared(authAbortCtx, abortReq)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
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
		commitErr:   errors.New("peer unavailable"),
	}

	svc := &authorizerService{
		engine:          eng,
		pub:             pub,
		logger:          libZap.InitializeLogger(),
		grpcAddr:        "authorizer-1:50051",
		ownedShardStart: 0,
		ownedShardEnd:   3,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			client:     peer,
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
	require.Contains(t, err.Error(), "cross-shard commit incomplete")

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

func TestAuthorizeCrossShardTreatsPeerCommitNotFoundAsIdempotentSuccess(t *testing.T) {
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
		logger:          libZap.InitializeLogger(),
		grpcAddr:        "authorizer-1:50051",
		ownedShardStart: 0,
		ownedShardEnd:   3,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			client:     peer,
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

	resp, err := svc.authorizeCrossShard(context.Background(), req, shardOps)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.GetAuthorized())

	statuses := make([]string, 0, len(pub.messages))
	for _, msg := range pub.messages {
		var intent commitIntent
		require.NoError(t, json.Unmarshal(msg.Payload, &intent))
		statuses = append(statuses, intent.Status)
	}

	require.Contains(t, statuses, commitIntentStatusPrepared)
	require.Contains(t, statuses, commitIntentStatusCommitted)
	require.Contains(t, statuses, commitIntentStatusCompleted)
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
		pub:             &capturingPublisher{err: errors.New("publish failed")},
		logger:          libZap.InitializeLogger(),
		grpcAddr:        "authorizer-1:50051",
		ownedShardStart: 0,
		ownedShardEnd:   3,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			client:     peer,
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
		logger:          libZap.InitializeLogger(),
		grpcAddr:        "authorizer-mid:50051",
		ownedShardStart: 2,
		ownedShardEnd:   5,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{
			{addr: "authorizer-low:50051", client: lowPeer, shardStart: 0, shardEnd: 1},
			{addr: "authorizer-high:50051", client: highPeer, shardStart: 6, shardEnd: 7},
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

	highPeer.mu.Lock()
	highAbortCalls := highPeer.abortCalls
	highPeer.mu.Unlock()
	require.Equal(t, 0, highAbortCalls)

	lowPeer.mu.Lock()
	lowAbortCalls := lowPeer.abortCalls
	lowPeer.mu.Unlock()
	require.Equal(t, 1, lowAbortCalls)
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
		logger:          libZap.InitializeLogger(),
		grpcAddr:        "authorizer-mid:50051",
		ownedShardStart: 4,
		ownedShardEnd:   7,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{
			{addr: "authorizer-low:50051", client: lowPeer, shardStart: 0, shardEnd: 1},
			{addr: "authorizer-high:50051", client: highPeer, shardStart: 2, shardEnd: 3},
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

	lowPeer.mu.Lock()
	lowAbortCalls := lowPeer.abortCalls
	lowPeer.mu.Unlock()
	require.Equal(t, 1, lowAbortCalls)

	highPeer.mu.Lock()
	highAbortCalls := highPeer.abortCalls
	highPeer.mu.Unlock()
	require.Equal(t, 0, highAbortCalls)
}

func TestAuthorizeCrossShardPreparesRemotePeersInShardOrder(t *testing.T) {
	pub := &capturingPublisher{}
	prepareOrder := make([]string, 0, 2)

	peerA := &stubPeerClient{
		prepareResp: &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-a"},
		onPrepare: func() {
			prepareOrder = append(prepareOrder, "peer-a")
		},
	}

	peerB := &stubPeerClient{
		prepareResp: &authorizerv1.PrepareAuthorizeResponse{Authorized: true, PreparedTxId: "ptx-b"},
		onPrepare: func() {
			prepareOrder = append(prepareOrder, "peer-b")
		},
	}

	svc := &authorizerService{
		pub:           pub,
		logger:        libZap.InitializeLogger(),
		grpcAddr:      "authorizer-1:50051",
		peerAuthToken: "peer-secret",
		peers: []*peerClient{
			{addr: "authorizer-a:50051", client: peerA, shardStart: 6, shardEnd: 7},
			{addr: "authorizer-b:50051", client: peerB, shardStart: 4, shardEnd: 5},
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
	require.Equal(t, []string{"peer-b", "peer-a"}, prepareOrder)
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
		logger:          libZap.InitializeLogger(),
		grpcAddr:        "authorizer-1:50051",
		ownedShardStart: 0,
		ownedShardEnd:   3,
		peerAuthToken:   "peer-secret",
		peers: []*peerClient{{
			addr:       "authorizer-2:50051",
			client:     peer,
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
