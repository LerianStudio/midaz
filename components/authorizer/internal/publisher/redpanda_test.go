// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package publisher

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRedpandaPublisher_ValidatesBrokers(t *testing.T) {
	pub, err := NewRedpandaPublisherWithSecurity(nil, nil, Config{}, SecurityConfig{})
	assert.Nil(t, pub)
	require.Error(t, err)
}

func TestRedpandaPublisher_Publish_Validation(t *testing.T) {
	var nilPublisher *RedpandaPublisher

	err := nilPublisher.Publish(context.Background(), Message{Payload: []byte("payload"), Topic: "topic"})
	require.Error(t, err)

	publisher := &RedpandaPublisher{}

	err = publisher.Publish(context.Background(), Message{Topic: "topic"})
	require.Error(t, err)

	err = publisher.Publish(context.Background(), Message{Payload: []byte("payload")})
	require.Error(t, err)
}

func TestNormalizeConfig_Defaults(t *testing.T) {
	normalized := normalizeConfig(Config{})

	assert.Equal(t, defaultProducerLinger, normalized.ProducerLinger)
	assert.Equal(t, defaultMaxBufferedRecords, normalized.MaxBufferedRecords)
	assert.Equal(t, defaultRecordDeliveryTimeout, normalized.RecordDeliveryTimeout)
	assert.Equal(t, defaultPublishTimeout, normalized.PublishTimeout)
	assert.Equal(t, BackpressurePolicyBoundedWait, normalized.BackpressurePolicy)
	assert.Equal(t, 0, normalized.RecordRetries)
}

func TestNormalizeConfig_FailFastPolicy(t *testing.T) {
	normalized := normalizeConfig(Config{
		ProducerLinger:        3 * time.Millisecond,
		MaxBufferedRecords:    2048,
		RecordRetries:         5,
		RecordDeliveryTimeout: 7 * time.Second,
		PublishTimeout:        9 * time.Second,
		BackpressurePolicy:    "fail_fast",
	})

	assert.Equal(t, 3*time.Millisecond, normalized.ProducerLinger)
	assert.Equal(t, 2048, normalized.MaxBufferedRecords)
	assert.Equal(t, 5, normalized.RecordRetries)
	assert.Equal(t, 7*time.Second, normalized.RecordDeliveryTimeout)
	assert.Equal(t, 9*time.Second, normalized.PublishTimeout)
	assert.Equal(t, BackpressurePolicyFailFast, normalized.BackpressurePolicy)
}

func TestNormalizeConfig_InvalidPolicyAndRetries(t *testing.T) {
	normalized := normalizeConfig(Config{
		BackpressurePolicy: "  INVALID  ",
		RecordRetries:      -10,
	})

	assert.Equal(t, BackpressurePolicyBoundedWait, normalized.BackpressurePolicy)
	assert.Equal(t, 0, normalized.RecordRetries)
}

func TestNewRedpandaPublisherWithSecurity_InvalidSecurityConfig(t *testing.T) {
	publisher, err := NewRedpandaPublisherWithSecurity(
		[]string{"127.0.0.1:9092"},
		nil,
		Config{},
		SecurityConfig{SASLEnabled: true},
	)

	assert.Nil(t, publisher)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid redpanda security configuration")
}

func TestRedpandaPublisher_NewPublishContextFailFast(t *testing.T) {
	p := &RedpandaPublisher{config: normalizeConfig(Config{PublishTimeout: 10 * time.Second, BackpressurePolicy: BackpressurePolicyFailFast})}

	ctx, cancel := p.newPublishContext(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	assert.True(t, ok)

	remaining := time.Until(deadline)
	assert.Greater(t, remaining, 0*time.Millisecond)
	assert.LessOrEqual(t, remaining, 1200*time.Millisecond)
}

func TestRedpandaPublisher_NewPublishContextRespectsExistingDeadline(t *testing.T) {
	p := &RedpandaPublisher{config: normalizeConfig(Config{PublishTimeout: 10 * time.Second, BackpressurePolicy: BackpressurePolicyBoundedWait})}

	baseCtx, baseCancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer baseCancel()

	ctx, cancel := p.newPublishContext(baseCtx)
	defer cancel()

	deadline, ok := ctx.Deadline()
	assert.True(t, ok)

	baseDeadline, baseHasDeadline := baseCtx.Deadline()
	assert.True(t, baseHasDeadline)
	assert.Equal(t, baseDeadline, deadline)
}

func TestRedpandaPublisher_NewPublishContextNilContextNoTimeout(t *testing.T) {
	p := &RedpandaPublisher{config: Config{PublishTimeout: 0, BackpressurePolicy: BackpressurePolicyBoundedWait}}

	ctx, cancel := p.newPublishContext(context.TODO())
	defer cancel()

	assert.NotNil(t, ctx)
	_, hasDeadline := ctx.Deadline()
	assert.False(t, hasDeadline)
}

func TestRedpandaPublisher_Publish_WrapsDeadlineExceeded(t *testing.T) {
	publisher, err := NewRedpandaPublisherWithSecurity(
		[]string{"127.0.0.1:1"},
		nil,
		Config{PublishTimeout: 50 * time.Millisecond},
		SecurityConfig{},
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, publisher.Close())
	})

	expiredCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	err = publisher.Publish(expiredCtx, Message{Topic: "ledger.balance.operations", PartitionKey: "k", Payload: []byte("payload")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRedpandaPublisher_Close_NilReceiver(t *testing.T) {
	var publisher *RedpandaPublisher
	assert.NoError(t, publisher.Close())
}
