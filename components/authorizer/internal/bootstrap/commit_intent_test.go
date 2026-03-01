// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/publisher"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

// capturingPublisher records all published messages for test assertions.
type capturingPublisher struct {
	messages []publisher.Message
	err      error // if set, Publish returns this error
}

func (p *capturingPublisher) Publish(_ context.Context, msg publisher.Message) error {
	if p.err != nil {
		return p.err
	}

	p.messages = append(p.messages, msg)

	return nil
}

func (p *capturingPublisher) Close() error { return nil }

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
	cap := &capturingPublisher{}
	svc := &authorizerService{pub: cap}

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
	require.Len(t, cap.messages, 1)

	msg := cap.messages[0]
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
	cap := &capturingPublisher{err: fmt.Errorf("broker unreachable")}
	svc := &authorizerService{pub: cap}

	err := svc.publishCommitIntent(context.Background(), &commitIntent{
		TransactionID: "tx-fail",
		Status:        "PREPARED",
		Participants:  []commitParticipant{},
		CreatedAt:     time.Now(),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "broker unreachable")
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
	logger := libZap.InitializeLogger()
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
	require.NoError(t, runner.processRecord(context.Background(), record))
}

func TestProcessRecordAcceptsAuthenticatedPayload(t *testing.T) {
	logger := libZap.InitializeLogger()
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
	sig := fmt.Sprintf("%x", mac.Sum(nil))

	record := &kgo.Record{
		Value: payload,
		Headers: []kgo.RecordHeader{{
			Key:   commitIntentAuthHeader,
			Value: []byte(sig),
		}},
	}

	require.NoError(t, runner.processRecord(context.Background(), record))
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

	require.NoError(t, svc.recoverCommitIntent(context.Background(), intent))
	require.True(t, intent.Participants[0].Committed)
	require.Equal(t, commitIntentStatusCompleted, intent.Status)
	require.Len(t, pub.messages, 1)
}

func TestRecoverCommitIntentRemoteParticipant(t *testing.T) {
	pub := &capturingPublisher{}
	peer := &stubPeerClient{commitResp: &authorizerv1.CommitPreparedResponse{Committed: true}}

	svc := &authorizerService{
		pub:           pub,
		logger:        libZap.InitializeLogger(),
		peerAuthToken: "peer-secret",
		peers: []*peerClient{{
			addr:   "authorizer-2:50051",
			client: peer,
		}},
	}

	intent := &commitIntent{
		TransactionID: "tx-recovery-remote",
		Status:        commitIntentStatusPrepared,
		Participants: []commitParticipant{
			{InstanceAddr: "authorizer-2:50051", PreparedTxID: "ptx-remote"},
		},
	}

	require.NoError(t, svc.recoverCommitIntent(context.Background(), intent))
	require.True(t, intent.Participants[0].Committed)
	require.Equal(t, commitIntentStatusCompleted, intent.Status)
	require.Len(t, pub.messages, 1)
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

	err := svc.recoverCommitIntent(context.Background(), intent)
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
		{name: "other error", err: errors.New("boom"), want: false},
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

	require.NoError(t, svc.recoverCommitIntent(context.Background(), intent))
	require.True(t, intent.Participants[0].Committed)
	require.Equal(t, commitIntentStatusCompleted, intent.Status)
	require.Len(t, pub.messages, 1)
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
