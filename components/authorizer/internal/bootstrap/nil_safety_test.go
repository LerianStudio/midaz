// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// nilCommitRespPeerClient returns (nil, nil) from CommitPrepared — a
// pathological but legal response that previously would have been appended
// to allSnapshots via commitResp.GetBalances(), which is itself nil-safe on
// the receiver but still routes through the guard we added.
type nilCommitRespPeerClient struct{ stubPeerClient }

func (n *nilCommitRespPeerClient) CommitPrepared(_ context.Context, _ *authorizerv1.CommitPreparedRequest, _ ...grpc.CallOption) (*authorizerv1.CommitPreparedResponse, error) {
	return nil, nil
}

// TestCommitAllRemotePeers_NilResponseDoesntPanic exercises the guard added
// to commitRemotePeerGuarded: when a peer returns (nil, nil) the function
// must return an empty snapshot slice and not mark the commit as failed,
// rather than deref-panic on commitResp.GetBalances().
func TestCommitAllRemotePeers_NilResponseDoesntPanic(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	peerStub := &nilCommitRespPeerClient{}

	svc := &authorizerService{
		engine:        eng,
		logger:        mustInitLogger(t),
		peerAuthToken: "peer-secret",
	}

	peer := &peerClient{
		addr:    "authorizer-2:50051",
		clients: []authorizerv1.BalanceAuthorizerClient{peerStub},
	}

	// Minimal prepareResult list: one remote participant with a prepared tx
	// ID so the peer commit branch runs.
	results := []prepareResult{
		{txID: "ptx-remote", isLocal: false, peer: peer},
	}

	intent := &commitIntent{
		TransactionID: "tx-nil-commit",
		Participants:  []commitParticipant{{PreparedTxID: "ptx-remote", IsLocal: false}},
	}

	committedAny := false
	publishCalls := 0
	publishStatus := func() { publishCalls++ }

	require.NotPanics(t, func() {
		snapshots, failed := svc.commitAllRemotePeers(
			context.Background(),
			results,
			"tx-nil-commit",
			intent,
			&committedAny,
			publishStatus,
		)

		require.Empty(t, snapshots,
			"nil commitResp must yield zero snapshots (not a panic, not garbage)")
		require.False(t, failed,
			"a successful RPC with empty response must not be treated as a failure")
	})
}

// errTestPeerCommitFailed drives handleRemotePeerCommitError through its
// log-and-exit path without tripping the codes.NotFound branch.
var errTestPeerCommitFailed = errors.New("peer commit failed")

// TestHandleRemotePeerCommitError_NilPeerGuarded proves the mirrored guard:
// commit error-handling now tolerates a prepareResult whose peer is nil
// (as the abort path already did). Prior behavior was a nil deref inside
// s.logger.Errorf(..., r.peer.addr, ...).
func TestHandleRemotePeerCommitError_NilPeerGuarded(t *testing.T) {
	svc := &authorizerService{logger: mustInitLogger(t)}

	intent := &commitIntent{
		TransactionID: "tx-nil-peer",
		Participants:  []commitParticipant{{PreparedTxID: "ptx-x", IsLocal: false}},
	}

	// peer intentionally nil — triggers the guard we added.
	r := &prepareResult{txID: "ptx-x", peer: nil}

	require.NotPanics(t, func() {
		svc.handleRemotePeerCommitError(
			context.Background(),
			errTestPeerCommitFailed,
			r,
			"tx-nil-peer",
			intent,
		)
	})
}

// TestHandleRemotePeerCommitError_NilPeerGuardedNotFound covers the second
// branch (codes.NotFound), ensuring both log sites use the nil-safe peerAddr
// derived by the guard. Without a publisher configured, publishCommitIntent
// returns errCommitIntentPublisherNotConfigured; that path is fine — we
// only care that the function completes without panic.
func TestHandleRemotePeerCommitError_NilPeerGuardedNotFound(t *testing.T) {
	svc := &authorizerService{logger: mustInitLogger(t)}

	intent := &commitIntent{
		TransactionID: "tx-nil-peer-notfound",
		Participants:  []commitParticipant{{PreparedTxID: "ptx-nf", IsLocal: false}},
	}

	r := &prepareResult{txID: "ptx-nf", peer: nil}

	require.NotPanics(t, func() {
		svc.handleRemotePeerCommitError(
			context.Background(),
			status.Error(codes.NotFound, "prepared_tx not found"),
			r,
			"tx-nil-peer-notfound",
			intent,
		)
	})
}

// TestReconcile_NilServicePubIsGuarded pins the mirrored guard added at
// reconcile entry. A reconciler whose service has a nil publisher (common
// in early boot or a test setup that constructs the reconciler before the
// publisher is wired) must short-circuit cleanly, matching the seedCompletedSet
// guard.
func TestReconcile_NilServicePubIsGuarded(t *testing.T) {
	// service.pub == nil
	svc := &authorizerService{
		logger: mustInitLogger(t),
	}

	rec := &walReconciler{
		service: svc,
		logger:  mustInitLogger(t),
	}

	require.NotPanics(t, func() {
		rec.reconcile(context.Background())
	})
}

// TestReconcile_NilReceiverIsGuarded documents that the top-level receiver
// guard is also intact. A zero-value walReconciler must not panic.
func TestReconcile_NilReceiverIsGuarded(t *testing.T) {
	var rec *walReconciler

	require.NotPanics(t, func() {
		rec.reconcile(context.Background())
	})
}
