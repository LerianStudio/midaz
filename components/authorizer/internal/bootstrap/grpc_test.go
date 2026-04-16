// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	mgrpc "github.com/LerianStudio/midaz/v3/pkg/mgrpc"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

func mustInitLogger(t *testing.T) libLog.Logger {
	t.Helper()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	return logger
}

type failingCommitWALWriter struct{}

func (f failingCommitWALWriter) Append(_ wal.Entry) error {
	return context.DeadlineExceeded
}

func (f failingCommitWALWriter) Close() error {
	return nil
}

func TestCommitPreparedErrorMapping(t *testing.T) {
	t.Run("returns not found for missing prepared tx", func(t *testing.T) {
		eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
		defer eng.Close()

		svc := &authorizerService{engine: eng, logger: mustInitLogger(t), peerAuthToken: "peer-secret", peerNonceStore: newPeerNonceStore(defaultPeerAuthMaxSkew, 100000)}
		commitReq := &authorizerv1.CommitPreparedRequest{PreparedTxId: "missing"}
		authCtx := peerAuthIncomingContext(t, "peer-secret", peerRPCMethodCommitPrepared, commitReq)

		_, err := svc.CommitPrepared(authCtx, commitReq)
		require.Error(t, err)
		require.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("returns internal for commit execution failures", func(t *testing.T) {
		eng := engine.New(shard.NewRouter(8), failingCommitWALWriter{})
		defer eng.Close()

		eng.UpsertBalances([]*engine.Balance{{
			ID:             "b1",
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

		svc := &authorizerService{engine: eng, logger: mustInitLogger(t), peerAuthToken: "peer-secret", peerNonceStore: newPeerNonceStore(defaultPeerAuthMaxSkew, 100000)}
		prepareReq := &authorizerv1.AuthorizeRequest{
			TransactionId:     "tx-1",
			OrganizationId:    "org",
			LedgerId:          "ledger",
			Pending:           false,
			TransactionStatus: constant.CREATED,
			Operations: []*authorizerv1.BalanceOperation{
				{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			},
		}
		authPrepareCtx := peerAuthIncomingContext(t, "peer-secret", peerRPCMethodPrepareAuthorize, prepareReq)

		prepareResp, err := svc.PrepareAuthorize(authPrepareCtx, prepareReq)
		require.NoError(t, err)
		require.NotEmpty(t, prepareResp.GetPreparedTxId())

		commitReq := &authorizerv1.CommitPreparedRequest{PreparedTxId: prepareResp.GetPreparedTxId()}
		authCommitCtx := peerAuthIncomingContext(t, "peer-secret", peerRPCMethodCommitPrepared, commitReq)
		_, err = svc.CommitPrepared(authCommitCtx, commitReq)
		require.Error(t, err)
		require.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("is idempotent for successful replay", func(t *testing.T) {
		eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
		defer eng.Close()

		eng.UpsertBalances([]*engine.Balance{{
			ID:             "b1",
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

		svc := &authorizerService{engine: eng, logger: mustInitLogger(t), peerAuthToken: "peer-secret", peerNonceStore: newPeerNonceStore(defaultPeerAuthMaxSkew, 100000)}
		prepareReq := &authorizerv1.AuthorizeRequest{
			TransactionId:     "tx-idempotent",
			OrganizationId:    "org",
			LedgerId:          "ledger",
			Pending:           false,
			TransactionStatus: constant.CREATED,
			Operations: []*authorizerv1.BalanceOperation{
				{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			},
		}

		authPrepareCtx := peerAuthIncomingContext(t, "peer-secret", peerRPCMethodPrepareAuthorize, prepareReq)
		prepareResp, err := svc.PrepareAuthorize(authPrepareCtx, prepareReq)
		require.NoError(t, err)

		commitReq := &authorizerv1.CommitPreparedRequest{PreparedTxId: prepareResp.GetPreparedTxId()}
		authCommitCtx := peerAuthIncomingContext(t, "peer-secret", peerRPCMethodCommitPrepared, commitReq)
		firstCommit, err := svc.CommitPrepared(authCommitCtx, commitReq)
		require.NoError(t, err)
		require.True(t, firstCommit.GetCommitted())

		authCommitReplayCtx := peerAuthIncomingContext(t, "peer-secret", peerRPCMethodCommitPrepared, commitReq)
		secondCommit, err := svc.CommitPrepared(authCommitReplayCtx, commitReq)
		require.NoError(t, err)
		require.True(t, secondCommit.GetCommitted())
		require.Equal(t, firstCommit.GetBalances(), secondCommit.GetBalances())
	})
}

func TestPrepareAuthorizeReturnsResourceExhaustedWhenLimitReached(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	sem := make(chan struct{}, 1)
	sem <- struct{}{}

	svc := &authorizerService{engine: eng, logger: mustInitLogger(t), peerAuthToken: "peer-secret", peerNonceStore: newPeerNonceStore(defaultPeerAuthMaxSkew, 100000), peerPrepareSem: sem, peerPrepareBoundedWait: 0}
	prepareReq := &authorizerv1.AuthorizeRequest{}
	authCtx := peerAuthIncomingContext(t, "peer-secret", peerRPCMethodPrepareAuthorize, prepareReq)

	_, err := svc.PrepareAuthorize(authCtx, prepareReq)
	require.Error(t, err)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
}

func TestPrepareAuthorizeBoundedWaitRespectsContextCancellation(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	sem := make(chan struct{}, 1)
	sem <- struct{}{}

	svc := &authorizerService{
		engine:                 eng,
		logger:                 mustInitLogger(t),
		peerAuthToken:          "peer-secret",
		peerNonceStore:         newPeerNonceStore(defaultPeerAuthMaxSkew, 100000),
		peerPrepareSem:         sem,
		peerPrepareBoundedWait: 5 * time.Second,
	}
	prepareReq := &authorizerv1.AuthorizeRequest{}
	authCtx := peerAuthIncomingContext(t, "peer-secret", peerRPCMethodPrepareAuthorize, prepareReq)

	// Cancel the context immediately — bounded wait should exit.
	cancelCtx, cancel := context.WithCancel(authCtx)
	cancel()

	// Reattach incoming metadata to cancelled context.
	md, _ := metadata.FromIncomingContext(authCtx)
	cancelCtx = metadata.NewIncomingContext(cancelCtx, md)

	_, err := svc.PrepareAuthorize(cancelCtx, prepareReq)
	require.Error(t, err)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
}

func TestInternalRPCsRequirePeerAuthWhenTokenConfigured(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	svc := &authorizerService{engine: eng, logger: mustInitLogger(t), peerAuthToken: "peer-secret", peerNonceStore: newPeerNonceStore(defaultPeerAuthMaxSkew, 100000)}

	_, err := svc.GetBalance(context.Background(), &authorizerv1.GetBalanceRequest{OrganizationId: "org", LedgerId: "ledger", AccountAlias: "@alice", BalanceKey: "default"})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	authCtx := peerAuthIncomingContext(t, "peer-secret", peerRPCMethodGetBalance, &authorizerv1.GetBalanceRequest{OrganizationId: "org", LedgerId: "ledger", AccountAlias: "@alice", BalanceKey: "default"})
	_, err = svc.GetBalance(authCtx, &authorizerv1.GetBalanceRequest{OrganizationId: "org", LedgerId: "ledger", AccountAlias: "@alice", BalanceKey: "default"})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))

	_, err = svc.PublishBalanceOperations(context.Background(), &authorizerv1.PublishBalanceOperationsRequest{Topic: "t", PartitionKey: "p", Payload: []byte("x")})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	_, err = svc.LoadBalances(context.Background(), &authorizerv1.LoadBalancesRequest{OrganizationId: "org", LedgerId: "ledger", ShardIds: []int32{0}})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestValidateShardCoverage(t *testing.T) {
	require.NoError(t, validateShardCoverage(8, 0, 3, []peerShardRange{{start: 4, end: 7}}))
	require.Error(t, validateShardCoverage(8, 0, 2, []peerShardRange{{start: 4, end: 7}}))
	require.Error(t, validateShardCoverage(8, 0, 3, []peerShardRange{{start: 3, end: 7}}))
}

func TestParsePeerShardRange(t *testing.T) {
	t.Run("valid range", func(t *testing.T) {
		rng, err := parsePeerShardRange(" 2-5 ", 8)
		require.NoError(t, err)
		require.Equal(t, peerShardRange{start: 2, end: 5}, rng)
	})

	t.Run("invalid format", func(t *testing.T) {
		_, err := parsePeerShardRange("2", 8)
		require.Error(t, err)
		require.Contains(t, err.Error(), "expected start-end")
	})

	t.Run("invalid bounds start greater than end", func(t *testing.T) {
		_, err := parsePeerShardRange("6-2", 8)
		require.Error(t, err)
		require.Contains(t, err.Error(), "start=6 end=2")
	})

	t.Run("invalid bounds exceeds shard count", func(t *testing.T) {
		_, err := parsePeerShardRange("0-8", 8)
		require.Error(t, err)
		require.Contains(t, err.Error(), "out of bounds")
	})
}

func TestValidateShardCoverageRejectsDuplicateAndEmptyCoverage(t *testing.T) {
	err := validateShardCoverage(8, 0, 3, []peerShardRange{{start: 2, end: 5}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "assigned to multiple owners")

	err = validateShardCoverage(8, 0, 1, []peerShardRange{{start: 2, end: 5}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "has no owner")
}

func TestAuthorizePeerRPCValidation(t *testing.T) {
	svc := &authorizerService{
		peerAuthToken:   "Str0ngPeerTokenValue!2026",
		peerAuthMaxSkew: defaultPeerAuthMaxSkew,
		peerNonceStore:  newPeerNonceStore(defaultPeerAuthMaxSkew, 100000),
	}

	req := &authorizerv1.CommitPreparedRequest{PreparedTxId: "ptx-1"}

	t.Run("missing headers", func(t *testing.T) {
		err := svc.authorizePeerRPC(context.Background(), peerRPCMethodCommitPrepared, req)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("malformed timestamp", func(t *testing.T) {
		ctx := tamperedPeerAuthContext(t, svc.peerAuthToken, peerRPCMethodCommitPrepared, req, func(md metadata.MD) {
			md.Set(mgrpc.PeerAuthTimestampHeader, "not-a-number")
		})

		err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("expired timestamp", func(t *testing.T) {
		ctx := tamperedPeerAuthContext(t, svc.peerAuthToken, peerRPCMethodCommitPrepared, req, func(md metadata.MD) {
			md.Set(mgrpc.PeerAuthTimestampHeader, "1")
		})

		err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("future timestamp", func(t *testing.T) {
		future := time.Now().Add(defaultPeerAuthMaxSkew + time.Second).Unix()
		ctx := tamperedPeerAuthContext(t, svc.peerAuthToken, peerRPCMethodCommitPrepared, req, func(md metadata.MD) {
			md.Set(mgrpc.PeerAuthTimestampHeader, strconv.FormatInt(future, 10))
		})

		err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("bad signature", func(t *testing.T) {
		ctx := tamperedPeerAuthContext(t, svc.peerAuthToken, peerRPCMethodCommitPrepared, req, func(md metadata.MD) {
			md.Set(mgrpc.PeerAuthSignatureHeader, "bad-signature")
		})

		err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("method mismatch", func(t *testing.T) {
		ctx := tamperedPeerAuthContext(t, svc.peerAuthToken, peerRPCMethodCommitPrepared, req, func(md metadata.MD) {
			md.Set(mgrpc.PeerAuthMethodHeader, peerRPCMethodAbortPrepared)
		})

		err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("replay nonce", func(t *testing.T) {
		ctx := tamperedPeerAuthContext(t, svc.peerAuthToken, peerRPCMethodCommitPrepared, req, nil)

		err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.NoError(t, err)

		err = svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.Contains(t, status.Convert(err).Message(), "replay", "error message should mention nonce replay")
	})
}

func tamperedPeerAuthContext(t *testing.T, token, method string, req proto.Message, tamper func(metadata.MD)) context.Context { //nolint:unparam // method is always CommitPrepared in current tests but kept for flexibility
	t.Helper()

	ctx, err := withPeerAuth(context.Background(), token, method, req)
	require.NoError(t, err)

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)

	copied := md.Copy()
	if tamper != nil {
		tamper(copied)
	}

	return metadata.NewIncomingContext(context.Background(), copied)
}

func TestPeerNonceStoreHighVolumeConcurrent(t *testing.T) {
	t.Parallel()

	store := newPeerNonceStore(30*time.Second, 200000)
	now := time.Now()

	const goroutines = 10000

	nonces := make([]string, goroutines)
	for i := range nonces {
		nonces[i] = "nonce-" + strconv.Itoa(i)
	}

	results := make([]bool, goroutines)
	done := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			results[idx] = store.MarkIfNew(nonces[idx], now)

			done <- struct{}{}
		}(i)
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}

	for i, ok := range results {
		require.True(t, ok, "nonce %d should have been accepted on first insert", i)
	}

	// Replay every nonce — all must be rejected.
	for i, nonce := range nonces {
		require.False(t, store.MarkIfNew(nonce, now), "nonce %d should be rejected on replay", i)
	}
}

func TestTokenRotationAcceptsOldSecret(t *testing.T) {
	svc := &authorizerService{
		peerAuthToken:     "new-secret",
		peerAuthTokenPrev: "old-secret",
		peerAuthMaxSkew:   defaultPeerAuthMaxSkew,
		peerNonceStore:    newPeerNonceStore(defaultPeerAuthMaxSkew, 100000),
	}

	req := &authorizerv1.CommitPreparedRequest{PreparedTxId: "ptx-rotate"}

	t.Run("previous token is accepted", func(t *testing.T) {
		ctx := tamperedPeerAuthContext(t, "old-secret", peerRPCMethodCommitPrepared, req, nil)
		err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.NoError(t, err, "request signed with previous token should be accepted")
	})

	t.Run("completely unknown token is rejected", func(t *testing.T) {
		ctx := tamperedPeerAuthContext(t, "unknown-secret", peerRPCMethodCommitPrepared, req, nil)
		err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})
}

func TestBodyHashTamperingReturnsPermissionDenied(t *testing.T) {
	svc := &authorizerService{
		peerAuthToken:   "peer-secret",
		peerAuthMaxSkew: defaultPeerAuthMaxSkew,
		peerNonceStore:  newPeerNonceStore(defaultPeerAuthMaxSkew, 100000),
	}

	req := &authorizerv1.CommitPreparedRequest{PreparedTxId: "ptx-tamper"}

	ctx := tamperedPeerAuthContext(t, svc.peerAuthToken, peerRPCMethodCommitPrepared, req, func(md metadata.MD) {
		md.Set(mgrpc.PeerAuthBodyHashHeader, "tampered-hash")
	})

	err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestAuthorizeInternalRPCFailedPreconditionWhenTokenEmptyButPeersExist(t *testing.T) {
	// B4: an empty peer-auth token now yields Unauthenticated regardless of
	// peer count. Previously returned FailedPrecondition only when peers > 0
	// and silently allowed the request when peers == 0 — single-instance
	// authorizers accepted LoadBalances/GetBalance/PublishBalanceOperations
	// from any caller that could reach :50051.
	svc := &authorizerService{
		peerAuthToken: "",
		peers: []*peerClient{
			{addr: "authorizer-2:50051"},
		},
	}

	err := svc.authorizeInternalRPC(context.Background(), peerRPCMethodGetBalance, &authorizerv1.GetBalanceRequest{})
	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestPeerNonceStoreCapacityExhaustion(t *testing.T) {
	t.Parallel()

	const maxSize = 10

	store := newPeerNonceStore(1*time.Minute, maxSize)

	now := time.Now()

	// Fill the store to capacity.
	for i := 0; i < maxSize; i++ {
		ok := store.MarkIfNew("nonce-"+strconv.Itoa(i), now)
		require.True(t, ok, "nonce %d should be accepted within capacity", i)
	}

	// The next nonce should be rejected because the window has not elapsed yet,
	// so rotation cannot happen, and capacity is exhausted.
	ok := store.MarkIfNew("nonce-overflow", now)
	require.False(t, ok, "nonce should be rejected when capacity is exhausted within the time window")
}

func TestAuthorizePeerRPC_AcceptsPreviousToken(t *testing.T) {
	svc := &authorizerService{
		peerAuthToken:     "new-secret",
		peerAuthTokenPrev: "old-secret",
		peerAuthMaxSkew:   defaultPeerAuthMaxSkew,
		peerNonceStore:    newPeerNonceStore(defaultPeerAuthMaxSkew, 100000),
	}

	req := &authorizerv1.CommitPreparedRequest{PreparedTxId: "ptx-rotate-h11"}

	t.Run("request signed with old-secret is accepted", func(t *testing.T) {
		ctx := peerAuthIncomingContext(t, "old-secret", peerRPCMethodCommitPrepared, req)
		err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.NoError(t, err, "request signed with previous token (old-secret) should be accepted")
	})

	t.Run("request signed with unknown-secret is rejected", func(t *testing.T) {
		ctx := peerAuthIncomingContext(t, "unknown-secret", peerRPCMethodCommitPrepared, req)
		err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})
}

func TestAuthorizePeerRPC_RejectsBodyHashTampering(t *testing.T) {
	svc := &authorizerService{
		peerAuthToken:   "peer-secret",
		peerAuthMaxSkew: defaultPeerAuthMaxSkew,
		peerNonceStore:  newPeerNonceStore(defaultPeerAuthMaxSkew, 100000),
	}

	req := &authorizerv1.CommitPreparedRequest{PreparedTxId: "ptx-tamper-h12"}

	ctx := tamperedPeerAuthContext(t, svc.peerAuthToken, peerRPCMethodCommitPrepared, req, func(md metadata.MD) {
		md.Set(mgrpc.PeerAuthBodyHashHeader, "tampered-hash")
	})

	err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestAuthorizeInternalRPC_FailsPreconditionWhenPeersWithoutToken(t *testing.T) {
	// B4: empty peer-auth token is Unauthenticated regardless of peer count.
	// Kept as a second (slightly different fixture) regression guard against
	// the pre-fix escape-hatch returning.
	svc := &authorizerService{
		peerAuthToken: "",
		peers: []*peerClient{
			{addr: "peer:50051"},
		},
	}

	err := svc.authorizeInternalRPC(context.Background(), peerRPCMethodGetBalance, &authorizerv1.GetBalanceRequest{})
	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestPeerNonceStoreReacceptsAfterWindowElapses(t *testing.T) {
	t.Parallel()

	const window = 100 * time.Millisecond
	// Use a generous maxSize so capacity never blocks inserts during the test.
	const maxSize = 1000

	const batchSize = 10

	store := newPeerNonceStore(window, maxSize)
	baseTime := time.Now()

	// Insert a small batch of initial nonces.
	initialNonces := make([]string, batchSize)
	for i := range initialNonces {
		initialNonces[i] = "old-" + strconv.Itoa(i)
		require.True(t, store.MarkIfNew(initialNonces[i], baseTime))
	}

	// Verify they are detected as duplicates right now.
	for _, nonce := range initialNonces {
		require.False(t, store.MarkIfNew(nonce, baseTime), "should reject duplicate nonce %q", nonce)
	}

	// Advance time past the window and insert enough nonces to trigger a rotation.
	// Rotation requires len(current) >= maxSize/2 (500). We insert 500 nonces.
	// After rotation: previous = {initial + phase1}, current = empty.
	afterWindow := baseTime.Add(window + time.Millisecond)
	for i := 0; i < maxSize/2; i++ {
		require.True(t, store.MarkIfNew("phase1-"+strconv.Itoa(i), afterWindow))
	}

	// Initial nonces are still in previous — they should still be rejected.
	for _, nonce := range initialNonces {
		require.False(t, store.MarkIfNew(nonce, afterWindow), "should still reject nonce %q in previous map", nonce)
	}

	// Advance time past the window again and trigger a second rotation.
	// After rotation: previous = phase2 nonces, current = empty.
	// The initial nonces (and phase1) have been fully evicted from both maps.
	afterSecondWindow := afterWindow.Add(window + time.Millisecond)
	for i := 0; i < maxSize/2; i++ {
		require.True(t, store.MarkIfNew("phase2-"+strconv.Itoa(i), afterSecondWindow))
	}

	// Old nonces should now be purged from both maps and re-acceptable.
	afterThirdWindow := afterSecondWindow.Add(time.Millisecond)
	for _, nonce := range initialNonces {
		require.True(t, store.MarkIfNew(nonce, afterThirdWindow), "old nonce %q should be re-accepted after full eviction", nonce)
	}
}

// TestAuthorizeInternalRPC_RejectsUnauthenticatedRequest_ZeroPeers exercises
// the B4 fix: a single-instance authorizer (peers == 0) with a configured
// peer-auth token must reject every internal RPC that lacks valid HMAC
// headers. Prior to the fix, the service short-circuited to `return nil`
// when peers == 0, meaning anyone reachable on :50051 could invoke
// LoadBalances/GetBalance/PublishBalanceOperations/Authorize without creds.
func TestAuthorizeInternalRPC_RejectsUnauthenticatedRequest_ZeroPeers(t *testing.T) {
	svc := &authorizerService{
		peerAuthToken:  "peer-secret",
		peers:          nil, // single-instance topology
		peerNonceStore: newPeerNonceStore(defaultPeerAuthMaxSkew, 100000),
	}

	// No metadata, no peer-auth headers — must be rejected.
	err := svc.authorizeInternalRPC(context.Background(), peerRPCMethodGetBalance, &authorizerv1.GetBalanceRequest{})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

// TestAuthorizeInternalRPC_AcceptsValidPeerToken_ZeroPeers is the positive
// complement to the rejection test: the same zero-peer bootstrap accepts
// requests that present valid peer-auth HMAC credentials. Proves the B4
// fix does not break the legitimate caller path (transaction service).
func TestAuthorizeInternalRPC_AcceptsValidPeerToken_ZeroPeers(t *testing.T) {
	svc := &authorizerService{
		peerAuthToken:   "peer-secret",
		peerAuthMaxSkew: defaultPeerAuthMaxSkew,
		peers:           nil,
		peerNonceStore:  newPeerNonceStore(defaultPeerAuthMaxSkew, 100000),
	}

	req := &authorizerv1.GetBalanceRequest{OrganizationId: "org", LedgerId: "ledger", AccountAlias: "@alice", BalanceKey: "default"}
	ctx := peerAuthIncomingContext(t, "peer-secret", peerRPCMethodGetBalance, req)

	err := svc.authorizeInternalRPC(ctx, peerRPCMethodGetBalance, req)
	require.NoError(t, err, "zero-peer authorizer must accept a properly-signed internal RPC")
}

// TestUnauthorizedRPCCounterIncrements proves the normalizer keeps metric
// cardinality bounded: the known reason labels pass through unchanged, the
// known method labels pass through unchanged, and unknown values collapse
// to "other" / "unknown" so a probing attacker cannot blow up the counter.
func TestUnauthorizedRPCCounterIncrements(t *testing.T) {
	t.Run("known reasons preserved", func(t *testing.T) {
		for _, reason := range []string{
			"missing_token", "missing_headers", "bad_timestamp", "timestamp_skew",
			"wrong_algo", "body_mismatch", "invalid_hmac", "nonce_replay",
			"nonce_internal", "hash_internal",
		} {
			require.Equal(t, reason, normalizeUnauthorizedReason(reason))
		}
	})

	t.Run("unknown reason collapses to other", func(t *testing.T) {
		require.Equal(t, labelOther, normalizeUnauthorizedReason("not-a-reason"))
	})

	t.Run("empty reason maps to unknown", func(t *testing.T) {
		require.Equal(t, labelUnknown, normalizeUnauthorizedReason(""))
	})

	t.Run("known methods preserved", func(t *testing.T) {
		for _, method := range []string{
			peerRPCMethodAuthorize,
			peerRPCMethodAuthorizeStream,
			peerRPCMethodPrepareAuthorize,
			peerRPCMethodCommitPrepared,
			peerRPCMethodAbortPrepared,
			peerRPCMethodLoadBalances,
			peerRPCMethodGetBalance,
			peerRPCMethodPublishBalanceOp,
		} {
			require.Equal(t, method, normalizeUnauthorizedMethod(method))
		}
	})

	t.Run("unknown method collapses to other", func(t *testing.T) {
		require.Equal(t, labelOther, normalizeUnauthorizedMethod("/attacker/probe"))
	})

	t.Run("empty method maps to unknown", func(t *testing.T) {
		require.Equal(t, labelUnknown, normalizeUnauthorizedMethod(""))
	})

	t.Run("recordUnauthorizedRPC is safe with nil metrics", func(t *testing.T) {
		svc := &authorizerService{peerAuthToken: "peer-secret", logger: mustInitLogger(t)}
		// Must not panic when metrics are nil (common in unit tests).
		svc.recordUnauthorizedRPC(context.Background(), peerRPCMethodLoadBalances, unauthorizedReasonInvalidHMAC)
	})
}

// TestAuthorize_RequiresPeerAuthHeaders proves the fast-path Authorize RPC is
// now protected — pre-B4 it had no peer-auth gate at all (Authorize went
// straight to engine.Authorize, relying on network isolation).
func TestAuthorize_RequiresPeerAuthHeaders(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	svc := &authorizerService{
		engine:          eng,
		logger:          mustInitLogger(t),
		peerAuthToken:   "peer-secret",
		peerAuthMaxSkew: defaultPeerAuthMaxSkew,
		peerNonceStore:  newPeerNonceStore(defaultPeerAuthMaxSkew, 100000),
	}

	// No peer-auth headers.
	_, err := svc.Authorize(context.Background(), &authorizerv1.AuthorizeRequest{
		OrganizationId: "org", LedgerId: "ledger",
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}
