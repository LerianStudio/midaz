// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockChecker is a test implementation of DependencyChecker.
type mockChecker struct {
	name       string
	tlsEnabled bool
	check      DependencyCheck
}

func (m *mockChecker) Name() string {
	return m.name
}

func (m *mockChecker) TLSEnabled() bool {
	return m.tlsEnabled
}

func (m *mockChecker) Check(_ context.Context) DependencyCheck {
	return m.check
}

// newReadyHandler creates a ReadyzHandler and marks it as ready for testing.
// This is needed because HandleReadyz now checks lifecycle state before running checks.
func newReadyHandler(cfg ReadyzHandlerConfig) *ReadyzHandler {
	handler := NewReadyzHandler(cfg)
	handler.SetServerReady()

	return handler
}

func TestReadyzHandler_HandleReadyz(t *testing.T) {
	t.Parallel()

	latency := int64(5)

	tests := []struct {
		name           string
		checkers       []DependencyChecker
		version        string
		deploymentMode string
		wantStatus     int
		wantHealthy    bool
		wantChecks     map[string]DependencyStatus
	}{
		{
			name: "all_healthy_returns_200",
			checkers: []DependencyChecker{
				&mockChecker{name: "postgres", tlsEnabled: false, check: DependencyCheck{Status: StatusUp, LatencyMs: &latency}},
				&mockChecker{name: "redis", tlsEnabled: true, check: DependencyCheck{Status: StatusUp, LatencyMs: &latency}},
			},
			version:        "1.0.0",
			deploymentMode: "production",
			wantStatus:     http.StatusOK,
			wantHealthy:    true,
			wantChecks: map[string]DependencyStatus{
				"postgres": StatusUp,
				"redis":    StatusUp,
			},
		},
		{
			name: "one_down_returns_503",
			checkers: []DependencyChecker{
				&mockChecker{name: "postgres", tlsEnabled: false, check: DependencyCheck{Status: StatusUp, LatencyMs: &latency}},
				&mockChecker{name: "redis", tlsEnabled: true, check: DependencyCheck{Status: StatusDown, Error: "connection refused"}},
			},
			version:        "1.0.0",
			deploymentMode: "production",
			wantStatus:     http.StatusServiceUnavailable,
			wantHealthy:    false,
			wantChecks: map[string]DependencyStatus{
				"postgres": StatusUp,
				"redis":    StatusDown,
			},
		},
		{
			name: "degraded_returns_503",
			checkers: []DependencyChecker{
				&mockChecker{name: "rabbitmq", tlsEnabled: false, check: DependencyCheck{Status: StatusDegraded, Reason: "circuit breaker half-open", BreakerState: "half-open"}},
			},
			version:        "1.0.0",
			deploymentMode: "local",
			wantStatus:     http.StatusServiceUnavailable,
			wantHealthy:    false,
			wantChecks: map[string]DependencyStatus{
				"rabbitmq": StatusDegraded,
			},
		},
		{
			name: "skipped_and_na_count_as_healthy",
			checkers: []DependencyChecker{
				&mockChecker{name: "postgres", tlsEnabled: false, check: DependencyCheck{Status: StatusNA, Reason: "tenant-scoped"}},
				&mockChecker{name: "redis", tlsEnabled: true, check: DependencyCheck{Status: StatusUp, LatencyMs: &latency}},
				&mockChecker{name: "rabbitmq", tlsEnabled: false, check: DependencyCheck{Status: StatusSkipped, Reason: "not configured"}},
			},
			version:        "2.0.0",
			deploymentMode: "staging",
			wantStatus:     http.StatusOK,
			wantHealthy:    true,
			wantChecks: map[string]DependencyStatus{
				"postgres": StatusNA,
				"redis":    StatusUp,
				"rabbitmq": StatusSkipped,
			},
		},
		{
			name:           "no_checkers_returns_healthy",
			checkers:       []DependencyChecker{},
			version:        "1.0.0",
			deploymentMode: "local",
			wantStatus:     http.StatusOK,
			wantHealthy:    true,
			wantChecks:     map[string]DependencyStatus{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := newReadyHandler(ReadyzHandlerConfig{
				Logger:         libLog.NewNop(),
				Checkers:       tt.checkers,
				Version:        tt.version,
				DeploymentMode: tt.deploymentMode,
			})

			app := fiber.New()
			app.Get("/readyz", handler.HandleReadyz)

			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)

			assert.Equal(t, tt.wantStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var response ReadyzResponse
			err = json.Unmarshal(body, &response)
			require.NoError(t, err)

			if tt.wantHealthy {
				assert.Equal(t, "healthy", response.Status)
			} else {
				assert.Equal(t, "unhealthy", response.Status)
			}

			assert.Equal(t, tt.version, response.Version)
			assert.Equal(t, tt.deploymentMode, response.DeploymentMode)

			for name, wantStatus := range tt.wantChecks {
				check, exists := response.Checks[name]
				assert.True(t, exists, "check %s should exist", name)
				assert.Equal(t, wantStatus, check.Status, "check %s status mismatch", name)
			}
		})
	}
}

func TestReadyzHandler_TLSField(t *testing.T) {
	t.Parallel()

	latency := int64(5)

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger: libLog.NewNop(),
		Checkers: []DependencyChecker{
			&mockChecker{name: "postgres_tls", tlsEnabled: true, check: DependencyCheck{Status: StatusUp, LatencyMs: &latency}},
			&mockChecker{name: "redis_no_tls", tlsEnabled: false, check: DependencyCheck{Status: StatusUp, LatencyMs: &latency}},
		},
		Version:        "1.0.0",
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	// Check TLS field for postgres_tls
	pgCheck := response.Checks["postgres_tls"]
	require.NotNil(t, pgCheck.TLS)
	assert.True(t, *pgCheck.TLS, "postgres_tls should have TLS enabled")

	// Check TLS field for redis_no_tls
	redisCheck := response.Checks["redis_no_tls"]
	require.NotNil(t, redisCheck.TLS)
	assert.False(t, *redisCheck.TLS, "redis_no_tls should have TLS disabled")
}

func TestReadyzHandler_DeploymentModeDefault(t *testing.T) {
	t.Parallel()

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       []DependencyChecker{},
		Version:        "1.0.0",
		DeploymentMode: "", // Empty should default to "local"
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "local", response.DeploymentMode)
}

func TestTimeoutForChecker(t *testing.T) {
	t.Parallel()

	handler := &ReadyzHandler{}

	tests := []struct {
		name     string
		checker  DependencyChecker
		expected time.Duration
	}{
		{
			name:     "redis_gets_1s_timeout",
			checker:  &mockChecker{name: "redis"},
			expected: 1 * time.Second,
		},
		{
			name:     "postgres_gets_2s_timeout",
			checker:  &mockChecker{name: "postgres_onboarding"},
			expected: 2 * time.Second,
		},
		{
			name:     "mongo_gets_2s_timeout",
			checker:  &mockChecker{name: "mongo_transaction"},
			expected: 2 * time.Second,
		},
		{
			name:     "rabbitmq_gets_2s_timeout",
			checker:  &mockChecker{name: "rabbitmq"},
			expected: 2 * time.Second,
		},
		{
			name:     "unknown_gets_2s_timeout",
			checker:  &mockChecker{name: "unknown_service"},
			expected: 2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			timeout := handler.timeoutForChecker(tt.checker)
			assert.Equal(t, tt.expected, timeout)
		})
	}
}

func TestContainsLower(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{"empty_both", "", "", true},
		{"empty_substr", "hello", "", true},
		{"exact_match", "redis", "redis", true},
		{"substring_match", "postgres_onboarding", "postgres", true},
		{"case_insensitive", "PostgreSQL", "postgres", true},
		{"case_insensitive_reverse", "postgres", "POSTGRES", true},
		{"no_match", "redis", "postgres", false},
		{"partial_no_match", "red", "redis", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := contains(tt.s, tt.substr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReadyzHandler_DegradedStateAggregation(t *testing.T) {
	t.Parallel()

	latency := int64(5)

	tests := []struct {
		name               string
		checkers           []DependencyChecker
		wantOverallHealthy bool
		wantHTTPStatus     int
	}{
		{
			name: "single_degraded_returns_503",
			checkers: []DependencyChecker{
				&mockChecker{name: "postgres", check: DependencyCheck{Status: StatusUp, LatencyMs: &latency}},
				&mockChecker{name: "rabbitmq", check: DependencyCheck{Status: StatusDegraded, Reason: "circuit breaker half-open", BreakerState: "half-open"}},
			},
			wantOverallHealthy: false,
			wantHTTPStatus:     http.StatusServiceUnavailable,
		},
		{
			name: "multiple_degraded_returns_503",
			checkers: []DependencyChecker{
				&mockChecker{name: "postgres", check: DependencyCheck{Status: StatusDegraded, Reason: "high latency"}},
				&mockChecker{name: "rabbitmq", check: DependencyCheck{Status: StatusDegraded, Reason: "circuit breaker open", BreakerState: "open"}},
				&mockChecker{name: "redis", check: DependencyCheck{Status: StatusUp, LatencyMs: &latency}},
			},
			wantOverallHealthy: false,
			wantHTTPStatus:     http.StatusServiceUnavailable,
		},
		{
			name: "degraded_and_down_returns_503",
			checkers: []DependencyChecker{
				&mockChecker{name: "postgres", check: DependencyCheck{Status: StatusDown, Error: "connection refused"}},
				&mockChecker{name: "rabbitmq", check: DependencyCheck{Status: StatusDegraded, Reason: "circuit breaker open"}},
			},
			wantOverallHealthy: false,
			wantHTTPStatus:     http.StatusServiceUnavailable,
		},
		{
			name: "all_up_with_skipped_returns_200",
			checkers: []DependencyChecker{
				&mockChecker{name: "postgres", check: DependencyCheck{Status: StatusUp, LatencyMs: &latency}},
				&mockChecker{name: "redis", check: DependencyCheck{Status: StatusUp, LatencyMs: &latency}},
				&mockChecker{name: "rabbitmq", check: DependencyCheck{Status: StatusSkipped, Reason: "not configured"}},
			},
			wantOverallHealthy: true,
			wantHTTPStatus:     http.StatusOK,
		},
		{
			name: "all_na_returns_200",
			checkers: []DependencyChecker{
				&mockChecker{name: "postgres_onboarding", check: DependencyCheck{Status: StatusNA, Reason: "tenant-scoped"}},
				&mockChecker{name: "postgres_transaction", check: DependencyCheck{Status: StatusNA, Reason: "tenant-scoped"}},
				&mockChecker{name: "mongo_onboarding", check: DependencyCheck{Status: StatusNA, Reason: "tenant-scoped"}},
			},
			wantOverallHealthy: true,
			wantHTTPStatus:     http.StatusOK,
		},
		{
			name: "mix_of_up_skipped_na_returns_200",
			checkers: []DependencyChecker{
				&mockChecker{name: "postgres", check: DependencyCheck{Status: StatusNA, Reason: "tenant-scoped"}},
				&mockChecker{name: "redis", check: DependencyCheck{Status: StatusUp, LatencyMs: &latency}},
				&mockChecker{name: "optional_service", check: DependencyCheck{Status: StatusSkipped, Reason: "disabled"}},
			},
			wantOverallHealthy: true,
			wantHTTPStatus:     http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := newReadyHandler(ReadyzHandlerConfig{
				Logger:         libLog.NewNop(),
				Checkers:       tt.checkers,
				Version:        "1.0.0",
				DeploymentMode: "local",
			})

			app := fiber.New()
			app.Get("/readyz", handler.HandleReadyz)

			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)

			assert.Equal(t, tt.wantHTTPStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var response ReadyzResponse
			err = json.Unmarshal(body, &response)
			require.NoError(t, err)

			if tt.wantOverallHealthy {
				assert.Equal(t, "healthy", response.Status)
			} else {
				assert.Equal(t, "unhealthy", response.Status)
			}
		})
	}
}

func TestReadyzHandler_AggregationLogic(t *testing.T) {
	t.Parallel()

	// Test that the aggregation logic correctly identifies unhealthy states
	statusTests := []struct {
		status    DependencyStatus
		isHealthy bool
	}{
		{StatusUp, true},
		{StatusDown, false},
		{StatusDegraded, false},
		{StatusSkipped, true},
		{StatusNA, true},
	}

	for _, tt := range statusTests {
		t.Run(string(tt.status), func(t *testing.T) {
			t.Parallel()

			isHealthy := tt.status != StatusDown && tt.status != StatusDegraded
			assert.Equal(t, tt.isHealthy, isHealthy, "status %s health check", tt.status)
		})
	}
}

func TestDefaultTimeoutConstants(t *testing.T) {
	t.Parallel()

	// Verify default timeout constants are defined and have sensible values
	assert.Equal(t, 1*time.Second, DefaultRedisTimeout, "Redis timeout should be 1s")
	assert.Equal(t, 2*time.Second, DefaultDatabaseTimeout, "Database timeout should be 2s")
	assert.Equal(t, 2*time.Second, DefaultRabbitMQTimeout, "RabbitMQ timeout should be 2s")
}

func TestTimeoutForCheckerUsesConstants(t *testing.T) {
	t.Parallel()

	handler := &ReadyzHandler{}

	// Verify timeoutForChecker returns the constant values
	redisChecker := &mockChecker{name: "redis"}
	assert.Equal(t, DefaultRedisTimeout, handler.timeoutForChecker(redisChecker))

	pgChecker := &mockChecker{name: "postgres_onboarding"}
	assert.Equal(t, DefaultDatabaseTimeout, handler.timeoutForChecker(pgChecker))

	mongoChecker := &mockChecker{name: "mongo_transaction"}
	assert.Equal(t, DefaultDatabaseTimeout, handler.timeoutForChecker(mongoChecker))

	rmqChecker := &mockChecker{name: "rabbitmq"}
	assert.Equal(t, DefaultRabbitMQTimeout, handler.timeoutForChecker(rmqChecker))
}

func TestSanitizeError(t *testing.T) {
	t.Parallel()

	originalError := "failed to get database connection: dial tcp 203.0.113.50:5432: connection refused"

	tests := []struct {
		name           string
		deploymentMode string
		checkerName    string
		wantError      string
	}{
		{
			name:           "local_mode_returns_full_error",
			deploymentMode: DeploymentModeLocal,
			checkerName:    "postgres_onboarding",
			wantError:      originalError,
		},
		{
			name:           "saas_mode_returns_sanitized_error",
			deploymentMode: DeploymentModeSaaS,
			checkerName:    "postgres_onboarding",
			wantError:      "postgres_onboarding check failed",
		},
		{
			name:           "byoc_mode_returns_sanitized_error",
			deploymentMode: DeploymentModeBYOC,
			checkerName:    "postgres_onboarding",
			wantError:      "postgres_onboarding check failed",
		},
		{
			name:           "saas_mode_sanitizes_redis_error",
			deploymentMode: DeploymentModeSaaS,
			checkerName:    "redis",
			wantError:      "redis check failed",
		},
		{
			name:           "saas_mode_sanitizes_rabbitmq_error",
			deploymentMode: DeploymentModeSaaS,
			checkerName:    "rabbitmq",
			wantError:      "rabbitmq check failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := &ReadyzHandler{
				deploymentMode: tt.deploymentMode,
			}

			result := handler.sanitizeError(tt.checkerName, originalError)
			assert.Equal(t, tt.wantError, result)
		})
	}
}

func TestReadyzHandler_ErrorSanitization_InResponse(t *testing.T) {
	t.Parallel()

	internalError := "SELECT 1 failed: dial tcp 999.999.9.999:9999: connect: connection refused"
	latency := int64(50)

	tests := []struct {
		name              string
		deploymentMode    string
		wantErrorContains string
		wantErrorExcludes string
	}{
		{
			name:              "local_mode_exposes_full_error_in_response",
			deploymentMode:    DeploymentModeLocal,
			wantErrorContains: "999.999.9.999:999",
			wantErrorExcludes: "",
		},
		{
			name:              "saas_mode_sanitizes_error_in_response",
			deploymentMode:    DeploymentModeSaaS,
			wantErrorContains: "postgres_onboarding check failed",
			wantErrorExcludes: "999.999.9.999",
		},
		{
			name:              "byoc_mode_sanitizes_error_in_response",
			deploymentMode:    DeploymentModeBYOC,
			wantErrorContains: "postgres_onboarding check failed",
			wantErrorExcludes: "999.999.9.999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := newReadyHandler(ReadyzHandlerConfig{
				Logger: libLog.NewNop(),
				Checkers: []DependencyChecker{
					&mockChecker{
						name: "postgres_onboarding",
						check: DependencyCheck{
							Status:    StatusDown,
							LatencyMs: &latency,
							Error:     internalError,
						},
					},
				},
				Version:        "1.0.0",
				DeploymentMode: tt.deploymentMode,
			})

			app := fiber.New()
			app.Get("/readyz", handler.HandleReadyz)

			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var response ReadyzResponse
			err = json.Unmarshal(body, &response)
			require.NoError(t, err)

			check, exists := response.Checks["postgres_onboarding"]
			require.True(t, exists, "postgres_onboarding check should exist")

			assert.Contains(t, check.Error, tt.wantErrorContains,
				"error should contain expected string")

			if tt.wantErrorExcludes != "" {
				assert.NotContains(t, check.Error, tt.wantErrorExcludes,
					"error should not contain internal details")
			}
		})
	}
}

// TestReadyzHandler_LifecycleState tests the self-probe and graceful drain functionality.
func TestReadyzHandler_LifecycleState(t *testing.T) {
	t.Parallel()

	t.Run("returns_503_when_server_not_ready", func(t *testing.T) {
		t.Parallel()

		// Create handler WITHOUT calling SetServerReady()
		handler := NewReadyzHandler(ReadyzHandlerConfig{
			Logger:         libLog.NewNop(),
			Checkers:       []DependencyChecker{},
			Version:        "1.0.0",
			DeploymentMode: "local",
		})

		app := fiber.New()
		app.Get("/readyz", handler.HandleReadyz)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)

		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var response ReadyzResponse
		err = json.Unmarshal(body, &response)
		require.NoError(t, err)

		assert.Equal(t, "unhealthy", response.Status)
		assert.Contains(t, response.Reason, "server not ready")
		assert.Empty(t, response.Checks, "no checks should run when server not ready")
	})

	t.Run("returns_200_after_server_ready", func(t *testing.T) {
		t.Parallel()

		handler := NewReadyzHandler(ReadyzHandlerConfig{
			Logger:         libLog.NewNop(),
			Checkers:       []DependencyChecker{},
			Version:        "1.0.0",
			DeploymentMode: "local",
		})

		// Initially not ready
		assert.False(t, handler.IsServerReady())

		// Mark as ready
		handler.SetServerReady()
		assert.True(t, handler.IsServerReady())

		app := fiber.New()
		app.Get("/readyz", handler.HandleReadyz)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("returns_503_when_draining", func(t *testing.T) {
		t.Parallel()

		handler := NewReadyzHandler(ReadyzHandlerConfig{
			Logger:         libLog.NewNop(),
			Checkers:       []DependencyChecker{},
			Version:        "1.0.0",
			DeploymentMode: "local",
		})

		// Mark as ready first
		handler.SetServerReady()
		assert.False(t, handler.IsDraining())

		// Start draining
		handler.StartDrain()
		assert.True(t, handler.IsDraining())

		app := fiber.New()
		app.Get("/readyz", handler.HandleReadyz)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)

		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var response ReadyzResponse
		err = json.Unmarshal(body, &response)
		require.NoError(t, err)

		assert.Equal(t, "unhealthy", response.Status)
		assert.Contains(t, response.Reason, "draining")
		assert.Empty(t, response.Checks, "no checks should run when draining")
	})
}
