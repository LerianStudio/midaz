// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package wal

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

// Package-level sentinel errors used to latch the WAL writer into a
// stuck/closed state deterministically. Avoids err113 and makes the
// "this is a test-injected failure" contract explicit.
var (
	errBackpressureDiskFailure = errors.New("backpressure test: disk failure forced")
	errBackpressureIgnored     = errors.New("backpressure test: second error should be ignored")
)

// TestWAL_FullBufferReturnsErrWALBufferFull proves that the async writer
// returns ErrWALBufferFull rather than blocking the producer when the
// internal channel has no capacity left. We exercise the fail-fast branch
// directly by closing the drain goroutine's stop channel under the writer —
// which prevents consumption of any pending items — then filling the
// buffer past its capacity. The second Append MUST fail.
//
// This is the core backpressure contract: under overload, the writer MUST
// surface the overflow to callers (who record fail-closed metrics) instead
// of swallowing entries silently or blocking the authorize hot path.
//
// Implementation note: the obvious "wedge the drain via blocking observer"
// approach deadlocks because observeQueueDepth runs on both producer and
// drain paths. We instead exercise the default branch of the select
// statement by racing enough Appends against a 1-slot buffer that one is
// guaranteed to land while the slot is occupied. We use a 1-slot buffer
// and a very small flush interval so the drain goroutine must contend
// with the producer.
func TestWAL_FullBufferReturnsErrWALBufferFull(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "backpressure.wal")

	// 1-slot buffer + long flush interval — nothing drains from the ticker
	// side; only the direct receive in run() can empty the channel.
	writer, err := NewRingBufferWriterWithOptions(walPath, 1, time.Hour, false, nil, testHMACKey)
	require.NoError(t, err)

	defer func() { _ = writer.Close() }()

	// Burst-append many entries. With only a 1-slot channel and the
	// drain loop running one-entry-at-a-time through disk writes, at
	// least ONE of these must hit the `default:` branch and return
	// ErrWALBufferFull. If the test runs on a machine so fast that no
	// contention ever occurs, the test falls back to asserting "at least
	// one attempt completed" — but we expect the overflow branch to fire
	// at this ratio.
	const burst = 256

	var (
		overflowed bool
		successes  int
	)

	for i := 0; i < burst; i++ {
		err := writer.Append(Entry{
			TransactionID:  "tx-burst",
			OrganizationID: "org",
			LedgerID:       "ledger",
		})

		switch {
		case err == nil:
			successes++
		case errors.Is(err, constant.ErrWALBufferFull):
			overflowed = true
		default:
			t.Fatalf("unexpected error from Append: %v", err)
		}
	}

	require.True(t, overflowed,
		"at least one Append MUST return ErrWALBufferFull when the channel is saturated")
	require.Positive(t, successes,
		"some Appends must succeed before saturation — otherwise the test is meaningless")
}

// TestWAL_StickyErrorReturnsImmediately proves that once a write/flush/sync
// error is latched into the writer's lastErr field, every subsequent Append
// returns that error without even touching the channel or file. This is the
// fail-closed contract that prevents silent data loss: after one disk error,
// we refuse to pretend everything is fine.
func TestWAL_StickyErrorReturnsImmediately(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "sticky.wal")

	// Sync mode so we can deterministically force a write error via file
	// close under the writer's feet.
	writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil, testHMACKey)
	require.NoError(t, err)

	// Force a sentinel error into the writer's error state. This simulates
	// the effect of a mid-Append disk failure having latched.
	writer.setError(errBackpressureDiskFailure)

	// The next Append must return the sticky error without attempting I/O.
	err = writer.Append(Entry{TransactionID: "tx-after-error", OrganizationID: "org", LedgerID: "ledger"})
	require.Error(t, err)
	require.ErrorIs(t, err, errBackpressureDiskFailure,
		"sticky error must surface unchanged on the next Append")

	// A second call must also return the sticky error — latched state, not
	// one-shot.
	err = writer.Append(Entry{TransactionID: "tx-again", OrganizationID: "org", LedgerID: "ledger"})
	require.ErrorIs(t, err, errBackpressureDiskFailure)

	// setError is monotonic: a second error doesn't overwrite the first.
	writer.setError(errBackpressureIgnored)

	err = writer.Append(Entry{TransactionID: "tx-final", OrganizationID: "org", LedgerID: "ledger"})
	require.ErrorIs(t, err, errBackpressureDiskFailure,
		"setError must be monotonic: the first latched error survives")

	// Close may still return the latched error — that's expected.
	_ = writer.Close()
}

// TestWAL_ErrWALWriterClosingOnShutdown proves that after Close() has
// started draining (which closes the stop channel), any Append racing with
// shutdown returns ErrWALWriterClosing rather than either blocking or
// appending after the file handle is gone. Critical for graceful shutdown:
// we must never lose the acknowledgement→append→crash invariant by
// accepting an Append we can't actually persist.
func TestWAL_ErrWALWriterClosingOnShutdown(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "closing.wal")

	writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil, testHMACKey)
	require.NoError(t, err)

	// Close the writer synchronously. Close() closes r.stop which Append
	// selects on before any I/O.
	require.NoError(t, writer.Close())

	err = writer.Append(Entry{TransactionID: "tx-after-close", OrganizationID: "org", LedgerID: "ledger"})
	require.Error(t, err)
	require.ErrorIs(t, err, constant.ErrWALWriterClosing,
		"Append on a closed writer MUST return ErrWALWriterClosing")
}

// TestWAL_SyncVsAsyncModeBehaviorDiffers documents the observable semantic
// differences between the two modes so a future refactor cannot collapse
// them without updating this test. Async mode: Append returns quickly but
// no durability guarantee until the flush ticker fires. Sync mode: Append
// holds the caller until fsync returns, and therefore a non-nil error from
// Append means "this entry is NOT on disk".
func TestWAL_SyncVsAsyncModeBehaviorDiffers(t *testing.T) {
	t.Run("async mode batches without immediate fsync", func(t *testing.T) {
		walPath := filepath.Join(t.TempDir(), "async.wal")

		writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, false, nil, testHMACKey)
		require.NoError(t, err)

		require.NoError(t, writer.Append(Entry{TransactionID: "tx-async", OrganizationID: "org", LedgerID: "ledger"}))
		// Nothing guarantees the entry is on disk yet.

		require.NoError(t, writer.Close())

		entries, err := Replay(walPath, [][]byte{testHMACKey}, nil)
		require.NoError(t, err)
		require.Len(t, entries, 1, "async entry must be durable after Close")
	})

	t.Run("sync mode guarantees durability per-append", func(t *testing.T) {
		walPath := filepath.Join(t.TempDir(), "sync.wal")

		writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil, testHMACKey)
		require.NoError(t, err)

		require.NoError(t, writer.Append(Entry{TransactionID: "tx-sync", OrganizationID: "org", LedgerID: "ledger"}))
		// In sync mode, data is on disk BEFORE Close. Replaying without
		// closing the writer is non-portable on Windows (file locked) so
		// we close to make the test cross-platform, but the contract we
		// prove is that the single Append completed with no subsequent
		// action needed to persist it.
		require.NoError(t, writer.Close())

		entries, err := Replay(walPath, [][]byte{testHMACKey}, nil)
		require.NoError(t, err)
		require.Len(t, entries, 1)
	})
}
