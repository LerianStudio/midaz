// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package testutil_integration

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/bootstrap"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg"
)

// envVarNames lists environment variables set by the test suite.
// Used for cleanup to avoid polluting the global environment.
var envVarNames = []string{
	"DB_HOST",
	"DB_PORT",
	"DB_USER",
	"DB_PASSWORD",
	"DB_NAME",
	"DB_MAX_OPEN_CONNS",
	"DB_MAX_IDLE_CONNS",
	"SERVER_PORT",
	"SERVER_ADDRESS",
	"API_KEY",
	"API_KEY_ENABLED",
	"PLUGIN_AUTH_ENABLED",
	"LOG_LEVEL",
	"OTEL_ENABLED",
	"MIGRATIONS_PATH",
	"FAULT_INJECTION_ENABLED",
	"READYZ_DRAIN_GRACE_SECONDS",
}

// savedEnvVars stores original environment variable values for restoration.
var savedEnvVars map[string]string

// saveEnvironment captures current environment variable values.
func saveEnvironment() {
	savedEnvVars = make(map[string]string)
	for _, name := range envVarNames {
		if val, exists := os.LookupEnv(name); exists {
			savedEnvVars[name] = val
		}
	}
}

// restoreEnvironment restores environment variables to their original values.
func restoreEnvironment() {
	for _, name := range envVarNames {
		if original, hadValue := savedEnvVars[name]; hadValue {
			os.Setenv(name, original)
		} else {
			os.Unsetenv(name)
		}
	}
}

// TestSuite manages the integration test environment with testcontainers.
type TestSuite struct {
	PostgresContainer *TestPostgresContainer
	ServerURL         string
	service           *bootstrap.Service
}

var globalSuite *TestSuite

// SetupTestSuite initializes the test environment with a fresh postgres container.
// Call this in TestMain to set up the environment once for all tests.
func SetupTestSuite(m *testing.M) int {
	ctx := context.Background()

	// Save current environment to restore after tests
	saveEnvironment()

	// Start postgres container
	pgContainer, err := NewTestPostgresContainer(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start postgres container: %v\n", err)
		restoreEnvironment()
		return 1
	}

	// Find free port for the server - bind to loopback only for security
	// Use port 0 to let the OS assign a free port, which minimizes race conditions.
	// The server will bind to this port immediately after startup.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find free port: %v\n", err)
		pgContainer.Terminate(ctx)
		restoreEnvironment()
		return 1
	}
	// Extract the actual port assigned by the OS
	port := listener.Addr().(*net.TCPAddr).Port
	// Close the listener to free the socket; on most systems SO_REUSEADDR is enabled
	// by default, allowing immediate reuse by the same process starting the server.
	listener.Close()

	// Get project root for migrations path
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	migrationsPath := filepath.Join(projectRoot, "migrations")

	// Set environment variables for the application
	os.Setenv("DB_HOST", pgContainer.Host)
	os.Setenv("DB_PORT", pgContainer.Port)
	os.Setenv("DB_USER", "tracer")
	os.Setenv("DB_PASSWORD", "tracer")
	os.Setenv("DB_NAME", "tracer_test")
	os.Setenv("SERVER_PORT", fmt.Sprintf("%d", port))
	os.Setenv("SERVER_ADDRESS", fmt.Sprintf("127.0.0.1:%d", port))
	os.Setenv("API_KEY", "test_api_key")
	os.Setenv("API_KEY_ENABLED", "true")
	os.Setenv("PLUGIN_AUTH_ENABLED", "false") // Disable plugin auth for integration tests
	os.Setenv("LOG_LEVEL", "error")           // Reduce noise during tests
	os.Setenv("OTEL_ENABLED", "false")
	os.Setenv("MIGRATIONS_PATH", migrationsPath)
	os.Setenv("FAULT_INJECTION_ENABLED", "true") // Enable fault injection for integration tests
	// Cap per-pool sql.DB sizing aggressively for the integration suite. The
	// production default (lib-commons: 25 max-open + 10 max-idle, applied to
	// BOTH primary AND replica pools) yields up to 50 connections per
	// bootstrap.InitServers(). RestartServerWithConfig reinitialises servers
	// dozens of times in a single `make test-integration` run; without these
	// caps cumulative connections (the previous pool's TIME_WAIT backends plus
	// the new pool's reservations) blow past Postgres' default
	// max_connections=100, surfacing as "FATAL: sorry, too many clients
	// already". 5 max-open × 2 pools = 10 conns/restart — comfortable margin
	// against 100, even with several restarts in flight under -p=1.
	os.Setenv("DB_MAX_OPEN_CONNS", "5")
	os.Setenv("DB_MAX_IDLE_CONNS", "2")
	// Shrink the readiness drain grace window during integration tests. The
	// production default (12s) honours Kubernetes endpoint propagation, but
	// tests have no probe in the loop and Shutdown is invoked with a 10s
	// context — leaving the grace at 12s causes the context to expire before
	// app.ShutdownWithContext runs, surfacing as "context deadline exceeded"
	// during test teardown / restart. Note: drainGracePeriod treats 0 (and
	// negatives) as "unset" and falls back to 12s for production safety, so
	// we use 1s here — long enough to exercise the drain path, short enough
	// to fit comfortably inside the 10s shutdown context.
	os.Setenv("READYZ_DRAIN_GRACE_SECONDS", "1")

	// Initialize local env config
	pkg.InitLocalEnvConfig()

	// Start the application server
	service, err := bootstrap.InitServers(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize servers: %v\n", err)
		pgContainer.Terminate(ctx)
		restoreEnvironment()
		return 1
	}

	go service.Run()

	// Wait for server to be ready
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitForServer(serverURL, 30*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "Server failed to start: %v\n", err)
		pgContainer.Terminate(ctx)
		restoreEnvironment()
		return 1
	}

	globalSuite = &TestSuite{
		PostgresContainer: pgContainer,
		ServerURL:         serverURL,
		service:           service,
	}

	// Override GetBaseURL to use test server
	os.Setenv("SERVER_ADDRESS", serverURL)

	// TRUNCATE protection is automatically created by migrations:
	// - Function: migrations/000003_prevent_truncate.up.sql
	// - Triggers: migrations/000004_initial_schema.up.sql
	// This ensures SOX/GLBA compliance for audit tables

	// Run tests
	code := m.Run()

	// Cleanup: shutdown service before terminating container
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if globalSuite.service != nil {
		if err := globalSuite.service.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to shutdown service: %v\n", err)
		}
	}

	// TRUNCATE protection cleanup is handled automatically by container termination
	// No need to manually drop triggers - they are part of the database schema

	pgContainer.Terminate(ctx)

	// Restore environment to avoid polluting other tests or processes
	restoreEnvironment()

	return code
}

// getTestDB creates a database connection using the test environment variables.
func getTestDB(ctx context.Context) (*sql.DB, error) {
	dsn := testutil.GetTestDSN()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	return db, nil
}

// waitForServer polls the canonical /readyz endpoint until the server is
// ready. /readyz is stricter than /health (it verifies every dependency, not
// just process liveness), so this gives integration tests an accurate signal
// that the service is ready to handle traffic — not just that the process
// answered a request. /readyz returns 200 once postgres + rule_cache are
// healthy and (in MT mode) tenant_manager + tenant_pubsub respond.
func waitForServer(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := testutil.HTTPClient.Get(baseURL + "/readyz")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("server did not become ready within %v", timeout)
}

// GetTestSuite returns the global test suite.
func GetTestSuite() *TestSuite {
	return globalSuite
}

// ServiceForTest exposes the running *bootstrap.Service so tests can inspect
// internal wiring (for example, to confirm that single-tenant mode left the
// multi-tenant components nil). Returns nil if the suite has not been
// initialised yet.
func (ts *TestSuite) ServiceForTest() *bootstrap.Service {
	if ts == nil {
		return nil
	}

	return ts.service
}

// RestartServerWithConfig stops the current server and starts a new one with different env vars.
// Returns a cleanup function to restore original config.
// WARNING: This function is NOT safe for parallel test execution.
// Tests using this function should NOT run in parallel with other tests.
func RestartServerWithConfig(envOverrides map[string]string) (cleanup func() error, err error) {
	if globalSuite == nil {
		return nil, fmt.Errorf("test suite not initialized")
	}

	ctx := context.Background()

	// Save current values for restoration (only the ones being overridden)
	// Use *string to track: nil = var didn't exist, non-nil = var existed with value
	savedValues := make(map[string]*string)
	for key := range envOverrides {
		if val, exists := os.LookupEnv(key); exists {
			savedValues[key] = &val
		} else {
			savedValues[key] = nil
		}
	}

	// Save original serverAddr and ServerURL for restoration
	originalServerURL := globalSuite.ServerURL
	originalServerAddr := strings.TrimPrefix(originalServerURL, "http://")
	originalServerAddr = strings.TrimPrefix(originalServerAddr, "https://")

	// Compute new serverAddr and ServerURL based on envOverrides
	// Start with original values
	newServerAddr := originalServerAddr
	newServerURL := originalServerURL

	// Check if SERVER_ADDRESS or SERVER_PORT is being overridden
	if addr, ok := envOverrides["SERVER_ADDRESS"]; ok {
		// SERVER_ADDRESS override - use it directly (strip scheme if present)
		newServerAddr = strings.TrimPrefix(addr, "http://")
		newServerAddr = strings.TrimPrefix(newServerAddr, "https://")
		newServerURL = "http://" + newServerAddr
	} else if port, ok := envOverrides["SERVER_PORT"]; ok {
		// SERVER_PORT override - extract host from original and combine with new port
		host := originalServerAddr
		if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
			host = host[:colonIdx]
		}
		newServerAddr = host + ":" + port
		newServerURL = "http://" + newServerAddr
	}

	// Shutdown current server
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	if err := globalSuite.service.Shutdown(shutdownCtx); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to shutdown server: %w", err)
	}
	cancel()

	// Apply new env vars
	for key, val := range envOverrides {
		os.Setenv(key, val)
	}

	// Ensure SERVER_ADDRESS is set correctly (without http:// prefix)
	os.Setenv("SERVER_ADDRESS", newServerAddr)

	// Update globalSuite.ServerURL to the new address for waitForServer
	globalSuite.ServerURL = newServerURL

	// Reinitialize config with new env vars
	pkg.InitLocalEnvConfig()

	// Start new server
	service, err := bootstrap.InitServers(ctx)
	if err != nil {
		// Restore all saved env vars on failure
		for key, val := range savedValues {
			if val == nil {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, *val)
			}
		}
		// Restore original SERVER_ADDRESS (with scheme stripped) and ServerURL
		os.Setenv("SERVER_ADDRESS", strings.TrimPrefix(strings.TrimPrefix(originalServerURL, "http://"), "https://"))
		globalSuite.ServerURL = originalServerURL
		return nil, fmt.Errorf("failed to init servers: %w", err)
	}

	go service.Run()

	// Wait for server to be ready (use newServerURL which may differ from original)
	if err := waitForServer(newServerURL, 30*time.Second); err != nil {
		// Shutdown the service we just started to avoid resource leak
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_ = service.Shutdown(shutdownCtx)
		cancel()
		// Restore all saved env vars on failure
		for key, val := range savedValues {
			if val == nil {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, *val)
			}
		}
		// Restore original SERVER_ADDRESS (with scheme stripped) and ServerURL
		os.Setenv("SERVER_ADDRESS", strings.TrimPrefix(strings.TrimPrefix(originalServerURL, "http://"), "https://"))
		globalSuite.ServerURL = originalServerURL
		return nil, fmt.Errorf("server failed to start: %w", err)
	}

	globalSuite.service = service

	// Return cleanup function that restores original config
	cleanup = func() error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := globalSuite.service.Shutdown(shutdownCtx); err != nil {
			cancel()
			return fmt.Errorf("failed to shutdown server during cleanup: %w", err)
		}
		cancel()

		// Restore original values (unset vars that didn't exist before)
		for key, val := range savedValues {
			if val == nil {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, *val)
			}
		}

		// Restore original SERVER_ADDRESS (with scheme stripped) and ServerURL
		os.Setenv("SERVER_ADDRESS", strings.TrimPrefix(strings.TrimPrefix(originalServerURL, "http://"), "https://"))
		globalSuite.ServerURL = originalServerURL

		// Reinitialize config
		pkg.InitLocalEnvConfig()

		// Restart with original config
		service, err := bootstrap.InitServers(ctx)
		if err != nil {
			return fmt.Errorf("failed to restart server with original config: %w", err)
		}

		go service.Run()

		if err := waitForServer(originalServerURL, 30*time.Second); err != nil {
			// Shutdown the service we just started to avoid resource leak
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = service.Shutdown(shutdownCtx)
			cancel()
			return fmt.Errorf("server failed to restart: %w", err)
		}

		globalSuite.service = service
		return nil
	}

	return cleanup, nil
}
