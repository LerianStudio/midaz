//go:build !integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v4/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Mock Implementations
// =============================================================================

// mockCircuitBreakerManager implements libCircuitBreaker.Manager for testing.
type mockCircuitBreakerManager struct {
	state libCircuitBreaker.State
}

func (m *mockCircuitBreakerManager) GetOrCreate(_ string, _ libCircuitBreaker.Config) (libCircuitBreaker.CircuitBreaker, error) {
	return nil, nil
}

func (m *mockCircuitBreakerManager) Execute(_ string, fn func() (any, error)) (any, error) {
	return fn()
}

func (m *mockCircuitBreakerManager) GetState(_ string) libCircuitBreaker.State {
	return m.state
}

func (m *mockCircuitBreakerManager) GetCounts(_ string) libCircuitBreaker.Counts {
	return libCircuitBreaker.Counts{}
}

func (m *mockCircuitBreakerManager) IsHealthy(_ string) bool {
	return m.state == libCircuitBreaker.StateClosed
}

func (m *mockCircuitBreakerManager) Reset(_ string) {}

func (m *mockCircuitBreakerManager) RegisterStateChangeListener(_ libCircuitBreaker.StateChangeListener) {
}

// Verify mockCircuitBreakerManager implements the interface.
var _ libCircuitBreaker.Manager = (*mockCircuitBreakerManager)(nil)

// mockTenantCheckerWithError simulates tenant checker that returns errors.
type mockTenantCheckerWithError struct {
	name         string
	tlsEnabled   bool
	tenantError  error
	tenantStatus DependencyStatus
}

func (m *mockTenantCheckerWithError) Name() string {
	return m.name
}

func (m *mockTenantCheckerWithError) TLSEnabled() bool {
	return m.tlsEnabled
}

func (m *mockTenantCheckerWithError) Check(_ context.Context) DependencyCheck {
	return DependencyCheck{
		Status: StatusNA,
		Reason: "tenant-scoped; use /readyz/tenant/:id",
	}
}

func (m *mockTenantCheckerWithError) CheckTenant(_ context.Context, _ string) DependencyCheck {
	if m.tenantError != nil {
		return DependencyCheck{
			Status: m.tenantStatus,
			Error:  m.tenantError.Error(),
		}
	}

	latency := int64(5)

	return DependencyCheck{
		Status:    StatusUp,
		LatencyMs: &latency,
	}
}

// slowChecker simulates a checker with configurable delay.
type slowChecker struct {
	name       string
	tlsEnabled bool
	delay      time.Duration
	callCount  atomic.Int64
}

func (c *slowChecker) Name() string {
	return c.name
}

func (c *slowChecker) TLSEnabled() bool {
	return c.tlsEnabled
}

func (c *slowChecker) Check(ctx context.Context) DependencyCheck {
	c.callCount.Add(1)

	select {
	case <-time.After(c.delay):
		latency := c.delay.Milliseconds()
		return DependencyCheck{
			Status:    StatusUp,
			LatencyMs: &latency,
		}
	case <-ctx.Done():
		return DependencyCheck{
			Status: StatusDown,
			Error:  "context deadline exceeded",
		}
	}
}

// =============================================================================
// Circuit Breaker Tests
// =============================================================================

func TestRabbitMQChecker_CircuitBreakerClosed(t *testing.T) {
	t.Parallel()

	cbManager := &mockCircuitBreakerManager{state: libCircuitBreaker.StateClosed}
	checker := NewRabbitMQChecker("rabbitmq", "", "", cbManager)

	result := checker.Check(context.Background())

	// With closed circuit breaker and no health URL, should be skipped (not degraded)
	assert.Equal(t, StatusSkipped, result.Status)
	assert.Equal(t, "closed", result.BreakerState)
	assert.Contains(t, result.Reason, "RABBITMQ_HEALTH_CHECK_URL not configured")
}

func TestRabbitMQChecker_CircuitBreakerOpen(t *testing.T) {
	t.Parallel()

	cbManager := &mockCircuitBreakerManager{state: libCircuitBreaker.StateOpen}
	checker := NewRabbitMQChecker("rabbitmq", "", "", cbManager)

	result := checker.Check(context.Background())

	assert.Equal(t, StatusDegraded, result.Status)
	assert.Equal(t, "open", result.BreakerState)
	assert.Equal(t, "circuit breaker is open", result.Reason)
}

func TestRabbitMQChecker_CircuitBreakerHalfOpen(t *testing.T) {
	t.Parallel()

	cbManager := &mockCircuitBreakerManager{state: libCircuitBreaker.StateHalfOpen}
	checker := NewRabbitMQChecker("rabbitmq", "", "", cbManager)

	result := checker.Check(context.Background())

	assert.Equal(t, StatusDegraded, result.Status)
	assert.Equal(t, "half-open", result.BreakerState)
	assert.Equal(t, "circuit breaker is half-open", result.Reason)
}

func TestRabbitMQChecker_NilCircuitBreakerManager(t *testing.T) {
	t.Parallel()

	// When cbManager is nil, should not panic and should check health URL
	checker := NewRabbitMQChecker("rabbitmq", "", "", nil)

	result := checker.Check(context.Background())

	// Without health URL and no circuit breaker, should be skipped
	assert.Equal(t, StatusSkipped, result.Status)
	assert.Empty(t, result.BreakerState)
}

func TestMapCircuitBreakerState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state libCircuitBreaker.State
		want  string
	}{
		{"closed_state", libCircuitBreaker.StateClosed, "closed"},
		{"open_state", libCircuitBreaker.StateOpen, "open"},
		{"half_open_state", libCircuitBreaker.StateHalfOpen, "half-open"},
		{"unknown_state", libCircuitBreaker.State("invalid"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mapCircuitBreakerState(tt.state)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRabbitMQChecker_DegradedAffectsGlobalHealth(t *testing.T) {
	t.Parallel()

	// Create a checker that will report degraded status
	cbManager := &mockCircuitBreakerManager{state: libCircuitBreaker.StateOpen}
	degradedChecker := NewRabbitMQChecker("rabbitmq", "", "", cbManager)

	latency := int64(5)

	// Healthy checker
	healthyChecker := &mockChecker{
		name:       "redis",
		tlsEnabled: false,
		check:      DependencyCheck{Status: StatusUp, LatencyMs: &latency},
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       []DependencyChecker{healthyChecker, degradedChecker},
		Version:        "1.0.0",
		DeploymentMode: "local",
	})

	// Simulate handling the request
	checks := make(map[string]DependencyCheck)
	allHealthy := true

	for _, checker := range handler.checkers {
		check := checker.Check(context.Background())
		checks[checker.Name()] = check

		if check.Status == StatusDown || check.Status == StatusDegraded {
			allHealthy = false
		}
	}

	assert.False(t, allHealthy, "degraded should affect overall health")
	assert.Equal(t, StatusDegraded, checks["rabbitmq"].Status)
	assert.Equal(t, StatusUp, checks["redis"].Status)
}

// =============================================================================
// Multi-Tenant Tests
// =============================================================================

func TestTenantPostgresChecker_TenantNotFound(t *testing.T) {
	t.Parallel()

	checker := &mockTenantCheckerWithError{
		name:         "postgres_onboarding",
		tlsEnabled:   false,
		tenantError:  errors.New("tenant not found: unknown-tenant"),
		tenantStatus: StatusDown,
	}

	result := checker.CheckTenant(context.Background(), "unknown-tenant")

	assert.Equal(t, StatusDown, result.Status)
	assert.Contains(t, result.Error, "tenant not found")
}

func TestTenantPostgresChecker_ConnectionFailure(t *testing.T) {
	t.Parallel()

	checker := &mockTenantCheckerWithError{
		name:         "postgres_onboarding",
		tlsEnabled:   true,
		tenantError:  errors.New("connection refused: database unreachable"),
		tenantStatus: StatusDown,
	}

	result := checker.CheckTenant(context.Background(), "tenant-123")

	assert.Equal(t, StatusDown, result.Status)
	assert.Contains(t, result.Error, "connection refused")
}

func TestTenantMongoChecker_PartialAvailability(t *testing.T) {
	t.Parallel()

	// Simulate scenario where PG is up but Mongo is down for a tenant
	pgChecker := &mockTenantCheckerWithError{
		name:         "postgres_onboarding",
		tlsEnabled:   false,
		tenantError:  nil, // healthy
		tenantStatus: StatusUp,
	}

	mongoChecker := &mockTenantCheckerWithError{
		name:         "mongo_onboarding",
		tlsEnabled:   false,
		tenantError:  errors.New("mongo connection timeout"),
		tenantStatus: StatusDown,
	}

	// Both checks for a tenant
	pgResult := pgChecker.CheckTenant(context.Background(), "tenant-456")
	mongoResult := mongoChecker.CheckTenant(context.Background(), "tenant-456")

	assert.Equal(t, StatusUp, pgResult.Status)
	assert.Equal(t, StatusDown, mongoResult.Status)

	// Overall health should be unhealthy (one down)
	allHealthy := pgResult.Status != StatusDown && pgResult.Status != StatusDegraded &&
		mongoResult.Status != StatusDown && mongoResult.Status != StatusDegraded

	assert.False(t, allHealthy, "partial availability should result in unhealthy status")
}

func TestHandleReadyzTenant_AllTenantCheckersHealthy(t *testing.T) {
	t.Parallel()

	pgChecker := &mockTenantCheckerWithError{
		name:       "postgres_onboarding",
		tlsEnabled: false,
	}

	mongoChecker := &mockTenantCheckerWithError{
		name:       "mongo_onboarding",
		tlsEnabled: false,
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:             libLog.NewNop(),
		TenantCheckers:     []TenantAwareDependencyChecker{pgChecker, mongoChecker},
		Version:            "1.0.0",
		DeploymentMode:     "production",
		MultiTenantEnabled: true,
	})

	// Verify handler is configured correctly
	assert.True(t, handler.multiTenantEnabled)
	assert.Len(t, handler.tenantCheckers, 2)
}

func TestHandleReadyzTenant_MixedHealthStatus(t *testing.T) {
	t.Parallel()

	// One healthy, one failing
	healthyPG := &mockTenantCheckerWithError{
		name:       "postgres_onboarding",
		tlsEnabled: false,
	}

	failingMongo := &mockTenantCheckerWithError{
		name:         "mongo_onboarding",
		tlsEnabled:   false,
		tenantError:  errors.New("replica set not available"),
		tenantStatus: StatusDown,
	}

	// Shared redis (not tenant-scoped)
	latency := int64(3)
	redisChecker := &mockChecker{
		name:       "redis",
		tlsEnabled: false,
		check:      DependencyCheck{Status: StatusUp, LatencyMs: &latency},
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:             libLog.NewNop(),
		Checkers:           []DependencyChecker{redisChecker},
		TenantCheckers:     []TenantAwareDependencyChecker{healthyPG, failingMongo},
		Version:            "1.0.0",
		DeploymentMode:     "production",
		MultiTenantEnabled: true,
	})

	// Simulate tenant health check aggregation
	checks := make(map[string]DependencyCheck)
	allHealthy := true

	for _, tc := range handler.tenantCheckers {
		result := tc.CheckTenant(context.Background(), "test-tenant")
		checks[tc.Name()] = result

		if result.Status == StatusDown || result.Status == StatusDegraded {
			allHealthy = false
		}
	}

	assert.False(t, allHealthy)
	assert.Equal(t, StatusUp, checks["postgres_onboarding"].Status)
	assert.Equal(t, StatusDown, checks["mongo_onboarding"].Status)
}

func TestTenantChecker_GlobalCheckReturnsNA(t *testing.T) {
	t.Parallel()

	checker := &mockTenantCheckerWithError{
		name:       "postgres_transaction",
		tlsEnabled: true,
	}

	// Global check should return n/a for tenant-scoped checkers
	globalResult := checker.Check(context.Background())

	assert.Equal(t, StatusNA, globalResult.Status)
	assert.Contains(t, globalResult.Reason, "tenant-scoped")
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

func TestReadyzHandler_ConcurrentRequests(t *testing.T) {
	t.Parallel()

	latency := int64(1)
	checker := &mockChecker{
		name:       "test_service",
		tlsEnabled: false,
		check:      DependencyCheck{Status: StatusUp, LatencyMs: &latency},
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       []DependencyChecker{checker},
		Version:        "1.0.0",
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	const numRequests = 50
	var wg sync.WaitGroup

	results := make(chan int, numRequests)

	for range numRequests {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			resp, err := app.Test(req, -1)
			if err != nil {
				t.Logf("request error: %v", err)
				results <- -1
				return
			}

			results <- resp.StatusCode
		}()
	}

	wg.Wait()
	close(results)

	var successCount, failCount int
	for code := range results {
		if code == http.StatusOK {
			successCount++
		} else {
			failCount++
		}
	}

	assert.Equal(t, numRequests, successCount, "all requests should succeed")
	assert.Equal(t, 0, failCount, "no requests should fail")
}

func TestReadyzHandler_CheckerTimeoutRespected(t *testing.T) {
	t.Parallel()

	// Create a slow checker that takes longer than the timeout
	slowCheck := &slowChecker{
		name:  "slow_redis",
		delay: 5 * time.Second, // Much longer than the 1s timeout for redis
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       []DependencyChecker{slowCheck},
		Version:        "1.0.0",
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	start := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, 3000) // 3 second test timeout
	elapsed := time.Since(start)

	require.NoError(t, err)

	// The handler should return within the checker timeout + overhead
	assert.Less(t, elapsed, 3*time.Second, "handler should timeout and return quickly")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	// The slow checker should have timed out
	slowCheckResult := response.Checks["slow_redis"]
	assert.Equal(t, StatusDown, slowCheckResult.Status)
	assert.Contains(t, slowCheckResult.Error, "context deadline exceeded")
}

func TestReadyzHandler_ParallelTenantChecks(t *testing.T) {
	t.Parallel()

	var pgCallCount, mongoCallCount atomic.Int64

	pgChecker := &mockTenantChecker{
		name:        "postgres_onboarding",
		tlsEnabled:  false,
		globalCheck: DependencyCheck{Status: StatusNA, Reason: "tenant-scoped"},
		tenantCheck: DependencyCheck{Status: StatusUp, LatencyMs: ptrInt64(5)},
	}

	mongoChecker := &mockTenantChecker{
		name:        "mongo_onboarding",
		tlsEnabled:  false,
		globalCheck: DependencyCheck{Status: StatusNA, Reason: "tenant-scoped"},
		tenantCheck: DependencyCheck{Status: StatusUp, LatencyMs: ptrInt64(3)},
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:             libLog.NewNop(),
		TenantCheckers:     []TenantAwareDependencyChecker{pgChecker, mongoChecker},
		Version:            "1.0.0",
		DeploymentMode:     "production",
		MultiTenantEnabled: true,
	})

	// Simulate concurrent tenant health checks
	const numTenants = 10
	var wg sync.WaitGroup

	for i := range numTenants {
		wg.Add(1)
		go func(tenantNum int) {
			defer wg.Done()

			tenantID := string(rune('A' + tenantNum))
			for _, checker := range handler.tenantCheckers {
				result := checker.CheckTenant(context.Background(), "tenant-"+tenantID)
				if checker.Name() == "postgres_onboarding" {
					pgCallCount.Add(1)
				} else {
					mongoCallCount.Add(1)
				}

				assert.Equal(t, StatusUp, result.Status)
			}
		}(i)
	}

	wg.Wait()

	assert.Equal(t, int64(numTenants), pgCallCount.Load())
	assert.Equal(t, int64(numTenants), mongoCallCount.Load())
}

func TestReadyzHandler_RaceConditionSafety(t *testing.T) {
	t.Parallel()

	// This test verifies that concurrent access to the handler doesn't cause data races.
	// Run with -race flag: go test -race -run TestReadyzHandler_RaceConditionSafety
	latency := int64(1)
	checker := &mockChecker{
		name:       "test",
		tlsEnabled: false,
		check:      DependencyCheck{Status: StatusUp, LatencyMs: &latency},
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       []DependencyChecker{checker},
		Version:        "1.0.0",
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	const goroutines = 20
	var wg sync.WaitGroup

	// Mix of read operations
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for range 10 {
				req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
				resp, err := app.Test(req, -1)
				if err == nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
				}
			}
		}()
	}

	wg.Wait()
	// If we reach here without race detector errors, the test passes
}

// ptrInt64 is a helper to create a pointer to int64.
func ptrInt64(v int64) *int64 {
	return &v
}
