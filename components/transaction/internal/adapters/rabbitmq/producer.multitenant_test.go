// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"testing"

	libConstants "github.com/LerianStudio/lib-commons/v3/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// =============================================================================
// HELPER
// =============================================================================

// setupMultiTenantMocks creates a gomock controller, mock logger, mock channel
// provider, and mock publishable channel for multi-tenant producer tests.
func setupMultiTenantMocks(t *testing.T) (
	*gomock.Controller,
	*libLog.MockLogger,
	*MockChannelProvider,
	*MockPublishableChannel,
) {
	t.Helper()

	ctrl := gomock.NewController(t)

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Warnf(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()

	provider := NewMockChannelProvider(ctrl)
	channel := NewMockPublishableChannel(ctrl)

	return ctrl, logger, provider, channel
}

// =============================================================================
// UNIT TESTS — Interface Compliance
// =============================================================================

func TestMultiTenantProducerRepository_ImplementsProducerRepository(t *testing.T) {
	t.Parallel()

	// Compile-time check: if MultiTenantProducerRepository does not implement
	// ProducerRepository this test file will not compile.
	var _ ProducerRepository = (*MultiTenantProducerRepository)(nil)
}

// =============================================================================
// UNIT TESTS — NewMultiTenantProducer (constructor via managerGetter)
// =============================================================================

func TestNewMultiTenantProducer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		managerNil     bool
		loggerNil      bool
		expectNonNil   bool
		expectProvider bool
	}{
		{
			name:           "valid_manager_and_logger",
			managerNil:     false,
			loggerNil:      false,
			expectNonNil:   true,
			expectProvider: true,
		},
		{
			name:           "nil_logger_still_creates_instance",
			managerNil:     false,
			loggerNil:      true,
			expectNonNil:   true,
			expectProvider: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			var mgr managerGetter
			if !tt.managerNil {
				mgr = NewMockmanagerGetter(ctrl)
			}

			var logger libLog.Logger
			if !tt.loggerNil {
				logger = libLog.NewMockLogger(ctrl)
			}

			producer := NewMultiTenantProducer(mgr, logger)

			if tt.expectNonNil {
				require.NotNil(t, producer, "producer should not be nil")
			}

			if tt.expectProvider {
				assert.NotNil(t, producer.channelProvider, "channelProvider should be set via managerAdapter")
			}
		})
	}
}

// =============================================================================
// UNIT TESTS — NewMultiTenantProducerWithProvider (constructor via ChannelProvider)
// =============================================================================

func TestNewMultiTenantProducerWithProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		providerNil  bool
		loggerNil    bool
		expectNonNil bool
	}{
		{
			name:         "valid_provider_and_logger",
			providerNil:  false,
			loggerNil:    false,
			expectNonNil: true,
		},
		{
			name:         "nil_provider",
			providerNil:  true,
			loggerNil:    false,
			expectNonNil: true,
		},
		{
			name:         "nil_logger",
			providerNil:  false,
			loggerNil:    true,
			expectNonNil: true,
		},
		{
			name:         "nil_provider_and_nil_logger",
			providerNil:  true,
			loggerNil:    true,
			expectNonNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			var provider ChannelProvider
			if !tt.providerNil {
				provider = NewMockChannelProvider(ctrl)
			}

			var logger libLog.Logger
			if !tt.loggerNil {
				logger = libLog.NewMockLogger(ctrl)
			}

			producer := NewMultiTenantProducerWithProvider(provider, logger)

			if tt.expectNonNil {
				require.NotNil(t, producer, "producer should not be nil")
			}

			if !tt.providerNil {
				assert.NotNil(t, producer.channelProvider, "channelProvider should be the provided mock")
			} else {
				assert.Nil(t, producer.channelProvider, "channelProvider should be nil when nil was provided")
			}
		})
	}
}

// =============================================================================
// UNIT TESTS — ProducerDefault (AC-1, AC-2, AC-7, AC-8)
// =============================================================================

func TestMultiTenantProducer_ProducerDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		tenantID        string
		exchange        string
		key             string
		message         []byte
		getChannelErr   error
		publishErr      error
		expectErrSubstr string
	}{
		{
			// AC-1: missing tenant ID — empty context
			name:            "missing_tenant_id_returns_error",
			tenantID:        "",
			exchange:        "test-exchange",
			key:             "test.key",
			message:         []byte(`{"data":"test"}`),
			expectErrSubstr: "tenant ID is required in context for multi-tenant producer",
		},
		{
			// AC-8: GetChannel returns error
			name:            "get_channel_error_is_propagated",
			tenantID:        "tenant-a",
			exchange:        "test-exchange",
			key:             "test.key",
			message:         []byte(`{"data":"test"}`),
			getChannelErr:   errors.New("connection refused"),
			expectErrSubstr: "failed to get channel for tenant tenant-a",
		},
		{
			// AC-8: PublishWithContext returns error
			name:            "publish_error_is_propagated",
			tenantID:        "tenant-a",
			exchange:        "test-exchange",
			key:             "test.key",
			message:         []byte(`{"data":"test"}`),
			publishErr:      errors.New("channel closed"),
			expectErrSubstr: "channel closed",
		},
		{
			// AC-2: successful publish path
			name:     "successful_publish",
			tenantID: "tenant-a",
			exchange: "test-exchange",
			key:      "test.key",
			message:  []byte(`{"data":"test"}`),
		},
		{
			// Edge case: empty message body
			name:     "empty_message_body_publishes_successfully",
			tenantID: "tenant-a",
			exchange: "test-exchange",
			key:      "test.key",
			message:  []byte{},
		},
		{
			// Edge case: nil message body
			name:     "nil_message_body_publishes_successfully",
			tenantID: "tenant-a",
			exchange: "test-exchange",
			key:      "test.key",
			message:  nil,
		},
		{
			// Edge case: empty exchange and key
			name:     "empty_exchange_and_key",
			tenantID: "tenant-a",
			exchange: "",
			key:      "",
			message:  []byte(`{"data":"test"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl, logger, provider, channel := setupMultiTenantMocks(t)

			producer := NewMultiTenantProducerWithProvider(provider, logger)

			// Build context with or without tenant ID
			ctx := context.Background()
			if tt.tenantID != "" {
				ctx = tmcore.SetTenantIDInContext(ctx, tt.tenantID)
			}

			// Only set up GetChannel expectation when tenant ID is present
			if tt.tenantID != "" {
				if tt.getChannelErr != nil {
					provider.EXPECT().
						GetChannel(gomock.Any(), tt.tenantID).
						Return(nil, tt.getChannelErr)
				} else {
					provider.EXPECT().
						GetChannel(gomock.Any(), tt.tenantID).
						Return(channel, nil)

					// Channel Close is always deferred after successful GetChannel
					channel.EXPECT().Close().Return(nil)

					if tt.publishErr != nil {
						channel.EXPECT().
							PublishWithContext(gomock.Any(), tt.exchange, tt.key, false, false, gomock.Any()).
							Return(tt.publishErr)
					} else {
						channel.EXPECT().
							PublishWithContext(gomock.Any(), tt.exchange, tt.key, false, false, gomock.Any()).
							DoAndReturn(func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
								// AC-7: Verify publishing parameters
								assert.Equal(t, "application/json", msg.ContentType, "content type should be application/json")
								assert.Equal(t, uint8(amqp.Persistent), msg.DeliveryMode, "delivery mode should be persistent")
								assert.NotNil(t, msg.Headers, "headers should be present")
								assert.Contains(t, msg.Headers, libConstants.HeaderID, "headers should contain request ID header")
								assert.Equal(t, tt.message, msg.Body, "message body should match")
								return nil
							})
					}
				}
			}

			// Act
			result, err := producer.ProducerDefault(ctx, tt.exchange, tt.key, tt.message)

			// Assert
			if tt.expectErrSubstr != "" {
				require.Error(t, err, "expected error but got nil")
				assert.Contains(t, err.Error(), tt.expectErrSubstr, "error message should contain expected substring")
				assert.Nil(t, result, "result should be nil on error")
			} else {
				require.NoError(t, err, "expected no error")
				assert.Nil(t, result, "result should be nil on success (current implementation)")
			}

			ctrl.Finish()
		})
	}
}

// =============================================================================
// UNIT TESTS — ProducerDefaultWithContext (AC-3)
// =============================================================================

func TestMultiTenantProducer_ProducerDefaultWithContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		tenantID        string
		exchange        string
		key             string
		message         []byte
		getChannelErr   error
		publishErr      error
		expectErrSubstr string
	}{
		{
			// AC-3 + AC-1: missing tenant ID behaves identically to ProducerDefault
			name:            "missing_tenant_id_returns_error",
			tenantID:        "",
			exchange:        "test-exchange",
			key:             "test.key",
			message:         []byte(`{"data":"test"}`),
			expectErrSubstr: "tenant ID is required in context for multi-tenant producer",
		},
		{
			// AC-3 + AC-8: GetChannel error propagated identically
			name:            "get_channel_error_is_propagated",
			tenantID:        "tenant-b",
			exchange:        "test-exchange",
			key:             "test.key",
			message:         []byte(`{"data":"test"}`),
			getChannelErr:   errors.New("connection refused"),
			expectErrSubstr: "failed to get channel for tenant tenant-b",
		},
		{
			// AC-3: successful publish identical to ProducerDefault
			name:     "successful_publish",
			tenantID: "tenant-b",
			exchange: "test-exchange",
			key:      "test.key",
			message:  []byte(`{"data":"test"}`),
		},
		{
			// AC-3 + publish error
			name:            "publish_error_is_propagated",
			tenantID:        "tenant-b",
			exchange:        "test-exchange",
			key:             "test.key",
			message:         []byte(`{"data":"test"}`),
			publishErr:      errors.New("timeout"),
			expectErrSubstr: "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl, logger, provider, channel := setupMultiTenantMocks(t)

			producer := NewMultiTenantProducerWithProvider(provider, logger)

			ctx := context.Background()
			if tt.tenantID != "" {
				ctx = tmcore.SetTenantIDInContext(ctx, tt.tenantID)
			}

			if tt.tenantID != "" {
				if tt.getChannelErr != nil {
					provider.EXPECT().
						GetChannel(gomock.Any(), tt.tenantID).
						Return(nil, tt.getChannelErr)
				} else {
					provider.EXPECT().
						GetChannel(gomock.Any(), tt.tenantID).
						Return(channel, nil)
					channel.EXPECT().Close().Return(nil)

					if tt.publishErr != nil {
						channel.EXPECT().
							PublishWithContext(gomock.Any(), tt.exchange, tt.key, false, false, gomock.Any()).
							Return(tt.publishErr)
					} else {
						channel.EXPECT().
							PublishWithContext(gomock.Any(), tt.exchange, tt.key, false, false, gomock.Any()).
							Return(nil)
					}
				}
			}

			// Act
			result, err := producer.ProducerDefaultWithContext(ctx, tt.exchange, tt.key, tt.message)

			// Assert
			if tt.expectErrSubstr != "" {
				require.Error(t, err, "expected error but got nil")
				assert.Contains(t, err.Error(), tt.expectErrSubstr, "error message should contain expected substring")
				assert.Nil(t, result, "result should be nil on error")
			} else {
				require.NoError(t, err, "expected no error")
				assert.Nil(t, result, "result should be nil on success")
			}

			ctrl.Finish()
		})
	}
}

// =============================================================================
// UNIT TESTS — AC-3: ProducerDefault and ProducerDefaultWithContext delegate
// to the same internal publish, differing only in span name.
// =============================================================================

func TestMultiTenantProducer_BothMethodsDelegateToPublish(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
	}{
		{
			name:   "ProducerDefault_delegates_to_publish",
			method: "ProducerDefault",
		},
		{
			name:   "ProducerDefaultWithContext_delegates_to_publish",
			method: "ProducerDefaultWithContext",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl, logger, provider, channel := setupMultiTenantMocks(t)

			producer := NewMultiTenantProducerWithProvider(provider, logger)

			tenantID := "tenant-delegate-test"
			ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)
			exchange := "test-exchange"
			key := "test.key"
			message := []byte(`{"verify":"delegate"}`)

			provider.EXPECT().
				GetChannel(gomock.Any(), tenantID).
				Return(channel, nil)
			channel.EXPECT().Close().Return(nil)
			channel.EXPECT().
				PublishWithContext(gomock.Any(), exchange, key, false, false, gomock.Any()).
				DoAndReturn(func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
					assert.Equal(t, message, msg.Body, "both methods should pass the same message body")
					assert.Equal(t, uint8(amqp.Persistent), msg.DeliveryMode, "delivery mode should be persistent")
					return nil
				})

			var err error

			switch tt.method {
			case "ProducerDefault":
				_, err = producer.ProducerDefault(ctx, exchange, key, message)
			case "ProducerDefaultWithContext":
				_, err = producer.ProducerDefaultWithContext(ctx, exchange, key, message)
			}

			require.NoError(t, err)

			ctrl.Finish()
		})
	}
}

// =============================================================================
// UNIT TESTS — CheckRabbitMQHealth (AC-4)
// =============================================================================

func TestMultiTenantProducer_CheckRabbitMQHealth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected bool
	}{
		{
			name:     "always_returns_true",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			provider := NewMockChannelProvider(ctrl)

			producer := NewMultiTenantProducerWithProvider(provider, nil)

			result := producer.CheckRabbitMQHealth()

			assert.Equal(t, tt.expected, result, "CheckRabbitMQHealth should return true unconditionally")

			ctrl.Finish()
		})
	}
}

// =============================================================================
// UNIT TESTS — Close (AC-5, AC-6)
// =============================================================================

func TestMultiTenantProducer_Close(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		nilReceiver     bool
		nilProvider     bool
		closeErr        error
		expectErrSubstr string
	}{
		{
			// AC-6: nil receiver returns nil (no panic)
			name:        "nil_receiver_returns_nil",
			nilReceiver: true,
		},
		{
			// AC-6: nil channelProvider returns nil (no panic)
			name:        "nil_channel_provider_returns_nil",
			nilProvider: true,
		},
		{
			// AC-5: delegates to ChannelProvider.Close
			name: "delegates_to_channel_provider_close",
		},
		{
			// AC-5: propagates error from ChannelProvider.Close
			name:            "propagates_close_error",
			closeErr:        errors.New("close failed: connection reset"),
			expectErrSubstr: "close failed: connection reset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.nilReceiver {
				var p *MultiTenantProducerRepository
				err := p.Close()
				assert.NoError(t, err, "Close on nil receiver should return nil")
				return
			}

			if tt.nilProvider {
				p := &MultiTenantProducerRepository{
					channelProvider: nil,
				}
				err := p.Close()
				assert.NoError(t, err, "Close with nil channelProvider should return nil")
				return
			}

			ctrl := gomock.NewController(t)
			provider := NewMockChannelProvider(ctrl)

			provider.EXPECT().
				Close(gomock.Any()).
				Return(tt.closeErr)

			producer := NewMultiTenantProducerWithProvider(provider, nil)

			err := producer.Close()

			if tt.expectErrSubstr != "" {
				require.Error(t, err, "expected error from Close")
				assert.Contains(t, err.Error(), tt.expectErrSubstr, "error should contain expected substring")
			} else {
				require.NoError(t, err, "expected no error from Close")
			}

			ctrl.Finish()
		})
	}
}

// =============================================================================
// UNIT TESTS — AC-7: Published messages include trace headers and persistent
// delivery mode (verified via DoAndReturn inside publish mock)
// =============================================================================

func TestMultiTenantProducer_PublishMessageParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message []byte
	}{
		{
			name:    "json_message_has_persistent_delivery_and_trace_headers",
			message: []byte(`{"amount":100,"currency":"USD"}`),
		},
		{
			name:    "large_message_body",
			message: make([]byte, 1024*1024), // 1 MB
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl, logger, provider, channel := setupMultiTenantMocks(t)

			producer := NewMultiTenantProducerWithProvider(provider, logger)

			tenantID := "tenant-params"
			ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)
			exchange := "test-exchange"
			key := "test.key"

			provider.EXPECT().
				GetChannel(gomock.Any(), tenantID).
				Return(channel, nil)
			channel.EXPECT().Close().Return(nil)
			channel.EXPECT().
				PublishWithContext(gomock.Any(), exchange, key, false, false, gomock.Any()).
				DoAndReturn(func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
					// AC-7: verify content type
					assert.Equal(t, "application/json", msg.ContentType, "content type must be application/json")
					// AC-7: verify persistent delivery mode
					assert.Equal(t, uint8(amqp.Persistent), msg.DeliveryMode, "delivery mode must be persistent")
					// AC-7: verify trace headers present
					assert.NotNil(t, msg.Headers, "headers must not be nil")
					assert.Contains(t, msg.Headers, libConstants.HeaderID, "headers must contain request ID")
					// AC-7: verify body matches input
					assert.Equal(t, tt.message, msg.Body, "body must match the original message")
					return nil
				})

			_, err := producer.ProducerDefault(ctx, exchange, key, tt.message)
			require.NoError(t, err)

			ctrl.Finish()
		})
	}
}

// =============================================================================
// UNIT TESTS — AC-1 Edge Cases: empty-string tenant ID in context
// =============================================================================

func TestMultiTenantProducer_ProducerDefault_EmptyStringTenantID(t *testing.T) {
	t.Parallel()

	// Edge case: tenant ID is explicitly set to empty string in context.
	// GetTenantIDFromContext returns "" → error path.

	tests := []struct {
		name     string
		tenantID string
	}{
		{
			name:     "empty_string_tenant_id_in_context",
			tenantID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			logger := libLog.NewMockLogger(ctrl)
			logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
			logger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()

			provider := NewMockChannelProvider(ctrl)
			producer := NewMultiTenantProducerWithProvider(provider, logger)

			// SetTenantIDInContext with empty string is equivalent to no tenant
			ctx := tmcore.SetTenantIDInContext(context.Background(), tt.tenantID)

			_, err := producer.ProducerDefault(ctx, "exchange", "key", []byte("msg"))

			require.Error(t, err, "empty tenant ID should fail")
			assert.Contains(t, err.Error(), "tenant ID is required in context for multi-tenant producer",
				"error should indicate missing tenant ID")

			ctrl.Finish()
		})
	}
}

// =============================================================================
// UNIT TESTS — AC-8: GetChannel error wrapping
// =============================================================================

func TestMultiTenantProducer_ProducerDefault_GetChannelErrorWrapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		tenantID      string
		getChannelErr error
		expectWrapped string
	}{
		{
			name:          "wraps_original_error_with_tenant_context",
			tenantID:      "tenant-wrap",
			getChannelErr: errors.New("SASL authentication failed"),
			expectWrapped: "failed to get channel for tenant tenant-wrap: SASL authentication failed",
		},
		{
			name:          "timeout_error_from_get_channel",
			tenantID:      "tenant-timeout",
			getChannelErr: context.DeadlineExceeded,
			expectWrapped: "failed to get channel for tenant tenant-timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl, logger, provider, _ := setupMultiTenantMocks(t)

			producer := NewMultiTenantProducerWithProvider(provider, logger)

			ctx := tmcore.SetTenantIDInContext(context.Background(), tt.tenantID)

			provider.EXPECT().
				GetChannel(gomock.Any(), tt.tenantID).
				Return(nil, tt.getChannelErr)

			_, err := producer.ProducerDefault(ctx, "exchange", "key", []byte("msg"))

			require.Error(t, err, "GetChannel error should be propagated")
			assert.Contains(t, err.Error(), tt.expectWrapped, "error should include tenant context")
			assert.ErrorIs(t, err, tt.getChannelErr, "wrapped error should be unwrappable to original")

			ctrl.Finish()
		})
	}
}

// =============================================================================
// UNIT TESTS — managerAdapter
// =============================================================================

func TestManagerAdapter_GetChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		tenantID      string
		getChannelErr error
	}{
		{
			name:     "delegates_to_underlying_manager",
			tenantID: "tenant-adapter",
		},
		{
			name:          "propagates_error_from_manager",
			tenantID:      "tenant-err",
			getChannelErr: errors.New("manager error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockMgr := NewMockmanagerGetter(ctrl)
			adapter := &managerAdapter{manager: mockMgr}

			ctx := context.Background()

			if tt.getChannelErr != nil {
				mockMgr.EXPECT().
					GetChannel(ctx, tt.tenantID).
					Return(nil, tt.getChannelErr)

				ch, err := adapter.GetChannel(ctx, tt.tenantID)
				require.Error(t, err, "expected error from adapter")
				assert.Nil(t, ch, "channel should be nil on error")
				assert.Equal(t, tt.getChannelErr, err, "error should match")
			} else {
				// Manager returns nil channel (we cannot construct a real *amqp.Channel
				// in unit tests, but nil satisfies the concrete return type).
				mockMgr.EXPECT().
					GetChannel(ctx, tt.tenantID).
					Return(nil, nil)

				ch, err := adapter.GetChannel(ctx, tt.tenantID)
				require.NoError(t, err, "expected no error from adapter")
				// *amqp.Channel nil is returned as PublishableChannel nil
				assert.Nil(t, ch, "channel should be nil (passthrough from manager)")
			}

			ctrl.Finish()
		})
	}
}

func TestManagerAdapter_Close(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		closeErr error
	}{
		{
			name: "delegates_close_to_manager",
		},
		{
			name:     "propagates_close_error",
			closeErr: errors.New("close failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockMgr := NewMockmanagerGetter(ctrl)
			adapter := &managerAdapter{manager: mockMgr}

			ctx := context.Background()
			mockMgr.EXPECT().Close(ctx).Return(tt.closeErr)

			err := adapter.Close(ctx)

			if tt.closeErr != nil {
				require.Error(t, err)
				assert.Equal(t, tt.closeErr, err, "error should pass through from manager")
			} else {
				require.NoError(t, err)
			}

			ctrl.Finish()
		})
	}
}

// =============================================================================
// UNIT TESTS — Context cancellation edge case
// =============================================================================

func TestMultiTenantProducer_ProducerDefault_CanceledContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		cancelContext bool
	}{
		{
			name:          "canceled_context_propagated_through_get_channel",
			cancelContext: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl, logger, provider, _ := setupMultiTenantMocks(t)

			producer := NewMultiTenantProducerWithProvider(provider, logger)

			ctx, cancel := context.WithCancel(context.Background())
			ctx = tmcore.SetTenantIDInContext(ctx, "tenant-cancel")

			if tt.cancelContext {
				cancel()
			} else {
				defer cancel()
			}

			provider.EXPECT().
				GetChannel(gomock.Any(), "tenant-cancel").
				Return(nil, context.Canceled)

			_, err := producer.ProducerDefault(ctx, "exchange", "key", []byte("msg"))

			require.Error(t, err, "canceled context should produce an error")
			assert.ErrorIs(t, err, context.Canceled, "should wrap context.Canceled")

			ctrl.Finish()
		})
	}
}
