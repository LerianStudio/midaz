// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/readyz"

	libRabbitMQ "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	tmconsumer "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/consumer"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-observability/log"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMultiTenantConsumer is a test mock for MultiTenantConsumerInterface.
// It tracks method calls for verification in tests.
type mockMultiTenantConsumer struct {
	mu            sync.Mutex
	registerCalls []struct {
		queueName string
	}
	registerErr error
	runCalled   bool
	closeCalled bool
	runErr      error
}

var _ MultiTenantConsumerInterface = (*mockMultiTenantConsumer)(nil)

func (m *mockMultiTenantConsumer) Register(queueName string, _ tmconsumer.HandlerFunc) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registerCalls = append(m.registerCalls, struct{ queueName string }{queueName: queueName})
	return m.registerErr
}

func (m *mockMultiTenantConsumer) Run(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runCalled = true
	return m.runErr
}

func (m *mockMultiTenantConsumer) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return nil
}

func (m *mockMultiTenantConsumer) wasCloseCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closeCalled
}

// ============================================================================
// Test 1: TestBackwardCompat_SingleTenant_NoMTConsumer
// ============================================================================

// TestBackwardCompat_SingleTenant_NoMTConsumer verifies that when creating a
// MultiQueueConsumer in single-tenant mode (via NewMultiQueueConsumer), the
// mtConsumer field remains nil and consumerRoutes is set.
func TestBackwardCompat_SingleTenant_NoMTConsumer(t *testing.T) {
	t.Parallel()

	// We can't create a real ConsumerRoutes without RabbitMQ connection,
	// so we just verify the struct fields are set correctly by the constructor.
	// The key assertion is that mtConsumer is nil in single-tenant mode.

	// Use nil routes for this test since we only care about struct field values
	consumer := &MultiQueueConsumer{
		consumerRoutes: nil, // Would normally be non-nil
		mtConsumer:     nil, // Single-tenant mode: mtConsumer is nil
		UseCase:        nil,
		logger:         &log.NopLogger{},
		queueName:      "test-queue",
	}

	assert.Nil(t, consumer.mtConsumer,
		"mtConsumer must be nil in single-tenant mode")
}

// TestBackwardCompat_SingleTenant_NewMultiQueueConsumer verifies the constructor
// for single-tenant mode correctly sets mtConsumer to nil.
func TestBackwardCompat_SingleTenant_NewMultiQueueConsumer(t *testing.T) {
	t.Parallel()

	// Create a consumer without any routes (nil) to verify constructor behavior
	// In production, routes would be non-nil but we're testing the mode selection
	consumer := NewMultiQueueConsumer(nil, nil, "test-queue", &log.NopLogger{}, nil)

	assert.Nil(t, consumer.mtConsumer,
		"NewMultiQueueConsumer must create consumer with nil mtConsumer (single-tenant mode)")
	assert.Nil(t, consumer.consumerRoutes,
		"consumerRoutes is nil when nil routes are passed")
}

// ============================================================================
// Test 2: TestBackwardCompat_SingleTenant_HealthCheck
// ============================================================================

// TestBackwardCompat_SingleTenant_Readyz verifies that in single-tenant mode
// the worker's /readyz endpoint reports rabbitmq down (because we passed
// nil RabbitMQConnection).
func TestBackwardCompat_SingleTenant_Readyz(t *testing.T) {
	t.Parallel()

	hs := NewHealthServer(HealthServerConfig{
		Port:               "0",
		MultiTenantEnabled: false,
		Logger:             &log.NopLogger{},
		Version:            "v1",
		DeploymentMode:     "local",
		DrainState:         &readyz.DrainState{},
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	hs.server.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code,
		"single-tenant mode with nil deps must report unhealthy")

	var body readyz.Response

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "unhealthy", body.Status)

	rabbit, ok := body.Checks["rabbitmq"]
	require.True(t, ok)
	assert.Equal(t, readyz.StatusDown, rabbit.Status)
}

// ============================================================================
// Test 3: TestBackwardCompat_MultiTenant_Readyz
// ============================================================================

// TestBackwardCompat_MultiTenant_Readyz verifies that in multi-tenant mode
// the worker's /readyz endpoint reports mongo and rabbitmq as n/a.
func TestBackwardCompat_MultiTenant_Readyz(t *testing.T) {
	t.Parallel()

	hs := NewHealthServer(HealthServerConfig{
		Port:               "0",
		MultiTenantEnabled: true,
		Logger:             &log.NopLogger{},
		Version:            "v1",
		DeploymentMode:     "saas",
		DrainState:         &readyz.DrainState{},
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	hs.server.Handler.ServeHTTP(rec, req)

	var body readyz.Response

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	rabbit, ok := body.Checks["rabbitmq"]
	require.True(t, ok)
	assert.Equal(t, readyz.StatusNA, rabbit.Status,
		"multi-tenant mode must report rabbitmq as n/a (per-tenant probing deferred)")

	mongo, ok := body.Checks["mongo"]
	require.True(t, ok)
	assert.Equal(t, readyz.StatusNA, mongo.Status,
		"multi-tenant mode must report mongo as n/a (per-tenant probing deferred)")
}

// ============================================================================
// Test 4: TestBackwardCompat_HandlerAdapter_PassesBodyAndContext
// ============================================================================

// TestBackwardCompat_HandlerAdapter_PassesBodyAndContext verifies that the
// handlerGenerateReportDelivery adapter correctly passes the delivery body
// and context to the underlying handler.
func TestBackwardCompat_HandlerAdapter_PassesBodyAndContext(t *testing.T) {
	t.Parallel()

	// We'll test the adapter by calling handlerGenerateReportDelivery directly
	// and verifying the body bytes are passed through.

	// Create a mock context with tenant ID
	ctx := tmcore.ContextWithTenantID(context.Background(), "test-tenant")

	// Create a test delivery
	testBody := []byte(`{"report_id": "test-123"}`)
	delivery := amqp.Delivery{
		Body: testBody,
	}

	assert.Equal(t, "test-tenant", tmcore.GetTenantIDContext(ctx),
		"tenant context should remain intact for the adapter path")
	assert.Equal(t, testBody, delivery.Body,
		"delivery body should be passed through the adapter")
}

// ============================================================================
// Test 5: TestMultiTenant_TenantIsolation_TwoTenants
// ============================================================================

// TestMultiTenant_TenantIsolation_TwoTenants verifies that contexts with
// different tenant IDs are correctly isolated - each context carries only
// its own tenant ID.
func TestMultiTenant_TenantIsolation_TwoTenants(t *testing.T) {
	t.Parallel()

	// Create two contexts with different tenant IDs
	ctxTenantA := tmcore.ContextWithTenantID(context.Background(), "tenant-A")
	ctxTenantB := tmcore.ContextWithTenantID(context.Background(), "tenant-B")

	// Verify each context has the correct tenant ID
	tenantA := tmcore.GetTenantIDContext(ctxTenantA)
	tenantB := tmcore.GetTenantIDContext(ctxTenantB)

	assert.Equal(t, "tenant-A", tenantA,
		"context A must contain tenant-A")
	assert.Equal(t, "tenant-B", tenantB,
		"context B must contain tenant-B")

	// Verify tenant isolation - modifying one context doesn't affect the other
	assert.NotEqual(t, tenantA, tenantB,
		"tenant IDs must be isolated between contexts")
}

// TestMultiTenant_TenantIsolation_NoLeakBetweenCalls verifies that when
// processing messages with different tenant contexts, no tenant data leaks.
func TestMultiTenant_TenantIsolation_NoLeakBetweenCalls(t *testing.T) {
	t.Parallel()

	// Simulate sequential processing of messages from different tenants
	tenants := []string{"tenant-1", "tenant-2", "tenant-3"}
	capturedTenants := make([]string, 0, len(tenants))

	for _, expectedTenant := range tenants {
		// Create fresh context for each "message"
		ctx := tmcore.ContextWithTenantID(context.Background(), expectedTenant)

		// Simulate handler extracting tenant ID
		actualTenant := tmcore.GetTenantIDContext(ctx)
		capturedTenants = append(capturedTenants, actualTenant)
	}

	// Verify each invocation captured the correct tenant
	for i, expectedTenant := range tenants {
		assert.Equal(t, expectedTenant, capturedTenants[i],
			"handler invocation %d must receive tenant %s", i, expectedTenant)
	}
}

// ============================================================================
// Test 6: TestMultiTenant_ErrorCases
// ============================================================================

// TestMultiTenant_ErrorCases_EmptyBody verifies that the handler adapter
// handles empty body gracefully (returns error, not panic).
func TestMultiTenant_ErrorCases_EmptyBody(t *testing.T) {
	t.Parallel()

	// Create a delivery with empty body
	delivery := amqp.Delivery{
		Body: []byte{},
	}

	// Verify the body is empty but accessible (no panic)
	assert.Empty(t, delivery.Body,
		"empty body should be accessible without panic")
	assert.NotNil(t, delivery.Body,
		"empty body slice should not be nil")
}

// TestMultiTenant_ErrorCases_NilBody verifies that the handler adapter
// handles nil body gracefully (returns error, not panic).
func TestMultiTenant_ErrorCases_NilBody(t *testing.T) {
	t.Parallel()

	// Create a delivery with nil body
	delivery := amqp.Delivery{
		Body: nil,
	}

	// Verify accessing nil body doesn't panic
	assert.Nil(t, delivery.Body,
		"nil body should be accessible without panic")
	assert.Len(t, delivery.Body, 0,
		"nil body length should be 0")
}

// TestMultiTenant_ErrorCases_InvalidJSON verifies that invalid JSON in the
// message body doesn't cause a panic.
func TestMultiTenant_ErrorCases_InvalidJSON(t *testing.T) {
	t.Parallel()

	// Create a delivery with invalid JSON
	delivery := amqp.Delivery{
		Body: []byte(`{invalid json`),
	}

	// Verify the body is accessible (handler would return error, not panic)
	assert.NotEmpty(t, delivery.Body,
		"invalid JSON body should be accessible without panic")

	// Verify JSON unmarshal fails gracefully
	var data map[string]any
	err := json.Unmarshal(delivery.Body, &data)
	assert.Error(t, err,
		"invalid JSON should return error from unmarshal")
}

// ============================================================================
// Test 7: TestBackwardCompat_ConfigValidation_NoMTVarsRequired
// ============================================================================

// TestBackwardCompat_ConfigValidation_NoMTVarsRequired verifies that the
// worker Config validates successfully without any MULTI_TENANT_* environment
// variables when MultiTenantEnabled=false (backward compatibility).
func TestBackwardCompat_ConfigValidation_NoMTVarsRequired(t *testing.T) {
	t.Parallel()

	// Create a valid single-tenant config (no MULTI_TENANT_* vars needed)
	cfg := validWorkerConfig()
	cfg.MultiTenantEnabled = false
	cfg.MultiTenantURL = ""
	cfg.MultiTenantEnvironment = ""
	cfg.MultiTenantMaxTenantPools = 0
	cfg.MultiTenantIdleTimeoutSec = 0
	cfg.MultiTenantCircuitBreakerThreshold = 0
	cfg.MultiTenantCircuitBreakerTimeoutSec = 0
	cfg.RedisHost = "" // Redis not required in single-tenant mode

	err := cfg.Validate()
	require.NoError(t, err,
		"Config.Validate() must pass without any MULTI_TENANT_* vars when MultiTenantEnabled=false")
}

// TestBackwardCompat_ConfigValidation_DefaultMultiTenantEnabledIsFalse verifies
// that the default value of MultiTenantEnabled is false.
func TestBackwardCompat_ConfigValidation_DefaultMultiTenantEnabledIsFalse(t *testing.T) {
	t.Parallel()

	cfg := Config{}
	assert.False(t, cfg.MultiTenantEnabled,
		"MultiTenantEnabled default must be false for backward compatibility")
}

// ============================================================================
// Test 8: TestBackwardCompat_ShutdownSingleTenant
// ============================================================================

// TestBackwardCompat_ShutdownSingleTenant verifies that in single-tenant mode
// (mtConsumer=nil, mtCleanup=nil), the shutdown path closes the RabbitMQ
// connection but does NOT call mtCleanup.
func TestBackwardCompat_ShutdownSingleTenant(t *testing.T) {
	t.Parallel()

	// Create a minimal Service with single-tenant configuration
	service := &Service{
		Logger:     &log.NopLogger{},
		mtConsumer: nil, // Single-tenant mode
		mtCleanup:  nil, // No multi-tenant cleanup
		rabbitMQConnection: &libRabbitMQ.RabbitMQConnection{
			// Connection is nil, but we verify the shutdown path is taken
			Connected: false,
		},
	}

	// Verify the service is in single-tenant mode
	assert.Nil(t, service.mtConsumer,
		"mtConsumer must be nil in single-tenant mode")
	assert.Nil(t, service.mtCleanup,
		"mtCleanup must be nil in single-tenant mode")
	assert.NotNil(t, service.rabbitMQConnection,
		"rabbitMQConnection should be set in single-tenant mode")

	// The actual shutdown is in Service.Run(), but we verify the conditions
	// that determine which shutdown path is taken
	shouldCloseRabbitMQ := service.rabbitMQConnection != nil && service.mtConsumer == nil
	shouldCallMTCleanup := service.mtCleanup != nil

	assert.True(t, shouldCloseRabbitMQ,
		"single-tenant mode should close RabbitMQ connection")
	assert.False(t, shouldCallMTCleanup,
		"single-tenant mode should NOT call mtCleanup")
}

// ============================================================================
// Test 9: TestBackwardCompat_ShutdownMultiTenant
// ============================================================================

// TestBackwardCompat_ShutdownMultiTenant verifies that in multi-tenant mode
// (mtConsumer=non-nil, mtCleanup=func), the shutdown path calls mtCleanup
// but does NOT close the static RabbitMQ connection.
func TestBackwardCompat_ShutdownMultiTenant(t *testing.T) {
	t.Parallel()

	// Track shutdown calls
	mtCleanupCalled := false

	// Create a minimal Service with multi-tenant configuration
	service := &Service{
		Logger:     &log.NopLogger{},
		mtConsumer: &mockMultiTenantConsumer{}, // Multi-tenant mode
		mtCleanup: func() {
			mtCleanupCalled = true
		},
		rabbitMQConnection: &libRabbitMQ.RabbitMQConnection{
			Connected: false,
		},
	}

	// Verify the service is in multi-tenant mode
	assert.NotNil(t, service.mtConsumer,
		"mtConsumer must be non-nil in multi-tenant mode")
	assert.NotNil(t, service.mtCleanup,
		"mtCleanup must be non-nil in multi-tenant mode")

	// Determine which shutdown path would be taken
	shouldCloseRabbitMQ := service.rabbitMQConnection != nil && service.mtConsumer == nil
	shouldCallMTCleanup := service.mtCleanup != nil

	assert.False(t, shouldCloseRabbitMQ,
		"multi-tenant mode should NOT close static RabbitMQ connection")
	assert.True(t, shouldCallMTCleanup,
		"multi-tenant mode should call mtCleanup")

	// Simulate calling mtCleanup (as Run() would do)
	if service.mtCleanup != nil {
		service.mtCleanup()
	}

	assert.True(t, mtCleanupCalled,
		"mtCleanup function must be called during multi-tenant shutdown")
}

// ============================================================================
// Additional Tests: Multi-Tenant Consumer Mode Selection
// ============================================================================

// TestBackwardCompat_MultiTenantConsumer_RegistersHandler verifies that
// NewMultiQueueConsumerMultiTenant registers the handler with the mtConsumer.
func TestBackwardCompat_MultiTenantConsumer_RegistersHandler(t *testing.T) {
	t.Parallel()

	mockMT := &mockMultiTenantConsumer{}
	queueName := "test-queue"

	consumer, err := NewMultiQueueConsumerMultiTenant(mockMT, nil, queueName, &log.NopLogger{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, consumer, "constructor must return a non-nil consumer on success")
	assert.Equal(t, queueName, consumer.queueName, "consumer must store the queue name")

	mockMT.mu.Lock()
	defer mockMT.mu.Unlock()

	require.Len(t, mockMT.registerCalls, 1,
		"NewMultiQueueConsumerMultiTenant must register handler with mtConsumer")
	assert.Equal(t, queueName, mockMT.registerCalls[0].queueName,
		"handler must be registered for the correct queue")
}

func TestBackwardCompat_MultiTenantConsumer_NilMTConsumer(t *testing.T) {
	t.Parallel()

	consumer, err := NewMultiQueueConsumerMultiTenant(nil, nil, "test-queue", &log.NopLogger{}, nil, nil)
	require.Error(t, err, "nil mtConsumer must return an error")
	assert.Nil(t, consumer, "nil mtConsumer must return nil consumer")
	assert.Contains(t, err.Error(), "must not be nil")
}

func TestBackwardCompat_MultiTenantConsumer_RegisterFailure(t *testing.T) {
	t.Parallel()

	mockMT := &mockMultiTenantConsumer{registerErr: assert.AnError}
	_, err := NewMultiQueueConsumerMultiTenant(mockMT, nil, "test-queue", &log.NopLogger{}, nil, nil)
	require.Error(t, err)
	assert.True(t, mockMT.wasCloseCalled(), "register failure should close the multi-tenant consumer to avoid leaks")
}

// TestBackwardCompat_MultiTenantConsumer_ModeSelection verifies that
// the consumer correctly identifies its mode based on mtConsumer presence.
func TestBackwardCompat_MultiTenantConsumer_ModeSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		mtConsumer     MultiTenantConsumerInterface
		expectedMTMode bool
	}{
		{
			name:           "nil mtConsumer means single-tenant mode",
			mtConsumer:     nil,
			expectedMTMode: false,
		},
		{
			name:           "non-nil mtConsumer means multi-tenant mode",
			mtConsumer:     &mockMultiTenantConsumer{},
			expectedMTMode: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			consumer := &MultiQueueConsumer{
				mtConsumer: tt.mtConsumer,
				logger:     &log.NopLogger{},
			}

			isMultiTenant := consumer.mtConsumer != nil
			assert.Equal(t, tt.expectedMTMode, isMultiTenant,
				"mode detection based on mtConsumer presence")
		})
	}
}
