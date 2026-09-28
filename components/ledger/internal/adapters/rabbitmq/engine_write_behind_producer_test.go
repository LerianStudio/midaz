// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type engineProducerChannelStub struct {
	mu            sync.Mutex
	confirmations chan amqp.Confirmation
	returns       chan amqp.Return
	closures      chan *amqp.Error
	published     []amqp.Publishing
	mandatory     []bool
	confirmCalls  int
	closeCalls    int
	confirmErr    error
	publishErr    error
	onPublish     func(*engineProducerChannelStub)
}

func (channel *engineProducerChannelStub) PublishWithContext(_ context.Context, _ string, _ string, mandatory, _ bool, message amqp.Publishing) error {
	channel.mu.Lock()
	channel.mandatory = append(channel.mandatory, mandatory)
	channel.published = append(channel.published, message)
	callback := channel.onPublish
	channel.mu.Unlock()
	if channel.publishErr != nil {
		return channel.publishErr
	}
	if callback != nil {
		callback(channel)
	}
	return nil
}

func (channel *engineProducerChannelStub) Confirm(bool) error {
	channel.confirmCalls++
	return channel.confirmErr
}

func (channel *engineProducerChannelStub) NotifyPublish(confirmations chan amqp.Confirmation) chan amqp.Confirmation {
	channel.confirmations = confirmations
	return confirmations
}

func (channel *engineProducerChannelStub) NotifyReturn(returns chan amqp.Return) chan amqp.Return {
	channel.returns = returns
	return returns
}

func (channel *engineProducerChannelStub) NotifyClose(closures chan *amqp.Error) chan *amqp.Error {
	channel.closures = closures
	return closures
}

func (channel *engineProducerChannelStub) Close() error {
	channel.closeCalls++
	return nil
}

type engineProducerProviderStub struct {
	channel      *engineProducerChannelStub
	tenantID     string
	acquireCalls int
	releaseState []bool
	acquireErr   error
}

func (provider *engineProducerProviderStub) acquire(_ context.Context, tenantID string) (engineWriteBehindChannelLease, error) {
	provider.acquireCalls++
	provider.tenantID = tenantID
	if provider.acquireErr != nil {
		return engineWriteBehindChannelLease{}, provider.acquireErr
	}

	return engineWriteBehindChannelLease{
		channel:  provider.channel,
		identity: "stub-channel",
		release: func(success bool) {
			provider.releaseState = append(provider.releaseState, success)
		},
	}, nil
}

func engineProducerMessage() EngineWriteBehindMessage {
	return EngineWriteBehindMessage{
		TenantID: "tenant-a", TransactionID: uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		ExecutionID: uuid.MustParse("22222222-2222-4222-8222-222222222222"), Body: []byte(`{"formatVersion":1}`),
	}
}

func TestEngineWriteBehindProducerRequiresAckAndPublishesPersistentIdentity(t *testing.T) {
	channel := &engineProducerChannelStub{onPublish: func(channel *engineProducerChannelStub) {
		channel.confirmations <- amqp.Confirmation{Ack: true}
	}}
	provider := &engineProducerProviderStub{channel: channel}
	producer, err := newEngineWriteBehindProducer(provider, "exchange", "routing", time.Second, engineWriteBehindMultiTenant)
	require.NoError(t, err)
	message := engineProducerMessage()
	ctx := tmcore.ContextWithTenantID(context.Background(), message.TenantID)

	require.NoError(t, producer.Publish(ctx, message))

	require.Len(t, channel.published, 1)
	published := channel.published[0]
	assert.Equal(t, amqp.Persistent, published.DeliveryMode)
	assert.Equal(t, "application/json", published.ContentType)
	assert.Equal(t, message.MessageID(), published.MessageId)
	assert.Equal(t, message.ExecutionID.String(), published.CorrelationId)
	assert.Equal(t, message.TenantID, published.Headers[headerTenantID])
	assert.Equal(t, message.Body, published.Body)
	assert.Equal(t, []bool{true}, channel.mandatory)
	assert.Equal(t, []bool{true}, provider.releaseState)
	assert.Equal(t, 1, channel.confirmCalls)
}

func TestEngineWriteBehindProducerRejectsNegativeAndUncertainConfirmations(t *testing.T) {
	tests := []struct {
		name      string
		onPublish func(*engineProducerChannelStub)
		want      error
	}{
		{name: "nack", onPublish: func(channel *engineProducerChannelStub) {
			channel.confirmations <- amqp.Confirmation{Ack: false}
		}, want: ErrEngineWriteBehindPublishNacked},
		{name: "unroutable", onPublish: func(channel *engineProducerChannelStub) {
			channel.returns <- amqp.Return{ReplyCode: 312, ReplyText: "NO_ROUTE"}
			channel.confirmations <- amqp.Confirmation{Ack: true}
		}, want: ErrEngineWriteBehindPublishUnroutable},
		{name: "channel closed", onPublish: func(channel *engineProducerChannelStub) {
			channel.closures <- &amqp.Error{Code: 504, Reason: "channel closed"}
		}, want: ErrEngineWriteBehindPublishUnconfirmed},
		{name: "confirm lost", want: ErrEngineWriteBehindPublishUnconfirmed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &engineProducerChannelStub{onPublish: test.onPublish}
			provider := &engineProducerProviderStub{channel: channel}
			producer, err := newEngineWriteBehindProducer(provider, "exchange", "routing", 5*time.Millisecond, engineWriteBehindMultiTenant)
			require.NoError(t, err)
			message := engineProducerMessage()
			ctx := tmcore.ContextWithTenantID(context.Background(), message.TenantID)

			err = producer.Publish(ctx, message)

			require.ErrorIs(t, err, test.want)
			assert.Equal(t, []bool{false}, provider.releaseState)
		})
	}
}

func TestEngineWriteBehindProducerPropagatesPublishAndConfirmSetupFailures(t *testing.T) {
	failure := errors.New("connection closed")
	for _, scenario := range []struct {
		name       string
		confirmErr error
		publishErr error
	}{
		{name: "confirm setup", confirmErr: failure},
		{name: "publish", publishErr: failure},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			channel := &engineProducerChannelStub{confirmErr: scenario.confirmErr, publishErr: scenario.publishErr}
			provider := &engineProducerProviderStub{channel: channel}
			producer, err := newEngineWriteBehindProducer(provider, "exchange", "routing", time.Second, engineWriteBehindMultiTenant)
			require.NoError(t, err)
			message := engineProducerMessage()
			ctx := tmcore.ContextWithTenantID(context.Background(), message.TenantID)

			err = producer.Publish(ctx, message)

			require.ErrorIs(t, err, failure)
			assert.Equal(t, []bool{false}, provider.releaseState)
		})
	}
}

func TestEngineWriteBehindProducerEnforcesTenantPolicyBeforeChannelAccess(t *testing.T) {
	validMessage := engineProducerMessage()

	tests := []struct {
		name    string
		policy  engineWriteBehindTenantPolicy
		ctx     context.Context
		message EngineWriteBehindMessage
		wantErr error
	}{
		{
			name: "single tenant accepts empty scope", policy: engineWriteBehindSingleTenant,
			ctx: context.Background(), message: func() EngineWriteBehindMessage {
				message := validMessage
				message.TenantID = ""
				return message
			}(),
		},
		{
			name: "single tenant rejects tenant in message", policy: engineWriteBehindSingleTenant,
			ctx: context.Background(), message: validMessage, wantErr: ErrEngineWriteBehindTenantUnexpected,
		},
		{
			name: "single tenant rejects tenant in context", policy: engineWriteBehindSingleTenant,
			ctx: tmcore.ContextWithTenantID(context.Background(), "tenant-a"), message: func() EngineWriteBehindMessage {
				message := validMessage
				message.TenantID = ""
				return message
			}(), wantErr: ErrEngineWriteBehindTenantUnexpected,
		},
		{
			name: "multi tenant requires trusted context", policy: engineWriteBehindMultiTenant,
			ctx: context.Background(), message: validMessage, wantErr: ErrEngineWriteBehindTenantRequired,
		},
		{
			name: "multi tenant requires message tenant", policy: engineWriteBehindMultiTenant,
			ctx: tmcore.ContextWithTenantID(context.Background(), "tenant-a"), message: func() EngineWriteBehindMessage {
				message := validMessage
				message.TenantID = ""
				return message
			}(), wantErr: ErrEngineWriteBehindTenantRequired,
		},
		{
			name: "multi tenant rejects mismatch", policy: engineWriteBehindMultiTenant,
			ctx: tmcore.ContextWithTenantID(context.Background(), "tenant-b"), message: validMessage,
			wantErr: ErrEngineWriteBehindTenantMismatch,
		},
		{
			name: "multi tenant accepts matching scope", policy: engineWriteBehindMultiTenant,
			ctx: tmcore.ContextWithTenantID(context.Background(), "tenant-a"), message: validMessage,
		},
		{
			name: "invalid identity rejected", policy: engineWriteBehindSingleTenant,
			ctx: context.Background(), message: EngineWriteBehindMessage{Body: []byte(`{"formatVersion":1}`)},
			wantErr: ErrEngineWriteBehindInvalidMessage,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &engineProducerChannelStub{onPublish: func(channel *engineProducerChannelStub) {
				channel.confirmations <- amqp.Confirmation{Ack: true}
			}}
			provider := &engineProducerProviderStub{channel: channel}
			producer, err := newEngineWriteBehindProducer(provider, "exchange", "routing", time.Second, test.policy)
			require.NoError(t, err)

			err = producer.Publish(test.ctx, test.message)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				assert.Zero(t, provider.acquireCalls)
				assert.Empty(t, channel.published)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, 1, provider.acquireCalls)
			assert.Equal(t, test.message.TenantID, provider.tenantID)
		})
	}
}

func TestEngineWriteBehindProducerTelemetryClassifiesLocalAndBrokerFailures(t *testing.T) {
	transportFailure := errors.New("transport unavailable")
	testCases := []struct {
		name              string
		message           EngineWriteBehindMessage
		contextTenant     string
		providerError     error
		channel           *engineProducerChannelStub
		wantError         error
		wantCause         string
		wantBrokerAttempt bool
	}{
		{
			name: "local validation", message: EngineWriteBehindMessage{Body: []byte(`sensitive-payload`)},
			wantError: ErrEngineWriteBehindInvalidMessage, wantCause: engineWriteBehindFailureInvalidMessage,
		},
		{
			name: "channel acquisition", message: engineProducerMessage(), contextTenant: "tenant-a",
			providerError: transportFailure, wantError: transportFailure,
			wantCause: engineWriteBehindFailureChannelAcquire, wantBrokerAttempt: true,
		},
		{
			name: "publish", message: engineProducerMessage(), contextTenant: "tenant-a",
			channel: &engineProducerChannelStub{publishErr: transportFailure}, wantError: transportFailure,
			wantCause: engineWriteBehindFailurePublish, wantBrokerAttempt: true,
		},
		{
			name: "nack", message: engineProducerMessage(), contextTenant: "tenant-a",
			channel: &engineProducerChannelStub{onPublish: func(channel *engineProducerChannelStub) {
				channel.confirmations <- amqp.Confirmation{Ack: false}
			}}, wantError: ErrEngineWriteBehindPublishNacked,
			wantCause: engineWriteBehindFailureNack, wantBrokerAttempt: true,
		},
		{
			name: "uncertain confirmation", message: engineProducerMessage(), contextTenant: "tenant-a",
			channel: &engineProducerChannelStub{}, wantError: ErrEngineWriteBehindPublishUnconfirmed,
			wantCause: engineWriteBehindFailureConfirmationUncertain, wantBrokerAttempt: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, recorder := engineProducerRecordingContext()
			if testCase.contextTenant != "" {
				ctx = tmcore.ContextWithTenantID(ctx, testCase.contextTenant)
			}
			channel := testCase.channel
			if channel == nil {
				channel = &engineProducerChannelStub{}
			}
			provider := &engineProducerProviderStub{channel: channel, acquireErr: testCase.providerError}
			producer, err := newEngineWriteBehindProducer(provider, "exchange", "routing", 5*time.Millisecond, engineWriteBehindMultiTenant)
			require.NoError(t, err)

			err = producer.Publish(ctx, testCase.message)

			require.ErrorIs(t, err, testCase.wantError)
			span := engineProducerEndedSpan(t, recorder)
			attributes := engineProducerSpanAttributes(span)
			assert.Equal(t, "failed", attributes["app.response.outcome"].AsString())
			assert.Equal(t, testCase.wantCause, attributes["app.response.failure_cause"].AsString())
			assert.Equal(t, testCase.wantBrokerAttempt, attributes["app.response.broker_io_attempted"].AsBool())
			assert.NotContains(t, attributes, "transaction_id")
			assert.NotContains(t, attributes, "execution_id")
			for _, value := range attributes {
				assert.NotContains(t, value.Emit(), string(testCase.message.Body))
				assert.NotContains(t, value.Emit(), testCase.message.TransactionID.String())
				assert.NotContains(t, value.Emit(), testCase.message.ExecutionID.String())
			}
			if !testCase.wantBrokerAttempt {
				assert.Zero(t, provider.acquireCalls)
			}
		})
	}
}

func TestEngineWriteBehindProducerTelemetryRecordsConfirmedPublish(t *testing.T) {
	ctx, recorder := engineProducerRecordingContext()
	message := engineProducerMessage()
	ctx = tmcore.ContextWithTenantID(ctx, message.TenantID)
	channel := &engineProducerChannelStub{onPublish: func(channel *engineProducerChannelStub) {
		channel.confirmations <- amqp.Confirmation{Ack: true}
	}}
	producer, err := newEngineWriteBehindProducer(&engineProducerProviderStub{channel: channel}, "exchange", "routing", time.Second, engineWriteBehindMultiTenant)
	require.NoError(t, err)

	require.NoError(t, producer.Publish(ctx, message))

	attributes := engineProducerSpanAttributes(engineProducerEndedSpan(t, recorder))
	assert.Equal(t, "confirmed", attributes["app.response.outcome"].AsString())
	assert.True(t, attributes["app.response.broker_io_attempted"].AsBool())
	assert.NotContains(t, attributes, "app.response.failure_cause")
}

func engineProducerRecordingContext() (context.Context, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	ctx := libObservability.ContextWithTracer(context.Background(), provider.Tracer("engine-write-behind-producer-test"))

	return ctx, recorder
}

func engineProducerEndedSpan(t testing.TB, recorder *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range recorder.Ended() {
		if span.Name() == "rabbitmq.engine_write_behind.publish" {
			return span
		}
	}

	t.Fatal("engine write-behind publish span was not recorded")

	return nil
}

func engineProducerSpanAttributes(span sdktrace.ReadOnlySpan) map[string]attribute.Value {
	attributes := make(map[string]attribute.Value, len(span.Attributes()))
	for _, pair := range span.Attributes() {
		attributes[string(pair.Key)] = pair.Value
	}

	return attributes
}
