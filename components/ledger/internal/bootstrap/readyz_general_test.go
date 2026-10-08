//go:build !integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v7/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
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
	checker := NewRabbitMQChecker("rabbitmq", "", "", "", "", cbManager)

	result := checker.Check(context.Background())

	// With closed circuit breaker and no health URL, should be skipped (not degraded)
	assert.Equal(t, StatusSkipped, result.Status)
	assert.Equal(t, "closed", result.BreakerState)
	assert.Contains(t, result.Reason, "RABBITMQ_HEALTH_CHECK_URL not configured")
}

func TestRabbitMQChecker_CircuitBreakerOpen(t *testing.T) {
	t.Parallel()

	cbManager := &mockCircuitBreakerManager{state: libCircuitBreaker.StateOpen}
	checker := NewRabbitMQChecker("rabbitmq", "", "", "", "", cbManager)

	result := checker.Check(context.Background())

	assert.Equal(t, StatusDegraded, result.Status)
	assert.Equal(t, "open", result.BreakerState)
	assert.Equal(t, "circuit breaker is open", result.Reason)
}

func TestRabbitMQChecker_CircuitBreakerHalfOpen(t *testing.T) {
	t.Parallel()

	cbManager := &mockCircuitBreakerManager{state: libCircuitBreaker.StateHalfOpen}
	checker := NewRabbitMQChecker("rabbitmq", "", "", "", "", cbManager)

	result := checker.Check(context.Background())

	assert.Equal(t, StatusDegraded, result.Status)
	assert.Equal(t, "half-open", result.BreakerState)
	assert.Equal(t, "circuit breaker is half-open", result.Reason)
}

func TestRabbitMQChecker_NilCircuitBreakerManager(t *testing.T) {
	t.Parallel()

	// When cbManager is nil, should not panic and should check health URL
	checker := NewRabbitMQChecker("rabbitmq", "", "", "", "", nil)

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
	degradedChecker := NewRabbitMQChecker("rabbitmq", "", "", "", "", cbManager)

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

// =============================================================================
// RabbitMQ Probe Tests
// =============================================================================

func TestNormalizeRabbitMQHealthURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"root", "http://rabbitmq:15672", "http://rabbitmq:15672/api/health/checks/alarms"},
		{"root_with_trailing_slash", "http://rabbitmq:15672/", "http://rabbitmq:15672/api/health/checks/alarms"},
		{"https_root", "https://rabbitmq:15671", "https://rabbitmq:15671/api/health/checks/alarms"},
		{"already_alarms", "http://rabbitmq:15672/api/health/checks/alarms", "http://rabbitmq:15672/api/health/checks/alarms"},
		{"already_alarms_with_trailing_slash", "http://rabbitmq:15672/api/health/checks/alarms/", "http://rabbitmq:15672/api/health/checks/alarms"},
		{"already_local_alarms", "http://rabbitmq:15672/api/health/checks/local-alarms", "http://rabbitmq:15672/api/health/checks/local-alarms"},
		{"already_local_alarms_with_trailing_slash", "http://rabbitmq:15672/api/health/checks/local-alarms/", "http://rabbitmq:15672/api/health/checks/local-alarms"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, normalizeRabbitMQHealthURL(tt.raw))
		})
	}
}

func TestRabbitMQChecker_ProbeRootURLUsesAlarmsPathWithBasicAuth(t *testing.T) {
	t.Parallel()

	srv, lastRequest := newRabbitMQProbeServer(t, http.StatusOK)
	checker := NewRabbitMQChecker("rabbitmq", srv.URL+"/", "", testRabbitMQUser, testRabbitMQPass, nil)

	result := checker.Check(context.Background())

	assert.Equal(t, StatusUp, result.Status)
	assert.NotNil(t, result.LatencyMs)
	assert.Empty(t, result.Error)

	got := lastRequest()
	assert.Equal(t, "/api/health/checks/alarms", got.path)
	assert.Equal(t, basicAuthHeader(testRabbitMQUser, testRabbitMQPass), got.authorization)
}

func TestRabbitMQChecker_ProbeFullURLKeptAsIs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{"alarms", "/api/health/checks/alarms"},
		{"local_alarms", "/api/health/checks/local-alarms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv, lastRequest := newRabbitMQProbeServer(t, http.StatusOK)
			checker := NewRabbitMQChecker("rabbitmq", srv.URL+tt.path, "", testRabbitMQUser, testRabbitMQPass, nil)

			result := checker.Check(context.Background())

			assert.Equal(t, StatusUp, result.Status)
			assert.Equal(t, tt.path, lastRequest().path)
			assert.Equal(t, basicAuthHeader(testRabbitMQUser, testRabbitMQPass), lastRequest().authorization)
		})
	}
}

func TestRabbitMQChecker_ProbeEmptyCredentialsStillSendsRequest(t *testing.T) {
	t.Parallel()

	srv, lastRequest := newRabbitMQProbeServer(t, http.StatusOK)
	checker := NewRabbitMQChecker("rabbitmq", srv.URL, "", "", "", nil)

	result := checker.Check(context.Background())

	assert.Equal(t, StatusUp, result.Status)
	assert.Equal(t, "/api/health/checks/alarms", lastRequest().path)
	assert.Equal(t, basicAuthHeader("", ""), lastRequest().authorization)
}

func TestRabbitMQChecker_ProbeNonSuccessStatusIsDown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
	}{
		{"unauthorized", http.StatusUnauthorized},
		{"forbidden", http.StatusForbidden},
		{"resource_alarm", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv, _ := newRabbitMQProbeServer(t, tt.status)
			checker := NewRabbitMQChecker("rabbitmq", srv.URL, "", testRabbitMQUser, testRabbitMQPass, nil)

			result := checker.Check(context.Background())

			assert.Equal(t, StatusDown, result.Status)
			assert.NotNil(t, result.LatencyMs)
			assert.Equal(t, fmt.Sprintf("health check returned status %d", tt.status), result.Error)
		})
	}
}

func TestRabbitMQChecker_ProbeConnectionRefusedIsDown(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	closedURL := srv.URL
	srv.Close()

	checker := NewRabbitMQChecker("rabbitmq", closedURL, "", testRabbitMQUser, testRabbitMQPass, nil)

	result := checker.Check(context.Background())

	assert.Equal(t, StatusDown, result.Status)
	assert.NotNil(t, result.LatencyMs)
	assert.True(t, strings.HasPrefix(result.Error, "health check request failed: "), result.Error)
}

func TestRabbitMQChecker_ProbeContextCanceledIsDown(t *testing.T) {
	t.Parallel()

	srv, _ := newRabbitMQProbeServer(t, http.StatusOK)
	checker := NewRabbitMQChecker("rabbitmq", srv.URL, "", testRabbitMQUser, testRabbitMQPass, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := checker.Check(ctx)

	assert.Equal(t, StatusDown, result.Status)
	assert.True(t, strings.HasPrefix(result.Error, "health check request failed: "), result.Error)
	assert.Contains(t, result.Error, context.Canceled.Error())
}

func TestRabbitMQChecker_ProbeContextDeadlineIsDown(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}

		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	checker := NewRabbitMQChecker("rabbitmq", srv.URL, "", testRabbitMQUser, testRabbitMQPass, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := checker.Check(ctx)

	assert.Equal(t, StatusDown, result.Status)
	assert.True(t, strings.HasPrefix(result.Error, "health check request failed: "), result.Error)
	assert.Contains(t, result.Error, context.DeadlineExceeded.Error())
}

func TestRabbitMQChecker_ProbeEmptyURLIsSkipped(t *testing.T) {
	t.Parallel()

	checker := NewRabbitMQChecker("rabbitmq", "", "", testRabbitMQUser, testRabbitMQPass, nil)

	result := checker.Check(context.Background())

	assert.Equal(t, StatusSkipped, result.Status)
	assert.Equal(t, "RABBITMQ_HEALTH_CHECK_URL not configured", result.Reason)
	assert.Empty(t, result.Error)
	assert.Nil(t, result.LatencyMs)
}

func TestRabbitMQChecker_ProbeDoesNotExposeCredentials(t *testing.T) {
	t.Parallel()

	unauthorized, _ := newRabbitMQProbeServer(t, http.StatusUnauthorized)

	refused := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	refusedURL := refused.URL
	refused.Close()

	tests := []struct {
		name string
		url  string
	}{
		{"rejected_credentials", unauthorized.URL},
		{"connection_refused", refusedURL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			checker := NewRabbitMQChecker("rabbitmq", tt.url, "", testRabbitMQUser, testRabbitMQPass, nil)

			result := checker.Check(context.Background())
			require.Equal(t, StatusDown, result.Status)

			body, err := json.Marshal(result)
			require.NoError(t, err)

			authHeader := basicAuthHeader(testRabbitMQUser, testRabbitMQPass)

			for _, secret := range []string{testRabbitMQUser, testRabbitMQPass, authHeader} {
				assert.NotContains(t, checker.healthCheckURL, secret)
				assert.NotContains(t, result.Error, secret)
				assert.NotContains(t, string(body), secret)
			}
		})
	}
}

func TestRabbitMQChecker_ProbeInvalidURLDoesNotExposeCredentials(t *testing.T) {
	t.Parallel()

	const (
		embeddedUser = "embedded-url-user"
		embeddedPass = "embedded-url-pass"
	)

	rawURL := "http://" + embeddedUser + ":" + embeddedPass + "@rabbitmq:bad port/"
	checker := NewRabbitMQChecker("rabbitmq", rawURL, "", testRabbitMQUser, testRabbitMQPass, nil)

	result := checker.Check(context.Background())

	require.Equal(t, StatusDown, result.Status)
	assert.Equal(t, "failed to create request: invalid health check URL", result.Error)

	body, err := json.Marshal(result)
	require.NoError(t, err)

	for _, secret := range []string{embeddedUser, embeddedPass, rawURL, testRabbitMQUser, testRabbitMQPass} {
		assert.NotContains(t, result.Error, secret)
		assert.NotContains(t, string(body), secret)
	}
}

// testRabbitMQUser and testRabbitMQPass are distinctive placeholders so that
// leak assertions cannot match unrelated text.
const (
	testRabbitMQUser = "readyz-probe-user"
	testRabbitMQPass = "readyz-probe-pass"
)

// rabbitMQProbeRequest captures what the fake management API received.
type rabbitMQProbeRequest struct {
	path          string
	authorization string
}

// newRabbitMQProbeServer starts a fake management API that answers every
// request with status and returns an accessor for the last request received.
func newRabbitMQProbeServer(t *testing.T, status int) (*httptest.Server, func() rabbitMQProbeRequest) {
	t.Helper()

	var (
		mu   sync.Mutex
		last rabbitMQProbeRequest
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		last = rabbitMQProbeRequest{path: r.URL.Path, authorization: r.Header.Get("Authorization")}
		mu.Unlock()

		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	return srv, func() rabbitMQProbeRequest {
		mu.Lock()
		defer mu.Unlock()

		return last
	}
}

func basicAuthHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
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
			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	start := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 3000 * time.Millisecond, FailOnTimeout: true}) // 3 second test timeout
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
				resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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
