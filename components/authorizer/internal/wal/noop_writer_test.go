// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package wal

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

var errForwardTest = errors.New("test: forwarded error")

// TestNewNoopWriter_AppendIsSafelyIgnored proves the noop writer never
// surfaces errors from Append so it can be used in dev/test mode or as the
// fallback when WAL is intentionally disabled. Hidden errors would cause
// spurious fail-closed rejections in the authorize path.
func TestNewNoopWriter_AppendIsSafelyIgnored(t *testing.T) {
	t.Parallel()

	w := NewNoopWriter()
	require.NotNil(t, w)

	// Many Appends — none should error or panic.
	for i := 0; i < 10; i++ {
		require.NoError(t, w.Append(Entry{}))
	}
}

// TestNewNoopWriter_CloseIdempotent proves repeated Close calls on the noop
// writer are safe (the real writer may be closed defensively from multiple
// shutdown paths; noop must match that contract).
func TestNewNoopWriter_CloseIdempotent(t *testing.T) {
	t.Parallel()

	w := NewNoopWriter()
	require.NoError(t, w.Close())
	require.NoError(t, w.Close())
	require.NoError(t, w.Close())
}

// TestNewRingBufferWriter_RejectsEmptyPath proves the constructor rejects
// an unset WAL path rather than silently writing to cwd.
func TestNewRingBufferWriter_RejectsEmptyPath(t *testing.T) {
	t.Parallel()

	_, err := NewRingBufferWriter("", 64, 0, make([]byte, MinHMACKeyLength))
	require.Error(t, err)
}

// TestNewRingBufferWriter_RejectsShortHMACKey proves integrity-key length
// enforcement: a short key must not silently downgrade MAC strength.
func TestNewRingBufferWriter_RejectsShortHMACKey(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir() + "/wal.log"
	_, err := NewRingBufferWriter(tmp, 64, 0, []byte("short"))
	require.Error(t, err)
}

// TestNewRingBufferWriterWithObserver_HappyPath proves the observer-aware
// constructor wires through to the same options path and produces a
// functional, closeable writer.
func TestNewRingBufferWriterWithObserver_HappyPath(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir() + "/wal-obs.log"
	key := make([]byte, MinHMACKeyLength)
	for i := range key {
		key[i] = byte(i + 1)
	}

	w, err := NewRingBufferWriterWithObserver(tmp, 128, 0, nil, key)
	require.NoError(t, err)
	require.NotNil(t, w)
	require.NoError(t, w.Close())
}

// TestRingBufferWriter_ObserverHelpersAreNilSafe proves the internal observe*
// helpers handle a nil receiver and a nil observer gracefully — otherwise
// any error path that tries to report would double-fault.
func TestRingBufferWriter_ObserverHelpersAreNilSafe(t *testing.T) {
	t.Parallel()

	// Nil receiver: guarded by `if r == nil` in the helpers.
	var r *RingBufferWriter

	require.NotPanics(t, func() {
		r.observeQueueDepth(1)
		r.observeAppendDropped(nil)
		r.observeWriteError("stage", nil)
		r.observeFsyncLatency(0)
	})

	// Non-nil receiver but nil obs: same no-panic contract.
	r2 := &RingBufferWriter{}
	require.NotPanics(t, func() {
		r2.observeQueueDepth(1)
		r2.observeAppendDropped(nil)
		r2.observeWriteError("stage", nil)
		r2.observeFsyncLatency(0)
	})
}

// TestRingBufferWriter_ObserverHelpersForwardToObserver proves that when an
// Observer is wired, each helper routes its argument through. This is a
// behavioural contract — the observability pipeline depends on it to surface
// backpressure, dropped appends, and write errors in production.
func TestRingBufferWriter_ObserverHelpersForwardToObserver(t *testing.T) {
	t.Parallel()

	obs := &recordingObserver{}
	r := &RingBufferWriter{obs: obs}

	r.observeQueueDepth(7)
	r.observeAppendDropped(errForwardTest)
	r.observeWriteError("fsync", errForwardTest)
	r.observeFsyncLatency(42)

	require.Equal(t, []int{7}, obs.queueDepth)
	require.Len(t, obs.appendDropped, 1)
	require.Equal(t, []string{"fsync"}, obs.writeErrors)
	require.Len(t, obs.fsyncLatency, 1)
}
