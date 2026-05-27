// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

type emitTestLogger struct{}

func (emitTestLogger) Log(context.Context, libLog.Level, string, ...libLog.Field) {}
func (emitTestLogger) With(...libLog.Field) libLog.Logger                         { return emitTestLogger{} }
func (emitTestLogger) WithGroup(string) libLog.Logger                             { return emitTestLogger{} }
func (emitTestLogger) Enabled(libLog.Level) bool                                  { return true }
func (emitTestLogger) Sync(context.Context) error                                 { return nil }

type deadlineCapturingEmitter struct {
	deadline time.Time
	ok       bool
}

func (e *deadlineCapturingEmitter) Emit(ctx context.Context, _ libStreaming.EmitRequest) error {
	e.deadline, e.ok = ctx.Deadline()

	return nil
}

func (e *deadlineCapturingEmitter) Close() error { return nil }

func (e *deadlineCapturingEmitter) Healthy(_ context.Context) error { return nil }

type blockingEmitter struct {
	mu  sync.Mutex
	err error
}

func (e *blockingEmitter) Emit(ctx context.Context, _ libStreaming.EmitRequest) error {
	<-ctx.Done()

	e.mu.Lock()
	defer e.mu.Unlock()

	e.err = ctx.Err()

	return ctx.Err()
}

func (e *blockingEmitter) Close() error { return nil }

func (e *blockingEmitter) Healthy(_ context.Context) error { return nil }

func (e *blockingEmitter) lastErr() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.err
}

// sampleEmitRequest returns a minimal valid EmitRequest. tenant/payload are
// realistic so log assertions stay legible.
func sampleEmitRequest(tenantID string) libStreaming.EmitRequest {
	return libStreaming.EmitRequest{
		DefinitionKey: "account.created",
		TenantID:      tenantID,
		Subject:       "account-123",
		Timestamp:     time.Date(2026, time.May, 15, 12, 0, 0, 0, time.UTC),
		Payload:       []byte(`{"id":"account-123"}`),
	}
}

func TestEmitImportant_NilEmitterDoesNotCallBuilder(t *testing.T) {
	called := false

	EmitImportant(context.Background(), trace.SpanFromContext(context.Background()), emitTestLogger{}, nil, "account.created",
		func(_ string) (libStreaming.EmitRequest, error) {
			called = true
			return libStreaming.EmitRequest{}, nil
		})

	assert.False(t, called, "nil emitter must skip building the event")
}

func TestEmitImportant_SuccessfulEmitUsesDefaultTenant(t *testing.T) {
	mockEmitter := NewMockEmitter()

	EmitImportant(context.Background(), trace.SpanFromContext(context.Background()), emitTestLogger{}, mockEmitter, "account.created",
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return sampleEmitRequest(tenantID), nil
		})

	events := mockEmitter.Events()
	require.Len(t, events, 1)
	assert.Equal(t, DefaultTenantID, events[0].TenantID)
	assert.Equal(t, "account.created", events[0].DefinitionKey)
}

func TestEmitImportant_BuildErrorDoesNotEmit(t *testing.T) {
	mockEmitter := NewMockEmitter()
	buildErr := errors.New("build failed")

	EmitImportant(context.Background(), trace.SpanFromContext(context.Background()), emitTestLogger{}, mockEmitter, "account.created",
		func(_ string) (libStreaming.EmitRequest, error) {
			return libStreaming.EmitRequest{}, buildErr
		})

	assert.Empty(t, mockEmitter.Events())
}

func TestEmitImportant_EmitErrorDoesNotPanic(t *testing.T) {
	mockEmitter := NewMockEmitter()
	mockEmitter.EmitErr = errors.New("emit failed")

	require.NotPanics(t, func() {
		EmitImportant(context.Background(), trace.SpanFromContext(context.Background()), emitTestLogger{}, mockEmitter, "account.created",
			func(tenantID string) (libStreaming.EmitRequest, error) {
				return sampleEmitRequest(tenantID), nil
			})
	})

	// EmitErr is captured BEFORE the request is appended, mirroring the
	// upstream MockEmitter contract: a publish-failure path captures
	// nothing for later inspection.
	assert.Empty(t, mockEmitter.Events())
}

func TestEmitImportant_PassesBoundedDeadlineToEmitter(t *testing.T) {
	t.Setenv(importantEmitTimeoutEnv, "25")

	emitter := &deadlineCapturingEmitter{}
	startedAt := time.Now()

	EmitImportant(context.Background(), trace.SpanFromContext(context.Background()), emitTestLogger{}, emitter, "account.created",
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return sampleEmitRequest(tenantID), nil
		})

	require.True(t, emitter.ok, "important emits must call emitter with a deadline")
	assert.LessOrEqual(t, emitter.deadline.Sub(startedAt), 100*time.Millisecond)
	assert.Greater(t, emitter.deadline.Sub(startedAt), 0*time.Millisecond)
}

func TestEmitImportant_BlockingEmitterReturnsAfterConfiguredTimeout(t *testing.T) {
	t.Setenv(importantEmitTimeoutEnv, "10")

	emitter := &blockingEmitter{}
	startedAt := time.Now()

	require.NotPanics(t, func() {
		EmitImportant(context.Background(), trace.SpanFromContext(context.Background()), emitTestLogger{}, emitter, "account.created",
			func(tenantID string) (libStreaming.EmitRequest, error) {
				return sampleEmitRequest(tenantID), nil
			})
	})

	assert.Less(t, time.Since(startedAt), 500*time.Millisecond)
	assert.ErrorIs(t, emitter.lastErr(), context.DeadlineExceeded)
}
