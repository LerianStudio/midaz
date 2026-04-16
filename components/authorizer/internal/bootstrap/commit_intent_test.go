// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc"

	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/publisher"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// Test sentinel errors for stub methods that should not be called.
var (
	errTestAuthorizeNotExpected                 = errors.New("Authorize not expected in this test")
	errTestAuthorizeStreamNotExpected           = errors.New("AuthorizeStream not expected in this test")
	errTestLoadBalancesNotExpected              = errors.New("LoadBalances not expected in this test")
	errTestGetBalanceNotExpected                = errors.New("GetBalance not expected in this test")
	errTestPublishBalanceOperationsNotExpect    = errors.New("PublishBalanceOperations not expected in this test")
	errTestPrepareAuthorizeNotExpected          = errors.New("PrepareAuthorize not expected in this test")
	errTestAbortPreparedNotExpected             = errors.New("AbortPrepared not expected in this test")
	errTestResolveManualInterventionNotExpected = errors.New("ResolveManualIntervention not expected in this test")
	errTestBoom                                 = errors.New("boom")
)

// capturingPublisher records all published messages for test assertions.
type capturingPublisher struct {
	mu       sync.Mutex
	messages []publisher.Message
	err      error // if set, Publish returns this error
}

func (p *capturingPublisher) Publish(_ context.Context, msg publisher.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.err != nil {
		return p.err
	}

	p.messages = append(p.messages, msg)

	return nil
}

func (p *capturingPublisher) Close() error { return nil }

// trackingStubPeerClient records each CommitPrepared invocation's PreparedTxId
// so tests can assert exactly which participants were targeted during recovery.
// Implements authorizerv1.BalanceAuthorizerClient.
type trackingStubPeerClient struct {
	commitResp *authorizerv1.CommitPreparedResponse
	commitErr  error

	mu    sync.Mutex
	calls []string // PreparedTxId values from each CommitPrepared call
}

func (t *trackingStubPeerClient) Authorize(_ context.Context, _ *authorizerv1.AuthorizeRequest, _ ...grpc.CallOption) (*authorizerv1.AuthorizeResponse, error) {
	return nil, errTestAuthorizeNotExpected
}

func (t *trackingStubPeerClient) AuthorizeStream(_ context.Context, _ ...grpc.CallOption) (grpc.BidiStreamingClient[authorizerv1.AuthorizeRequest, authorizerv1.AuthorizeResponse], error) {
	return nil, errTestAuthorizeStreamNotExpected
}

func (t *trackingStubPeerClient) LoadBalances(_ context.Context, _ *authorizerv1.LoadBalancesRequest, _ ...grpc.CallOption) (*authorizerv1.LoadBalancesResponse, error) {
	return nil, errTestLoadBalancesNotExpected
}

func (t *trackingStubPeerClient) GetBalance(_ context.Context, _ *authorizerv1.GetBalanceRequest, _ ...grpc.CallOption) (*authorizerv1.GetBalanceResponse, error) {
	return nil, errTestGetBalanceNotExpected
}

func (t *trackingStubPeerClient) PublishBalanceOperations(_ context.Context, _ *authorizerv1.PublishBalanceOperationsRequest, _ ...grpc.CallOption) (*authorizerv1.PublishBalanceOperationsResponse, error) {
	return nil, errTestPublishBalanceOperationsNotExpect
}

func (t *trackingStubPeerClient) PrepareAuthorize(_ context.Context, _ *authorizerv1.AuthorizeRequest, _ ...grpc.CallOption) (*authorizerv1.PrepareAuthorizeResponse, error) {
	return nil, errTestPrepareAuthorizeNotExpected
}

func (t *trackingStubPeerClient) CommitPrepared(_ context.Context, req *authorizerv1.CommitPreparedRequest, _ ...grpc.CallOption) (*authorizerv1.CommitPreparedResponse, error) {
	t.mu.Lock()
	t.calls = append(t.calls, req.GetPreparedTxId())
	t.mu.Unlock()

	if t.commitErr != nil {
		return nil, t.commitErr
	}

	if t.commitResp != nil {
		return t.commitResp, nil
	}

	return &authorizerv1.CommitPreparedResponse{Committed: true}, nil
}

func (t *trackingStubPeerClient) AbortPrepared(_ context.Context, _ *authorizerv1.AbortPreparedRequest, _ ...grpc.CallOption) (*authorizerv1.AbortPreparedResponse, error) {
	return nil, errTestAbortPreparedNotExpected
}

// ResolveManualIntervention satisfies the BalanceAuthorizerClient interface so
// trackingStubPeerClient can be used in peer lists. Peers never invoke the
// admin RPC on each other, so a reject-by-default is safe.
func (t *trackingStubPeerClient) ResolveManualIntervention(_ context.Context, _ *authorizerv1.ResolveManualInterventionRequest, _ ...grpc.CallOption) (*authorizerv1.ResolveManualInterventionResponse, error) {
	return nil, errTestResolveManualInterventionNotExpected
}

func (t *trackingStubPeerClient) commitPreparedCalls() []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := make([]string, len(t.calls))
	copy(result, t.calls)

	return result
}

func TestBuildParticipants(t *testing.T) {
	tests := []struct {
		name      string
		results   []prepareResult
		localAddr string
		want      []commitParticipant
	}{
		{
			name:      "empty results",
			results:   nil,
			localAddr: ":50051",
			want:      []commitParticipant{},
		},
		{
			name: "skips results with no txID",
			results: []prepareResult{
				{txID: "", isLocal: true},
				{txID: "", peer: &peerClient{addr: "peer:50051"}},
			},
			localAddr: ":50051",
			want:      []commitParticipant{},
		},
		{
			name: "local only",
			results: []prepareResult{
				{txID: "ptx-local-1", isLocal: true},
			},
			localAddr: ":50051",
			want: []commitParticipant{
				{InstanceAddr: ":50051", PreparedTxID: "ptx-local-1", IsLocal: true},
			},
		},
		{
			name: "remote only",
			results: []prepareResult{
				{txID: "ptx-remote-1", peer: &peerClient{addr: "authorizer-2:50051"}},
			},
			localAddr: ":50051",
			want: []commitParticipant{
				{InstanceAddr: "authorizer-2:50051", PreparedTxID: "ptx-remote-1"},
			},
		},
		{
			name: "local and remote mixed",
			results: []prepareResult{
				{txID: "ptx-local-1", isLocal: true},
				{txID: "ptx-remote-1", peer: &peerClient{addr: "authorizer-2:50051"}},
				{txID: "", isLocal: false}, // skipped
				{txID: "ptx-remote-2", peer: &peerClient{addr: "authorizer-3:50051"}},
			},
			localAddr: "authorizer-1:50051",
			want: []commitParticipant{
				{InstanceAddr: "authorizer-1:50051", PreparedTxID: "ptx-local-1", IsLocal: true},
				{InstanceAddr: "authorizer-2:50051", PreparedTxID: "ptx-remote-1"},
				{InstanceAddr: "authorizer-3:50051", PreparedTxID: "ptx-remote-2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildParticipants(tt.results, tt.localAddr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPublishCommitIntent_NilPublisher(t *testing.T) {
	svc := &authorizerService{pub: nil}

	err := svc.publishCommitIntent(context.Background(), &commitIntent{
		TransactionID: "tx-1",
		Status:        "PREPARED",
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "publisher is not configured")
}

func TestPublishCommitIntent_Success(t *testing.T) {
	pub := &capturingPublisher{}
	svc := &authorizerService{pub: pub}

	intent := &commitIntent{
		TransactionID:  "tx-abc-123",
		OrganizationID: "org-1",
		LedgerID:       "ledger-1",
		Status:         "PREPARED",
		Participants: []commitParticipant{
			{InstanceAddr: ":50051", PreparedTxID: "ptx-1", IsLocal: true},
			{InstanceAddr: "peer:50051", PreparedTxID: "ptx-2"},
		},
		CreatedAt: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	err := svc.publishCommitIntent(context.Background(), intent)
	require.NoError(t, err)
	require.Len(t, pub.messages, 1)

	msg := pub.messages[0]
	assert.Equal(t, crossShardCommitTopic, msg.Topic)
	assert.Equal(t, "tx-abc-123", msg.PartitionKey)
	assert.Equal(t, "application/json", msg.ContentType)

	// Verify payload deserializes correctly.
	var decoded commitIntent

	err = json.Unmarshal(msg.Payload, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "tx-abc-123", decoded.TransactionID)
	assert.Equal(t, "org-1", decoded.OrganizationID)
	assert.Equal(t, "ledger-1", decoded.LedgerID)
	assert.Equal(t, "PREPARED", decoded.Status)
	assert.Len(t, decoded.Participants, 2)
	assert.True(t, decoded.Participants[0].IsLocal)
	assert.Equal(t, "ptx-1", decoded.Participants[0].PreparedTxID)
	assert.False(t, decoded.Participants[1].IsLocal)
	assert.Equal(t, "ptx-2", decoded.Participants[1].PreparedTxID)
}

func TestPublishCommitIntent_PublishError(t *testing.T) {
	pub := &capturingPublisher{err: errTestBoom}
	svc := &authorizerService{pub: pub}

	err := svc.publishCommitIntent(context.Background(), &commitIntent{
		TransactionID: "tx-fail",
		Status:        "PREPARED",
		Participants:  []commitParticipant{},
		CreatedAt:     time.Now(),
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errTestBoom)
}

func TestCommitIntentJSON_RoundTrip(t *testing.T) {
	original := commitIntent{
		TransactionID:  "tx-round-trip",
		OrganizationID: "org-42",
		LedgerID:       "ledger-7",
		Status:         "COMPLETED",
		Participants: []commitParticipant{
			{InstanceAddr: "auth-1:50051", PreparedTxID: "ptx-a", Committed: true, IsLocal: true},
			{InstanceAddr: "auth-2:50051", PreparedTxID: "ptx-b", Committed: true},
		},
		CreatedAt: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded commitIntent

	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.TransactionID, decoded.TransactionID)
	assert.Equal(t, original.OrganizationID, decoded.OrganizationID)
	assert.Equal(t, original.LedgerID, decoded.LedgerID)
	assert.Equal(t, original.Status, decoded.Status)
	assert.Equal(t, original.CreatedAt, decoded.CreatedAt)
	require.Len(t, decoded.Participants, 2)
	assert.Equal(t, original.Participants[0], decoded.Participants[0])
	assert.Equal(t, original.Participants[1], decoded.Participants[1])
}

func TestCrossShardCommitTopic_IsCorrect(t *testing.T) {
	assert.Equal(t, "authorizer.cross-shard.commits", crossShardCommitTopic)
}

func TestMarkParticipantCommitted(t *testing.T) {
	intent := &commitIntent{
		Participants: []commitParticipant{
			{PreparedTxID: "ptx-1", Committed: false},
			{PreparedTxID: "ptx-2", Committed: false},
		},
	}

	require.True(t, markParticipantCommitted(intent, "ptx-2"))
	require.False(t, markParticipantCommitted(intent, "ptx-missing"))
	require.False(t, markParticipantCommitted(nil, "ptx-1"))
	require.True(t, intent.Participants[1].Committed)
}

func TestProcessRecordSkipsUnauthenticatedPayload(t *testing.T) {
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	runner := &commitIntentRecoveryRunner{
		service: &authorizerService{
			peerAuthToken: "Str0ngPeerTokenValue!2026",
			logger:        logger,
		},
		logger: logger,
	}

	payload, err := json.Marshal(commitIntent{TransactionID: "tx-unauthenticated"})
	require.NoError(t, err)

	record := &kgo.Record{Value: payload}
	recovered, processErr := runner.processRecord(context.Background(), record)
	require.NoError(t, processErr)
	require.False(t, recovered, "unauthenticated payload should not count as recovered")
}

func TestProcessRecordAcceptsAuthenticatedPayload(t *testing.T) {
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	runner := &commitIntentRecoveryRunner{
		service: &authorizerService{
			peerAuthToken: "Str0ngPeerTokenValue!2026",
			logger:        logger,
		},
		logger: logger,
	}

	payload, err := json.Marshal(commitIntent{TransactionID: "tx-authenticated"})
	require.NoError(t, err)

	mac := hmac.New(sha256.New, []byte("Str0ngPeerTokenValue!2026"))
	_, _ = mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))

	record := &kgo.Record{
		Value: payload,
		Headers: []kgo.RecordHeader{{
			Key:   commitIntentAuthHeader,
			Value: []byte(sig),
		}},
	}

	recovered, processErr := runner.processRecord(context.Background(), record)
	require.NoError(t, processErr)
	// Empty intent with no participants -- no actual recovery to do.
	require.False(t, recovered)
}

func TestRecoverCommitIntentLocalParticipant(t *testing.T) {
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

	ptx, _, err := eng.PrepareAuthorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-recovery-local",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, ptx)

	pub := &capturingPublisher{}
	svc := &authorizerService{
		engine:       eng,
		pub:          pub,
		instanceAddr: "authorizer-1:50051",
	}

	intent := &commitIntent{
		TransactionID: "tx-recovery-local",
		Status:        commitIntentStatusPrepared,
		Participants: []commitParticipant{
			{InstanceAddr: "authorizer-1:50051", PreparedTxID: ptx.ID, IsLocal: true},
		},
	}

	recovered, recoverErr := svc.recoverCommitIntent(context.Background(), intent)
	require.NoError(t, recoverErr)
	require.True(t, recovered, "local participant should count as recovered")
	require.True(t, intent.Participants[0].Committed)
	require.Equal(t, commitIntentStatusCompleted, intent.Status)
	// Drive-to-completion publishes twice: COMPLETED intent to commits topic
	// + cache-invalidation event to the invalidation topic (see
	// TestRecovery_InvalidatesTransactionCacheOnDriveToCompletion for the
	// topic/payload contract).
	require.Len(t, pub.messages, 2)
}

func TestRecoverCommitIntentRemoteParticipant(t *testing.T) {
	pub := &capturingPublisher{}
	peer := &stubPeerClient{commitResp: &authorizerv1.CommitPreparedResponse{Committed: true}}

	logger, logErr := libZap.InitializeLoggerWithError()
	require.NoError(t, logErr)

	svc := &authorizerService{
		pub:           pub,
		logger:        logger,
		peerAuthToken: "peer-secret",
		peers: []*peerClient{{
			addr:    "authorizer-2:50051",
			clients: []authorizerv1.BalanceAuthorizerClient{peer},
		}},
	}

	intent := &commitIntent{
		TransactionID: "tx-recovery-remote",
		Status:        commitIntentStatusPrepared,
		Participants: []commitParticipant{
			{InstanceAddr: "authorizer-2:50051", PreparedTxID: "ptx-remote"},
		},
	}

	recovered, recoverErr := svc.recoverCommitIntent(context.Background(), intent)
	require.NoError(t, recoverErr)
	require.True(t, recovered, "remote participant should count as recovered")
	require.True(t, intent.Participants[0].Committed)
	require.Equal(t, commitIntentStatusCompleted, intent.Status)
	// Drive-to-completion publishes COMPLETED intent + cache-invalidation.
	require.Len(t, pub.messages, 2)
}

func TestRecoverCommitIntentMissingPeerReturnsError(t *testing.T) {
	svc := &authorizerService{pub: &capturingPublisher{}}

	intent := &commitIntent{
		TransactionID: "tx-missing-peer",
		Status:        commitIntentStatusPrepared,
		Participants: []commitParticipant{
			{InstanceAddr: "missing-peer:50051", PreparedTxID: "ptx-remote"},
		},
	}

	_, err := svc.recoverCommitIntent(context.Background(), intent)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not configured")
}

func TestIsExpectedPollError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "deadline exceeded", err: context.DeadlineExceeded, want: true},
		{name: "context canceled", err: context.Canceled, want: true},
		{name: "wrapped deadline exceeded", err: fmt.Errorf("wrapped: %w", context.DeadlineExceeded), want: true},
		{name: "other error", err: errTestBoom, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isExpectedPollError(tt.err))
		})
	}
}

func TestRecoverCommitIntentCommitsParticipantOwnedByAddressEvenIfNotLocalFlag(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b-owned-by-address",
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

	ptx, _, err := eng.PrepareAuthorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-owned-by-address",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, ptx)

	pub := &capturingPublisher{}
	svc := &authorizerService{
		engine:       eng,
		pub:          pub,
		instanceAddr: "authorizer-2:50051",
	}

	intent := &commitIntent{
		TransactionID: "tx-owned-by-address",
		Status:        commitIntentStatusPrepared,
		Participants: []commitParticipant{
			{InstanceAddr: "authorizer-2:50051", PreparedTxID: ptx.ID, IsLocal: false},
		},
	}

	recovered, recoverErr := svc.recoverCommitIntent(context.Background(), intent)
	require.NoError(t, recoverErr)
	require.True(t, recovered, "owned-by-address participant should count as recovered")
	require.True(t, intent.Participants[0].Committed)
	require.Equal(t, commitIntentStatusCompleted, intent.Status)
	// Drive-to-completion publishes COMPLETED intent + cache-invalidation.
	require.Len(t, pub.messages, 2)
}

func TestRecoverCommitIntentSkippedParticipantReturnsNotRecovered(t *testing.T) {
	// Simulates the orphaned-intent scenario: an intent where the only
	// uncommitted participant is local-marked but belongs to a different
	// instance. The recovery should return recovered=false so the backoff
	// logic can detect consecutive no-op cycles.
	pub := &capturingPublisher{}

	logger, logErr := libZap.InitializeLoggerWithError()
	require.NoError(t, logErr)

	svc := &authorizerService{
		pub:          pub,
		logger:       logger,
		instanceAddr: "authorizer-1:50051",
	}

	intent := &commitIntent{
		TransactionID: "tx-orphaned",
		Status:        commitIntentStatusPrepared,
		Participants: []commitParticipant{
			{
				InstanceAddr: "authorizer-2:50051",
				PreparedTxID: "ptx-other-instance",
				IsLocal:      true,
				Committed:    false,
			},
		},
	}

	recovered, err := svc.recoverCommitIntent(context.Background(), intent)
	require.NoError(t, err)
	require.False(t, recovered, "skipped participant must not count as recovered")
	// The intent is re-published with no status change since nothing was committed.
	require.Len(t, pub.messages, 1)
	require.False(t, intent.Participants[0].Committed)
}

func TestRecoverCommitIntentPartialCommitRecovery(t *testing.T) {
	// Scenario: a commitIntent has 2 participants. One is already marked
	// Committed=true, the other is Committed=false. Recovery should only
	// attempt to commit the uncommitted participant.
	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b-partial",
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

	ptx, _, err := eng.PrepareAuthorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-partial-commit",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, ptx)

	pub := &capturingPublisher{}
	svc := &authorizerService{
		engine:       eng,
		pub:          pub,
		instanceAddr: "authorizer-1:50051",
	}

	intent := &commitIntent{
		TransactionID: "tx-partial-commit",
		Status:        commitIntentStatusCommitted, // Already partially committed
		Participants: []commitParticipant{
			{
				InstanceAddr: "authorizer-2:50051",
				PreparedTxID: "ptx-already-committed",
				IsLocal:      false,
				Committed:    true, // Already committed by a prior recovery pass
			},
			{
				InstanceAddr: "authorizer-1:50051",
				PreparedTxID: ptx.ID,
				IsLocal:      true,
				Committed:    false, // Not yet committed
			},
		},
	}

	recovered, recoverErr := svc.recoverCommitIntent(context.Background(), intent)
	require.NoError(t, recoverErr)
	require.True(t, recovered, "should report recovery for the uncommitted participant")

	// Verify: participant 0 was already committed and should remain so.
	require.True(t, intent.Participants[0].Committed)
	// Verify: participant 1 should now be committed.
	require.True(t, intent.Participants[1].Committed)
	// Verify: since all participants are committed, status should be COMPLETED.
	require.Equal(t, commitIntentStatusCompleted, intent.Status)
	// Verify: a completion intent was published + cache-invalidation event.
	// The invalidation is the ripple-effect fix for stale Redis cache; see
	// TestRecovery_InvalidatesTransactionCacheOnDriveToCompletion.
	require.Len(t, pub.messages, 2)
}

func TestRecoverCommitIntent_PartialCommit_SkipsAlreadyCommitted(t *testing.T) {
	// Scenario: 2 participants, participant 0 is already Committed=true,
	// participant 1 (remote, uncommitted) needs recovery. The test verifies
	// that CommitPrepared is called ONLY for participant 1 and that
	// participant 0 is completely skipped.
	//
	// A tracking mock records every CommitPrepared invocation so we can
	// assert exactly which prepared-tx-IDs were committed.
	trackingPeer := &trackingStubPeerClient{
		commitResp: &authorizerv1.CommitPreparedResponse{Committed: true},
	}

	pub := &capturingPublisher{}

	logger, logErr := libZap.InitializeLoggerWithError()
	require.NoError(t, logErr)

	svc := &authorizerService{
		pub:           pub,
		logger:        logger,
		peerAuthToken: "peer-secret",
		instanceAddr:  "authorizer-1:50051",
		peers: []*peerClient{
			{
				addr:    "authorizer-2:50051",
				clients: []authorizerv1.BalanceAuthorizerClient{trackingPeer},
			},
			{
				addr:    "authorizer-3:50051",
				clients: []authorizerv1.BalanceAuthorizerClient{trackingPeer},
			},
		},
	}

	intent := &commitIntent{
		TransactionID: "tx-partial-skip",
		Status:        commitIntentStatusPrepared,
		Participants: []commitParticipant{
			{
				InstanceAddr: "authorizer-2:50051",
				PreparedTxID: "ptx-already-done",
				IsLocal:      false,
				Committed:    true, // Participant 0: already committed
			},
			{
				InstanceAddr: "authorizer-3:50051",
				PreparedTxID: "ptx-needs-recovery",
				IsLocal:      false,
				Committed:    false, // Participant 1: needs recovery
			},
		},
	}

	recovered, recoverErr := svc.recoverCommitIntent(context.Background(), intent)
	require.NoError(t, recoverErr)
	require.True(t, recovered, "should report recovery for the uncommitted participant")

	// Verify: CommitPrepared was called exactly once.
	commitCalls := trackingPeer.commitPreparedCalls()
	require.Len(t, commitCalls, 1, "CommitPrepared should be called exactly once")

	// Verify: the single call was for participant 1 (the uncommitted one).
	assert.Equal(t, "ptx-needs-recovery", commitCalls[0],
		"CommitPrepared should target the uncommitted participant's prepared-tx-id")

	// Verify: participant 0 was NOT targeted (its ptx-ID never appeared).
	for _, callID := range commitCalls {
		assert.NotEqual(t, "ptx-already-done", callID,
			"CommitPrepared must NOT be called for the already-committed participant")
	}

	// Verify: both participants are now committed.
	require.True(t, intent.Participants[0].Committed, "participant 0 should remain committed")
	require.True(t, intent.Participants[1].Committed, "participant 1 should now be committed")

	// Verify: intent status advanced to COMPLETED since all participants are committed.
	require.Equal(t, commitIntentStatusCompleted, intent.Status)

	// Verify: a completion intent was published + cache-invalidation event.
	require.Len(t, pub.messages, 2)
}

func TestRecoverCommitIntentAlreadyCompleted(t *testing.T) {
	svc := &authorizerService{}

	intent := &commitIntent{
		TransactionID: "tx-already-done",
		Status:        commitIntentStatusCompleted,
		Participants: []commitParticipant{
			{PreparedTxID: "ptx-1", Committed: true},
		},
	}

	recovered, err := svc.recoverCommitIntent(context.Background(), intent)
	require.NoError(t, err)
	require.False(t, recovered, "already-completed intent should not count as recovered")
}

func TestRecoveryBackoffConstants(t *testing.T) {
	// Sanity-check the backoff constants to prevent accidental changes
	// that could re-introduce the hot-spin problem.
	assert.Equal(t, 3, recoveryBackoffThreshold)
	assert.Equal(t, 1*time.Second, recoveryBackoffInitial)
	assert.Equal(t, 30*time.Second, recoveryBackoffMax)
}

func TestAddrEquivalent(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  bool
	}{
		{name: "exact host and port", left: "authorizer-1:50051", right: "authorizer-1:50051", want: true},
		{name: "same host missing port", left: "authorizer-1", right: "authorizer-1:50051", want: true},
		{name: "different host", left: "authorizer-1:50051", right: "authorizer-2:50051", want: false},
		{name: "different ports", left: "authorizer-1:50051", right: "authorizer-1:50052", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, addrEquivalent(tt.left, tt.right))
		})
	}
}
