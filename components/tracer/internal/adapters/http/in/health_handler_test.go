// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/api"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// Test configuration constants.
const concurrentRequestCount = 10

// createTestFiberApp creates a Fiber app with telemetry context for testing.
// Uses lib-commons context functions to inject tracer, matching production behavior.
// Routes the canonical /readyz endpoint — the legacy /ready alias was removed
// per Gate 2 of the readyz contract.
func createTestFiberApp(hc *HealthChecker) *fiber.App {
	app := fiber.New()

	app.Use(func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		ctx = libObservability.ContextWithTracer(ctx, otel.Tracer("tracer-test"))
		c.SetUserContext(ctx)

		return c.Next()
	})
	app.Get("/readyz", hc.ReadyzHandler())

	return app
}

// TestReadinessHandler_WithMockDB verifies the postgres probe contract
// against the canonical /readyz response shape.
func TestReadinessHandler_WithMockDB(t *testing.T) {
	tests := []struct {
		name           string
		connected      bool
		pingError      error
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:           "database healthy",
			connected:      true,
			pingError:      nil,
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response api.ReadyzResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err, "failed to unmarshal response")

				assert.Equal(t, "healthy", response.Status, "top-level status must be canonical 'healthy'")
				require.Contains(t, response.Checks, "postgres")
				assert.Equal(t, StatusUp, response.Checks["postgres"].Status)
			},
		},
		{
			name:           "returns 503 when connection not established",
			connected:      false,
			pingError:      nil,
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody: func(t *testing.T, body []byte) {
				var response api.ReadyzResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err, "failed to unmarshal response")

				assert.Equal(t, "unhealthy", response.Status)
				require.Contains(t, response.Checks, "postgres")
				assert.Equal(t, StatusDown, response.Checks["postgres"].Status)
				assert.Equal(t, ErrConnectionNotEstablished.Error(), response.Checks["postgres"].Error)
			},
		},
		{
			name:           "returns 503 when ping fails",
			connected:      true,
			pingError:      errors.New("connection refused"),
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody: func(t *testing.T, body []byte) {
				var response api.ReadyzResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err, "failed to unmarshal response")

				assert.Equal(t, "unhealthy", response.Status)
				require.Contains(t, response.Checks, "postgres")
				assert.Equal(t, StatusDown, response.Checks["postgres"].Status)
				assert.Equal(t, ErrPingFailed.Error(), response.Checks["postgres"].Error)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupTestTracing(t)

			ctrl := gomock.NewController(t)

			db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
			require.NoError(t, err)
			defer db.Close()

			if tt.connected {
				if tt.pingError != nil {
					mock.ExpectPing().WillReturnError(tt.pingError)
				} else {
					mock.ExpectPing()
				}
			}

			provider := NewMockPostgresDBProvider(ctrl)
			provider.EXPECT().IsConnected().Return(tt.connected)

			if tt.connected {
				provider.EXPECT().GetDB(gomock.Any()).Return(db, nil)
			}

			hc := NewTestableHealthChecker(provider)
			// Provide a healthy cache so the only failing dep is postgres for the
			// down/ping cases — keeps the assertions on the postgres check tight.
			hc.SetCacheHealthProvider(&mockCacheHealth{ready: true, staleness: time.Second, size: 1})

			app := createTestFiberApp(hc)

			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if tt.expectedBody != nil {
				tt.expectedBody(t, body)
			}

			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestReadinessHandler_GetDBError(t *testing.T) {
	t.Run("returns 503 when GetDB returns error", func(t *testing.T) {
		testutil.SetupTestTracing(t)

		ctrl := gomock.NewController(t)

		provider := NewMockPostgresDBProvider(ctrl)
		provider.EXPECT().IsConnected().Return(true)
		provider.EXPECT().GetDB(gomock.Any()).Return(nil, errors.New("failed to get database connection"))

		hc := NewTestableHealthChecker(provider)
		hc.SetCacheHealthProvider(&mockCacheHealth{ready: true})

		app := createTestFiberApp(hc)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var response api.ReadyzResponse
		err = json.Unmarshal(body, &response)
		require.NoError(t, err)

		assert.Equal(t, "unhealthy", response.Status)
		assert.Equal(t, StatusDown, response.Checks["postgres"].Status)
		assert.Equal(t, ErrConnectionFailed.Error(), response.Checks["postgres"].Error)
	})
}

func TestReadinessHandler_NilProvider(t *testing.T) {
	t.Run("returns 503 when provider is nil", func(t *testing.T) {
		testutil.SetupTestTracing(t)

		hc := NewTestableHealthChecker(nil)
		hc.SetCacheHealthProvider(&mockCacheHealth{ready: true})

		app := createTestFiberApp(hc)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var response api.ReadyzResponse
		err = json.Unmarshal(body, &response)
		require.NoError(t, err)

		assert.Equal(t, "unhealthy", response.Status)
		assert.Equal(t, StatusDown, response.Checks["postgres"].Status)
		assert.Equal(t, ErrConnectionNotEstablished.Error(), response.Checks["postgres"].Error)
	})
}

func TestReadinessHandler_ConcurrentRequests(t *testing.T) {
	t.Run("handles concurrent health check requests", func(t *testing.T) {
		testutil.SetupTestTracing(t)

		ctrl := gomock.NewController(t)

		db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
		require.NoError(t, err)
		defer db.Close()

		for range concurrentRequestCount {
			mock.ExpectPing()
		}

		provider := NewMockPostgresDBProvider(ctrl)
		provider.EXPECT().IsConnected().Return(true).AnyTimes()
		provider.EXPECT().GetDB(gomock.Any()).Return(db, nil).AnyTimes()

		hc := NewTestableHealthChecker(provider)
		hc.SetCacheHealthProvider(&mockCacheHealth{ready: true})

		app := createTestFiberApp(hc)

		var wg sync.WaitGroup
		results := make(chan int, concurrentRequestCount)

		for range concurrentRequestCount {
			wg.Add(1)

			go func() {
				defer wg.Done()

				req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

				resp, err := app.Test(req, -1)
				if err != nil {
					results <- -1

					return
				}
				defer resp.Body.Close()

				results <- resp.StatusCode
			}()
		}

		wg.Wait()
		close(results)

		successCount := 0

		for status := range results {
			if status == http.StatusOK {
				successCount++
			}
		}

		assert.Equal(t, concurrentRequestCount, successCount, "all concurrent requests should succeed")
	})
}

func TestReadinessHandler_Timeout(t *testing.T) {
	t.Run("returns 503 when health check exceeds timeout", func(t *testing.T) {
		testutil.SetupTestTracing(t)

		ctrl := gomock.NewController(t)

		// Slow ping that will exceed the per-dep postgres timeout (2s).
		db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
		require.NoError(t, err)
		defer db.Close()

		// 5s delay > 2s probe timeout ⇒ context cancels the ping.
		mock.ExpectPing().WillDelayFor(5 * time.Second)

		provider := NewMockPostgresDBProvider(ctrl)
		provider.EXPECT().IsConnected().Return(true)
		provider.EXPECT().GetDB(gomock.Any()).Return(db, nil)

		hc := NewTestableHealthChecker(provider)
		hc.SetCacheHealthProvider(&mockCacheHealth{ready: true})

		app := createTestFiberApp(hc)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

		// Bound the test to 10s in case the handler hangs.
		resp, err := app.Test(req, 10000)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var response api.ReadyzResponse
		err = json.Unmarshal(body, &response)
		require.NoError(t, err)

		assert.Equal(t, "unhealthy", response.Status)
		assert.Equal(t, StatusDown, response.Checks["postgres"].Status)
	})
}

func TestDefaultHealthCheckTimeout(t *testing.T) {
	t.Run("default timeout is 3 seconds", func(t *testing.T) {
		assert.Equal(t, 3*time.Second, DefaultHealthCheckTimeout)
	})

}

func TestReadiness_CacheNotReady_Returns503Down(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectPing()

	provider := NewMockPostgresDBProvider(ctrl)
	provider.EXPECT().IsConnected().Return(true)
	provider.EXPECT().GetDB(gomock.Any()).Return(db, nil)

	hc := NewTestableHealthChecker(provider)
	mockCache := &mockCacheHealth{ready: false, staleness: time.Duration(math.MaxInt64), size: 0}
	hc.SetCacheHealthProvider(mockCache)

	app := createTestFiberApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)

	require.NoError(t, err)
	defer resp.Body.Close()

	// Canonical contract: "down" check forces 503.
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
		"cache not ready ⇒ down ⇒ 503 (canonical contract)")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response api.ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)
	assert.Equal(t, "unhealthy", response.Status)

	rc := response.Checks["rule_cache"]
	assert.Equal(t, StatusDown, rc.Status)
	assert.Equal(t, ErrCacheNotReady.Error(), rc.Error)
}

func TestReadiness_CacheReady_ReturnsUp(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectPing()

	provider := NewMockPostgresDBProvider(ctrl)
	provider.EXPECT().IsConnected().Return(true)
	provider.EXPECT().GetDB(gomock.Any()).Return(db, nil)

	hc := NewTestableHealthChecker(provider)
	mockCache := &mockCacheHealth{ready: true, staleness: 5 * time.Second, size: 10}
	hc.SetCacheHealthProvider(mockCache)

	app := createTestFiberApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response api.ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)
	assert.Equal(t, "healthy", response.Status)

	rc := response.Checks["rule_cache"]
	assert.Equal(t, StatusUp, rc.Status)
}

// TestReadiness_CacheStalenessExceeded_Returns503Degraded asserts the
// documented behavior change: stale cache (single-tenant) now maps to
// "degraded" + 503 (was 200 + DEGRADED before Gate 2).
func TestReadiness_CacheStalenessExceeded_Returns503Degraded(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectPing()

	provider := NewMockPostgresDBProvider(ctrl)
	provider.EXPECT().IsConnected().Return(true)
	provider.EXPECT().GetDB(gomock.Any()).Return(db, nil)

	hc := NewTestableHealthChecker(provider)
	mockCache := &mockCacheHealth{ready: true, staleness: 10 * time.Minute, size: 5}
	hc.SetCacheHealthProvider(mockCache)

	app := createTestFiberApp(hc)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
		"stale cache ⇒ degraded ⇒ 503 (canonical contract)")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response api.ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)
	assert.Equal(t, "unhealthy", response.Status)

	rc := response.Checks["rule_cache"]
	assert.Equal(t, StatusDegraded, rc.Status)
	assert.Equal(t, ErrCacheStale.Error(), rc.Error)
}

// mockCacheHealth implements RuleCacheHealthProvider for testing.
type mockCacheHealth struct {
	ready     bool
	staleness time.Duration
	size      int
}

func (m *mockCacheHealth) IsReady(_ context.Context) bool            { return m.ready }
func (m *mockCacheHealth) Staleness(_ context.Context) time.Duration { return m.staleness }
func (m *mockCacheHealth) Size(_ context.Context) int                { return m.size }

// TestHealthChecker_RuleCacheNotReady_ReportsDown is the regression guard:
// the /readyz cycle is single-tenant only — cacheHealth.IsReady=false must
// always map to "down".
func TestHealthChecker_RuleCacheNotReady_ReportsDown(t *testing.T) {
	t.Parallel()

	testutil.SetupTestTracing(t)

	hc := NewTestableHealthChecker(nil)
	hc.SetCacheHealthProvider(&mockCacheHealth{
		ready:     false,
		staleness: time.Duration(math.MaxInt64),
	})

	ctx := libObservability.ContextWithTracer(context.Background(), otel.Tracer("tracer-test"))
	check := hc.probeReadyzRuleCache(ctx)

	assert.Equal(t, StatusDown, check.Status,
		"single-tenant /readyz cycle reports 'down' when cacheHealth.IsReady=false")
}
