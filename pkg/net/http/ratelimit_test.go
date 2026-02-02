package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/gofiber/fiber/v2"
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
