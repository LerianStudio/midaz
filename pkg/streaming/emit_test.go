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

func (e *deadlineCapturingEmitter) Emit(ctx context.Context, _ libStreaming.Event) error {
	e.deadline, e.ok = ctx.Deadline()

	return nil
}

func (e *deadlineCapturingEmitter) Close() error { return nil }

func (e *deadlineCapturingEmitter) Healthy(_ context.Context) error { return nil }

type blockingEmitter struct {
	mu  sync.Mutex
	err error
}

func (e *blockingEmitter) Emit(ctx context.Context, _ libStreaming.Event) error {
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

func TestEmitImportant_NilEmitterDoesNotCallBuilder(t *testing.T) {
	called := false

	EmitImportant(context.Background(), trace.SpanFromContext(context.Background()), emitTestLogger{}, nil, "lerian.midaz.test", "account.created",
		func(_, _ string) (libStreaming.Event, error) {
			called = true
			return libStreaming.Event{}, nil
		})

	assert.False(t, called, "nil emitter must skip building the event")
}

func TestEmitImportant_SuccessfulEmitUsesDefaultTenantAndSource(t *testing.T) {
	mockEmitter := libStreaming.NewMockEmitter()
	source := "lerian.midaz.ledger.test"

	EmitImportant(context.Background(), trace.SpanFromContext(context.Background()), emitTestLogger{}, mockEmitter, source, "account.created",
		func(tenantID, gotSource string) (libStreaming.Event, error) {
			return libStreaming.Event{
				TenantID:      tenantID,
				Source:        gotSource,
				ResourceType:  "account",
				EventType:     "created",
				SchemaVersion: "1.0.0",
				Subject:       "account-123",
				Timestamp:     time.Date(2026, time.May, 15, 12, 0, 0, 0, time.UTC),
				Payload:       []byte(`{"id":"account-123"}`),
			}, nil
		})

	events := mockEmitter.Events()
	require.Len(t, events, 1)
	assert.Equal(t, DefaultTenantID, events[0].TenantID)
	assert.Equal(t, source, events[0].Source)
}

func TestEmitImportant_BuildErrorDoesNotEmit(t *testing.T) {
	mockEmitter := libStreaming.NewMockEmitter()
	buildErr := errors.New("build failed")

	EmitImportant(context.Background(), trace.SpanFromContext(context.Background()), emitTestLogger{}, mockEmitter, "lerian.midaz.test", "account.created",
		func(_, _ string) (libStreaming.Event, error) {
			return libStreaming.Event{}, buildErr
		})

	assert.Empty(t, mockEmitter.Events())
}

func TestEmitImportant_EmitErrorDoesNotPanic(t *testing.T) {
	mockEmitter := libStreaming.NewMockEmitter()
	mockEmitter.SetError(errors.New("emit failed"))

	require.NotPanics(t, func() {
		EmitImportant(context.Background(), trace.SpanFromContext(context.Background()), emitTestLogger{}, mockEmitter, "lerian.midaz.test", "account.created",
			func(tenantID, source string) (libStreaming.Event, error) {
				return libStreaming.Event{
					TenantID:      tenantID,
					Source:        source,
					ResourceType:  "account",
					EventType:     "created",
					SchemaVersion: "1.0.0",
					Subject:       "account-123",
					Timestamp:     time.Date(2026, time.May, 15, 12, 0, 0, 0, time.UTC),
					Payload:       []byte(`{"id":"account-123"}`),
				}, nil
			})
	})
	assert.Empty(t, mockEmitter.Events())
}

func TestEmitImportant_PassesBoundedDeadlineToEmitter(t *testing.T) {
	t.Setenv(importantEmitTimeoutEnv, "25")

	emitter := &deadlineCapturingEmitter{}
	startedAt := time.Now()

	EmitImportant(context.Background(), trace.SpanFromContext(context.Background()), emitTestLogger{}, emitter, "lerian.midaz.test", "account.created",
		func(tenantID, source string) (libStreaming.Event, error) {
			return libStreaming.Event{
				TenantID:      tenantID,
				Source:        source,
				ResourceType:  "account",
				EventType:     "created",
				SchemaVersion: "1.0.0",
				Subject:       "account-123",
				Timestamp:     time.Date(2026, time.May, 15, 12, 0, 0, 0, time.UTC),
				Payload:       []byte(`{"id":"account-123"}`),
			}, nil
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
		EmitImportant(context.Background(), trace.SpanFromContext(context.Background()), emitTestLogger{}, emitter, "lerian.midaz.test", "account.created",
			func(tenantID, source string) (libStreaming.Event, error) {
				return libStreaming.Event{
					TenantID:      tenantID,
					Source:        source,
					ResourceType:  "account",
					EventType:     "created",
					SchemaVersion: "1.0.0",
					Subject:       "account-123",
					Timestamp:     time.Date(2026, time.May, 15, 12, 0, 0, 0, time.UTC),
					Payload:       []byte(`{"id":"account-123"}`),
				}, nil
			})
	})

	assert.Less(t, time.Since(startedAt), 500*time.Millisecond)
	assert.ErrorIs(t, emitter.lastErr(), context.DeadlineExceeded)
}
