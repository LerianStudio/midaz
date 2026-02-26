//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	tmrabbitmq "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/rabbitmq"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	rmqtestutil "github.com/LerianStudio/midaz/v3/tests/utils/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TEST INFRASTRUCTURE
// =============================================================================

// tenantVHost represents a tenant's vhost with its inspection channel.
type tenantVHost struct {
	name    string
	channel *amqp.Channel
	conn    *amqp.Connection
}

// multiTenantTestInfra holds infrastructure for multi-tenant producer integration tests.
type multiTenantTestInfra struct {
	rmqContainer *rmqtestutil.ContainerResult
	producer     *MultiTenantProducerRepository
	manager      *tmrabbitmq.Manager
	tenants      map[string]*tenantVHost // tenantID → vhost info
	exchange     string
	routingKey   string
	queue        string
	mockServer   *httptest.Server
}

// setupMultiTenantInfra sets up the infrastructure for multi-tenant RabbitMQ testing.
// Creates a single RabbitMQ container with multiple vhosts (one per tenant),
// a mock tenant-manager API, and a MultiTenantProducerRepository wired to it.
func setupMultiTenantInfra(t *testing.T, tenantIDs []string) *multiTenantTestInfra {
	t.Helper()

	// Start RabbitMQ container
	rmqContainer := rmqtestutil.SetupContainer(t)

	exchange := "test-exchange"
	routingKey := "test.routing.key"
	queue := "test-queue"

	// Create a vhost, permissions, exchange and queue for each tenant
	tenants := make(map[string]*tenantVHost, len(tenantIDs))

	for _, tenantID := range tenantIDs {
		vhostName := tenantID

		// Create vhost via RabbitMQ management API
		createVHost(t, rmqContainer, vhostName)
		setVHostPermissions(t, rmqContainer, vhostName, rmqtestutil.DefaultUser)

		// Connect to the vhost for queue inspection
		vhostURI := fmt.Sprintf("amqp://%s:%s@%s:%s/%s",
			rmqtestutil.DefaultUser, rmqtestutil.DefaultPassword,
			rmqContainer.Host, rmqContainer.AMQPPort, vhostName)

		conn, err := amqp.Dial(vhostURI)
		require.NoError(t, err, "failed to connect to vhost %s", vhostName)

		ch, err := conn.Channel()
		require.NoError(t, err, "failed to open channel on vhost %s", vhostName)

		// Declare exchange and queue in this vhost
		rmqtestutil.SetupExchange(t, ch, exchange, "topic")
		rmqtestutil.SetupQueue(t, ch, queue, exchange, routingKey)

		tenants[tenantID] = &tenantVHost{
			name:    vhostName,
			channel: ch,
			conn:    conn,
		}

		t.Cleanup(func() {
			if ch != nil {
				ch.Close()
			}

			if conn != nil {
				conn.Close()
			}
		})
	}

	// Start mock tenant-manager API server
	mockServer := newMockTenantManagerServer(t, rmqContainer, tenants)

	t.Cleanup(func() {
		mockServer.Close()
	})

	// Create tmclient pointing to mock server
	logger := libZap.InitializeLogger()
	client := tmclient.NewClient(mockServer.URL, logger)

	// Create tmrabbitmq.Manager using the client
	manager := tmrabbitmq.NewManager(client, "ledger",
		tmrabbitmq.WithLogger(logger),
		tmrabbitmq.WithModule("transaction"),
	)

	t.Cleanup(func() {
		manager.Close(context.Background())
	})

	// Create the multi-tenant producer under test
	producer := NewMultiTenantProducer(manager, logger)

	return &multiTenantTestInfra{
		rmqContainer: rmqContainer,
		producer:     producer,
		manager:      manager,
		tenants:      tenants,
		exchange:     exchange,
		routingKey:   routingKey,
		queue:        queue,
		mockServer:   mockServer,
	}
}

// newMockTenantManagerServer creates an httptest server that serves tenant configs.
// Responds to GET /tenants/{tenantID}/services/{service}/settings with the
// RabbitMQ connection details for the tenant's vhost.
func newMockTenantManagerServer(t *testing.T, rmqContainer *rmqtestutil.ContainerResult, tenants map[string]*tenantVHost) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/tenants/", func(w http.ResponseWriter, r *http.Request) {
		// Path format: /tenants/{tenantID}/services/{service}/settings
		// splitPath returns: ["tenants", tenantID, "services", service, "settings"]
		parts := splitPath(r.URL.Path)
		if len(parts) < 5 || parts[0] != "tenants" || parts[2] != "services" {
			http.Error(w, "invalid path format", http.StatusBadRequest)
			return
		}

		tenantID := parts[1]

		tenant, ok := tenants[tenantID]
		if !ok {
			http.Error(w, "tenant not found", http.StatusNotFound)
			return
		}

		// Return RabbitMQ port as int
		var port int

		_, err := fmt.Sscanf(rmqContainer.AMQPPort, "%d", &port)
		require.NoError(t, err, "failed to parse AMQP port")

		config := &tmcore.TenantConfig{
			ID:         tenantID,
			TenantSlug: tenantID,
			Status:     "active",
			Messaging: &tmcore.MessagingConfig{
				RabbitMQ: &tmcore.RabbitMQConfig{
					Host:     rmqContainer.Host,
					Port:     port,
					VHost:    tenant.name,
					Username: rmqtestutil.DefaultUser,
					Password: rmqtestutil.DefaultPassword,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(config); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
	})

	return httptest.NewServer(mux)
}

// splitPath splits a URL path into segments, ignoring empty segments from leading/trailing slashes.
func splitPath(path string) []string {
	var parts []string

	for _, p := range strings.Split(path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}

	return parts
}

// =============================================================================
// RABBITMQ MANAGEMENT API HELPERS
// =============================================================================

// managementAPICall executes an HTTP PUT against the RabbitMQ management API with retry.
// The management API can briefly return EOF right after container startup.
func managementAPICall(t *testing.T, rmq *rmqtestutil.ContainerResult, path, body string) {
	t.Helper()

	mgmtURL := fmt.Sprintf("http://%s:%s%s", rmq.Host, rmq.MgmtPort, path)

	var lastErr error

	for attempt := 0; attempt < 5; attempt++ {
		var reqBody io.Reader
		if body != "" {
			reqBody = strings.NewReader(body)
		}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, mgmtURL, reqBody)
		require.NoError(t, err)

		req.SetBasicAuth(rmqtestutil.DefaultUser, rmqtestutil.DefaultPassword)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)

			continue
		}

		resp.Body.Close()

		if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
			return
		}

		lastErr = fmt.Errorf("unexpected status %d for PUT %s", resp.StatusCode, path)
		time.Sleep(500 * time.Millisecond)
	}

	require.NoError(t, lastErr, "management API call failed after retries: PUT %s", path)
}

// createVHost creates a vhost in RabbitMQ via the management API.
func createVHost(t *testing.T, rmq *rmqtestutil.ContainerResult, vhost string) {
	t.Helper()

	managementAPICall(t, rmq, fmt.Sprintf("/api/vhosts/%s", vhost), "")
}

// setVHostPermissions grants full permissions to a user on a vhost.
func setVHostPermissions(t *testing.T, rmq *rmqtestutil.ContainerResult, vhost, user string) {
	t.Helper()

	managementAPICall(t, rmq,
		fmt.Sprintf("/api/permissions/%s/%s", vhost, user),
		`{"configure":".*","write":".*","read":".*"}`,
	)
}

// =============================================================================
// TESTS
// =============================================================================

// TestIntegration_MultiTenantProducer_MessageIsolation verifies that messages
// published with different tenant contexts arrive in the correct tenant vhost queue.
// This is the core guarantee of multi-tenant RabbitMQ: tenant A's messages
// never appear in tenant B's queue, and vice versa.
func TestIntegration_MultiTenantProducer_MessageIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tenantA := "tenant-a"
	tenantB := "tenant-b"
	infra := setupMultiTenantInfra(t, []string{tenantA, tenantB})

	ctx := context.Background()

	// Publish 3 messages as tenant A
	for i := 0; i < 3; i++ {
		msg := fmt.Sprintf(`{"tenant":"%s","seq":%d}`, tenantA, i)
		ctxA := tmcore.SetTenantIDInContext(ctx, tenantA)

		_, err := infra.producer.ProducerDefault(ctxA, infra.exchange, infra.routingKey, []byte(msg))
		require.NoError(t, err, "tenant-a publish %d should succeed", i)
	}

	// Publish 2 messages as tenant B
	for i := 0; i < 2; i++ {
		msg := fmt.Sprintf(`{"tenant":"%s","seq":%d}`, tenantB, i)
		ctxB := tmcore.SetTenantIDInContext(ctx, tenantB)

		_, err := infra.producer.ProducerDefault(ctxB, infra.exchange, infra.routingKey, []byte(msg))
		require.NoError(t, err, "tenant-b publish %d should succeed", i)
	}

	// Wait for messages to be routed
	rmqtestutil.WaitForQueueCount(t, infra.tenants[tenantA].channel, infra.queue, 3, 5*time.Second)
	rmqtestutil.WaitForQueueCount(t, infra.tenants[tenantB].channel, infra.queue, 2, 5*time.Second)

	// Verify: tenant A queue has exactly 3 messages, all belonging to tenant A
	messagesA := consumeAll(t, infra.tenants[tenantA].channel, infra.queue, 3)
	for _, msg := range messagesA {
		assert.Contains(t, string(msg), `"tenant":"tenant-a"`,
			"tenant-a queue should only contain tenant-a messages")
	}

	// Verify: tenant B queue has exactly 2 messages, all belonging to tenant B
	messagesB := consumeAll(t, infra.tenants[tenantB].channel, infra.queue, 2)
	for _, msg := range messagesB {
		assert.Contains(t, string(msg), `"tenant":"tenant-b"`,
			"tenant-b queue should only contain tenant-b messages")
	}
}

// TestIntegration_MultiTenantProducer_NoTenantContext verifies that publishing
// without a tenant ID in context returns an error (not a panic or silent failure).
func TestIntegration_MultiTenantProducer_NoTenantContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupMultiTenantInfra(t, []string{"tenant-a"})

	// Publish without tenant context
	ctx := context.Background()
	msg := []byte(`{"test":"no-tenant"}`)

	_, err := infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msg)
	require.Error(t, err, "should fail without tenant context")
	assert.Contains(t, err.Error(), "tenant ID is required",
		"error should indicate missing tenant ID")
}

// TestIntegration_MultiTenantProducer_WithContext verifies that
// ProducerDefaultWithContext works identically to ProducerDefault.
func TestIntegration_MultiTenantProducer_WithContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tenantID := "tenant-ctx"
	infra := setupMultiTenantInfra(t, []string{tenantID})

	ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)
	msg := []byte(`{"method":"with-context"}`)

	_, err := infra.producer.ProducerDefaultWithContext(ctx, infra.exchange, infra.routingKey, msg)
	require.NoError(t, err, "ProducerDefaultWithContext should succeed")

	rmqtestutil.WaitForQueueCount(t, infra.tenants[tenantID].channel, infra.queue, 1, 5*time.Second)

	messages := consumeAll(t, infra.tenants[tenantID].channel, infra.queue, 1)
	assert.Equal(t, `{"method":"with-context"}`, string(messages[0]),
		"message body should be preserved")
}

// TestIntegration_MultiTenantProducer_ConnectionReuse verifies that the Manager
// reuses connections across multiple publishes to the same tenant (not creating
// a new connection per publish).
func TestIntegration_MultiTenantProducer_ConnectionReuse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tenantID := "tenant-reuse"
	infra := setupMultiTenantInfra(t, []string{tenantID})

	ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)

	// Publish 10 messages rapidly
	for i := 0; i < 10; i++ {
		msg := fmt.Sprintf(`{"seq":%d}`, i)

		_, err := infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, []byte(msg))
		require.NoError(t, err, "publish %d should succeed", i)
	}

	// All messages should arrive
	rmqtestutil.WaitForQueueCount(t, infra.tenants[tenantID].channel, infra.queue, 10, 5*time.Second)

	// Verify connection stats show only 1 connection (reused)
	stats := infra.manager.Stats()
	assert.Equal(t, 1, stats.TotalConnections,
		"should reuse a single connection for all publishes to the same tenant")
}

// TestIntegration_MultiTenantProducer_HealthCheck verifies that CheckRabbitMQHealth
// always returns true for the multi-tenant producer (Manager handles its own lifecycle).
func TestIntegration_MultiTenantProducer_HealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupMultiTenantInfra(t, []string{"tenant-health"})

	assert.True(t, infra.producer.CheckRabbitMQHealth(),
		"multi-tenant producer health check should always return true")
}

// TestIntegration_MultiTenantProducer_PersistentDelivery verifies that messages
// are published with persistent delivery mode (survives broker restart).
func TestIntegration_MultiTenantProducer_PersistentDelivery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tenantID := "tenant-persist"
	infra := setupMultiTenantInfra(t, []string{tenantID})

	ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)
	msg := []byte(`{"persist":true}`)

	_, err := infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msg)
	require.NoError(t, err)

	rmqtestutil.WaitForQueueCount(t, infra.tenants[tenantID].channel, infra.queue, 1, 5*time.Second)

	// Consume and check delivery mode
	delivery, ok, err := infra.tenants[tenantID].channel.Get(infra.queue, true)
	require.NoError(t, err)
	require.True(t, ok, "should have a message")

	assert.Equal(t, uint8(amqp.Persistent), delivery.DeliveryMode,
		"messages should be published with persistent delivery mode")
}

// =============================================================================
// HELPERS
// =============================================================================

// consumeAll retrieves exactly count messages from a queue using basic.Get.
// Fails the test if fewer messages are available.
func consumeAll(t *testing.T, ch *amqp.Channel, queue string, count int) [][]byte {
	t.Helper()

	var messages [][]byte

	for i := 0; i < count; i++ {
		delivery, ok, err := ch.Get(queue, true)
		require.NoError(t, err, "failed to get message %d from queue %s", i, queue)
		require.True(t, ok, "expected message %d in queue %s but none available", i, queue)

		messages = append(messages, delivery.Body)
	}

	return messages
}
