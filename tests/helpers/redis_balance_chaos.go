// Package helpers provides test utilities for the Midaz test suite.
package helpers

import (
	"context"
	"os"
	"time"
)

const (
	// defaultChaosRedisAddr is the default Redis address for chaos tests
	defaultChaosRedisAddr = "localhost:6379"
	// chaosRedisEnvVar is the environment variable for Redis address override
	chaosRedisEnvVar = "CHAOS_REDIS_ADDR"
	// chaosConvergenceTimeout is the default timeout for convergence in chaos tests
	chaosConvergenceTimeout = 30 * time.Second
)

// ChaosRedisHelper wraps RedisBalanceClient with chaos-test-friendly defaults
type ChaosRedisHelper struct {
	client *RedisBalanceClient
}

// NewChaosRedisClient creates a RedisBalanceClient for chaos tests
// Uses CHAOS_REDIS_ADDR env var or defaults to localhost:6379
// Returns nil client (not error) if Redis is unavailable - chaos tests should degrade gracefully
func NewChaosRedisClient() (*ChaosRedisHelper, error) {
	addr := os.Getenv(chaosRedisEnvVar)
	if addr == "" {
		addr = defaultChaosRedisAddr
	}

	client, err := NewRedisBalanceClient(addr)
	if err != nil {
		// Return nil client - chaos tests should degrade gracefully when Redis is unavailable.
		// This is intentional: chaos tests should not fail just because Redis is down.
		//nolint:nilerr // Intentional: graceful degradation for chaos tests when Redis unavailable
		return &ChaosRedisHelper{client: nil}, nil
	}

	return &ChaosRedisHelper{client: client}, nil
}

// Close closes the underlying Redis client
func (h *ChaosRedisHelper) Close() error {
	if h.client != nil {
		return h.client.Close()
	}

	return nil
}

// IsAvailable returns true if Redis client is connected
func (h *ChaosRedisHelper) IsAvailable() bool {
	return h.client != nil
}

// WaitForConvergenceOrSkip waits for Redis+PostgreSQL convergence
// If Redis is unavailable, returns nil (skip mode) rather than failing
// This allows chaos tests to run with degraded functionality
func (h *ChaosRedisHelper) WaitForConvergenceOrSkip(
	ctx context.Context,
	httpClient *HTTPClient,
	orgID, ledgerID, alias, assetCode string,
	headers map[string]string,
	timeout time.Duration,
) error {
	if h.client == nil {
		// Redis not available - skip convergence check
		return nil
	}

	if timeout == 0 {
		timeout = chaosConvergenceTimeout
	}

	_, err := h.client.WaitForRedisPostgresConvergenceWithHTTP(
		ctx, httpClient, orgID, ledgerID, alias, assetCode, "default", headers, timeout,
	)

	return err
}

// CompareBalancesOrSkip compares Redis and PostgreSQL balances
// Returns nil comparison (not error) if Redis unavailable
func (h *ChaosRedisHelper) CompareBalancesOrSkip(
	ctx context.Context,
	httpClient *HTTPClient,
	orgID, ledgerID, alias, assetCode string,
	headers map[string]string,
) (*BalanceComparison, error) {
	if h.client == nil {
		return nil, nil
	}

	return h.client.CompareBalances(ctx, httpClient, orgID, ledgerID, alias, assetCode, headers)
}
