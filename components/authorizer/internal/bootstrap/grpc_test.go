// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"strconv"
	"testing"
	"time"

	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

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

		svc := &authorizerService{engine: eng, logger: libZap.InitializeLogger(), peerAuthToken: "peer-secret"}
		commitReq := &authorizerv1.CommitPreparedRequest{PreparedTxId: "missing"}
		authCtx := peerAuthIncomingContext("peer-secret", peerRPCMethodCommitPrepared, commitReq)

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

		svc := &authorizerService{engine: eng, logger: libZap.InitializeLogger(), peerAuthToken: "peer-secret"}
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
		authPrepareCtx := peerAuthIncomingContext("peer-secret", peerRPCMethodPrepareAuthorize, prepareReq)

		prepareResp, err := svc.PrepareAuthorize(authPrepareCtx, prepareReq)
		require.NoError(t, err)
		require.NotEmpty(t, prepareResp.GetPreparedTxId())

		commitReq := &authorizerv1.CommitPreparedRequest{PreparedTxId: prepareResp.GetPreparedTxId()}
		authCommitCtx := peerAuthIncomingContext("peer-secret", peerRPCMethodCommitPrepared, commitReq)
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

		svc := &authorizerService{engine: eng, logger: libZap.InitializeLogger(), peerAuthToken: "peer-secret", peerNonceStore: newPeerNonceStore(defaultPeerAuthMaxSkew, 100000)}
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

		authPrepareCtx := peerAuthIncomingContext("peer-secret", peerRPCMethodPrepareAuthorize, prepareReq)
		prepareResp, err := svc.PrepareAuthorize(authPrepareCtx, prepareReq)
		require.NoError(t, err)

		commitReq := &authorizerv1.CommitPreparedRequest{PreparedTxId: prepareResp.GetPreparedTxId()}
		authCommitCtx := peerAuthIncomingContext("peer-secret", peerRPCMethodCommitPrepared, commitReq)
		firstCommit, err := svc.CommitPrepared(authCommitCtx, commitReq)
		require.NoError(t, err)
		require.True(t, firstCommit.GetCommitted())

		authCommitReplayCtx := peerAuthIncomingContext("peer-secret", peerRPCMethodCommitPrepared, commitReq)
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

	svc := &authorizerService{engine: eng, logger: libZap.InitializeLogger(), peerAuthToken: "peer-secret", peerPrepareSem: sem}
	prepareReq := &authorizerv1.AuthorizeRequest{}
	authCtx := peerAuthIncomingContext("peer-secret", peerRPCMethodPrepareAuthorize, prepareReq)

	_, err := svc.PrepareAuthorize(authCtx, prepareReq)
	require.Error(t, err)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
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

	t.Run("invalid bounds", func(t *testing.T) {
		_, err := parsePeerShardRange("6-2", 8)
		require.Error(t, err)
		require.Contains(t, err.Error(), "out of bounds")

		_, err = parsePeerShardRange("0-8", 8)
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
			md.Set(peerAuthTimestampHeader, "not-a-number")
		})

		err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("expired timestamp", func(t *testing.T) {
		ctx := tamperedPeerAuthContext(t, svc.peerAuthToken, peerRPCMethodCommitPrepared, req, func(md metadata.MD) {
			md.Set(peerAuthTimestampHeader, "1")
		})

		err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("future timestamp", func(t *testing.T) {
		future := time.Now().Add(defaultPeerAuthMaxSkew + time.Second).Unix()
		ctx := tamperedPeerAuthContext(t, svc.peerAuthToken, peerRPCMethodCommitPrepared, req, func(md metadata.MD) {
			md.Set(peerAuthTimestampHeader, strconv.FormatInt(future, 10))
		})

		err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("bad signature", func(t *testing.T) {
		ctx := tamperedPeerAuthContext(t, svc.peerAuthToken, peerRPCMethodCommitPrepared, req, func(md metadata.MD) {
			md.Set(peerAuthSignatureHeader, "bad-signature")
		})

		err := svc.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("method mismatch", func(t *testing.T) {
		ctx := tamperedPeerAuthContext(t, svc.peerAuthToken, peerRPCMethodCommitPrepared, req, func(md metadata.MD) {
			md.Set(peerAuthMethodHeader, peerRPCMethodAbortPrepared)
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
	})
}

func tamperedPeerAuthContext(t *testing.T, token, method string, req proto.Message, tamper func(metadata.MD)) context.Context {
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
