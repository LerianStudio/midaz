// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"crypto/subtle"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// securityLogger is a package-level structured logger for security-related events.
var securityLogger libLog.Logger = &libLog.GoLogger{Level: libLog.WarnLevel}

const deniedCORSOrigin = "https://denied.invalid"

// SwaggerTokenCookieName is the cookie name used for Swagger UI authentication.
// ShardingControlTokenHeader is the HTTP header name used for sharding admin authentication.
const (
	SwaggerTokenCookieName     = "midaz_swagger_token"
	ShardingControlTokenHeader = "X-Sharding-Token"
)

const knownPlaceholderToken = "change-me-to-a-32-char-or-longer-secret"

const (
	defaultSwaggerRateLimitMax        = 60
	defaultSwaggerRateLimitWindow     = time.Minute
	maxSwaggerRateLimitWindowInSecond = 3600
	defaultShardingRateLimitMax       = 30
	defaultShardingRateLimitWindow    = time.Minute
	minShardingAdminTokenLength       = 32
	minEntropyCharClasses             = 3
)

type swaggerRateLimiterState struct {
	mu          sync.Mutex
	windowStart time.Time
	hitsByKey   map[string]int
}

func newSwaggerRateLimiterState() *swaggerRateLimiterState {
	return &swaggerRateLimiterState{
		windowStart: time.Now().UTC(),
		hitsByKey:   make(map[string]int),
	}
}

func (s *swaggerRateLimiterState) allow(key string, limit int, window time.Duration, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if now.Sub(s.windowStart) >= window {
		s.windowStart = now
		s.hitsByKey = make(map[string]int)
	}

	s.hitsByKey[key]++

	return s.hitsByKey[key] <= limit
}

// swaggerLimiterState holds per-process rate limit counters for Swagger endpoints.
// NOTE: In multi-pod deployments, each pod maintains independent counters.
// For distributed rate limiting, configure limits at the ingress/load-balancer layer.
var swaggerLimiterState = newSwaggerRateLimiterState()

// shardingLimiterState holds per-process rate limit counters for sharding admin endpoints.
// NOTE: In multi-pod deployments, each pod maintains independent counters.
// The sharding admin token provides the primary security layer; this rate limiter
// is a defense-in-depth measure.
var shardingLimiterState = newSwaggerRateLimiterState()

// CORSAllowedOrigins returns the allowed CORS origins based on environment configuration.
func CORSAllowedOrigins() string {
	origins := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if origins == "" {
		if isStrictEnv() {
			return deniedCORSOrigin
		}

		return "http://localhost,http://127.0.0.1,https://localhost,https://127.0.0.1"
	}

	parts := strings.Split(origins, ",")
	clean := make([]string, 0, len(parts))
	hasWildcard := false

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}

		if trimmed == "*" {
			hasWildcard = true
			continue
		}

		clean = append(clean, trimmed)
	}

	if isStrictEnv() {
		if hasWildcard || len(clean) == 0 {
			return deniedCORSOrigin
		}

		return strings.Join(clean, ",")
	}

	if hasWildcard {
		return "*"
	}

	if len(clean) > 0 {
		return strings.Join(clean, ",")
	}

	return "http://localhost,http://127.0.0.1,https://localhost,https://127.0.0.1"
}

// SecurityHeadersMiddleware adds security-related HTTP headers to every response.
func SecurityHeadersMiddleware(c *fiber.Ctx) error {
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("X-Frame-Options", "DENY")
	c.Set("Referrer-Policy", "no-referrer")

	if strings.HasPrefix(c.Path(), "/swagger") {
		c.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	} else {
		c.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
	}

	if isStrictEnv() {
		c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}

	return c.Next()
}

// ParseCommaSeparated splits a comma-separated string and returns trimmed non-empty values.
func ParseCommaSeparated(raw string) []string {
	parts := strings.Split(raw, ",")
	parsed := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			parsed = append(parsed, trimmed)
		}
	}

	return parsed
}

// SwaggerEnabled returns whether the Swagger UI should be served.
func SwaggerEnabled() bool {
	raw := os.Getenv("SWAGGER_ENABLED")
	if strings.TrimSpace(raw) == "" {
		return !isStrictEnv()
	}

	return IsTruthyString(raw)
}

// SwaggerRateLimitMiddleware returns a Fiber middleware that enforces per-IP rate limits for Swagger endpoints.
func SwaggerRateLimitMiddleware() fiber.Handler {
	limit := defaultSwaggerRateLimitMax
	if parsed, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SWAGGER_RATE_LIMIT_MAX"))); err == nil && parsed > 0 {
		limit = parsed
	}

	expiration := defaultSwaggerRateLimitWindow
	if parsed, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SWAGGER_RATE_LIMIT_WINDOW_SECONDS"))); err == nil && parsed > 0 && parsed <= maxSwaggerRateLimitWindowInSecond {
		expiration = time.Duration(parsed) * time.Second
	}

	return func(c *fiber.Ctx) error {
		if !swaggerLimiterState.allow(c.IP(), limit, expiration, time.Now().UTC()) {
			return c.SendStatus(fiber.StatusTooManyRequests)
		}

		return c.Next()
	}
}

// SwaggerTokenAuthorized checks whether the provided token matches the configured Swagger auth token.
func SwaggerTokenAuthorized(provided string) bool {
	token := strings.TrimSpace(os.Getenv("SWAGGER_AUTH_TOKEN"))
	if token == "" {
		return true // no token configured — allow access (backwards-compatible)
	}

	provided = strings.TrimSpace(provided)

	return subtle.ConstantTimeCompare([]byte(token), []byte(provided)) == 1
}

// SwaggerRequestToken extracts the Swagger authentication token from the request header or cookie.
func SwaggerRequestToken(c *fiber.Ctx) string {
	token := strings.TrimSpace(c.Get("X-Swagger-Token"))
	if token != "" {
		return token
	}

	return strings.TrimSpace(c.Cookies(SwaggerTokenCookieName))
}

// ShardingControlPlaneMiddleware returns a Fiber middleware that authenticates and rate-limits sharding admin requests.
func ShardingControlPlaneMiddleware() fiber.Handler {
	limit := defaultShardingRateLimitMax
	if parsed, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SHARDING_ADMIN_RATE_LIMIT_MAX"))); err == nil && parsed > 0 {
		limit = parsed
	}

	window := defaultShardingRateLimitWindow
	if parsed, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SHARDING_ADMIN_RATE_LIMIT_WINDOW_SECONDS"))); err == nil && parsed > 0 && parsed <= maxSwaggerRateLimitWindowInSecond {
		window = time.Duration(parsed) * time.Second
	}

	configuredToken := strings.TrimSpace(os.Getenv("SHARDING_ADMIN_TOKEN"))
	if configuredToken != "" && len(configuredToken) >= minShardingAdminTokenLength && configuredToken != knownPlaceholderToken {
		if !hasMinimumEntropy(configuredToken, minEntropyCharClasses) {
			securityLogger.Warn("WARNING: Sharding admin token has low entropy (fewer than 3 character classes). Consider using a stronger token.")
		}
	}

	return func(c *fiber.Ctx) error {
		token := strings.TrimSpace(os.Getenv("SHARDING_ADMIN_TOKEN"))
		if token == "" {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "sharding control plane token is not configured",
			})
		}

		if len(token) < minShardingAdminTokenLength {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "sharding control plane token does not meet minimum length",
			})
		}

		if token == knownPlaceholderToken {
			securityEventLog(c, "sharding token matches example placeholder — replace SHARDING_ADMIN_TOKEN")

			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "sharding control plane token must be changed from the example value",
			})
		}

		provided := strings.TrimSpace(c.Get(ShardingControlTokenHeader))
		if subtle.ConstantTimeCompare([]byte(token), []byte(provided)) != 1 {
			securityEventLog(c, "invalid sharding token")

			return c.SendStatus(fiber.StatusUnauthorized)
		}

		rateLimitKey := "sharding:" + c.IP()
		if !shardingLimiterState.allow(rateLimitKey, limit, window, time.Now().UTC()) {
			securityEventLog(c, "sharding rate limit exceeded")

			return c.SendStatus(fiber.StatusTooManyRequests)
		}

		return c.Next()
	}
}

// hasMinimumEntropy checks that a string has at least minClasses distinct character classes
// (lowercase, uppercase, digits, special characters).
func hasMinimumEntropy(s string, minClasses int) bool {
	var hasLower, hasUpper, hasDigit, hasSpecial bool

	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= '0' && c <= '9':
			hasDigit = true
		default:
			hasSpecial = true
		}
	}

	count := 0
	if hasLower {
		count++
	}

	if hasUpper {
		count++
	}

	if hasDigit {
		count++
	}

	if hasSpecial {
		count++
	}

	return count >= minClasses
}

func securityEventLog(c *fiber.Ctx, event string) {
	entry := map[string]string{
		"level":      "warn",
		"event":      "security",
		"type":       event,
		"ip":         c.IP(),
		"method":     c.Method(),
		"path":       c.Path(),
		"request_id": strings.TrimSpace(c.Get("X-Request-Id")),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		securityLogger.Warn("failed to marshal security event: " + err.Error())
		return
	}

	securityLogger.Warn(string(data))
}

func currentEnv() string {
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if appEnv != "" {
		return appEnv
	}

	return strings.ToLower(strings.TrimSpace(os.Getenv("ENV_NAME")))
}

func isStrictEnv() bool {
	switch currentEnv() {
	case "dev", "development", "local", "localhost", "test", "testing":
		return false
	default:
		return true
	}
}
