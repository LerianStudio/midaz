package http

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

//go:embed scripts/rate_limit_check_and_increment.lua
var rateLimitCheckAndIncrementLua string

// RateLimitConfig holds configuration for rate limiting middleware.
type RateLimitConfig struct {
	// Max requests per window
	Max int
	// Window duration
	Expiration time.Duration
	// Function to generate unique key for rate limiting (e.g., by IP, user ID)
	KeyGenerator func(*fiber.Ctx) string
	// Handler called when rate limit is exceeded
	LimitReached fiber.Handler
	// Skip failed requests from counting
	SkipFailedRequests bool
	// Redis client for distributed rate limiting
	RedisClient *redis.Client
}

// RateLimitError represents a rate limit exceeded error.
type RateLimitError struct {
	Code       string `json:"code"`
	Title      string `json:"title"`
	Message    string `json:"message"`
	RetryAfter int    `json:"retryAfter,omitempty"`
}

// Error implements the error interface.
func (e RateLimitError) Error() string {
	return e.Message
}

// DefaultKeyGenerator generates a rate limit key based on client IP.
func DefaultKeyGenerator(c *fiber.Ctx) string {
	return c.IP()
}

// DefaultLimitReachedHandler returns a 429 Too Many Requests response.
func DefaultLimitReachedHandler(c *fiber.Ctx) error {
	return c.Status(http.StatusTooManyRequests).JSON(RateLimitError{
		Code:    constant.ErrRateLimitExceeded.Error(),
		Title:   "Rate Limit Exceeded",
		Message: "You have exceeded the rate limit. Please try again later.",
	})
}

// safeInt64 safely extracts an int64 from an interface{} value.
// Redis Lua scripts may return different numeric types depending on the Redis version.
func safeInt64(v any) (int64, bool) {
	switch val := v.(type) {
	case int64:
		return val, true
	case int:
		return int64(val), true
	case float64:
		return int64(val), true
	case nil:
		return 0, false
	default:
		return 0, false
	}
}

// NewRateLimiter creates a rate limiting middleware using Redis for distributed counting.
func NewRateLimiter(cfg RateLimitConfig) fiber.Handler {
	if cfg.KeyGenerator == nil {
		cfg.KeyGenerator = DefaultKeyGenerator
	}

	if cfg.LimitReached == nil {
		cfg.LimitReached = DefaultLimitReachedHandler
	}

	return func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

		ctx, span := tracer.Start(ctx, "middleware.rate_limiter")
		defer span.End()

		if cfg.RedisClient == nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Rate limiter: Redis client not configured", fmt.Errorf("rate limiting unavailable"))
			logger.Error("Rate limiter: Redis client not configured, rejecting request (fail-closed)")

			return c.Status(http.StatusServiceUnavailable).JSON(RateLimitError{
				Code:    constant.ErrRateLimitingUnavailable.Error(),
				Title:   "Rate Limiting Unavailable",
				Message: "Rate limiting service is unavailable. Request rejected for safety.",
			})
		}

		key := fmt.Sprintf("ratelimit:%s", cfg.KeyGenerator(c))

		// Use atomic Lua script for check-and-increment
		script := redis.NewScript(rateLimitCheckAndIncrementLua)
		result, err := script.Run(ctx, cfg.RedisClient, []string{key}, 1, cfg.Max, int(cfg.Expiration.Seconds())).Result()
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to execute rate limit script", err)
			logger.Errorf("Rate limiter: failed to execute script: %v", err)

			// Fail-closed: reject request when Redis is unavailable
			return c.Status(http.StatusServiceUnavailable).JSON(RateLimitError{
				Code:    constant.ErrRateLimitingUnavailable.Error(),
				Title:   "Rate Limiting Unavailable",
				Message: "Rate limiting service is unavailable. Request rejected for safety.",
			})
		}

		// Parse Lua script result: {allowed, count, ttl}
		results, ok := result.([]any)
		if !ok || len(results) != 3 {
			libOpentelemetry.HandleSpanError(&span, "Invalid rate limit script result", fmt.Errorf("unexpected result format"))
			logger.Errorf("Rate limiter: invalid script result format")

			return c.Status(http.StatusServiceUnavailable).JSON(RateLimitError{
				Code:    constant.ErrRateLimitingUnavailable.Error(),
				Title:   "Rate Limiting Unavailable",
				Message: "Rate limiting service error. Request rejected for safety.",
			})
		}

		allowed, okAllowed := safeInt64(results[0])
		count, okCount := safeInt64(results[1])
		ttlSeconds, okTTL := safeInt64(results[2])

		if !okAllowed || !okCount || !okTTL {
			libOpentelemetry.HandleSpanError(&span, "Invalid rate limit result types",
				fmt.Errorf("type assertion failed: allowed=%T, count=%T, ttl=%T", results[0], results[1], results[2]))
			logger.Errorf("Rate limiter: invalid result types from Redis script")

			return c.Status(http.StatusServiceUnavailable).JSON(RateLimitError{
				Code:    constant.ErrRateLimitingUnavailable.Error(),
				Title:   "Rate Limiting Unavailable",
				Message: "Rate limiting service error. Request rejected for safety.",
			})
		}

		// Check if limit exceeded
		if allowed == 0 {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Rate limit exceeded", fmt.Errorf("rate limit exceeded: %d/%d", count, cfg.Max))
			logger.Warnf("Rate limit exceeded for key %s: %d/%d", key, count, cfg.Max)

			// Rate limit exceeded
			ttl := time.Duration(ttlSeconds) * time.Second
			c.Set("Retry-After", fmt.Sprintf("%d", int(ttlSeconds)))
			c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.Max))
			c.Set("X-RateLimit-Remaining", "0")
			c.Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(ttl).Unix()))

			return cfg.LimitReached(c)
		}

		// Set rate limit headers
		ttl := time.Duration(ttlSeconds) * time.Second
		c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.Max))
		c.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", cfg.Max-int(count)))
		c.Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(ttl).Unix()))

		return c.Next()
	}
}

// BatchRateLimiterConfig holds configuration for batch-specific rate limiting.
type BatchRateLimiterConfig struct {
	// Maximum batch items per window
	MaxItemsPerWindow int
	// Window duration
	Expiration time.Duration
	// Function to generate unique key for rate limiting
	KeyGenerator func(*fiber.Ctx) string
	// Redis client for distributed rate limiting
	RedisClient *redis.Client
	// Maximum batch size per request
	MaxBatchSize int
}

// NewBatchRateLimiter creates a rate limiter that counts batch items instead of requests.
// This ensures fair usage by counting the actual number of operations, not just HTTP requests.
func NewBatchRateLimiter(cfg BatchRateLimiterConfig) fiber.Handler {
	if cfg.KeyGenerator == nil {
		cfg.KeyGenerator = DefaultKeyGenerator
	}

	if cfg.MaxBatchSize <= 0 {
		cfg.MaxBatchSize = 100 // Default max batch size
	}

	return func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

		ctx, span := tracer.Start(ctx, "middleware.batch_rate_limiter")
		defer span.End()

		// Parse batch request to count items
		var batchReq mmodel.BatchRequest
		if err := json.Unmarshal(c.Body(), &batchReq); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to parse batch request for rate limiting", err)
			// Fail-closed: reject unparseable requests to prevent bypass
			return c.Status(http.StatusBadRequest).JSON(RateLimitError{
				Code:    constant.ErrInvalidBatchRequest.Error(),
				Title:   "Invalid Batch Request",
				Message: "Failed to parse batch request body",
			})
		}

		// Store parsed batch request in context to avoid double parsing in WithBody middleware
		c.Locals("batchRequest", &batchReq)

		itemCount := len(batchReq.Requests)

		// Check max batch size
		if itemCount > cfg.MaxBatchSize {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Batch size exceeded", fmt.Errorf("batch size %d exceeds max %d", itemCount, cfg.MaxBatchSize))

			return c.Status(http.StatusBadRequest).JSON(RateLimitError{
				Code:    constant.ErrBatchSizeExceeded.Error(),
				Title:   "Batch Size Exceeded",
				Message: fmt.Sprintf("Batch size %d exceeds maximum allowed size of %d", itemCount, cfg.MaxBatchSize),
			})
		}

		if cfg.RedisClient == nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Batch rate limiter: Redis client not configured", fmt.Errorf("rate limiting unavailable"))
			logger.Error("Batch rate limiter: Redis client not configured, rejecting request (fail-closed)")

			return c.Status(http.StatusServiceUnavailable).JSON(RateLimitError{
				Code:    constant.ErrRateLimitingUnavailable.Error(),
				Title:   "Rate Limiting Unavailable",
				Message: "Rate limiting service is unavailable. Request rejected for safety.",
			})
		}

		key := fmt.Sprintf("batchratelimit:%s", cfg.KeyGenerator(c))

		// Use atomic Lua script for check-and-increment
		script := redis.NewScript(rateLimitCheckAndIncrementLua)
		result, err := script.Run(ctx, cfg.RedisClient, []string{key}, itemCount, cfg.MaxItemsPerWindow, int(cfg.Expiration.Seconds())).Result()
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to execute batch rate limit script", err)
			logger.Errorf("Batch rate limiter: failed to execute script: %v", err)

			// Fail-closed: reject request when Redis is unavailable
			return c.Status(http.StatusServiceUnavailable).JSON(RateLimitError{
				Code:    constant.ErrRateLimitingUnavailable.Error(),
				Title:   "Rate Limiting Unavailable",
				Message: "Rate limiting service is unavailable. Request rejected for safety.",
			})
		}

		// Parse Lua script result: {allowed, count, ttl}
		results, ok := result.([]any)
		if !ok || len(results) != 3 {
			libOpentelemetry.HandleSpanError(&span, "Invalid batch rate limit script result", fmt.Errorf("unexpected result format"))
			logger.Errorf("Batch rate limiter: invalid script result format")

			return c.Status(http.StatusServiceUnavailable).JSON(RateLimitError{
				Code:    constant.ErrRateLimitingUnavailable.Error(),
				Title:   "Rate Limiting Unavailable",
				Message: "Rate limiting service error. Request rejected for safety.",
			})
		}

		allowed, okAllowed := safeInt64(results[0])
		currentCount, okCount := safeInt64(results[1])
		ttlSeconds, okTTL := safeInt64(results[2])

		if !okAllowed || !okCount || !okTTL {
			libOpentelemetry.HandleSpanError(&span, "Invalid batch rate limit result types",
				fmt.Errorf("type assertion failed: allowed=%T, count=%T, ttl=%T", results[0], results[1], results[2]))
			logger.Errorf("Batch rate limiter: invalid result types from Redis script")

			return c.Status(http.StatusServiceUnavailable).JSON(RateLimitError{
				Code:    constant.ErrRateLimitingUnavailable.Error(),
				Title:   "Rate Limiting Unavailable",
				Message: "Rate limiting service error. Request rejected for safety.",
			})
		}

		// Check if limit exceeded
		if allowed == 0 {
			// When denied, currentCount from Lua script is the count BEFORE the attempted increment
			countBefore := int(currentCount)
			remaining := cfg.MaxItemsPerWindow - countBefore
			if remaining < 0 {
				remaining = 0
			}

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Batch rate limit exceeded",
				fmt.Errorf("batch items would exceed limit: current=%d, requested=%d, max=%d", countBefore, itemCount, cfg.MaxItemsPerWindow))
			logger.Warnf("Batch rate limit exceeded for key %s: current=%d, requested=%d, max=%d",
				key, countBefore, itemCount, cfg.MaxItemsPerWindow)

			ttl := time.Duration(ttlSeconds) * time.Second
			c.Set("Retry-After", fmt.Sprintf("%d", int(ttlSeconds)))
			c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.MaxItemsPerWindow))
			c.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			c.Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(ttl).Unix()))

			return c.Status(http.StatusTooManyRequests).JSON(RateLimitError{
				Code:       constant.ErrBatchRateLimitExceeded.Error(),
				Title:      "Batch Rate Limit Exceeded",
				Message:    fmt.Sprintf("Adding %d items would exceed the rate limit. Current: %d, Max: %d per window. Remaining: %d", itemCount, countBefore, cfg.MaxItemsPerWindow, remaining),
				RetryAfter: int(ttlSeconds),
			})
		}

		// Set rate limit headers - when allowed, currentCount is the NEW count after increment
		remaining := cfg.MaxItemsPerWindow - int(currentCount)
		if remaining < 0 {
			remaining = 0
		}

		ttl := time.Duration(ttlSeconds) * time.Second
		c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.MaxItemsPerWindow))
		c.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(ttl).Unix()))

		logger.Infof("Batch rate limit: key=%s, items=%d, total=%d, max=%d", key, itemCount, currentCount, cfg.MaxItemsPerWindow)

		return c.Next()
	}
}

// RateLimitEnabled checks if rate limiting is enabled via environment variable.
func RateLimitEnabled() bool {
	return libCommons.GetenvBoolOrDefault("RATE_LIMIT_ENABLED", false)
}

// GetRateLimitMaxRequests returns the maximum requests per minute from environment.
func GetRateLimitMaxRequests() int {
	return libCommons.SafeInt64ToInt(libCommons.GetenvIntOrDefault("RATE_LIMIT_MAX_REQUESTS_PER_MINUTE", 1000))
}

// GetRateLimitMaxBatchItems returns the maximum batch items per minute from environment.
func GetRateLimitMaxBatchItems() int {
	return libCommons.SafeInt64ToInt(libCommons.GetenvIntOrDefault("RATE_LIMIT_MAX_BATCH_ITEMS_PER_MINUTE", 5000))
}

// GetRateLimitMaxBatchSize returns the maximum batch size per request from environment.
func GetRateLimitMaxBatchSize() int {
	return libCommons.SafeInt64ToInt(libCommons.GetenvIntOrDefault("RATE_LIMIT_MAX_BATCH_SIZE", 100))
}
