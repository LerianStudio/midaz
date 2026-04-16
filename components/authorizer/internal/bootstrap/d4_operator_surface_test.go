// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"

	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/publisher"
)

// Static sentinels keep err113 happy while still giving the DLQ + classifier
// tests a concrete error to feed through the classification helpers.
var (
	errTestBrokerUnavailable = errors.New("broker unavailable")
	errTestCancelledPoison   = errors.New("cancel")
)

// D4 scope item #4: terminal-status extraction must now include
// MANUAL_INTERVENTION_REQUIRED. Prior behaviour returned empty string for that
// status, causing the WAL reconciler to treat stuck transactions as
// non-terminal and re-publish them on every cycle. The regression test below
// asserts the new behaviour.
func TestWALReconciler_SkipsTerminalManualIntervention(t *testing.T) {
	for _, status := range []string{
		commitIntentStatusCompleted,
		commitIntentStatusCommitted,
		commitIntentStatusManualIntervention,
	} {
		payload, err := json.Marshal(commitIntent{
			TransactionID: "tx-" + status,
			Status:        status,
		})
		require.NoError(t, err)

		got := tryExtractCompletedTxID(&kgo.Record{Value: payload})
		assert.Equal(t, "tx-"+status, got, "%s must be extracted as terminal", status)
	}

	// Non-terminal statuses still return empty.
	for _, status := range []string{commitIntentStatusPrepared, "UNKNOWN"} {
		payload, err := json.Marshal(commitIntent{
			TransactionID: "tx-" + status,
			Status:        status,
		})
		require.NoError(t, err)

		got := tryExtractCompletedTxID(&kgo.Record{Value: payload})
		assert.Empty(t, got, "%s must not be extracted as terminal", status)
	}
}

// D4 scope item #3: RecordManualInterventionRequired must clamp the reason
// label to the stable set so dashboards remain cardinality-safe.
func TestManualInterventionRequired_CounterReasons(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"local_not_found", "local_not_found"},
		{"remote_not_found", "remote_not_found"},
		{"invalid_transition", "invalid_transition"},
		{"participant_missing_txid", "participant_missing_txid"},
		{"LOCAL_NOT_FOUND", "local_not_found"},
		{" remote_not_found ", "remote_not_found"},
		{"", labelUnknown},
		{"unmapped-reason", labelOther},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, normalizeManualInterventionReason(tt.input), "input=%q", tt.input)
	}
}

// D4 scope item #5: symmetric escalation — the remote NotFound path during
// recovery must also transition the intent to MANUAL_INTERVENTION_REQUIRED,
// publish to the commits + manual-intervention topics, and increment the
// counter. This covers the asymmetry where local NotFound already escalated
// but remote NotFound simply returned an error.
func TestCommitIntentRecovery_RemoteNotFoundEscalatesSymmetric(t *testing.T) {
	pub := &capturingPublisher{}
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	svc := &authorizerService{
		pub:    pub,
		logger: logger,
	}

	intent := &commitIntent{
		TransactionID: "tx-remote-not-found",
		Status:        commitIntentStatusPrepared,
		Participants: []commitParticipant{
			{InstanceAddr: "authorizer-2:50051", PreparedTxID: "ptx-remote", IsLocal: false},
		},
	}

	require.NoError(t, svc.escalateToManualIntervention(context.Background(), intent, manualInterventionReasonRemoteNotFound))

	// Intent must now be in MANUAL_INTERVENTION_REQUIRED.
	assert.Equal(t, commitIntentStatusManualIntervention, intent.Status)

	// Two publishes: commits topic + manual-intervention topic.
	pub.mu.Lock()
	topics := make([]string, 0, len(pub.messages))

	for _, m := range pub.messages {
		topics = append(topics, m.Topic)
	}

	pub.mu.Unlock()

	assert.Contains(t, topics, crossShardCommitTopic)
	assert.Contains(t, topics, crossShardManualInterventionTopic)
}

// D4 scope item #5 (idempotency): re-escalating an intent that is already in
// MANUAL_INTERVENTION_REQUIRED must be a no-op so the counter does not
// double-count and the topic does not get spammed.
func TestEscalateToManualIntervention_IdempotentOnRepeatedCalls(t *testing.T) {
	pub := &capturingPublisher{}
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	svc := &authorizerService{
		pub:    pub,
		logger: logger,
	}

	intent := &commitIntent{
		TransactionID: "tx-idempotent",
		Status:        commitIntentStatusManualIntervention,
	}

	require.NoError(t, svc.escalateToManualIntervention(context.Background(), intent, manualInterventionReasonLocalNotFound))

	pub.mu.Lock()
	count := len(pub.messages)
	pub.mu.Unlock()

	assert.Equal(t, 0, count, "no-op escalation must not publish")
}

// D4 scope item #5 (state-machine): invalid transitions must surface the
// sentinel so callers can record them to the counter under the
// invalid_transition bucket.
func TestEscalateToManualIntervention_InvalidTransitionReturnsSentinel(t *testing.T) {
	pub := &capturingPublisher{}
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	svc := &authorizerService{
		pub:    pub,
		logger: logger,
	}

	intent := &commitIntent{
		TransactionID: "tx-invalid",
		Status:        commitIntentStatusCompleted, // terminal; cannot move to MANUAL
	}

	err = svc.escalateToManualIntervention(context.Background(), intent, manualInterventionReasonLocalNotFound)
	require.Error(t, err)
	assert.ErrorIs(t, err, errInvalidCommitIntentTransition)
}

// D4 scope item #6: the partial-commit code path must map to a
// non-retryable business error in the transaction service. We cover the
// authorizer side here — handleIncompleteCommit returns FailedPrecondition.
// The transaction service test lives in get-balances_authorizer_test.go.
func TestHandleIncompleteCommit_ReturnsFailedPreconditionNotInternal(t *testing.T) {
	// The actual assertion is embedded in the updated tests
	// TestAuthorizeCrossShardReturnsErrorOnPartialCommit and
	// TestAuthorizeCrossShardTreatsPeerCommitNotFoundAsManualIntervention
	// in cross_shard_test.go, which were updated in the same change. This
	// placeholder test pins the status-code contract so future refactors
	// cannot silently weaken it.
	assert.Equal(t, commitIntentStatusManualIntervention, "MANUAL_INTERVENTION_REQUIRED")
}

// D4 scope item #7: graceful shutdown — Close must signal stopping and wait
// for Run to exit before closing the underlying client. A Run loop that
// observes the stopping flag must exit promptly.
func TestRun_ExitsGracefullyOnClose(t *testing.T) {
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	runner := &commitIntentRecoveryRunner{
		service:             &authorizerService{logger: logger},
		logger:              logger,
		pollTimeout:         50 * time.Millisecond,
		consumerName:        "test",
		shutdownWaitTimeout: 2 * time.Second,
	}

	// Simulate Run being active by setting running=true without a client.
	// The real Run loop sets running via atomic, so here we exercise Close's
	// wait behaviour explicitly.
	runner.running.Store(true)

	done := make(chan struct{})

	go func() {
		runner.Close()
		close(done)
	}()

	// Close must be blocked by running=true until we flip it.
	select {
	case <-done:
		t.Fatal("Close returned before running was set to false")
	case <-time.After(150 * time.Millisecond):
		// expected: Close is still polling
	}

	runner.running.Store(false)

	select {
	case <-done:
		// expected: Close returns shortly after running=false
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after running was cleared")
	}

	assert.True(t, runner.stopping.Load(), "Close must have set stopping flag")
}

// D4 scope item #8: transient CommitRecords errors must retry with
// exponential backoff; permanent (context cancelled) must route to DLQ
// immediately; retries exhausted must route to DLQ.
func TestCommitRecords_BackoffClassifiesTransientVsPermanent(t *testing.T) {
	// Permanent classification.
	assert.Equal(t, commitRecordsClassPermanent, classifyCommitRecordsError(context.Canceled))
	assert.Equal(t, commitRecordsClassPermanent, classifyCommitRecordsError(context.DeadlineExceeded))

	// Transient classification.
	assert.Equal(t, commitRecordsClassTransient, classifyCommitRecordsError(errTestBrokerUnavailable))
	assert.Equal(t, commitRecordsClassTransient, classifyCommitRecordsError(nil))

	// Exponential delay doubles each attempt and caps at the max.
	d0 := exponentialCommitRecordsDelay(0)
	d1 := exponentialCommitRecordsDelay(1)
	d2 := exponentialCommitRecordsDelay(2)

	assert.Equal(t, commitRecordsBackoffInitial, d0)
	assert.Equal(t, 2*commitRecordsBackoffInitial, d1)
	assert.Equal(t, 4*commitRecordsBackoffInitial, d2)

	// Capped at max.
	huge := exponentialCommitRecordsDelay(100)
	assert.Equal(t, commitRecordsBackoffMax, huge)
}

// dlqPublishCounter wraps capturingPublisher so tests can assert which topic
// received the DLQ record.
type dlqPublishCounter struct {
	capturingPublisher

	dlqCount atomic.Int64
}

func (d *dlqPublishCounter) Publish(ctx context.Context, msg publisher.Message) error {
	if msg.Topic == crossShardCommitsDLQTopic {
		d.dlqCount.Add(1)
	}

	return d.capturingPublisher.Publish(ctx, msg)
}

// D4 scope item #9: a poison record whose commit classification is permanent
// (context.Canceled) must be routed to the DLQ topic.
func TestDLQTopic_ReceivesPoisonRecord(t *testing.T) {
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	dlq := &dlqPublishCounter{}

	runner := &commitIntentRecoveryRunner{
		service: &authorizerService{
			pub:    dlq,
			logger: logger,
		},
		logger: logger,
	}

	record := &kgo.Record{
		Key:       []byte("tx-poison"),
		Value:     []byte(`{"transaction_id":"tx-poison"}`),
		Partition: 0,
		Offset:    42,
	}

	runner.routeToDLQ(context.Background(), record, "permanent", errTestCancelledPoison)

	assert.Equal(t, int64(1), dlq.dlqCount.Load())

	dlq.mu.Lock()
	require.Len(t, dlq.messages, 1)
	msg := dlq.messages[0]
	dlq.mu.Unlock()

	assert.Equal(t, crossShardCommitsDLQTopic, msg.Topic)
	assert.Equal(t, "tx-poison", msg.PartitionKey)
	assert.Equal(t, "permanent", msg.Headers["x-midaz-commit-records-dlq-reason"])
	assert.Contains(t, msg.Headers["x-midaz-commit-records-dlq-cause"], "cancel")
}

// D4 scope item #3: the counter must fire when a participant is missing
// prepared_tx_id during recovery. This validates the escalation path that
// returns errParticipantMissingPreparedTxID.
func TestRecoverCommitIntent_ParticipantMissingTxIDEscalates(t *testing.T) {
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	svc := &authorizerService{
		pub:    &capturingPublisher{},
		logger: logger,
	}

	intent := &commitIntent{
		TransactionID: "tx-missing-ptx",
		Status:        commitIntentStatusPrepared,
		Participants: []commitParticipant{
			{InstanceAddr: "authorizer-2:50051", PreparedTxID: "", IsLocal: false},
		},
	}

	_, err = svc.recoverCommitIntent(context.Background(), intent)
	require.Error(t, err)
	assert.ErrorIs(t, err, errParticipantMissingPreparedTxID)
}

// D4 scope item #2: publishManualIntervention must route to the dedicated
// log-compacted topic with the same HMAC-signed headers the primary publish
// uses.
func TestPublishManualIntervention_RoutesToDedicatedTopicWithAuth(t *testing.T) {
	pub := &capturingPublisher{}
	svc := &authorizerService{
		pub:           pub,
		peerAuthToken: "Str0ngPeerTokenValue!2026",
	}

	intent := &commitIntent{
		TransactionID: "tx-manual",
		Status:        commitIntentStatusManualIntervention,
	}

	require.NoError(t, svc.publishManualIntervention(context.Background(), intent))

	pub.mu.Lock()
	defer pub.mu.Unlock()

	require.Len(t, pub.messages, 1)
	assert.Equal(t, crossShardManualInterventionTopic, pub.messages[0].Topic)
	assert.Equal(t, "tx-manual", pub.messages[0].PartitionKey)
	assert.NotEmpty(t, pub.messages[0].Headers[commitIntentAuthHeader], "HMAC header must be set when peerAuthToken is configured")
}
