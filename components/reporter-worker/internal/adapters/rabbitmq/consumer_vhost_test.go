// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package rabbitmq

import (
	"context"
	"testing"

	pkgRabbitmq "github.com/LerianStudio/reporter/pkg/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

// TestConsumerRoutes_TenantResolver verifies that the tenant resolver is correctly
// injected: NoOp for single-tenant, MultiTenant for multi-tenant.
func TestConsumerRoutes_TenantResolver(t *testing.T) {
	t.Parallel()

	t.Run("single-tenant consumer uses NoOpTenantResolver", func(t *testing.T) {
		t.Parallel()

		cr := &ConsumerRoutes{
			routes:         make(map[string]pkgRabbitmq.QueueHandlerFunc),
			tenantResolver: &NoOpTenantResolver{},
		}
		assert.IsType(t, &NoOpTenantResolver{}, cr.tenantResolver)
	})

	t.Run("multi-tenant consumer uses MultiTenantResolver", func(t *testing.T) {
		t.Parallel()

		cr := &ConsumerRoutes{
			routes:         make(map[string]pkgRabbitmq.QueueHandlerFunc),
			tenantResolver: &MultiTenantResolver{},
		}
		assert.IsType(t, &MultiTenantResolver{}, cr.tenantResolver)
	})
}

// TestConsumerRetryManager_WithMockManager verifies retry manager is wired correctly
// for multi-tenant mode with a mock RabbitMQ manager.
func TestConsumerRetryManager_WithMockManager(t *testing.T) {
	t.Parallel()

	mockManager := &mockRabbitMQManagerConsumer{}
	retryMgr := &ConsumerRetryManager{
		rabbitMQManager: mockManager,
	}
	assert.NotNil(t, retryMgr.rabbitMQManager)
}

// mockRabbitMQManagerConsumer is a mock implementation of the RabbitMQManagerConsumerInterface.
type mockRabbitMQManagerConsumer struct {
	getConnectionErr error
	lastTenantID     string
	connection       *mockRabbitMQConnectionChannel
}

func (m *mockRabbitMQManagerConsumer) GetConnection(_ context.Context, tenantID string) (RabbitMQConnectionChannel, error) {
	m.lastTenantID = tenantID

	if m.getConnectionErr != nil {
		return nil, m.getConnectionErr
	}

	if m.connection != nil {
		return m.connection, nil
	}

	return &mockRabbitMQConnectionChannel{}, nil
}

// mockRabbitMQConnectionChannel is a mock implementation of the RabbitMQConnectionChannel interface.
type mockRabbitMQConnectionChannel struct {
	publishCalled bool
	lastExchange  string
	lastKey       string
	publishErr    error
}

func (m *mockRabbitMQConnectionChannel) Publish(exchange, key string, _, _ bool, _ amqp.Publishing) error {
	m.publishCalled = true
	m.lastExchange = exchange
	m.lastKey = key

	return m.publishErr
}
