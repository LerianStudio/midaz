// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStreamingProducerRunnable_drain_ClosesOnce verifies that calling drain()
// multiple times invokes the underlying close hook exactly once (sync.Once).
func TestStreamingProducerRunnable_drain_ClosesOnce(t *testing.T) {
	var calls int32

	r := &streamingProducerRunnable{
		close: func() error {
			atomic.AddInt32(&calls, 1)
			return nil
		},
	}

	r.drain()
	r.drain()

	require.Equal(t, int32(1), atomic.LoadInt32(&calls), "close hook must run exactly once across repeated drain() calls")
}

// TestStreamingProducerRunnable_drain_NilCloseNoPanic verifies drain() is
// nil-safe: a runnable with a nil close hook must not panic.
func TestStreamingProducerRunnable_drain_NilCloseNoPanic(t *testing.T) {
	r := &streamingProducerRunnable{close: nil}

	require.NotPanics(t, func() {
		r.drain()
		r.drain()
	})
}

// TestShouldRegisterStreamingProducer locks the registration predicate: the
// producer drain runnable is registered only when streaming is enabled AND the
// close hook is non-nil.
func TestShouldRegisterStreamingProducer(t *testing.T) {
	nonNilClose := func() error { return nil }

	tests := []struct {
		name    string
		enabled bool
		close   func() error
		want    bool
	}{
		{name: "enabled and non-nil close registers", enabled: true, close: nonNilClose, want: true},
		{name: "disabled registers nothing", enabled: false, close: nonNilClose, want: false},
		{name: "enabled but nil close registers nothing", enabled: true, close: nil, want: false},
		{name: "disabled and nil close registers nothing", enabled: false, close: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRegisterStreamingProducer(tt.enabled, tt.close)
			require.Equal(t, tt.want, got)
		})
	}
}
