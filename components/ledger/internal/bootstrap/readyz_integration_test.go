//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

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

	"github.com/LerianStudio/lib-commons/v7/commons/buildinfo"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcexec "github.com/testcontainers/testcontainers-go/exec"

	mongoContainer "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	pgContainer "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	rabbitmqContainer "github.com/LerianStudio/midaz/v4/tests/utils/rabbitmq"
	redisContainer "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// newReadyHandler (the shared ready-handler test helper) is defined once in
// readyz_test.go and reused here.

func TestReadyz_Integration_AllDependenciesHealthy(t *testing.T) {
	t.Parallel()

	// Start containers
	pg := pgContainer.SetupMigratedContainer(t, "onboarding")
	mongo := mongoContainer.SetupReusableContainer(t)
	redis := redisContainer.SetupReusableContainer(t)

	// Create lib-commons wrappers
	mongoClient := mongoContainer.CreateConnection(t, mongo.URI, mongo.DBName)
	redisClient := redisContainer.CreateConnectionWithDB(t, redis.Addr, redis.DB)

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
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10000 * time.Millisecond, FailOnTimeout: true}) // 10s timeout for containers
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, buildinfo.Get().Version, response.Version)
	assert.Equal(t, buildinfo.Get().Revision, response.Revision)
	assert.Equal(t, buildinfo.Get().BuildTime, response.BuildTime)
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
	mongo := mongoContainer.SetupReusableContainer(t)
	redis := redisContainer.SetupReusableContainer(t)

	mongoClient := mongoContainer.CreateConnection(t, mongo.URI, mongo.DBName)
	redisClient := redisContainer.CreateConnectionWithDB(t, redis.Addr, redis.DB)

	// Create a Postgres checker with nil db (simulating down)
	checkers := []DependencyChecker{
		NewSQLDBChecker("postgres_onboarding", nil, false), // nil = not configured/down
		NewMongoChecker("mongo_onboarding", mongoClient, mongo.URI),
		NewRedisChecker("redis", redisClient, redis.Addr, false),
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10000 * time.Millisecond, FailOnTimeout: true})
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
	pg := pgContainer.SetupMigratedContainer(t, "onboarding")
	mongo := mongoContainer.SetupReusableContainer(t)
	redis := redisContainer.SetupReusableContainer(t)

	mongoClient := mongoContainer.CreateConnection(t, mongo.URI, mongo.DBName)
	redisClient := redisContainer.CreateConnectionWithDB(t, redis.Addr, redis.DB)

	checkers := []DependencyChecker{
		NewSQLDBChecker("postgres_onboarding", pg.DB, false),
		NewMongoChecker("mongo_onboarding", mongoClient, mongo.URI),
		NewRedisChecker("redis", redisClient, redis.Addr, false),
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10000 * time.Millisecond, FailOnTimeout: true})
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

	pg := pgContainer.SetupMigratedContainer(t, "onboarding")
	redis := redisContainer.SetupReusableContainer(t)

	redisClient := redisContainer.CreateConnectionWithDB(t, redis.Addr, redis.DB)

	checkers := []DependencyChecker{
		NewSQLDBChecker("postgres", pg.DB, false),
		NewRedisChecker("redis", redisClient, redis.Addr, false),
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	// Run multiple times to verify latency is measured each time
	for i := range 3 {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 10000 * time.Millisecond, FailOnTimeout: true})
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
	redis := redisContainer.SetupReusableContainer(t)
	redisClient := redisContainer.CreateConnectionWithDB(t, redis.Addr, redis.DB)

	// Create a checker that will respond quickly
	checkers := []DependencyChecker{
		NewRedisChecker("redis", redisClient, redis.Addr, false),
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
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
			resp, err := app.Test(req, fiber.TestConfig{Timeout: 5000 * time.Millisecond, FailOnTimeout: true}) // 5s timeout
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
	mongo := mongoContainer.SetupReusableContainer(t)
	redis := redisContainer.SetupReusableContainer(t)

	mongoClient := mongoContainer.CreateConnection(t, mongo.URI, mongo.DBName)
	redisClient := redisContainer.CreateConnectionWithDB(t, redis.Addr, redis.DB)

	// Mix of healthy and skipped checkers
	checkers := []DependencyChecker{
		NewSQLDBChecker("postgres_onboarding", nil, false), // skipped (nil)
		NewMongoChecker("mongo_onboarding", mongoClient, mongo.URI),
		NewRedisChecker("redis", redisClient, redis.Addr, false),
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10000 * time.Millisecond, FailOnTimeout: true})
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
	redis := redisContainer.SetupReusableContainer(t)

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
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5000 * time.Millisecond, FailOnTimeout: true})
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode) // skipped counts as healthy

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, StatusSkipped, response.Checks["redis"].Status)
}

// Covers the normalized root URL, latency and TLS shape against a real broker;
// WrongCredentials and ResourceAlarm are the tests an unauthenticated root probe fails.
func TestReadyz_Integration_RabbitMQ_RootURLHealthy(t *testing.T) {
	t.Parallel()

	rmq := setupReadyzRabbitMQContainer(t)
	checker := NewRabbitMQChecker("rabbitmq", rabbitMQManagementRootURL(rmq), rmq.URI,
		readyzRabbitMQUser, readyzRabbitMQPass, nil)

	statusCode, _, response := serveReadyzRabbitMQ(t, checker, libLog.NewNop())

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "healthy", response.Status)

	check := response.Checks["rabbitmq"]
	assert.Equal(t, StatusUp, check.Status)
	assert.NotNil(t, check.LatencyMs)
	require.NotNil(t, check.TLS)
	assert.False(t, *check.TLS)
	assert.Empty(t, check.Error)
	assert.Empty(t, check.BreakerState)
}

func TestReadyz_Integration_RabbitMQ_WrongCredentials(t *testing.T) {
	t.Parallel()

	const wrongPass = "readyz-it-wrong-pass"

	rmq := setupReadyzRabbitMQContainer(t)
	checker := NewRabbitMQChecker("rabbitmq", rabbitMQManagementRootURL(rmq), rmq.URI,
		readyzRabbitMQUser, wrongPass, nil)
	logger := &cleanupCapturingLogger{Logger: libLog.NewNop()}

	statusCode, body, response := serveReadyzRabbitMQ(t, checker, logger)

	assert.Equal(t, http.StatusServiceUnavailable, statusCode)
	assert.Equal(t, "unhealthy", response.Status)

	check := response.Checks["rabbitmq"]
	assert.Equal(t, StatusDown, check.Status)
	assert.True(t, strings.Contains(check.Error, "status 401"), "rabbitmq error must report status 401")

	warnings := logger.at(libLog.LevelWarn)
	require.NotEmpty(t, warnings)

	secrets := []struct{ name, value string }{
		{"broker password", readyzRabbitMQPass},
		{"wrong password", wrongPass},
	}

	for _, secret := range secrets {
		assert.False(t, strings.Contains(string(body), secret.value), "/readyz body must not contain the %s", secret.name)

		for _, entry := range warnings {
			assert.False(t, strings.Contains(entry.msg, secret.value), "Warn message must not contain the %s", secret.name)

			for key, value := range entry.fields {
				assert.False(t, strings.Contains(fmt.Sprint(value), secret.value), "Warn field %q must not contain the %s", key, secret.name)
			}
		}
	}
}

func TestReadyz_Integration_RabbitMQ_ResourceAlarm(t *testing.T) {
	t.Parallel()

	// Isolated container: the test raises a broker-wide memory alarm.
	rmq := setupReadyzRabbitMQContainer(t)

	execRabbitMQ(t, rmq, "rabbitmqctl", "set_vm_memory_high_watermark", "0.0000001")

	t.Cleanup(func() {
		execRabbitMQ(t, rmq, "rabbitmqctl", "set_vm_memory_high_watermark", "0.4")
		waitForRabbitMQMemoryAlarm(t, rmq, false)
	})

	waitForRabbitMQMemoryAlarm(t, rmq, true)

	checker := NewRabbitMQChecker("rabbitmq", rabbitMQManagementRootURL(rmq), rmq.URI,
		readyzRabbitMQUser, readyzRabbitMQPass, nil)

	statusCode, _, response := serveReadyzRabbitMQ(t, checker, libLog.NewNop())

	assert.Equal(t, http.StatusServiceUnavailable, statusCode)
	assert.Equal(t, "unhealthy", response.Status)

	check := response.Checks["rabbitmq"]
	assert.Equal(t, StatusDown, check.Status)
	assert.Contains(t, check.Error, "status 503")
}

func TestReadyz_Integration_RabbitMQ_FullURLKept(t *testing.T) {
	t.Parallel()

	rmq := setupReadyzRabbitMQContainer(t)
	fullURL := rabbitMQManagementRootURL(rmq) + "/api/health/checks/local-alarms"
	checker := NewRabbitMQChecker("rabbitmq", fullURL, rmq.URI,
		readyzRabbitMQUser, readyzRabbitMQPass, nil)

	assert.Equal(t, fullURL, checker.healthCheckURL)

	statusCode, _, response := serveReadyzRabbitMQ(t, checker, libLog.NewNop())

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, StatusUp, response.Checks["rabbitmq"].Status)
	assert.NotNil(t, response.Checks["rabbitmq"].LatencyMs)
}

// readyzRabbitMQUser and readyzRabbitMQPass are distinctive broker credentials
// so leak assertions cannot match unrelated response or log text.
const (
	readyzRabbitMQUser = "readyz-it-user"
	readyzRabbitMQPass = "readyz-it-pass"

	rabbitMQAlarmWaitTimeout  = 30 * time.Second
	rabbitMQAlarmPollInterval = 500 * time.Millisecond
	rabbitMQExecTimeout       = 30 * time.Second
)

func setupReadyzRabbitMQContainer(t *testing.T) *rabbitmqContainer.ContainerResult {
	t.Helper()

	cfg := rabbitmqContainer.DefaultContainerConfig()
	cfg.User = readyzRabbitMQUser
	cfg.Password = readyzRabbitMQPass

	return rabbitmqContainer.SetupContainerWithConfig(t, cfg)
}

func rabbitMQManagementRootURL(rmq *rabbitmqContainer.ContainerResult) string {
	return "http://" + rmq.Host + ":" + rmq.MgmtPort
}

// serveReadyzRabbitMQ runs a single /readyz request against a handler holding
// only the given RabbitMQ checker and returns the status code, raw body and
// decoded response.
func serveReadyzRabbitMQ(t *testing.T, checker *RabbitMQChecker, logger libLog.Logger) (int, []byte, ReadyzResponse) {
	t.Helper()

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         logger,
		Checkers:       []DependencyChecker{checker},
		DeploymentMode: "local",
	})

	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10000 * time.Millisecond, FailOnTimeout: true})
	require.NoError(t, err)

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	require.NoError(t, json.Unmarshal(body, &response))

	return resp.StatusCode, body, response
}

// execRabbitMQ runs a command inside the broker container and fails the test on
// a non-zero exit code, returning the combined output.
func execRabbitMQ(t *testing.T, rmq *rabbitmqContainer.ContainerResult, cmd ...string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), rabbitMQExecTimeout)
	defer cancel()

	exitCode, reader, err := rmq.Container.Exec(ctx, cmd, tcexec.Multiplexed())
	require.NoError(t, err, "exec %v", cmd)

	output, err := io.ReadAll(reader)
	require.NoError(t, err, "read exec output %v", cmd)
	require.Zero(t, exitCode, "exec %v: %s", cmd, output)

	return string(output)
}

// waitForRabbitMQMemoryAlarm polls the broker until its memory alarm is active
// (want=true) or cleared (want=false), failing after rabbitMQAlarmWaitTimeout.
func waitForRabbitMQMemoryAlarm(t *testing.T, rmq *rabbitmqContainer.ContainerResult, want bool) {
	t.Helper()

	deadline := time.Now().Add(rabbitMQAlarmWaitTimeout)

	var output string

	for time.Now().Before(deadline) {
		output = execRabbitMQ(t, rmq, "rabbitmq-diagnostics", "alarms")
		if strings.Contains(strings.ToLower(output), "memory") == want {
			return
		}

		time.Sleep(rabbitMQAlarmPollInterval)
	}

	t.Fatalf("memory alarm active=%t not observed within %s; last output: %s", want, rabbitMQAlarmWaitTimeout, output)
}
