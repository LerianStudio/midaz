// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg"

	"github.com/LerianStudio/lib-observability/log"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// testAcknowledger is a test double for amqp.Acknowledger.
type testAcknowledger struct {
	acked   bool
	nacked  bool
	requeue bool
	ackErr  error
	nackErr error
}

func (f *testAcknowledger) Ack(_ uint64, _ bool) error {
	f.acked = true
	return f.ackErr
}

func (f *testAcknowledger) Nack(_ uint64, _ bool, requeue bool) error {
	f.nacked = true
	f.requeue = requeue

	return f.nackErr
}

func (f *testAcknowledger) Reject(_ uint64, requeue bool) error {
	f.requeue = requeue
	return nil
}

func buildDelivery(ack *testAcknowledger, headers amqp.Table) amqp.Delivery {
	return amqp.Delivery{
		Acknowledger: ack,
		Exchange:     "test-exchange",
		RoutingKey:   "test-routing-key",
		ContentType:  "application/json",
		Headers:      headers,
		Body:         []byte(`{"test":"data"}`),
	}
}

func testSpan() trace.Span {
	tracer := noop.NewTracerProvider().Tracer("test")
	_, span := tracer.Start(context.Background(), "test")

	return span
}

// recordingRepublish captures the headers passed to it and returns a configurable error.
type recordingRepublish struct {
	called      bool
	gotHeaders  amqp.Table
	returnErr   error
	nackOnError *testAcknowledger
}

func (r *recordingRepublish) fn(_ context.Context, _ int, _ string, message amqp.Delivery, headers amqp.Table, _ trace.Span) error {
	r.called = true
	r.gotHeaders = headers

	if r.returnErr != nil {
		// Mirror the contract: a republish hook nacks to DLQ itself on failure.
		NackToDLQ(context.Background(), log.NewNop(), 1, "test-queue", message)
	}

	return r.returnErr
}

func newEngine(maxRetries int, republish RepublishFunc) *RetryManager {
	return NewRetryManager(Config{
		Classifier: NewDefaultClassifier(),
		Backoff:    func(int) time.Duration { return 0 },
		Republish:  republish,
		MaxRetries: maxRetries,
		Logger:     log.NewNop(),
	})
}

func TestHandleFailure_CanceledContext_AbandonsRetryWithoutAckOrNack(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	message := buildDelivery(ack, amqp.Table{RetryCountHeader: 0})

	// Non-zero backoff so WaitContext would block if the context were live; a
	// cancelled context must short-circuit it.
	rm := NewRetryManager(Config{
		Classifier: NewDefaultClassifier(),
		Backoff:    func(int) time.Duration { return time.Hour },
		Republish:  func(context.Context, int, string, amqp.Delivery, amqp.Table, trace.Span) error { return nil },
		MaxRetries: 5,
		Logger:     log.NewNop(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		rm.HandleFailure(ctx, 0, "test-queue", message, errors.New("transient"), 0, testSpan())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("HandleFailure blocked on backoff despite cancelled context")
	}

	assert.False(t, ack.acked, "cancelled retry must not ack the delivery")
	assert.False(t, ack.nacked, "cancelled retry must leave the delivery unacked for redelivery")
}

func TestHandleFailure_NonRetryableError_SendsToDLQ(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	rp := &recordingRepublish{}
	rm := newEngine(5, rp.fn)
	message := buildDelivery(ack, nil)

	rm.HandleFailure(context.Background(), 1, "test-queue", message, pkg.ValidationError{Code: "VAL-001"}, 0, testSpan())

	assert.True(t, ack.nacked, "non-retryable error should nack to DLQ")
	assert.False(t, ack.requeue, "DLQ nack must not requeue")
	assert.False(t, rp.called, "republish must not be attempted for non-retryable error")
}

func TestHandleFailure_MaxRetriesExceeded_SendsToDLQ(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	rp := &recordingRepublish{}
	rm := newEngine(5, rp.fn)
	message := buildDelivery(ack, nil)

	rm.HandleFailure(context.Background(), 1, "test-queue", message, errors.New("timeout"), 5, testSpan())

	assert.True(t, ack.nacked, "exhausted retries should nack to DLQ")
	assert.False(t, ack.requeue)
	assert.False(t, rp.called, "republish must not be attempted once retries are exhausted")
}

func TestHandleFailure_RetryableError_RepublishesAndAcks(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	rp := &recordingRepublish{}
	rm := newEngine(5, rp.fn)
	message := buildDelivery(ack, amqp.Table{"traceparent": "tp-1"})

	rm.HandleFailure(context.Background(), 1, "test-queue", message, errors.New("transient"), 0, testSpan())

	assert.True(t, rp.called, "retryable error should be republished")
	assert.True(t, ack.acked, "message should be acked after successful republish")
	assert.False(t, ack.nacked)

	// Engine builds incremented retry headers + failure reason and hands them to the hook.
	assert.Equal(t, 1, rp.gotHeaders[RetryCountHeader])
	assert.Equal(t, "retryable_error", rp.gotHeaders[RetryFailureReasonHeader])
	assert.Equal(t, "tp-1", rp.gotHeaders["traceparent"], "original headers must survive the retry")
}

func TestHandleFailure_RepublishFails_DoesNotAck(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	rp := &recordingRepublish{returnErr: errors.New("publish failed")}
	rm := newEngine(5, rp.fn)
	message := buildDelivery(ack, nil)

	rm.HandleFailure(context.Background(), 1, "test-queue", message, errors.New("transient"), 0, testSpan())

	assert.True(t, rp.called)
	assert.False(t, ack.acked, "message must NOT be acked when republish fails")
	assert.True(t, ack.nacked, "republish hook owns the DLQ nack on its failure path")
}

func TestHandleFailure_AckFailsAfterRepublish_DoesNotPanic(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{ackErr: errors.New("ack failed")}
	rp := &recordingRepublish{}
	rm := newEngine(5, rp.fn)
	message := buildDelivery(ack, nil)

	require.NotPanics(t, func() {
		rm.HandleFailure(context.Background(), 1, "test-queue", message, errors.New("transient"), 0, testSpan())
	})

	assert.True(t, rp.called)
	assert.True(t, ack.acked, "ack was attempted")
}

func TestNackToDLQ(t *testing.T) {
	t.Parallel()

	t.Run("nacks without requeue", func(t *testing.T) {
		t.Parallel()

		ack := &testAcknowledger{}
		NackToDLQ(context.Background(), log.NewNop(), 1, "test-queue", buildDelivery(ack, nil))

		assert.True(t, ack.nacked)
		assert.False(t, ack.requeue, "DLQ nack must not requeue")
	})

	t.Run("nack error does not panic", func(t *testing.T) {
		t.Parallel()

		ack := &testAcknowledger{nackErr: errors.New("nack failed")}
		require.NotPanics(t, func() {
			NackToDLQ(context.Background(), log.NewNop(), 1, "test-queue", buildDelivery(ack, nil))
		})
	})

	t.Run("nil logger does not panic on nack error", func(t *testing.T) {
		t.Parallel()

		ack := &testAcknowledger{nackErr: errors.New("nack failed")}
		require.NotPanics(t, func() {
			NackToDLQ(context.Background(), nil, 1, "test-queue", buildDelivery(ack, nil))
		})
	})
}
