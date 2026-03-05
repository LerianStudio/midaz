// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
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

// writeWALEntry writes a single WAL entry to the given path.
func writeWALEntry(t *testing.T, walPath string, entry wal.Entry) {
	t.Helper()

	writer, err := wal.NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil)
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
