// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package rabbitmq

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/reporter/pkg/model"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProducerRabbitMQRepository_MultiTenantFields verifies that the multi-tenant
// fields (rabbitMQManager and multiTenantMode) are correctly initialized.
func TestProducerRabbitMQRepository_MultiTenantFields(t *testing.T) {
	t.Parallel()

	t.Run("single-tenant constructor sets multiTenantMode to false", func(t *testing.T) {
		t.Parallel()
		producer := newTestProducer()
		assert.False(t, producer.multiTenantMode, "multiTenantMode should be false for single-tenant producer")
		assert.Nil(t, producer.rabbitMQManager, "rabbitMQManager should be nil for single-tenant producer")
		assert.NotNil(t, producer.conn, "conn should be set for single-tenant producer")
	})

	t.Run("multi-tenant constructor sets multiTenantMode to true", func(t *testing.T) {
		t.Parallel()

		// Create a mock manager using an interface we control
		mockManager := &mockRabbitMQManager{}

		producer := NewProducerRabbitMQMultiTenant(mockManager)

		assert.True(t, producer.multiTenantMode, "multiTenantMode should be true for multi-tenant producer")
		assert.NotNil(t, producer.rabbitMQManager, "rabbitMQManager should be set for multi-tenant producer")
		assert.Nil(t, producer.conn, "conn should be nil for multi-tenant producer")
	})
}

// TestProducerDefault_MultiTenant_RequiresTenantID verifies that multi-tenant
// producer returns an error when no tenant ID is provided in the context.
func TestProducerDefault_MultiTenant_RequiresTenantID(t *testing.T) {
	t.Parallel()

	mockManager := &mockRabbitMQManager{}
	producer := NewProducerRabbitMQMultiTenant(mockManager)

	msg := model.ReportMessage{
		ReportID:     uuid.New(),
		TemplateID:   uuid.New(),
		OutputFormat: "pdf",
	}

	// Context without tenant ID
	ctx := context.Background()
	_, err := producer.ProducerDefault(ctx, "test-exchange", "test-key", msg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant ID is required in multi-tenant mode")
}

// TestProducerDefault_MultiTenant_UsesManagerGetChannel verifies that multi-tenant
// producer uses rabbitMQManager.GetChannel to get a tenant-specific channel.
func TestProducerDefault_MultiTenant_UsesManagerGetChannel(t *testing.T) {
	t.Parallel()

	mockManager := &mockRabbitMQManager{
		getChannelErr: errors.New("mocked: tenant vhost connection error"),
	}
	producer := NewProducerRabbitMQMultiTenant(mockManager)

	msg := model.ReportMessage{
		ReportID:     uuid.New(),
		TemplateID:   uuid.New(),
		OutputFormat: "pdf",
	}

	// Context with tenant ID
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-xyz")
	_, err := producer.ProducerDefault(ctx, "test-exchange", "test-key", msg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mocked: tenant vhost connection error")

	// Verify GetChannel was called with the correct tenant ID
	assert.Equal(t, "tenant-xyz", mockManager.lastTenantID, "GetChannel should be called with tenant ID from context")
}

// TestProducerDefault_MultiTenant_SuccessfulPublish verifies that multi-tenant
// producer successfully publishes messages to the tenant's vhost.
func TestProducerDefault_MultiTenant_SuccessfulPublish(t *testing.T) {
	t.Parallel()

	mockChannel := &mockChannel{}
	mockManager := &mockRabbitMQManager{
		channel: mockChannel,
	}
	producer := NewProducerRabbitMQMultiTenant(mockManager)

	msg := model.ReportMessage{
		ReportID:     uuid.New(),
		TemplateID:   uuid.New(),
		OutputFormat: "pdf",
	}

	// Context with tenant ID
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-success")
	_, err := producer.ProducerDefault(ctx, "test-exchange", "test-key", msg)

	require.NoError(t, err)
	assert.Equal(t, "tenant-success", mockManager.lastTenantID)
	assert.True(t, mockChannel.publishCalled, "PublishWithContext should be called")
	assert.Equal(t, "test-exchange", mockChannel.lastExchange)
	assert.Equal(t, "test-key", mockChannel.lastKey)
}

// TestProducerDefault_SingleTenant_DoesNotRequireTenantID verifies that single-tenant
// producer works without tenant ID in context.
func TestProducerDefault_SingleTenant_DoesNotRequireTenantID(t *testing.T) {
	producer := newTestProducer()

	msg := model.ReportMessage{
		ReportID:     uuid.New(),
		TemplateID:   uuid.New(),
		OutputFormat: "pdf",
	}

	// Context without tenant ID (single-tenant mode)
	ctx := context.Background()
	_, err := producer.ProducerDefault(ctx, "test-exchange", "test-key", msg)
	// Should not error with "tenant ID is required" (will fail for connection reasons instead)
	if err != nil {
		assert.NotContains(t, err.Error(), "tenant ID is required", "single-tenant producer should not require tenant ID")
	}
}

// mockRabbitMQManager is a mock implementation of the RabbitMQManagerInterface
// used for unit testing multi-tenant producer behavior.
type mockRabbitMQManager struct {
	getChannelErr error
	lastTenantID  string
	channel       *mockChannel
}

// GetChannel implements the interface required by ProducerRabbitMQRepository.
func (m *mockRabbitMQManager) GetChannel(ctx context.Context, tenantID string) (RabbitMQChannel, error) {
	m.lastTenantID = tenantID

	if m.getChannelErr != nil {
		return nil, m.getChannelErr
	}

	if m.channel != nil {
		return m.channel, nil
	}

	return &mockChannel{}, nil
}

// mockChannel is a mock implementation of the RabbitMQChannel interface.
type mockChannel struct {
	publishCalled bool
	lastExchange  string
	lastKey       string
	publishErr    error
}

func (m *mockChannel) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	m.publishCalled = true
	m.lastExchange = exchange
	m.lastKey = key

	return m.publishErr
}

func (m *mockChannel) Close() error {
	return nil
}
