// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redisgo "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
)

// bytesCallerSpan stands in for the use-case span that calls the thin Redis wrappers.
const bytesCallerSpan = "command.get_or_create_transaction_route_cache"

// removedThinWrapperSpans are the wrapper spans that no longer exist: each mirrored one
// Redis command the driver already traces.
var removedThinWrapperSpans = []string{
	"redis.get_bytes",
	"redis.set_bytes",
	"redis.add_message_to_queue",
	"redis.read_message_from_queue",
	"redis.read_recovery_message",
	"redis.remove_message_from_queue",
}

// miniredisRepository wires the repository to an in-memory Redis.
func miniredisRepository(t *testing.T) (*RedisConsumerRepository, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redisgo.NewClient(&redisgo.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}, server
}

// underCallerSpan runs fn under a recorded caller span and returns the recorder once the
// caller span has ended.
func underCallerSpan(t *testing.T, fn func(ctx context.Context)) *tracetest.SpanRecorder {
	t.Helper()

	ctx, recorder := spantest.RecordingContext()
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, bytesCallerSpan)
	fn(ctx)
	span.End()

	return recorder
}

// Scenario: Miss no cache de rota de transação / Miss no cache de pacote de fees. Both
// callers read through GetBytes; a miss returns redis.Nil and marks no span as an error.
func TestThinWrapperSpans_GetBytesMissIsNotASpanError(t *testing.T) {
	repository, _ := miniredisRepository(t)

	var err error

	recorder := underCallerSpan(t, func(ctx context.Context) {
		_, err = repository.GetBytes(ctx, "absent-key")
	})

	require.ErrorIs(t, err, redisgo.Nil, "the miss is still returned for the caller to treat as such")

	spantest.AssertNoSpan(t, recorder, removedThinWrapperSpans...)

	for _, s := range recorder.Ended() {
		assert.NotEqual(t, codes.Error, s.Status().Code, "span %q must not carry an error for a cache miss", s.Name())
	}

	assert.Empty(t, spantest.FindSpan(t, recorder, bytesCallerSpan).Events(), "a miss records no exception event")
}

// Scenario: Redis indisponível continua erro técnico (client unavailable).
func TestThinWrapperSpans_GetBytesClientUnavailableIsATechnicalErrorOnTheCaller(t *testing.T) {
	clientErr := errors.New("redis client unavailable")
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{err: clientErr}}

	var err error

	recorder := underCallerSpan(t, func(ctx context.Context) {
		_, err = repository.GetBytes(ctx, "any-key")
	})

	require.ErrorIs(t, err, clientErr)

	spantest.AssertNoSpan(t, recorder, removedThinWrapperSpans...)

	span := spantest.FindSpan(t, recorder, bytesCallerSpan)
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Contains(t, span.Status().Description, "Failed to get redis")
}

// Scenario: Redis indisponível continua erro técnico (server down while reading).
func TestThinWrapperSpans_GetBytesServerDownIsATechnicalErrorOnTheCaller(t *testing.T) {
	repository, server := miniredisRepository(t)
	server.Close()

	var err error

	recorder := underCallerSpan(t, func(ctx context.Context) {
		_, err = repository.GetBytes(ctx, "any-key")
	})

	require.Error(t, err)
	require.NotErrorIs(t, err, redisgo.Nil)

	span := spantest.FindSpan(t, recorder, bytesCallerSpan)
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Contains(t, span.Status().Description, "Failed to get bytes on redis")
}

// A hit and a write go through without any wrapper span.
func TestThinWrapperSpans_SetAndGetBytesOpenNoSpan(t *testing.T) {
	repository, _ := miniredisRepository(t)

	var (
		got    []byte
		setErr error
		getErr error
	)

	recorder := underCallerSpan(t, func(ctx context.Context) {
		setErr = repository.SetBytes(ctx, "present-key", []byte("value"), 60)
		got, getErr = repository.GetBytes(ctx, "present-key")
	})

	require.NoError(t, setErr)
	require.NoError(t, getErr)
	assert.Equal(t, []byte("value"), got)

	assert.Equal(t, []string{bytesCallerSpan}, spantest.SpanNames(recorder))
	assert.Equal(t, codes.Unset, spantest.FindSpan(t, recorder, bytesCallerSpan).Status().Code)
}

// A failed write is a technical error on the caller's span.
func TestThinWrapperSpans_SetBytesClientUnavailableIsATechnicalErrorOnTheCaller(t *testing.T) {
	clientErr := errors.New("redis client unavailable")
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{err: clientErr}}

	var err error

	recorder := underCallerSpan(t, func(ctx context.Context) {
		err = repository.SetBytes(ctx, "any-key", []byte("value"), 60)
	})

	require.ErrorIs(t, err, clientErr)

	spantest.AssertNoSpan(t, recorder, removedThinWrapperSpans...)
	assert.Equal(t, codes.Error, spantest.FindSpan(t, recorder, bytesCallerSpan).Status().Code)
}

// The backup-queue helpers and the recovery read open no span of their own.
func TestThinWrapperSpans_QueueHelpersOpenNoSpan(t *testing.T) {
	repository, _ := miniredisRepository(t)

	field := uuid.NewString() + ":" + uuid.NewString()

	recorder := underCallerSpan(t, func(ctx context.Context) {
		require.NoError(t, repository.AddMessageToQueue(ctx, field, []byte("payload")))

		data, err := repository.ReadMessageFromQueue(ctx, field)
		require.NoError(t, err)
		assert.Equal(t, []byte("payload"), data)

		raw, err := repository.ReadRecoveryMessage(ctx, RecoveryQueueSourceLegacyBackup, field)
		require.NoError(t, err)
		assert.Equal(t, "payload", raw)

		require.NoError(t, repository.RemoveMessageFromQueue(ctx, field))

		// A field gone from the hash is an idempotent empty read, not an error.
		raw, err = repository.ReadRecoveryMessage(ctx, RecoveryQueueSourceLegacyBackup, field)
		require.NoError(t, err)
		assert.Empty(t, raw)
	})

	assert.Equal(t, []string{bytesCallerSpan}, spantest.SpanNames(recorder))
	assert.Equal(t, codes.Unset, spantest.FindSpan(t, recorder, bytesCallerSpan).Status().Code)
}

const listBalanceByKeySpan = "redis.list_balance_by_key"

// A balance missing from the cache is a miss: redis.Nil is returned unchanged and the
// adapter's own span keeps an unset status.
func TestListBalanceByKeySpans_MissIsNotASpanError(t *testing.T) {
	repository, _ := miniredisRepository(t)

	ctx, recorder := spantest.RecordingContext()

	balance, err := repository.ListBalanceByKey(ctx, uuid.New(), uuid.New(), "@absent#default")
	require.ErrorIs(t, err, redisgo.Nil)
	assert.Nil(t, balance)

	span := spantest.FindSpan(t, recorder, listBalanceByKeySpan)
	assert.Equal(t, codes.Unset, span.Status().Code)
	assert.Empty(t, span.Events(), "a miss records no exception event")
}

// A real Redis failure still marks the adapter's span as a technical error.
func TestListBalanceByKeySpans_ServerDownIsATechnicalError(t *testing.T) {
	repository, server := miniredisRepository(t)
	server.Close()

	ctx, recorder := spantest.RecordingContext()

	_, err := repository.ListBalanceByKey(ctx, uuid.New(), uuid.New(), "@any#default")
	require.Error(t, err)
	require.NotErrorIs(t, err, redisgo.Nil)

	span := spantest.FindSpan(t, recorder, listBalanceByKeySpan)
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Contains(t, span.Status().Description, "Failed to get balance on redis")
}
