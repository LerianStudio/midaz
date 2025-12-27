// File: tests/helpers/redis_balance_chaos_test.go
package helpers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewChaosRedisClient_ReturnsClientWhenRedisAvailable(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	client, err := NewChaosRedisClient()
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	defer client.Close()

	assert.NotNil(t, client)
}

func TestChaosRedisClient_WaitForConvergenceWithFallback(t *testing.T) {
	// This test validates the helper returns gracefully when Redis unavailable
	client := &ChaosRedisHelper{client: nil}

	ctx := context.Background()
	httpClient := &HTTPClient{} // mock
	headers := map[string]string{}

	// Should not panic, should return gracefully
	err := client.WaitForConvergenceOrSkip(ctx, httpClient, "org", "ledger", "alias", "USD", headers, 100*time.Millisecond)
	assert.NoError(t, err) // Returns nil when client is nil (skip mode)
}

func TestChaosRedisClient_IsAvailable(t *testing.T) {
	tests := []struct {
		name     string
		client   *ChaosRedisHelper
		expected bool
	}{
		{
			name:     "nil client returns false",
			client:   &ChaosRedisHelper{client: nil},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.client.IsAvailable()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChaosRedisClient_CompareBalancesOrSkip_NilClient(t *testing.T) {
	client := &ChaosRedisHelper{client: nil}

	ctx := context.Background()
	httpClient := &HTTPClient{}
	headers := map[string]string{}

	comparison, err := client.CompareBalancesOrSkip(ctx, httpClient, "org", "ledger", "alias", "USD", headers)

	assert.NoError(t, err)
	assert.Nil(t, comparison, "should return nil comparison when client is nil")
}

func TestChaosRedisClient_Close_NilClient(t *testing.T) {
	client := &ChaosRedisHelper{client: nil}

	err := client.Close()
	assert.NoError(t, err, "closing nil client should not error")
}
