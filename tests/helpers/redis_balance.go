// Package helpers provides test utilities for the Midaz test suite.
//
// SECURITY NOTICE: These helpers are designed for LOCAL TESTING ONLY
// against Docker containers. They do NOT support production-grade security
// configurations (password auth, TLS encryption, IAM). DO NOT use for production.
//
// TODO(review): Add unit tests for buildBalanceKey, convergence wait, error handling (reported by code-reviewer and business-logic-reviewer on 2025-12-14, severity: Medium)
package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

const (
	// redisBalanceTimeout is the maximum time to wait for Redis+PostgreSQL convergence
	redisBalanceTimeout = 30 * time.Second
	// redisBalancePollInterval is the interval between convergence checks
	// TODO(review): Standardize poll interval across helpers (balances.go uses 150ms, cache.go uses 100ms) (reported by code-reviewer on 2025-12-14, severity: Medium)
	redisBalancePollInterval = 100 * time.Millisecond
	// redisConnectionTimeout is the maximum time to wait for initial Redis connection
	redisConnectionTimeout = 5 * time.Second
)

var (
	// ErrRedisBalanceTimeout indicates timeout waiting for Redis+PostgreSQL convergence
	ErrRedisBalanceTimeout = errors.New("timeout waiting for Redis+PostgreSQL convergence")
	// ErrRedisBalanceNotFound indicates the balance was not found in Redis
	ErrRedisBalanceNotFound = errors.New("balance not found in Redis")
	// ErrRedisBalanceUnmarshal indicates failure to unmarshal Redis balance data
	ErrRedisBalanceUnmarshal = errors.New("failed to unmarshal Redis balance data")
)

// RedisBalanceClient provides methods to read balance data from Redis
// and wait for convergence between Redis and PostgreSQL
type RedisBalanceClient struct {
	client *redis.Client
}

// NewRedisBalanceClient creates a new RedisBalanceClient and verifies connectivity
func NewRedisBalanceClient(addr string) (*RedisBalanceClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	ctx, cancel := context.WithTimeout(context.Background(), redisConnectionTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("failed to connect to Redis at %s: %w", addr, err)
	}

	return &RedisBalanceClient{
		client: client,
	}, nil
}

// Close closes the Redis client connection
func (r *RedisBalanceClient) Close() error {
	if r.client != nil {
		if err := r.client.Close(); err != nil {
			//nolint:wrapcheck // Error already wrapped with context for test helpers
			return fmt.Errorf("failed to close Redis client: %w", err)
		}
	}

	return nil
}

// buildBalanceKey constructs the Redis key for a balance
// Format: balance:{transactions}:orgID:ledgerID:@alias#key
func buildBalanceKey(orgID, ledgerID, alias, key string) string {
	return fmt.Sprintf("balance:{transactions}:%s:%s:%s#%s", orgID, ledgerID, alias, key)
}

// GetBalanceFromRedis retrieves a balance from Redis by its key components
// Returns nil without error if the balance is not found (redis.Nil)
func (r *RedisBalanceClient) GetBalanceFromRedis(ctx context.Context, orgID, ledgerID, alias, key string) (*mmodel.BalanceRedis, error) {
	redisKey := buildBalanceKey(orgID, ledgerID, alias, key)

	value, err := r.client.Get(ctx, redisKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Balance not found in Redis - return nil without error
			return nil, nil
		}

		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("failed to get balance from Redis key %s: %w", redisKey, err)
	}

	var balance mmodel.BalanceRedis
	if err := json.Unmarshal([]byte(value), &balance); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("%w for key %s: %w", ErrRedisBalanceUnmarshal, redisKey, err)
	}

	return &balance, nil
}

// GetBalanceFromRedisByFullKey retrieves a balance using the full Redis key
// Returns nil without error if the balance is not found (redis.Nil)
func (r *RedisBalanceClient) GetBalanceFromRedisByFullKey(ctx context.Context, redisKey string) (*mmodel.BalanceRedis, error) {
	value, err := r.client.Get(ctx, redisKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}

		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("failed to get balance from Redis key %s: %w", redisKey, err)
	}

	var balance mmodel.BalanceRedis
	if err := json.Unmarshal([]byte(value), &balance); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("%w for key %s: %w", ErrRedisBalanceUnmarshal, redisKey, err)
	}

	return &balance, nil
}

// ConvergenceCheck defines the function signature for checking if PostgreSQL has converged
// It should return the current PostgreSQL available balance
type ConvergenceCheck func(ctx context.Context) (decimal.Decimal, error)

// WaitForRedisPostgresConvergence polls until PostgreSQL matches the expected balance
// or until the timeout is reached. This is useful during chaos testing when Redis
// (source of truth) may be correct but PostgreSQL sync lags behind.
//
// Parameters:
//   - ctx: Context for cancellation
//   - expectedAvailable: The expected available balance (typically from Redis)
//   - checkPostgres: Function that returns the current PostgreSQL available balance
//   - timeout: Maximum time to wait for convergence (0 uses default of 30s)
//
// Returns:
//   - The final PostgreSQL available balance
//   - Error if timeout occurs or if checkPostgres returns an error
func (r *RedisBalanceClient) WaitForRedisPostgresConvergence(
	ctx context.Context,
	expectedAvailable decimal.Decimal,
	checkPostgres ConvergenceCheck,
	timeout time.Duration,
) (decimal.Decimal, error) {
	if timeout == 0 {
		timeout = redisBalanceTimeout
	}

	deadline := time.Now().Add(timeout)

	var (
		lastValue decimal.Decimal
		lastErr   error
	)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			//nolint:wrapcheck // Error already wrapped with context for test helpers
			return lastValue, fmt.Errorf("context cancelled while waiting for convergence: %w", ctx.Err())
		default:
		}

		pgValue, err := checkPostgres(ctx)
		if err != nil {
			lastErr = err

			time.Sleep(redisBalancePollInterval)

			continue
		}

		lastValue = pgValue
		lastErr = nil

		if pgValue.Equal(expectedAvailable) {
			return pgValue, nil
		}

		time.Sleep(redisBalancePollInterval)
	}

	if lastErr != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return lastValue, fmt.Errorf("%w: last error: %w, expected=%s, last=%s",
			ErrRedisBalanceTimeout, lastErr, expectedAvailable.String(), lastValue.String())
	}

	//nolint:wrapcheck // Error already wrapped with context for test helpers
	return lastValue, fmt.Errorf("%w: expected=%s, got=%s",
		ErrRedisBalanceTimeout, expectedAvailable.String(), lastValue.String())
}

// WaitForRedisPostgresConvergenceWithHTTP is a convenience method that combines
// Redis balance lookup with HTTP-based PostgreSQL checking
//
// Parameters:
//   - ctx: Context for cancellation
//   - httpClient: HTTP client for checking PostgreSQL via API
//   - orgID, ledgerID, alias, assetCode: Identifiers for the balance
//   - headers: HTTP headers for authentication
//   - timeout: Maximum time to wait (0 uses default)
//
// Returns:
//   - The Redis balance if found and converged
//   - Error if Redis balance not found, timeout, or API errors
func (r *RedisBalanceClient) WaitForRedisPostgresConvergenceWithHTTP(
	ctx context.Context,
	httpClient *HTTPClient,
	orgID, ledgerID, alias, assetCode string,
	headers map[string]string,
	timeout time.Duration,
) (*mmodel.BalanceRedis, error) {
	// First get the Redis balance (source of truth)
	// TODO(review): Make balance key configurable instead of hardcoded "default" (reported by business-logic-reviewer on 2025-12-14, severity: Low)
	redisBalance, err := r.GetBalanceFromRedis(ctx, orgID, ledgerID, alias, "default")
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("failed to get Redis balance: %w", err)
	}

	if redisBalance == nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("%w for %s in org=%s ledger=%s",
			ErrRedisBalanceNotFound, alias, orgID, ledgerID)
	}

	// Create a check function using the HTTP client
	checkPostgres := func(ctx context.Context) (decimal.Decimal, error) {
		return GetAvailableSumByAlias(ctx, httpClient, orgID, ledgerID, alias, assetCode, headers)
	}

	// Wait for PostgreSQL to converge to Redis value
	_, err = r.WaitForRedisPostgresConvergence(ctx, redisBalance.Available, checkPostgres, timeout)
	if err != nil {
		return redisBalance, err
	}

	return redisBalance, nil
}

// CompareRedisPostgres compares Redis balance with PostgreSQL balance
// Returns the difference (Redis - PostgreSQL) and both values
type BalanceComparison struct {
	RedisAvailable    decimal.Decimal
	PostgresAvailable decimal.Decimal
	Difference        decimal.Decimal
	IsConverged       bool
}

// CompareBalances fetches balances from both Redis and PostgreSQL and compares them
func (r *RedisBalanceClient) CompareBalances(
	ctx context.Context,
	httpClient *HTTPClient,
	orgID, ledgerID, alias, assetCode string,
	headers map[string]string,
) (*BalanceComparison, error) {
	// Get Redis balance
	redisBalance, err := r.GetBalanceFromRedis(ctx, orgID, ledgerID, alias, "default")
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("failed to get Redis balance: %w", err)
	}

	redisAvailable := decimal.Zero
	if redisBalance != nil {
		redisAvailable = redisBalance.Available
	}

	// Get PostgreSQL balance via HTTP API
	pgAvailable, err := GetAvailableSumByAlias(ctx, httpClient, orgID, ledgerID, alias, assetCode, headers)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("failed to get PostgreSQL balance: %w", err)
	}

	return &BalanceComparison{
		RedisAvailable:    redisAvailable,
		PostgresAvailable: pgAvailable,
		Difference:        redisAvailable.Sub(pgAvailable),
		IsConverged:       redisAvailable.Equal(pgAvailable),
	}, nil
}
