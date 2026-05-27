// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

import (
	"context"
	"errors"
	"sync"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMockEmitter_ReturnsEmptyMock(t *testing.T) {
	m := NewMockEmitter()

	require.NotNil(t, m)
	assert.Empty(t, m.Events())
}

func TestMockEmitter_CapturesEmitRequest(t *testing.T) {
	m := NewMockEmitter()

	req := libStreaming.EmitRequest{
		DefinitionKey: "account.created",
		TenantID:      "tenant-1",
		Subject:       "account-123",
		Payload:       []byte(`{"id":"account-123"}`),
	}

	require.NoError(t, m.Emit(context.Background(), req))

	events := m.Events()
	require.Len(t, events, 1)
	assert.Equal(t, "account.created", events[0].DefinitionKey)
	assert.Equal(t, "tenant-1", events[0].TenantID)
	assert.Equal(t, "account-123", events[0].Subject)
}

func TestMockEmitter_EmitErrPropagates(t *testing.T) {
	m := NewMockEmitter()
	want := errors.New("simulated publish failure")
	m.EmitErr = want

	got := m.Emit(context.Background(), libStreaming.EmitRequest{
		DefinitionKey: "account.created",
		TenantID:      "tenant-1",
	})
	assert.ErrorIs(t, got, want)

	// Failure path must NOT capture the request.
	assert.Empty(t, m.Events())
}

func TestMockEmitter_SetErrorTogglesFailurePath(t *testing.T) {
	m := NewMockEmitter()

	require.NoError(t, m.Emit(context.Background(), libStreaming.EmitRequest{
		DefinitionKey: "account.created",
		TenantID:      "tenant-1",
	}))
	require.Len(t, m.Events(), 1)

	want := errors.New("publish failed")
	m.SetError(want)
	assert.ErrorIs(t, m.Emit(context.Background(), libStreaming.EmitRequest{
		DefinitionKey: "account.created",
		TenantID:      "tenant-1",
	}), want)

	m.SetError(nil)
	require.NoError(t, m.Emit(context.Background(), libStreaming.EmitRequest{
		DefinitionKey: "account.created",
		TenantID:      "tenant-1",
	}))
	assert.Len(t, m.Events(), 2)
}

func TestAssertEventEmitted_Matches(t *testing.T) {
	m := NewMockEmitter()

	require.NoError(t, m.Emit(context.Background(), libStreaming.EmitRequest{
		DefinitionKey: "account.created",
		TenantID:      "tenant-1",
	}))

	// Should NOT call t.Fatalf on the wrapping test — the helper passes.
	AssertEventEmitted(t, m, "account", "created")
}

// stubTB implements testing.TB for the no-match assertion test. It
// records the first Fatalf message and signals FailNow via a sentinel
// the caller can observe without actually terminating the goroutine.
type stubTB struct {
	testing.TB
	fatalMsg string
	failed   bool
}

func (s *stubTB) Helper()                                   {}
func (s *stubTB) Errorf(format string, args ...interface{}) { s.failed = true }
func (s *stubTB) Fatalf(format string, args ...interface{}) {
	s.failed = true
	s.fatalMsg = format
}
func (s *stubTB) FailNow()     { s.failed = true }
func (s *stubTB) Failed() bool { return s.failed }

// TestAssertEventEmitted_NoMatch verifies the no-match branch by routing
// the helper through a testing.TB stub. The stub captures Fatalf without
// terminating the outer goroutine, so the outer test does not get
// (incorrectly) marked as failed.
func TestAssertEventEmitted_NoMatch(t *testing.T) {
	m := NewMockEmitter()

	require.NoError(t, m.Emit(context.Background(), libStreaming.EmitRequest{
		DefinitionKey: "account.created",
		TenantID:      "tenant-1",
	}))

	stub := &stubTB{}
	assertEventEmittedTB(stub, m, "account", "deleted")

	assert.True(t, stub.failed, "stub must record the assertion failure")
	assert.Contains(t, stub.fatalMsg, "expected emitted event with definition key")
}

// TestMockEmitter_ConcurrentEmitsAreSafe drives Emit from many goroutines
// at once and confirms the captured slice contains the expected count
// with no race detector trips.
func TestMockEmitter_ConcurrentEmitsAreSafe(t *testing.T) {
	m := NewMockEmitter()

	const n = 100

	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = m.Emit(context.Background(), libStreaming.EmitRequest{
				DefinitionKey: "account.created",
				TenantID:      "tenant-1",
			})
		}()
	}

	wg.Wait()
	assert.Len(t, m.Events(), n)
}

func TestMockEmitter_CloseAndHealthyAreNoops(t *testing.T) {
	m := NewMockEmitter()

	assert.NoError(t, m.Close())
	assert.NoError(t, m.Healthy(context.Background()))
}

// TestMockEmitter_NilReceiverSafe documents that helper methods on a nil
// receiver do not panic — matches the upstream MockEmitter contract.
func TestMockEmitter_NilReceiverSafe(t *testing.T) {
	var m *MockEmitter

	assert.NotPanics(t, func() {
		_ = m.Emit(context.Background(), libStreaming.EmitRequest{
			DefinitionKey: "account.created",
		})
	})
	assert.Nil(t, m.Events())
	assert.NotPanics(t, func() {
		m.SetError(errors.New("ignored"))
	})
	assert.NoError(t, m.Close())
	assert.NoError(t, m.Healthy(context.Background()))
}
