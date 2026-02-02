package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter_WithoutRedis_RejectsRequests(t *testing.T) {
	app := fiber.New()

	// No Redis client configured - should fail-closed
	app.Use(NewRateLimiter(RateLimitConfig{
		Max:         5,
		Expiration:  time.Minute,
		RedisClient: nil, // No Redis - should reject
	}))

	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Should reject requests when Redis is unavailable (fail-closed)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var errResp RateLimitError
	err = json.NewDecoder(resp.Body).Decode(&errResp)
	require.NoError(t, err)

	assert.Equal(t, "0146", errResp.Code) // ErrRateLimitingUnavailable
	assert.Equal(t, "Rate Limiting Unavailable", errResp.Title)
}

func TestBatchRateLimiter_WithoutRedis_RejectsBatch(t *testing.T) {
	app := fiber.New()

	app.Use(NewBatchRateLimiter(BatchRateLimiterConfig{
		MaxItemsPerWindow: 10,
		Expiration:        time.Minute,
		RedisClient:       nil, // No Redis - should reject
		MaxBatchSize:      100,
	}))

	app.Post("/batch", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	batchReq := mmodel.BatchRequest{
		Requests: make([]mmodel.BatchRequestItem, 5),
	}
	for i := 0; i < 5; i++ {
		batchReq.Requests[i] = mmodel.BatchRequestItem{
			ID:     fmt.Sprintf("req-%d", i),
			Method: "GET",
			Path:   "/test",
		}
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should reject when Redis is unavailable (fail-closed)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var errResp RateLimitError
	err = json.NewDecoder(resp.Body).Decode(&errResp)
	require.NoError(t, err)

	assert.Equal(t, "0146", errResp.Code) // ErrRateLimitingUnavailable
	assert.Equal(t, "Rate Limiting Unavailable", errResp.Title)
}

func TestBatchRateLimiter_RejectsBatchSizeExceeded_WithoutRedis(t *testing.T) {
	app := fiber.New()

	app.Use(NewBatchRateLimiter(BatchRateLimiterConfig{
		MaxItemsPerWindow: 1000,
		Expiration:        time.Minute,
		RedisClient:       nil, // No Redis
		MaxBatchSize:      5,   // Max 5 items per batch
	}))

	app.Post("/batch", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Batch with 10 items - exceeds max batch size of 5
	batchReq := mmodel.BatchRequest{
		Requests: make([]mmodel.BatchRequestItem, 10),
	}
	for i := 0; i < 10; i++ {
		batchReq.Requests[i] = mmodel.BatchRequestItem{
			ID:     fmt.Sprintf("req-%d", i),
			Method: "GET",
			Path:   "/test",
		}
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Max batch size check happens before Redis check, so should still reject
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestDefaultKeyGenerator(t *testing.T) {
	app := fiber.New()

	var capturedKey string
	app.Use(func(c *fiber.Ctx) error {
		capturedKey = DefaultKeyGenerator(c)
		return c.Next()
	})

	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")

	_, err := app.Test(req, -1)
	require.NoError(t, err)

	// Key should be based on IP
	assert.NotEmpty(t, capturedKey)
}

func TestDefaultLimitReachedHandler(t *testing.T) {
	app := fiber.New()

	app.Get("/test", DefaultLimitReachedHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

	var errResp RateLimitError
	err = json.NewDecoder(resp.Body).Decode(&errResp)
	require.NoError(t, err)

	assert.Equal(t, "0139", errResp.Code) // ErrRateLimitExceeded
	assert.Equal(t, "Rate Limit Exceeded", errResp.Title)
}

func TestRateLimitConfig_DefaultKeyGenerator(t *testing.T) {
	cfg := RateLimitConfig{
		Max:        10,
		Expiration: time.Minute,
	}

	// KeyGenerator should be nil initially
	assert.Nil(t, cfg.KeyGenerator)
}

func TestBatchRateLimiterConfig_DefaultMaxBatchSize(t *testing.T) {
	// This test verifies that MaxBatchSize defaults to 100 when set to 0
	// Note: The rate limiter will reject requests when Redis is nil (fail-closed)
	// so we only test that the max batch size check happens before the Redis check

	app := fiber.New()

	// MaxBatchSize is 0, should default to 100
	// RedisClient is nil, so requests will be rejected after max batch size check passes
	app.Use(NewBatchRateLimiter(BatchRateLimiterConfig{
		MaxItemsPerWindow: 1000,
		Expiration:        time.Minute,
		RedisClient:       nil,
		MaxBatchSize:      0, // Should default to 100
	}))

	app.Post("/batch", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Batch with 101 items - should be rejected due to max batch size (default 100)
	batchReq := mmodel.BatchRequest{
		Requests: make([]mmodel.BatchRequestItem, 101),
	}
	for i := 0; i < 101; i++ {
		batchReq.Requests[i] = mmodel.BatchRequestItem{
			ID:     fmt.Sprintf("req-%d", i),
			Method: "GET",
			Path:   "/test",
		}
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should be rejected with 400 because batch size (101) exceeds default max (100)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errResp RateLimitError
	err = json.NewDecoder(resp.Body).Decode(&errResp)
	require.NoError(t, err)

	assert.Equal(t, "0140", errResp.Code) // ErrBatchSizeExceeded
}

func TestBatchRateLimiter_InvalidJSON_RejectsBadRequest(t *testing.T) {
	app := fiber.New()

	app.Use(NewBatchRateLimiter(BatchRateLimiterConfig{
		MaxItemsPerWindow: 100,
		Expiration:        time.Minute,
		RedisClient:       nil,
		MaxBatchSize:      50,
	}))

	app.Post("/batch", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Send invalid JSON - should fail-closed and reject
	req := httptest.NewRequest(http.MethodPost, "/batch", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should reject with 400 Bad Request (fail-closed)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errResp RateLimitError
	err = json.NewDecoder(resp.Body).Decode(&errResp)
	require.NoError(t, err)

	assert.Equal(t, "0142", errResp.Code) // ErrInvalidBatchRequest
	assert.Equal(t, "Invalid Batch Request", errResp.Title)
}

func TestRateLimitEnabled(t *testing.T) {
	// Default should be false
	assert.False(t, RateLimitEnabled())
}

func TestGetRateLimitMaxRequests(t *testing.T) {
	// Default should be 1000
	result := GetRateLimitMaxRequests()
	assert.Equal(t, 1000, result)
}

func TestGetRateLimitMaxBatchItems(t *testing.T) {
	// Default should be 5000
	result := GetRateLimitMaxBatchItems()
	assert.Equal(t, 5000, result)
}

func TestGetRateLimitMaxBatchSize(t *testing.T) {
	// Default should be 100
	result := GetRateLimitMaxBatchSize()
	assert.Equal(t, 100, result)
}

func TestSafeInt64(t *testing.T) {
	testCases := []struct {
		name     string
		input    any
		expected int64
		ok       bool
	}{
		{"int64", int64(42), 42, true},
		{"int", int(42), 42, true},
		{"float64", float64(42.5), 42, true}, // Truncates decimal
		{"nil", nil, 0, false},
		{"string", "42", 0, false},
		{"bool", true, 0, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, ok := safeInt64(tc.input)
			assert.Equal(t, tc.expected, result)
			assert.Equal(t, tc.ok, ok)
		})
	}
}

// setupTestRedis creates a miniredis instance for testing
func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() {
		client.Close()
		mr.Close()
	})
	return mr, client
}

func TestRateLimiter_ExactLimit(t *testing.T) {
	_, redisClient := setupTestRedis(t)
	maxRequests := 5

	app := fiber.New()
	app.Use(NewRateLimiter(RateLimitConfig{
		Max:         maxRequests,
		Expiration:  time.Minute,
		RedisClient: redisClient,
	}))

	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Make exactly Max requests - all should succeed
	for i := 0; i < maxRequests; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345" // Fixed IP for consistent key
		resp, err := app.Test(req, -1)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Request %d should succeed", i+1)

		// Verify rate limit headers
		remaining := resp.Header.Get("X-RateLimit-Remaining")
		expectedRemaining := maxRequests - (i + 1)
		assert.Equal(t, fmt.Sprintf("%d", expectedRemaining), remaining, "Request %d should have correct remaining count", i+1)
		resp.Body.Close()
	}

	// Make one more request - should be rejected
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345" // Same IP
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "Request exceeding limit should be rejected")

	var errResp RateLimitError
	err = json.NewDecoder(resp.Body).Decode(&errResp)
	require.NoError(t, err)

	assert.Equal(t, "0139", errResp.Code) // ErrRateLimitExceeded
	assert.Equal(t, "Rate Limit Exceeded", errResp.Title)

	// Verify rate limit headers on rejection
	assert.Equal(t, fmt.Sprintf("%d", maxRequests), resp.Header.Get("X-RateLimit-Limit"))
	assert.Equal(t, "0", resp.Header.Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, resp.Header.Get("Retry-After"))
	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Reset"))
}

func TestRateLimiter_WindowExpiration(t *testing.T) {
	mr, redisClient := setupTestRedis(t)
	maxRequests := 3
	windowDuration := 2 * time.Second // Short window for testing

	app := fiber.New()
	app.Use(NewRateLimiter(RateLimitConfig{
		Max:         maxRequests,
		Expiration:  windowDuration,
		RedisClient: redisClient,
	}))

	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Make Max requests - all should succeed
	for i := 0; i < maxRequests; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.2:12345" // Fixed IP for consistent key
		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Request %d should succeed", i+1)
	}

	// Verify limit is reached
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.2:12345"
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "Request should be rejected after reaching limit")

	// Fast-forward time to expire the window
	mr.FastForward(windowDuration + 100*time.Millisecond)

	// After expiration, requests should be allowed again
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.2:12345"
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Request should succeed after window expiration")

	// Verify rate limit headers show reset
	remaining := resp.Header.Get("X-RateLimit-Remaining")
	assert.Equal(t, fmt.Sprintf("%d", maxRequests-1), remaining, "Remaining should be Max-1 after first request in new window")
}

// =============================================================================
// Redis Integration Tests for Rate Limiting (Lua Script Execution Path)
// These tests use miniredis to actually execute the Lua script and verify
// the full rate limiting logic including atomic check-and-increment operations.
// =============================================================================

// TestRateLimiter_LuaScript_AtomicIncrementAndCheck tests that the Lua script
// correctly performs atomic check-and-increment operations.
func TestRateLimiter_LuaScript_AtomicIncrementAndCheck(t *testing.T) {
	mr, redisClient := setupTestRedis(t)
	_ = mr // Keep reference to mr to prevent GC

	app := fiber.New()

	app.Use(NewRateLimiter(RateLimitConfig{
		Max:         3,
		Expiration:  time.Minute,
		RedisClient: redisClient,
	}))

	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Test the atomic nature of the Lua script by making sequential requests
	// and verifying the counter increments correctly

	// Request 1: Should allow, counter becomes 1
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "2", resp.Header.Get("X-RateLimit-Remaining"))
	resp.Body.Close()

	// Request 2: Should allow, counter becomes 2
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "1", resp.Header.Get("X-RateLimit-Remaining"))
	resp.Body.Close()

	// Request 3: Should allow, counter becomes 3 (at limit)
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "0", resp.Header.Get("X-RateLimit-Remaining"))
	resp.Body.Close()

	// Request 4: Should reject, counter stays at 3
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Equal(t, "0", resp.Header.Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, resp.Header.Get("Retry-After"))
	resp.Body.Close()
}

// TestRateLimiter_LuaScript_SetsExpirationOnFirstRequest tests that the Lua script
// correctly sets the TTL only on the first request (when key doesn't exist).
func TestRateLimiter_LuaScript_SetsExpirationOnFirstRequest(t *testing.T) {
	mr, redisClient := setupTestRedis(t)

	app := fiber.New()

	windowDuration := 30 * time.Second
	// Use custom key generator to ensure unique key for this test
	app.Use(NewRateLimiter(RateLimitConfig{
		Max:         5,
		Expiration:  windowDuration,
		RedisClient: redisClient,
		KeyGenerator: func(c *fiber.Ctx) string {
			return "ttl-test-key"
		},
	}))

	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// First request creates the key with TTL
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verify the key was created with TTL
	ctx := context.Background()
	ttl, err := redisClient.TTL(ctx, "ratelimit:ttl-test-key").Result()
	require.NoError(t, err)
	assert.True(t, ttl > 0, "Key should have a positive TTL")
	assert.True(t, ttl <= windowDuration, "TTL should not exceed window duration")

	// Make another request after some "time" passes
	mr.FastForward(10 * time.Second)

	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// TTL should still be decreasing, not reset
	ttl2, err := redisClient.TTL(ctx, "ratelimit:ttl-test-key").Result()
	require.NoError(t, err)
	assert.True(t, ttl2 < ttl, "TTL should decrease between requests, not reset")
}

// TestBatchRateLimiter_LuaScript_CountsBatchItems tests that the batch rate limiter
// correctly counts individual batch items (not just the request count).
func TestBatchRateLimiter_LuaScript_CountsBatchItems(t *testing.T) {
	_, redisClient := setupTestRedis(t)

	app := fiber.New()

	app.Use(NewBatchRateLimiter(BatchRateLimiterConfig{
		MaxItemsPerWindow: 15,
		Expiration:        time.Minute,
		RedisClient:       redisClient,
		MaxBatchSize:      10,
	}))

	app.Post("/batch", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// First batch with 5 items - should succeed
	batchReq1 := mmodel.BatchRequest{
		Requests: make([]mmodel.BatchRequestItem, 5),
	}
	for i := 0; i < 5; i++ {
		batchReq1.Requests[i] = mmodel.BatchRequestItem{
			ID: fmt.Sprintf("batch1-req-%d", i), Method: "GET", Path: "/test",
		}
	}

	body1, _ := json.Marshal(batchReq1)
	req := httptest.NewRequest(http.MethodPost, "/batch", bytes.NewReader(body1))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.3:12345"
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "10", resp.Header.Get("X-RateLimit-Remaining")) // 15-5=10
	resp.Body.Close()

	// Second batch with 8 items - should succeed (5+8=13 < 15)
	batchReq2 := mmodel.BatchRequest{
		Requests: make([]mmodel.BatchRequestItem, 8),
	}
	for i := 0; i < 8; i++ {
		batchReq2.Requests[i] = mmodel.BatchRequestItem{
			ID: fmt.Sprintf("batch2-req-%d", i), Method: "GET", Path: "/test",
		}
	}

	body2, _ := json.Marshal(batchReq2)
	req = httptest.NewRequest(http.MethodPost, "/batch", bytes.NewReader(body2))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.3:12345"
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "2", resp.Header.Get("X-RateLimit-Remaining")) // 15-13=2
	resp.Body.Close()

	// Third batch with 5 items - should be rejected (13+5=18 > 15)
	batchReq3 := mmodel.BatchRequest{
		Requests: make([]mmodel.BatchRequestItem, 5),
	}
	for i := 0; i < 5; i++ {
		batchReq3.Requests[i] = mmodel.BatchRequestItem{
			ID: fmt.Sprintf("batch3-req-%d", i), Method: "GET", Path: "/test",
		}
	}

	body3, _ := json.Marshal(batchReq3)
	req = httptest.NewRequest(http.MethodPost, "/batch", bytes.NewReader(body3))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.3:12345"
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	resp.Body.Close()
}

// TestRateLimiter_LuaScript_DifferentKeysIndependent tests that rate limits
// for different keys (different IPs/users) are tracked independently.
func TestRateLimiter_LuaScript_DifferentKeysIndependent(t *testing.T) {
	_, redisClient := setupTestRedis(t)

	// Track which user key to use based on request header
	app := fiber.New()

	app.Use(NewRateLimiter(RateLimitConfig{
		Max:         2,
		Expiration:  time.Minute,
		RedisClient: redisClient,
		KeyGenerator: func(c *fiber.Ctx) string {
			// Use custom header to distinguish users since RemoteAddr doesn't work in tests
			return c.Get("X-User-ID")
		},
	}))

	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// User 1: Make 2 requests (at limit)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-User-ID", "user-1")
		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()
	}

	// User 1: 3rd request should be rejected
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-User-ID", "user-1")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	resp.Body.Close()

	// User 2: Should still have full quota (independent counter)
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-User-ID", "user-2")
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "1", resp.Header.Get("X-RateLimit-Remaining")) // First request for User 2
	resp.Body.Close()
}

// TestRateLimiter_LuaScript_ConcurrentRequests tests that the Lua script handles
// concurrent requests correctly without race conditions.
func TestRateLimiter_LuaScript_ConcurrentRequests(t *testing.T) {
	_, redisClient := setupTestRedis(t)

	app := fiber.New()

	maxRequests := 10
	app.Use(NewRateLimiter(RateLimitConfig{
		Max:         maxRequests,
		Expiration:  time.Minute,
		RedisClient: redisClient,
	}))

	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Make concurrent requests
	concurrency := 20
	successCount := 0
	failCount := 0
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = "10.0.0.4:12345" // Same IP for all requests
			resp, err := app.Test(req, -1)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			mu.Lock()
			if resp.StatusCode == http.StatusOK {
				successCount++
			} else if resp.StatusCode == http.StatusTooManyRequests {
				failCount++
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Exactly maxRequests should succeed, rest should fail
	assert.Equal(t, maxRequests, successCount, "Exactly %d requests should succeed", maxRequests)
	assert.Equal(t, concurrency-maxRequests, failCount, "Remaining %d requests should be rejected", concurrency-maxRequests)
}

// TestBatchRateLimiter_LuaScript_PartialBatchDoesNotIncrementOnReject tests that
// when a batch is rejected, the counter is NOT incremented (atomic behavior).
func TestBatchRateLimiter_LuaScript_PartialBatchDoesNotIncrementOnReject(t *testing.T) {
	_, redisClient := setupTestRedis(t)

	app := fiber.New()

	app.Use(NewBatchRateLimiter(BatchRateLimiterConfig{
		MaxItemsPerWindow: 10,
		Expiration:        time.Minute,
		RedisClient:       redisClient,
		MaxBatchSize:      20,
	}))

	app.Post("/batch", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// First batch with 8 items - should succeed
	batchReq1 := mmodel.BatchRequest{
		Requests: make([]mmodel.BatchRequestItem, 8),
	}
	for i := 0; i < 8; i++ {
		batchReq1.Requests[i] = mmodel.BatchRequestItem{
			ID: fmt.Sprintf("req-%d", i), Method: "GET", Path: "/test",
		}
	}

	body1, _ := json.Marshal(batchReq1)
	req := httptest.NewRequest(http.MethodPost, "/batch", bytes.NewReader(body1))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.5:12345"
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "2", resp.Header.Get("X-RateLimit-Remaining")) // 10-8=2
	resp.Body.Close()

	// Second batch with 5 items - should be rejected (8+5=13 > 10)
	batchReq2 := mmodel.BatchRequest{
		Requests: make([]mmodel.BatchRequestItem, 5),
	}
	for i := 0; i < 5; i++ {
		batchReq2.Requests[i] = mmodel.BatchRequestItem{
			ID: fmt.Sprintf("req2-%d", i), Method: "GET", Path: "/test",
		}
	}

	body2, _ := json.Marshal(batchReq2)
	req = httptest.NewRequest(http.MethodPost, "/batch", bytes.NewReader(body2))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.5:12345"
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	// Remaining should still be 2 (not 0) because rejected batch doesn't increment
	assert.Equal(t, "2", resp.Header.Get("X-RateLimit-Remaining"))
	resp.Body.Close()

	// A smaller batch with 2 items should still succeed
	batchReq3 := mmodel.BatchRequest{
		Requests: make([]mmodel.BatchRequestItem, 2),
	}
	for i := 0; i < 2; i++ {
		batchReq3.Requests[i] = mmodel.BatchRequestItem{
			ID: fmt.Sprintf("req3-%d", i), Method: "GET", Path: "/test",
		}
	}

	body3, _ := json.Marshal(batchReq3)
	req = httptest.NewRequest(http.MethodPost, "/batch", bytes.NewReader(body3))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.5:12345"
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "0", resp.Header.Get("X-RateLimit-Remaining")) // 10-8-2=0
	resp.Body.Close()
}
