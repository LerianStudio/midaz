// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/publisher"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
)

// reconcilerTestLogger implements libLog.Logger for reconciler tests.
type reconcilerTestLogger struct{}

func (reconcilerTestLogger) Info(_ ...any)                     {}
func (reconcilerTestLogger) Infof(_ string, _ ...any)          {}
func (reconcilerTestLogger) Infoln(_ ...any)                   {}
func (reconcilerTestLogger) Error(_ ...any)                    {}
func (reconcilerTestLogger) Errorf(_ string, _ ...any)         {}
func (reconcilerTestLogger) Errorln(_ ...any)                  {}
func (reconcilerTestLogger) Warn(_ ...any)                     {}
func (reconcilerTestLogger) Warnf(_ string, _ ...any)          {}
func (reconcilerTestLogger) Warnln(_ ...any)                   {}
func (reconcilerTestLogger) Debug(_ ...any)                    {}
func (reconcilerTestLogger) Debugf(_ string, _ ...any)         {}
func (reconcilerTestLogger) Debugln(_ ...any)                  {}
func (reconcilerTestLogger) Fatal(_ ...any)                    {}
func (reconcilerTestLogger) Fatalf(_ string, _ ...any)         {}
func (reconcilerTestLogger) Fatalln(_ ...any)                  {}
func (reconcilerTestLogger) WithFields(_ ...any) libLog.Logger { return reconcilerTestLogger{} }
func (reconcilerTestLogger) WithDefaultMessageTemplate(_ string) libLog.Logger {
	return reconcilerTestLogger{}
}
func (reconcilerTestLogger) Sync() error { return nil }

// reconcilerCapturingPublisher records published messages for assertion (thread-safe).
type reconcilerCapturingPublisher struct {
	mu       sync.Mutex
	messages []publisher.Message
}

func (p *reconcilerCapturingPublisher) Publish(_ context.Context, msg publisher.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.messages = append(p.messages, msg)

	return nil
}

func (p *reconcilerCapturingPublisher) Close() error { return nil }

func (p *reconcilerCapturingPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.messages)
}

// testWALHMACKey is a fixed 32-byte key used by bootstrap-package tests that
// need to produce a verifiable WAL file. Real keys are loaded from the
// environment at bootstrap time; see TestLoadConfig_AcceptsValidWALHMACKey.
var testWALHMACKey = []byte("reconciler-test-hmac-key-32bytes")

// writeWALEntry writes a single WAL entry to the given path.
func writeWALEntry(t *testing.T, walPath string, entry wal.Entry) {
	t.Helper()

	writer, err := wal.NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil, testWALHMACKey)
	require.NoError(t, err)

	require.NoError(t, writer.Append(entry))
	require.NoError(t, writer.Close())
}

func newTestReconciler(walPath string, pub publisher.Publisher) *walReconciler {
	svc := &authorizerService{
		pub: pub,
	}

	return &walReconciler{
		service:      svc,
		logger:       reconcilerTestLogger{},
		walPath:      walPath,
		walHMACKeys:  [][]byte{testWALHMACKey},
		interval:     10 * time.Second,
		lookback:     5 * time.Minute,
		grace:        30 * time.Second,
		completedSet: make(map[string]time.Time),
		completedTTL: 10 * time.Minute,
	}
}

func TestReconcilerSkipsSingleShardEntries(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "authorizer.wal")
	pub := &reconcilerCapturingPublisher{}
	rec := newTestReconciler(walPath, pub)

	// Write a single-shard entry (CrossShard=false, no participants).
	writeWALEntry(t, walPath, wal.Entry{
		TransactionID:     "tx-single-shard",
		OrganizationID:    "org",
		LedgerID:          "ledger",
		TransactionStatus: "CREATED",
		CrossShard:        false,
		CreatedAt:         time.Now().Add(-1 * time.Minute),
	})

	rec.reconcile(context.Background())

	assert.Equal(t, 0, pub.count(), "single-shard entry should not trigger a publish")
}

func TestReconcilerSkipsCompletedTransactions(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "authorizer.wal")
	pub := &reconcilerCapturingPublisher{}
	rec := newTestReconciler(walPath, pub)

	txID := "tx-already-completed"

	// Pre-seed the completed set.
	rec.markCompleted(txID)

	writeWALEntry(t, walPath, wal.Entry{
		TransactionID:  txID,
		OrganizationID: "org",
		LedgerID:       "ledger",
		CrossShard:     true,
		Participants: []wal.WALParticipant{
			{InstanceAddr: "localhost:50051", PreparedTxID: "ptx-1", IsLocal: true},
			{InstanceAddr: "localhost:50052", PreparedTxID: "ptx-2", IsLocal: false},
		},
		CreatedAt: time.Now().Add(-1 * time.Minute),
	})

	rec.reconcile(context.Background())

	assert.Equal(t, 0, pub.count(), "completed transaction should not be republished")
}

func TestReconcilerRepublishesOrphanedEntry(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "authorizer.wal")
	pub := &reconcilerCapturingPublisher{}
	rec := newTestReconciler(walPath, pub)

	// Write a cross-shard entry within the [lookback, grace] window.
	writeWALEntry(t, walPath, wal.Entry{
		TransactionID:  "tx-orphaned",
		OrganizationID: "org",
		LedgerID:       "ledger",
		CrossShard:     true,
		Participants: []wal.WALParticipant{
			{InstanceAddr: "localhost:50051", PreparedTxID: "ptx-local", IsLocal: true},
			{InstanceAddr: "localhost:50052", PreparedTxID: "ptx-remote", IsLocal: false},
		},
		CreatedAt: time.Now().Add(-1 * time.Minute),
	})

	rec.reconcile(context.Background())

	assert.Equal(t, 1, pub.count(), "orphaned cross-shard entry should be republished")
	assert.True(t, rec.isCompleted("tx-orphaned"), "transaction should be marked completed after republish")
}

func TestReconcilerRespectsGraceWindow(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "authorizer.wal")
	pub := &reconcilerCapturingPublisher{}
	rec := newTestReconciler(walPath, pub)

	// Write a cross-shard entry that is too recent (within grace period).
	writeWALEntry(t, walPath, wal.Entry{
		TransactionID:  "tx-too-recent",
		OrganizationID: "org",
		LedgerID:       "ledger",
		CrossShard:     true,
		Participants: []wal.WALParticipant{
			{InstanceAddr: "localhost:50051", PreparedTxID: "ptx-1", IsLocal: true},
		},
		CreatedAt: time.Now().Add(-5 * time.Second), // 5s ago, well within 30s grace
	})

	rec.reconcile(context.Background())

	assert.Equal(t, 0, pub.count(), "entry within grace window should not be republished")
}

func TestReconcilerRespectsLookbackWindow(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "authorizer.wal")
	pub := &reconcilerCapturingPublisher{}
	rec := newTestReconciler(walPath, pub)

	// Write a cross-shard entry that is too old (outside lookback).
	writeWALEntry(t, walPath, wal.Entry{
		TransactionID:  "tx-too-old",
		OrganizationID: "org",
		LedgerID:       "ledger",
		CrossShard:     true,
		Participants: []wal.WALParticipant{
			{InstanceAddr: "localhost:50051", PreparedTxID: "ptx-1", IsLocal: true},
		},
		CreatedAt: time.Now().Add(-10 * time.Minute), // 10m ago, outside 5m lookback
	})

	rec.reconcile(context.Background())

	assert.Equal(t, 0, pub.count(), "entry outside lookback window should not be republished")
}

func TestReconcilerIdempotent(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "authorizer.wal")
	pub := &reconcilerCapturingPublisher{}
	rec := newTestReconciler(walPath, pub)

	writeWALEntry(t, walPath, wal.Entry{
		TransactionID:  "tx-idem",
		OrganizationID: "org",
		LedgerID:       "ledger",
		CrossShard:     true,
		Participants: []wal.WALParticipant{
			{InstanceAddr: "localhost:50051", PreparedTxID: "ptx-1", IsLocal: true},
			{InstanceAddr: "localhost:50052", PreparedTxID: "ptx-2", IsLocal: false},
		},
		CreatedAt: time.Now().Add(-1 * time.Minute),
	})

	rec.reconcile(context.Background())
	assert.Equal(t, 1, pub.count(), "first reconcile should publish")

	rec.reconcile(context.Background())
	assert.Equal(t, 1, pub.count(), "second reconcile should be idempotent (no new publishes)")
}

func TestReconcilerCompletedSetTTL(t *testing.T) {
	rec := &walReconciler{
		completedSet: make(map[string]time.Time),
		completedTTL: 10 * time.Minute,
	}

	// Add an entry with a timestamp that is already expired.
	rec.completedMu.Lock()
	rec.completedSet["tx-expired"] = time.Now().Add(-15 * time.Minute)
	rec.completedSet["tx-fresh"] = time.Now()
	rec.completedMu.Unlock()

	rec.pruneCompletedSet(time.Now())

	assert.False(t, rec.isCompleted("tx-expired"), "expired entry should be pruned")
	assert.True(t, rec.isCompleted("tx-fresh"), "fresh entry should be retained")
}

func TestReconcilerConcurrentReconcileAndMarkCompleted(t *testing.T) {
	// This test exercises the race detector by running reconcile() in one goroutine
	// while calling markCompleted() from another goroutine. Verifies no data races.
	walPath := filepath.Join(t.TempDir(), "authorizer.wal")
	pub := &reconcilerCapturingPublisher{}
	rec := newTestReconciler(walPath, pub)

	// Write several cross-shard entries within the reconciliation window.
	for i := 0; i < 5; i++ {
		txID := fmt.Sprintf("tx-concurrent-%d", i)
		writeWALEntry(t, walPath, wal.Entry{
			TransactionID:  txID,
			OrganizationID: "org",
			LedgerID:       "ledger",
			CrossShard:     true,
			Participants: []wal.WALParticipant{
				{InstanceAddr: "localhost:50051", PreparedTxID: fmt.Sprintf("ptx-%d", i), IsLocal: true},
				{InstanceAddr: "localhost:50052", PreparedTxID: fmt.Sprintf("ptx-remote-%d", i), IsLocal: false},
			},
			CreatedAt: time.Now().Add(-1 * time.Minute),
		})
	}

	var wg sync.WaitGroup

	// Goroutine 1: run reconcile multiple times
	wg.Add(1)

	go func() {
		defer wg.Done()

		for i := 0; i < 10; i++ {
			rec.reconcile(context.Background())
		}
	}()

	// Goroutine 2: concurrently mark transactions as completed
	wg.Add(1)

	go func() {
		defer wg.Done()

		for i := 0; i < 5; i++ {
			rec.markCompleted(fmt.Sprintf("tx-concurrent-%d", i))
		}
	}()

	wg.Wait()

	// After concurrent operations, all transactions should be marked completed.
	for i := 0; i < 5; i++ {
		assert.True(t, rec.isCompleted(fmt.Sprintf("tx-concurrent-%d", i)),
			"tx-concurrent-%d should be completed after concurrent operations", i)
	}
}

func TestWALReconciler_ConcurrentMarkAndReconcile(t *testing.T) {
	// Exercises the race detector with a time-bounded concurrent stress test.
	// One goroutine continuously calls reconcile() while the main goroutine
	// concurrently marks transactions as completed. The test must complete
	// within 100ms with no data races or panics.
	walPath := filepath.Join(t.TempDir(), "authorizer.wal")
	pub := &reconcilerCapturingPublisher{}
	rec := newTestReconciler(walPath, pub)

	const numEntries = 8

	// Write several cross-shard entries within the reconciliation window.
	for i := 0; i < numEntries; i++ {
		txID := fmt.Sprintf("tx-race-%d", i)
		writeWALEntry(t, walPath, wal.Entry{
			TransactionID:  txID,
			OrganizationID: "org",
			LedgerID:       "ledger",
			CrossShard:     true,
			Participants: []wal.WALParticipant{
				{InstanceAddr: "localhost:50051", PreparedTxID: fmt.Sprintf("ptx-race-%d-local", i), IsLocal: true},
				{InstanceAddr: "localhost:50052", PreparedTxID: fmt.Sprintf("ptx-race-%d-remote", i), IsLocal: false},
			},
			CreatedAt: time.Now().Add(-1 * time.Minute),
		})
	}

	var wg sync.WaitGroup

	deadline := time.After(80 * time.Millisecond)

	// Goroutine: run reconcile in a tight loop until deadline.
	wg.Add(1)

	go func() {
		defer wg.Done()

		for {
			select {
			case <-deadline:
				return
			default:
				rec.reconcile(context.Background())
			}
		}
	}()

	// Main goroutine: concurrently mark transactions as completed and
	// call isCompleted, exercising both read and write locks.
	for i := 0; i < numEntries; i++ {
		txID := fmt.Sprintf("tx-race-%d", i)
		rec.markCompleted(txID)
		// Interleave a read check to exercise RWMutex contention.
		_ = rec.isCompleted(txID)
	}

	wg.Wait()

	// After all concurrent operations complete, every transaction
	// must be present in the completed set.
	for i := 0; i < numEntries; i++ {
		txID := fmt.Sprintf("tx-race-%d", i)
		assert.True(t, rec.isCompleted(txID),
			"%s should be marked completed after concurrent operations", txID)
	}

	// The publisher should have received at most numEntries messages
	// (some entries may have been skipped because markCompleted ran first).
	assert.LessOrEqual(t, pub.count(), numEntries,
		"publisher should not receive more than %d messages", numEntries)
}

func TestBuildIntentFromWALEntryMarksLocalCommitted(t *testing.T) {
	rec := &walReconciler{}

	entry := &wal.Entry{
		TransactionID:  "tx-build",
		OrganizationID: "org",
		LedgerID:       "ledger",
		CrossShard:     true,
		Participants: []wal.WALParticipant{
			{InstanceAddr: "authorizer-1:50051", PreparedTxID: "ptx-local", IsLocal: true},
			{InstanceAddr: "authorizer-2:50051", PreparedTxID: "ptx-remote", IsLocal: false},
		},
		CreatedAt: time.Now().Add(-2 * time.Minute),
	}

	intent := rec.buildIntentFromWALEntry(entry)
	require.NotNil(t, intent)

	assert.Equal(t, "tx-build", intent.TransactionID)
	assert.Equal(t, "org", intent.OrganizationID)
	assert.Equal(t, "ledger", intent.LedgerID)
	assert.Equal(t, commitIntentStatusPrepared, intent.Status)
	require.Len(t, intent.Participants, 2)

	// Local participant should be marked as committed (WAL proves it).
	assert.True(t, intent.Participants[0].Committed, "local participant should be marked committed")
	assert.True(t, intent.Participants[0].IsLocal)
	assert.Equal(t, "ptx-local", intent.Participants[0].PreparedTxID)

	// Remote participant should NOT be marked as committed.
	assert.False(t, intent.Participants[1].Committed, "remote participant should not be marked committed")
	assert.False(t, intent.Participants[1].IsLocal)
	assert.Equal(t, "ptx-remote", intent.Participants[1].PreparedTxID)
}

func TestBuildIntentFromWALEntryNilSafety(t *testing.T) {
	rec := &walReconciler{}

	assert.Nil(t, rec.buildIntentFromWALEntry(nil), "nil entry should return nil")
	assert.Nil(t, rec.buildIntentFromWALEntry(&wal.Entry{}), "entry with no participants should return nil")
}

// TestReconciler_StopsRepublishingAfterPartialCommit closes the ripple-effect
// vector uncovered by the audit: when a cross-shard transaction hits a
// partial-commit path (handleIncompleteCommit / handleRemotePeerCommitError
// funnel through escalateToManualIntervention), the intent reaches
// MANUAL_INTERVENTION_REQUIRED terminal status, but the reconciler's 5-minute
// lookback window would still see the original WAL entry and re-publish a
// PREPARED intent on every reconcile cycle. That kicked off a loop:
// reconciler → PREPARED → recovery re-escalates → MANUAL_INTERVENTION_REQUIRED
// → reconciler (still sees WAL entry, still doesn't know it's terminal) →
// PREPARED → ... until the seed consumer's next cold-start poll picked up the
// terminal status.
//
// The fix is to have escalateToManualIntervention call markCompleted so the
// reconciler's in-memory completed set reflects the terminal state
// immediately, not only after the next seed. This test drives that path and
// asserts the reconciler does NOT republish on the cycle after escalation.
func TestReconciler_StopsRepublishingAfterPartialCommit(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "authorizer.wal")
	pub := &reconcilerCapturingPublisher{}
	rec := newTestReconciler(walPath, pub)

	// The service escalate path needs a back-reference to the reconciler so
	// escalateToManualIntervention can mark the transaction completed.
	rec.service.walReconciler = rec

	txID := "tx-partial-commit"

	// Write a cross-shard WAL entry within the reconciliation window. This
	// simulates the local commit having succeeded (WAL entry persisted) with
	// a remote peer commit having failed downstream.
	writeWALEntry(t, walPath, wal.Entry{
		TransactionID:  txID,
		OrganizationID: "org",
		LedgerID:       "ledger",
		CrossShard:     true,
		Participants: []wal.WALParticipant{
			{InstanceAddr: "localhost:50051", PreparedTxID: "ptx-local", IsLocal: true},
			{InstanceAddr: "localhost:50052", PreparedTxID: "ptx-remote", IsLocal: false},
		},
		CreatedAt: time.Now().Add(-1 * time.Minute),
	})

	// Drive the intent through escalation the same way the partial-commit
	// paths in cross_shard.go do. escalateToManualIntervention must mark the
	// transaction completed on the reconciler so the next reconcile cycle
	// does not re-publish PREPARED from the WAL.
	intent := &commitIntent{
		TransactionID:  txID,
		OrganizationID: "org",
		LedgerID:       "ledger",
		Status:         commitIntentStatusPrepared,
		Participants: []commitParticipant{
			{InstanceAddr: "localhost:50051", PreparedTxID: "ptx-local", IsLocal: true},
			{InstanceAddr: "localhost:50052", PreparedTxID: "ptx-remote"},
		},
	}

	require.NoError(t, rec.service.escalateToManualIntervention(
		context.Background(), intent, manualInterventionReasonRemoteNotFound,
	))

	// escalateToManualIntervention publishes twice: commits topic + manual-
	// intervention topic. Record that baseline so we can assert the next
	// cycle adds zero additional publishes.
	baseline := pub.count()
	require.GreaterOrEqual(t, baseline, 1, "escalation must publish at least once")
	require.True(t, rec.isCompleted(txID),
		"escalation must mark the transaction completed so the reconciler skips it")

	// Drive one full reconcile cycle. With the fix, the reconciler observes
	// the WAL entry, sees the completed flag, and skips. Without the fix
	// (pre-ripple-fix behavior), the reconciler would add one more PREPARED
	// publish here — the test asserts the count is stable.
	rec.reconcile(context.Background())

	assert.Equal(t, baseline, pub.count(),
		"reconciler must NOT republish PREPARED after partial-commit escalation")

	// A second cycle exercises the idempotency contract at the reconciler
	// level: even a second scan of the same WAL entry must remain a no-op.
	rec.reconcile(context.Background())
	assert.Equal(t, baseline, pub.count(),
		"second reconcile cycle must remain a no-op after escalation")
}

// TestRecovery_InvalidatesTransactionCacheOnDriveToCompletion asserts the
// recovery runner emits a cache-invalidation event on the dedicated topic
// whenever finalizeRecoveryStatus transitions an intent to COMPLETED. Without
// this signal, transaction-service Redis caches can hold pre-recovery balances
// indefinitely because the cache only refreshes on the next LoadBalances
// (which may be hours away for a cold alias). The publish is best-effort —
// failures must NOT roll back the recovery — but the message MUST appear on
// the invalidation topic on the happy path.
func TestRecovery_InvalidatesTransactionCacheOnDriveToCompletion(t *testing.T) {
	pub := &reconcilerCapturingPublisher{}
	svc := &authorizerService{pub: pub}

	intent := &commitIntent{
		TransactionID:  "tx-recovery-invalidate",
		OrganizationID: "org-abc",
		LedgerID:       "ledger-xyz",
		Status:         commitIntentStatusCommitted,
		Participants: []commitParticipant{
			{InstanceAddr: "localhost:50051", PreparedTxID: "ptx-a", Committed: true, IsLocal: true},
			{InstanceAddr: "localhost:50052", PreparedTxID: "ptx-b", Committed: true},
		},
	}

	// anyParticipantCommitted=true triggers the COMMITTED→COMPLETED transition
	// inside finalizeRecoveryStatus, which is the drive-to-completion path.
	require.NoError(t, svc.finalizeRecoveryStatus(context.Background(), intent, true))
	assert.Equal(t, commitIntentStatusCompleted, intent.Status)

	// finalizeRecoveryStatus publishes the commit-intent COMPLETED record
	// first, then the cache-invalidation event. Assert BOTH topics received
	// the right message.
	pub.mu.Lock()
	defer pub.mu.Unlock()

	var sawCommit, sawInvalidation bool

	for _, msg := range pub.messages {
		switch msg.Topic {
		case crossShardCommitTopic:
			sawCommit = true
		case crossShardCacheInvalidationTopic:
			sawInvalidation = true

			var evt cacheInvalidationEvent
			require.NoError(t, json.Unmarshal(msg.Payload, &evt),
				"cache-invalidation payload must decode as cacheInvalidationEvent")

			assert.Equal(t, "tx-recovery-invalidate", evt.TransactionID)
			assert.Equal(t, "org-abc", evt.OrganizationID)
			assert.Equal(t, "ledger-xyz", evt.LedgerID)
			assert.Equal(t, "recovery_drive_to_completion", evt.Reason)
			assert.False(t, evt.EmittedAt.IsZero(), "emitted_at must be populated")
			assert.Equal(t, "tx-recovery-invalidate", msg.PartitionKey,
				"partition key must be transaction_id so all events for the same tx land in order")
		}
	}

	assert.True(t, sawCommit, "recovery must publish COMPLETED intent to commits topic")
	assert.True(t, sawInvalidation, "recovery must publish to cache-invalidation topic on drive-to-completion")
}
