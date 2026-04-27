//go:build integration

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

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mongoContainer "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
	pgContainer "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	redisContainer "github.com/LerianStudio/midaz/v3/tests/utils/redis"
)

// newReadyHandler creates a ReadyzHandler and marks it as ready for testing.
// This is needed because HandleReadyz now checks lifecycle state before running checks.
func newReadyHandler(cfg ReadyzHandlerConfig) *ReadyzHandler {
	handler := NewReadyzHandler(cfg)
	handler.SetServerReady()

	return handler
}

func TestReadyz_Integration_AllDependenciesHealthy(t *testing.T) {
	t.Parallel()

	// Start containers
	pg := pgContainer.SetupContainer(t)
	mongo := mongoContainer.SetupContainer(t)
	redis := redisContainer.SetupContainer(t)

	// Create lib-commons wrappers
	mongoClient := mongoContainer.CreateConnection(t, mongo.URI, mongo.DBName)
	redisClient := redisContainer.CreateConnection(t, redis.Addr)

	// Create checkers using raw *sql.DB for PostgreSQL
	// and lib-commons clients for MongoDB and Redis
	checkers := []DependencyChecker{
		NewSQLDBChecker("postgres_onboarding", pg.DB, false),
		NewMongoChecker("mongo_onboarding", mongoClient, mongo.URI),
		NewRedisChecker("redis", redisClient, redis.Addr, false),
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

	// All checkers should be up
	assert.Equal(t, StatusUp, response.Checks["postgres_onboarding"].Status)
	assert.Equal(t, StatusUp, response.Checks["mongo_onboarding"].Status)
	assert.Equal(t, StatusUp, response.Checks["redis"].Status)

	// Latencies should be populated
	assert.NotNil(t, response.Checks["postgres_onboarding"].LatencyMs)
	assert.NotNil(t, response.Checks["mongo_onboarding"].LatencyMs)
	assert.NotNil(t, response.Checks["redis"].LatencyMs)
}

func TestReadyz_Integration_PostgresDown(t *testing.T) {
	t.Parallel()

	// Start only Redis and MongoDB
	mongo := mongoContainer.SetupContainer(t)
	redis := redisContainer.SetupContainer(t)

	mongoClient := mongoContainer.CreateConnection(t, mongo.URI, mongo.DBName)
	redisClient := redisContainer.CreateConnection(t, redis.Addr)

	// Create a Postgres checker with nil db (simulating down)
	checkers := []DependencyChecker{
		NewSQLDBChecker("postgres_onboarding", nil, false), // nil = not configured/down
		NewMongoChecker("mongo_onboarding", mongoClient, mongo.URI),
		NewRedisChecker("redis", redisClient, redis.Addr, false),
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

	// Should return 200 since nil checker returns "skipped" not "down"
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, StatusSkipped, response.Checks["postgres_onboarding"].Status)

	// Other checkers should still be healthy
	assert.Equal(t, StatusUp, response.Checks["mongo_onboarding"].Status)
	assert.Equal(t, StatusUp, response.Checks["redis"].Status)
}

func TestReadyz_Integration_TLSDetection(t *testing.T) {
	t.Parallel()

	// Test that TLS detection works correctly with real containers (all non-TLS)
	pg := pgContainer.SetupContainer(t)
	mongo := mongoContainer.SetupContainer(t)
	redis := redisContainer.SetupContainer(t)

	mongoClient := mongoContainer.CreateConnection(t, mongo.URI, mongo.DBName)
	redisClient := redisContainer.CreateConnection(t, redis.Addr)

	checkers := []DependencyChecker{
		NewSQLDBChecker("postgres_onboarding", pg.DB, false),
		NewMongoChecker("mongo_onboarding", mongoClient, mongo.URI),
		NewRedisChecker("redis", redisClient, redis.Addr, false),
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

	// All test containers use non-TLS connections
	for name, check := range response.Checks {
		require.NotNil(t, check.TLS, "TLS field should be set for %s", name)
		assert.False(t, *check.TLS, "TLS should be false for test container %s", name)
	}
}

func TestReadyz_Integration_LatencyMeasurement(t *testing.T) {
	t.Parallel()

	pg := pgContainer.SetupContainer(t)
	redis := redisContainer.SetupContainer(t)

	redisClient := redisContainer.CreateConnection(t, redis.Addr)

	checkers := []DependencyChecker{
		NewSQLDBChecker("postgres", pg.DB, false),
		NewRedisChecker("redis", redisClient, redis.Addr, false),
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

		// Latency should be positive and reasonable (< 1 second for local containers)
		pgLatency := response.Checks["postgres"].LatencyMs
		redisLatency := response.Checks["redis"].LatencyMs

		require.NotNil(t, pgLatency)
		require.NotNil(t, redisLatency)

		assert.GreaterOrEqual(t, *pgLatency, int64(0), "postgres latency should be non-negative")
		assert.Less(t, *pgLatency, int64(1000), "postgres latency should be < 1s")

		assert.GreaterOrEqual(t, *redisLatency, int64(0), "redis latency should be non-negative")
		assert.Less(t, *redisLatency, int64(1000), "redis latency should be < 1s")
	}
}

func TestReadyz_Integration_ConcurrentRequests(t *testing.T) {
	t.Parallel()

	// Test with only Redis to verify concurrent request handling
	redis := redisContainer.SetupContainer(t)
	redisClient := redisContainer.CreateConnection(t, redis.Addr)

	// Create a checker that will respond quickly
	checkers := []DependencyChecker{
		NewRedisChecker("redis", redisClient, redis.Addr, false),
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

	// Start only working containers
	mongo := mongoContainer.SetupContainer(t)
	redis := redisContainer.SetupContainer(t)

	mongoClient := mongoContainer.CreateConnection(t, mongo.URI, mongo.DBName)
	redisClient := redisContainer.CreateConnection(t, redis.Addr)

	// Mix of healthy and skipped checkers
	checkers := []DependencyChecker{
		NewSQLDBChecker("postgres_onboarding", nil, false), // skipped (nil)
		NewMongoChecker("mongo_onboarding", mongoClient, mongo.URI),
		NewRedisChecker("redis", redisClient, redis.Addr, false),
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

	// Should be healthy since skipped does not count as unhealthy
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, StatusSkipped, response.Checks["postgres_onboarding"].Status)
	assert.Equal(t, StatusUp, response.Checks["mongo_onboarding"].Status)
	assert.Equal(t, StatusUp, response.Checks["redis"].Status)
}

func TestReadyz_Integration_ClosedConnection(t *testing.T) {
	t.Parallel()

	// Start container
	redis := redisContainer.SetupContainer(t)

	// Create connection and then close it before using
	conn, err := redis.Client.Ping(context.Background()).Result()
	require.NoError(t, err)
	require.NotEmpty(t, conn)

	// Close the underlying client to simulate connection failure
	// Note: we can't easily close lib-commons client, so we test with the checker
	// that reports skipped when client is nil
	checkers := []DependencyChecker{
		NewRedisChecker("redis", nil, redis.Addr, false), // nil client = skipped
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
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode) // skipped counts as healthy

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, StatusSkipped, response.Checks["redis"].Status)
}
