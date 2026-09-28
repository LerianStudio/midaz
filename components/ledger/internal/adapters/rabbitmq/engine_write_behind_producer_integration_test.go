//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationEngineWriteBehindProducerSingleTenant(t *testing.T) {
	if testing.Short() {
		t.Skip("requires RabbitMQ")
	}
	infra := setupIntegrationInfra(t)
	message := EngineWriteBehindMessage{
		TransactionID: uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		ExecutionID:   uuid.MustParse("22222222-2222-4222-8222-222222222222"), Body: []byte(`{"formatVersion":1}`),
	}

	unroutable, err := NewSingleTenantEngineWriteBehindProducer(infra.conn, infra.exchange, "missing.route", time.Second)
	require.NoError(t, err)
	require.ErrorIs(t, unroutable.Publish(context.Background(), message), ErrEngineWriteBehindPublishUnroutable)

	producer, err := NewSingleTenantEngineWriteBehindProducer(infra.conn, infra.exchange, infra.routingKey, time.Second)
	require.NoError(t, err)
	require.NoError(t, producer.Publish(context.Background(), message), "a fresh channel must recover after an uncertain/unroutable attempt")

	delivery, ok, err := infra.rmqContainer.Channel.Get(infra.queue, true)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, message.Body, delivery.Body)
	assert.Equal(t, message.MessageID(), delivery.MessageId)
	assert.Equal(t, message.ExecutionID.String(), delivery.CorrelationId)
	assert.Empty(t, delivery.Headers[headerTenantID])
	assert.Equal(t, uint8(amqp.Persistent), delivery.DeliveryMode)
}

func TestIntegrationEngineWriteBehindProducerMultiTenant(t *testing.T) {
	if testing.Short() {
		t.Skip("requires RabbitMQ")
	}
	const tenantID = "tenant-a"
	infra := setupMultiTenantInfra(t, []string{tenantID})
	producer, err := NewMultiTenantEngineWriteBehindProducer(infra.producer.channelProvider, infra.exchange, infra.routingKey, time.Second)
	require.NoError(t, err)
	message := EngineWriteBehindMessage{
		TenantID: tenantID, TransactionID: uuid.MustParse("33333333-3333-4333-8333-333333333333"),
		ExecutionID: uuid.MustParse("44444444-4444-4444-8444-444444444444"), Body: []byte(`{"formatVersion":1}`),
	}
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)

	require.NoError(t, producer.Publish(ctx, message))

	delivery, ok, err := infra.tenants[tenantID].channel.Get(infra.queue, true)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, message.MessageID(), delivery.MessageId)
	assert.Equal(t, tenantID, delivery.Headers[headerTenantID])
	assert.Equal(t, uint8(amqp.Persistent), delivery.DeliveryMode)
}
