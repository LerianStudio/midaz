// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

import (
	"context"
	"sync"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
)

// MockEmitter is a midaz-local test double for libStreaming.Emitter that
// captures every Emit call for later assertion. Safe for concurrent use.
//
// The mock is intentionally midaz-local so command-package unit tests stay
// decoupled from upstream lib-streaming test helpers.
type MockEmitter struct {
	mu     sync.Mutex
	events []libStreaming.EmitRequest

	// EmitErr, when non-nil, is returned from every Emit call. The
	// request is NOT captured on this path — a publish-failure path
	// captures nothing for later inspection.
	EmitErr error
}

// Compile-time assertion: *MockEmitter must satisfy libStreaming.Emitter.
// A renamed or missing method fails the build here rather than at the
// test site that fails to type-assert.
var _ libStreaming.Emitter = (*MockEmitter)(nil)

// NewMockEmitter returns a fresh MockEmitter with an empty event buffer
// and no injected error.
func NewMockEmitter() *MockEmitter {
	return &MockEmitter{}
}

// Emit captures the request and returns m.EmitErr (nil by default). When
// EmitErr is set the request is NOT recorded.
func (m *MockEmitter) Emit(_ context.Context, request libStreaming.EmitRequest) error {
	if m == nil {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.EmitErr != nil {
		return m.EmitErr
	}

	m.events = append(m.events, request)

	return nil
}

// Close is idempotent and always returns nil.
func (m *MockEmitter) Close() error { return nil }

// Healthy always returns nil — the mock is always "healthy".
func (m *MockEmitter) Healthy(_ context.Context) error { return nil }

// Events returns a copy of every captured emit request in arrival order.
// The returned slice is independent of the mock's internal state; callers
// may mutate it without affecting subsequent calls.
func (m *MockEmitter) Events() []libStreaming.EmitRequest {
	if m == nil {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]libStreaming.EmitRequest, len(m.events))
	copy(out, m.events)

	return out
}

// SetError makes subsequent Emit calls return err without capturing the
// request. Pass nil to restore the happy path. Provided alongside the
// public EmitErr field so test code can use whichever style feels more
// natural at the call site.
func (m *MockEmitter) SetError(err error) {
	if m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.EmitErr = err
}

// AssertEventEmitted fails the test if no captured event has a
// DefinitionKey of "<resourceType>.<eventType>". Example call shape:
//
//	pkgStreaming.AssertEventEmitted(t, mockEmitter, "account", "created")
func AssertEventEmitted(t *testing.T, m *MockEmitter, resourceType, eventType string) {
	t.Helper()
	assertEventEmittedTB(t, m, resourceType, eventType)
}

// assertEventEmittedTB is the testing.TB-typed inner helper so unit
// tests can substitute a stub for the no-match path without driving a
// real subtest. Unexported on purpose — the public API takes *testing.T
// for symmetry with the rest of midaz's test helpers.
func assertEventEmittedTB(tb testing.TB, m *MockEmitter, resourceType, eventType string) {
	tb.Helper()

	key := resourceType + "." + eventType

	for _, evt := range m.Events() {
		if evt.DefinitionKey == key {
			return
		}
	}

	tb.Fatalf("expected emitted event with definition key %q, got %d events: %v",
		key, len(m.Events()), m.Events())
}
