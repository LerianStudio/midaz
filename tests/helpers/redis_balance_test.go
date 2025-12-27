// Package helpers provides test utilities for the Midaz test suite.
package helpers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBalanceKey(t *testing.T) {
	tests := []struct {
		name     string
		orgID    string
		ledgerID string
		alias    string
		key      string
		expected string
	}{
		{
			name:     "standard key format",
			orgID:    "org-123",
			ledgerID: "ledger-456",
			alias:    "@account",
			key:      "default",
			expected: "balance:{transactions}:org-123:ledger-456:@account#default",
		},
		{
			name:     "with UUID identifiers",
			orgID:    "01234567-89ab-cdef-0123-456789abcdef",
			ledgerID: "fedcba98-7654-3210-fedc-ba9876543210",
			alias:    "@myalias",
			key:      "default",
			expected: "balance:{transactions}:01234567-89ab-cdef-0123-456789abcdef:fedcba98-7654-3210-fedc-ba9876543210:@myalias#default",
		},
		{
			name:     "empty key",
			orgID:    "org",
			ledgerID: "ledger",
			alias:    "@alias",
			key:      "",
			expected: "balance:{transactions}:org:ledger:@alias#",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildBalanceKey(tt.orgID, tt.ledgerID, tt.alias, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWaitForRedisPostgresConvergence_ImmediateMatch(t *testing.T) {
	// Test convergence when PostgreSQL immediately matches expected value
	client := &RedisBalanceClient{client: nil} // nil client for unit test

	expected := decimal.RequireFromString("100.00")
	callCount := 0

	checkPostgres := func(ctx context.Context) (decimal.Decimal, error) {
		callCount++
		return expected, nil
	}

	ctx := context.Background()
	result, err := client.WaitForRedisPostgresConvergence(ctx, expected, checkPostgres, 1*time.Second)

	require.NoError(t, err)
	assert.True(t, result.Equal(expected))
	assert.Equal(t, 1, callCount, "should only call checkPostgres once on immediate match")
}

func TestWaitForRedisPostgresConvergence_EventualMatch(t *testing.T) {
	// Test convergence when PostgreSQL eventually matches after a few polls
	client := &RedisBalanceClient{client: nil}

	expected := decimal.RequireFromString("100.00")
	callCount := 0

	checkPostgres := func(ctx context.Context) (decimal.Decimal, error) {
		callCount++
		if callCount < 3 {
			return decimal.RequireFromString("50.00"), nil
		}
		return expected, nil
	}

	ctx := context.Background()
	result, err := client.WaitForRedisPostgresConvergence(ctx, expected, checkPostgres, 5*time.Second)

	require.NoError(t, err)
	assert.True(t, result.Equal(expected))
	assert.GreaterOrEqual(t, callCount, 3, "should poll multiple times before match")
}

func TestWaitForRedisPostgresConvergence_Timeout(t *testing.T) {
	// Test timeout when PostgreSQL never matches
	client := &RedisBalanceClient{client: nil}

	expected := decimal.RequireFromString("100.00")
	wrongValue := decimal.RequireFromString("50.00")

	checkPostgres := func(ctx context.Context) (decimal.Decimal, error) {
		return wrongValue, nil
	}

	ctx := context.Background()
	result, err := client.WaitForRedisPostgresConvergence(ctx, expected, checkPostgres, 200*time.Millisecond)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRedisBalanceTimeout))
	assert.True(t, result.Equal(wrongValue), "should return last value on timeout")
}

func TestWaitForRedisPostgresConvergence_ContextCancellation(t *testing.T) {
	// Test context cancellation is respected
	client := &RedisBalanceClient{client: nil}

	expected := decimal.RequireFromString("100.00")

	checkPostgres := func(ctx context.Context) (decimal.Decimal, error) {
		return decimal.RequireFromString("50.00"), nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.WaitForRedisPostgresConvergence(ctx, expected, checkPostgres, 5*time.Second)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context cancelled")
}

func TestWaitForRedisPostgresConvergence_CheckErrors(t *testing.T) {
	// Test handling of errors from checkPostgres - should continue polling
	client := &RedisBalanceClient{client: nil}

	expected := decimal.RequireFromString("100.00")
	callCount := 0
	checkError := errors.New("database connection failed")

	checkPostgres := func(ctx context.Context) (decimal.Decimal, error) {
		callCount++
		if callCount < 3 {
			return decimal.Zero, checkError
		}
		return expected, nil
	}

	ctx := context.Background()
	result, err := client.WaitForRedisPostgresConvergence(ctx, expected, checkPostgres, 5*time.Second)

	require.NoError(t, err)
	assert.True(t, result.Equal(expected), "should succeed after transient errors")
}
