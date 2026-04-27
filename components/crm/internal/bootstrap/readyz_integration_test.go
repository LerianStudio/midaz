//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mongoContainer "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
)

// newReadyHandler creates a ReadyzHandler and marks it as ready for testing.
// This is needed because HandleReadyz checks lifecycle state before running checks.
func newReadyHandler(cfg ReadyzHandlerConfig) *ReadyzHandler {
	handler := NewReadyzHandler(cfg)
	handler.SetServerReady()

	return handler
}

func TestReadyz_Integration_AllDependenciesHealthy(t *testing.T) {
	t.Parallel()

	// Start MongoDB container
	mongo := mongoContainer.SetupContainer(t)

	// Create lib-commons wrapper
	mongoClient := mongoContainer.CreateConnection(t, mongo.URI, mongo.DBName)

	// Create checker using lib-commons client for MongoDB
	checkers := []DependencyChecker{
		NewMongoChecker("mongo", mongoClient, mongo.URI),
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
		Version:        "1.0.0-test",
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, 10000) // 10s timeout for containers
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, "1.0.0-test", response.Version)
	assert.Equal(t, "local", response.DeploymentMode)

	// MongoDB checker should be up
	assert.Equal(t, StatusUp, response.Checks["mongo"].Status)

	// Latency should be populated
	assert.NotNil(t, response.Checks["mongo"].LatencyMs)
}

func TestReadyz_Integration_MongoSkipped(t *testing.T) {
	t.Parallel()

	// Create a Mongo checker with nil client (simulating not configured)
	checkers := []DependencyChecker{
		NewMongoChecker("mongo", nil, ""), // nil = not configured
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
		Version:        "1.0.0-test",
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)

	// Should return 200 since nil checker returns "skipped" not "down"
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, StatusSkipped, response.Checks["mongo"].Status)
}

func TestReadyz_Integration_TLSDetection(t *testing.T) {
	t.Parallel()

	// Test that TLS detection works correctly with real containers (non-TLS)
	mongo := mongoContainer.SetupContainer(t)
	mongoClient := mongoContainer.CreateConnection(t, mongo.URI, mongo.DBName)

	checkers := []DependencyChecker{
		NewMongoChecker("mongo", mongoClient, mongo.URI),
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
		Version:        "1.0.0-test",
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, 10000)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	// Test container uses non-TLS connection
	for name, check := range response.Checks {
		require.NotNil(t, check.TLS, "TLS field should be set for %s", name)
		assert.False(t, *check.TLS, "TLS should be false for test container %s", name)
	}
}

func TestReadyz_Integration_LatencyMeasurement(t *testing.T) {
	t.Parallel()

	mongo := mongoContainer.SetupContainer(t)
	mongoClient := mongoContainer.CreateConnection(t, mongo.URI, mongo.DBName)

	checkers := []DependencyChecker{
		NewMongoChecker("mongo", mongoClient, mongo.URI),
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
		Version:        "1.0.0",
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	// Run multiple times to verify latency is measured each time
	for i := range 3 {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		resp, err := app.Test(req, 10000)
		require.NoError(t, err, "iteration %d failed", i)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var response ReadyzResponse
		err = json.Unmarshal(body, &response)
		require.NoError(t, err)

		// Latency should be non-negative and reasonable (< 1 second for local containers)
		// Note: Very fast pings may return 0ms at millisecond precision
		mongoLatency := response.Checks["mongo"].LatencyMs

		require.NotNil(t, mongoLatency)

		assert.GreaterOrEqual(t, *mongoLatency, int64(0), "mongo latency should be non-negative")
		assert.Less(t, *mongoLatency, int64(1000), "mongo latency should be < 1s")
	}
}

func TestReadyz_Integration_ConcurrentRequests(t *testing.T) {
	t.Parallel()

	mongo := mongoContainer.SetupContainer(t)
	mongoClient := mongoContainer.CreateConnection(t, mongo.URI, mongo.DBName)

	checkers := []DependencyChecker{
		NewMongoChecker("mongo", mongoClient, mongo.URI),
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
		Version:        "1.0.0",
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	// Make multiple concurrent requests
	const numRequests = 5
	results := make(chan int, numRequests)

	for range numRequests {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			resp, err := app.Test(req, 5000) // 5s timeout
			if err != nil {
				results <- -1
				return
			}
			results <- resp.StatusCode
		}()
	}

	// Collect results
	successCount := 0
	for range numRequests {
		if <-results == http.StatusOK {
			successCount++
		}
	}

	assert.Equal(t, numRequests, successCount, "all concurrent requests should succeed")
}

func TestReadyz_Integration_MixedHealthStatus(t *testing.T) {
	t.Parallel()

	// Start only working container
	mongo := mongoContainer.SetupContainer(t)
	mongoClient := mongoContainer.CreateConnection(t, mongo.URI, mongo.DBName)

	// Mix of healthy and n/a checkers
	checkers := []DependencyChecker{
		NewMongoChecker("mongo", mongoClient, mongo.URI),
		NewNAChecker("mongo_tenant", "tenant-scoped", false), // n/a
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
		Version:        "1.0.0",
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, 10000)
	require.NoError(t, err)

	// Should be healthy since n/a doesn't count as unhealthy
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, StatusUp, response.Checks["mongo"].Status)
	assert.Equal(t, StatusNA, response.Checks["mongo_tenant"].Status)
}

func TestReadyz_Integration_LifecycleState(t *testing.T) {
	t.Parallel()

	mongo := mongoContainer.SetupContainer(t)
	mongoClient := mongoContainer.CreateConnection(t, mongo.URI, mongo.DBName)

	checkers := []DependencyChecker{
		NewMongoChecker("mongo", mongoClient, mongo.URI),
	}

	// Create handler but do NOT mark as ready
	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
		Version:        "1.0.0",
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	// Test 1: Server not ready should return 503
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)
	assert.Equal(t, "unhealthy", response.Status)
	assert.Contains(t, response.Reason, "server not ready")

	// Test 2: Mark as ready, should return 200
	handler.SetServerReady()

	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test 3: Start drain, should return 503
	handler.StartDrain()

	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	err = json.Unmarshal(body, &response)
	require.NoError(t, err)
	assert.Equal(t, "unhealthy", response.Status)
	assert.Contains(t, response.Reason, "server draining")
}
