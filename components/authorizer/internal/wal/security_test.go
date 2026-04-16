// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package wal

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type recordingObserver struct {
	mu               sync.Mutex
	queueDepth       []int
	appendDropped    []error
	writeErrors      []string
	fsyncLatency     []time.Duration
	hmacVerifyFailed []observedHMACFailure
	truncations      []observedTruncation
}

type observedHMACFailure struct {
	offset int64
	reason string
}

type observedTruncation struct {
	offset int64
	reason string
}

func (r *recordingObserver) ObserveWALQueueDepth(depth int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.queueDepth = append(r.queueDepth, depth)
}

func (r *recordingObserver) ObserveWALAppendDropped(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.appendDropped = append(r.appendDropped, err)
}

func (r *recordingObserver) ObserveWALWriteError(stage string, _ error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.writeErrors = append(r.writeErrors, stage)
}

func (r *recordingObserver) ObserveWALFsyncLatency(latency time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.fsyncLatency = append(r.fsyncLatency, latency)
}

func (r *recordingObserver) ObserveWALHMACVerifyFailed(offset int64, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.hmacVerifyFailed = append(r.hmacVerifyFailed, observedHMACFailure{offset: offset, reason: reason})
}

func (r *recordingObserver) ObserveWALTruncation(offset int64, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.truncations = append(r.truncations, observedTruncation{offset: offset, reason: reason})
}

func (r *recordingObserver) hmacFailures() []observedHMACFailure {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]observedHMACFailure, len(r.hmacVerifyFailed))
	copy(out, r.hmacVerifyFailed)

	return out
}

func (r *recordingObserver) truncationEvents() []observedTruncation {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]observedTruncation, len(r.truncations))
	copy(out, r.truncations)

	return out
}

// TestHMACVerifyRejectsForgedEntry proves that a single-byte mutation in the
// payload causes HMAC verification to fail and the file to be truncated.
// Without HMAC this test could not distinguish tampering from a random flip,
// because CRC32 detects only random corruption -- not authenticated forgery.
func TestHMACVerifyRejectsForgedEntry(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "forged.wal")

	writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil, testHMACKey)
	require.NoError(t, err)

	require.NoError(t, writer.Append(Entry{
		TransactionID:  "tx-genuine",
		OrganizationID: "org",
		LedgerID:       "ledger",
		Mutations: []BalanceMutation{{
			AccountAlias: "@alice", BalanceKey: "default",
			Available: 1000, PreviousVersion: 1, NextVersion: 2,
		}},
	}))
	require.NoError(t, writer.Close())

	f, err := os.OpenFile(walPath, os.O_RDWR, 0)
	require.NoError(t, err)

	fi, err := f.Stat()
	require.NoError(t, err)
	require.Greater(t, fi.Size(), int64(recordHeaderSize+1))

	originalByte := make([]byte, 1)
	_, err = f.ReadAt(originalByte, int64(recordHeaderSize))
	require.NoError(t, err)

	mutated := originalByte[0] ^ 0xFF
	_, err = f.WriteAt([]byte{mutated}, int64(recordHeaderSize))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	obs := &recordingObserver{}

	entries, err := Replay(walPath, [][]byte{testHMACKey}, obs)
	require.NoError(t, err)
	require.Empty(t, entries, "forged entry must be discarded")

	failures := obs.hmacFailures()
	require.Len(t, failures, 1, "HMAC failure metric must fire exactly once")
	require.Equal(t, "hmac_mismatch", failures[0].reason)

	truncations := obs.truncationEvents()
	require.NotEmpty(t, truncations, "truncation metric must fire")
	require.Equal(t, "hmac_mismatch", truncations[0].reason)

	fi2, err := os.Stat(walPath)
	require.NoError(t, err)
	require.Equal(t, int64(0), fi2.Size(), "file must be truncated at the forged frame")
}

// TestHMACKeyRotationAcceptsPreviousKey proves that during key rotation the
// reader accepts frames signed by either the current OR the previous key,
// so no restart/flush-all-WAL step is required when rotating.
func TestHMACKeyRotationAcceptsPreviousKey(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "rotation.wal")

	previousKey := []byte("previous-rotation-key-thirty-2b!")
	currentKey := []byte("current-rotation-key-thirty-2by!")

	writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil, previousKey)
	require.NoError(t, err)

	require.NoError(t, writer.Append(Entry{
		TransactionID:  "tx-prerotation",
		OrganizationID: "org",
		LedgerID:       "ledger",
	}))
	require.NoError(t, writer.Close())

	entries, err := Replay(walPath, [][]byte{currentKey, previousKey}, nil)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "tx-prerotation", entries[0].TransactionID)

	walPath2 := filepath.Join(t.TempDir(), "rotation-nofallback.wal")

	writer2, err := NewRingBufferWriterWithOptions(walPath2, 16, time.Hour, true, nil, previousKey)
	require.NoError(t, err)

	require.NoError(t, writer2.Append(Entry{
		TransactionID:  "tx-dropped",
		OrganizationID: "org",
		LedgerID:       "ledger",
	}))
	require.NoError(t, writer2.Close())

	entries2, err := Replay(walPath2, [][]byte{currentKey}, nil)
	require.NoError(t, err)
	require.Empty(t, entries2, "frame must be dropped when previous key is not supplied")
}

// TestOpenWALFileRejectsSymlinkAttack proves that O_NOFOLLOW blocks the
// classic symlink-swap attack where an attacker replaces the WAL path with a
// symlink pointing at an arbitrary victim file.
func TestOpenWALFileRejectsSymlinkAttack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("O_NOFOLLOW is a POSIX flag; Windows reparse points are a different attack surface")
	}

	tmpDir := t.TempDir()
	victim := filepath.Join(tmpDir, "victim.txt")
	require.NoError(t, os.WriteFile(victim, []byte("sensitive"), 0o600))

	walPath := filepath.Join(tmpDir, "wal.log")
	require.NoError(t, os.Symlink(victim, walPath))

	_, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil, testHMACKey)
	require.Error(t, err, "opening a symlinked WAL path must fail under O_NOFOLLOW")
	require.Contains(t, err.Error(), "open wal file")

	content, readErr := os.ReadFile(victim)
	require.NoError(t, readErr)
	require.Equal(t, "sensitive", string(content))
}

// TestReplayRejectsShortHMACKey ensures a key shorter than MinHMACKeyLength
// is rejected up front so operators do not accidentally weaken the WAL by
// supplying a short rotation key.
func TestReplayRejectsShortHMACKey(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "short-key.wal")

	writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil, testHMACKey)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	_, err = Replay(walPath, [][]byte{[]byte("too-short")}, nil)
	require.Error(t, err)
}
