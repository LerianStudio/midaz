// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCORSAllowedOrigins(t *testing.T) {
	t.Setenv("ENV_NAME", "production")
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")

	assert.Equal(t, deniedCORSOrigin, CORSAllowedOrigins())

	t.Setenv("CORS_ALLOWED_ORIGINS", "*,https://safe.example")
	assert.Equal(t, deniedCORSOrigin, CORSAllowedOrigins())

	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	assert.Equal(t, deniedCORSOrigin, CORSAllowedOrigins())

	t.Setenv("ENV_NAME", "")
	assert.Equal(t, deniedCORSOrigin, CORSAllowedOrigins())

	t.Setenv("ENV_NAME", "dev")
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	assert.Equal(t, "*", CORSAllowedOrigins())

	t.Setenv("CORS_ALLOWED_ORIGINS", " https://a.test, https://b.test ")
	assert.Equal(t, "https://a.test,https://b.test", CORSAllowedOrigins())
}

func TestParseCommaSeparated(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, ParseCommaSeparated("a, b, c"))
	assert.Empty(t, ParseCommaSeparated(" , , "))
}

func TestSwaggerEnabled(t *testing.T) {
	// When SWAGGER_ENABLED is unset and ENV_NAME is unset, isStrictEnv() returns
	// true (default switch case), so SwaggerEnabled() returns !true == false.
	t.Run("unset_swagger_enabled_strict_env_returns_false", func(t *testing.T) {
		t.Setenv("SWAGGER_ENABLED", "")
		t.Setenv("ENV_NAME", "")
		t.Setenv("APP_ENV", "")
		assert.False(t, SwaggerEnabled())
	})

	// When SWAGGER_ENABLED is unset and ENV_NAME is a non-strict value,
	// isStrictEnv() returns false, so SwaggerEnabled() returns !false == true.
	t.Run("unset_swagger_enabled_dev_env_returns_true", func(t *testing.T) {
		t.Setenv("SWAGGER_ENABLED", "")
		t.Setenv("ENV_NAME", "dev")
		t.Setenv("APP_ENV", "")
		assert.True(t, SwaggerEnabled())
	})

	// When SWAGGER_ENABLED is explicitly "false", it overrides the env check.
	t.Run("explicit_false_returns_false", func(t *testing.T) {
		t.Setenv("SWAGGER_ENABLED", "false")
		t.Setenv("ENV_NAME", "dev")
		assert.False(t, SwaggerEnabled())
	})

	// When SWAGGER_ENABLED is explicitly "true", it overrides the env check.
	t.Run("explicit_true_returns_true", func(t *testing.T) {
		t.Setenv("SWAGGER_ENABLED", "true")
		t.Setenv("ENV_NAME", "production")
		assert.True(t, SwaggerEnabled())
	})
}

func TestSwaggerTokenAuthorized(t *testing.T) {
	t.Setenv("SWAGGER_AUTH_TOKEN", "")
	assert.True(t, SwaggerTokenAuthorized(""))

	t.Setenv("SWAGGER_AUTH_TOKEN", "secret")
	assert.False(t, SwaggerTokenAuthorized("wrong"))
	assert.True(t, SwaggerTokenAuthorized("secret"))
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	t.Run("strict_csp_for_api_paths", func(t *testing.T) {
		t.Setenv("ENV_NAME", "production")

		app := fiber.New()
		app.Use(SecurityHeadersMiddleware)
		app.Get("/v1/health", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

		req := httptest.NewRequest(http.MethodGet, "/v1/health", http.NoBody)
		resp, err := app.Test(req)
		require.NoError(t, err)

		defer resp.Body.Close()

		assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
		assert.Equal(t, "no-referrer", resp.Header.Get("Referrer-Policy"))
		assert.Equal(t, "default-src 'none'; frame-ancestors 'none'; base-uri 'none'", resp.Header.Get("Content-Security-Policy"))
		assert.Equal(t, "max-age=31536000; includeSubDomains", resp.Header.Get("Strict-Transport-Security"))
	})

	t.Run("swagger_csp_allows_ui_assets", func(t *testing.T) {
		t.Setenv("ENV_NAME", "dev")

		app := fiber.New()
		app.Use(SecurityHeadersMiddleware)
		app.Get("/swagger/index.html", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

		req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", http.NoBody)
		resp, err := app.Test(req)
		require.NoError(t, err)

		defer resp.Body.Close()

		assert.Contains(t, resp.Header.Get("Content-Security-Policy"), "default-src 'self'")
		assert.Empty(t, resp.Header.Get("Strict-Transport-Security"))
	})
}

func TestSwaggerRequestToken(t *testing.T) {
	t.Run("reads_header_token", func(t *testing.T) {
		app := fiber.New()
		app.Get("/swagger/index.html", func(c *fiber.Ctx) error {
			assert.Equal(t, "header-token", SwaggerRequestToken(c))
			return c.SendStatus(fiber.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", http.NoBody)
		req.Header.Set("X-Swagger-Token", "header-token")

		resp, err := app.Test(req)
		require.NoError(t, err)

		defer resp.Body.Close()
	})

	t.Run("reads_cookie_token", func(t *testing.T) {
		app := fiber.New()
		app.Get("/swagger/index.html", func(c *fiber.Ctx) error {
			assert.Equal(t, "cookie-token", SwaggerRequestToken(c))
			return c.SendStatus(fiber.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", http.NoBody)
		req.AddCookie(&http.Cookie{Name: SwaggerTokenCookieName, Value: "cookie-token"})

		resp, err := app.Test(req)
		require.NoError(t, err)

		defer resp.Body.Close()
	})

	t.Run("ignores_query_token_to_avoid_leakage", func(t *testing.T) {
		app := fiber.New()
		app.Get("/swagger/index.html", func(c *fiber.Ctx) error {
			assert.Empty(t, SwaggerRequestToken(c))
			return c.SendStatus(fiber.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/swagger/index.html?token=query-token", http.NoBody)

		resp, err := app.Test(req)
		require.NoError(t, err)

		defer resp.Body.Close()
	})
}

func TestSwaggerRateLimitMiddleware(t *testing.T) {
	t.Setenv("SWAGGER_RATE_LIMIT_MAX", "2")
	t.Setenv("SWAGGER_RATE_LIMIT_WINDOW_SECONDS", "60")

	swaggerLimiterState = newSwaggerRateLimiterState()

	app := fiber.New()
	app.Use(SwaggerRateLimitMiddleware())
	app.Get("/swagger/index.html", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", http.NoBody)
		req.RemoteAddr = "10.0.0.1:12345"

		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		resp.Body.Close()
	}

	req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", http.NoBody)
	req.RemoteAddr = "10.0.0.1:12345"

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusTooManyRequests, resp.StatusCode)

	resp.Body.Close()
}

func TestShardingControlPlaneMiddleware(t *testing.T) { //nolint:funlen
	t.Run("strict_env_requires_configured_token", func(t *testing.T) {
		shardingLimiterState = newSwaggerRateLimiterState()

		t.Setenv("ENV_NAME", "production")
		t.Setenv("SHARDING_ADMIN_TOKEN", "")

		app := fiber.New()
		app.Use(ShardingControlPlaneMiddleware())
		app.Get("/v1/sharding/rebalance/status", func(c *fiber.Ctx) error {
			return c.SendStatus(fiber.StatusOK)
		})

		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/sharding/rebalance/status", http.NoBody))
		require.NoError(t, err)
		assert.Equal(t, fiber.StatusServiceUnavailable, resp.StatusCode)

		resp.Body.Close()
	})

	t.Run("configured_token_rejects_missing_or_invalid_header", func(t *testing.T) {
		shardingLimiterState = newSwaggerRateLimiterState()

		t.Setenv("ENV_NAME", "production")
		t.Setenv("SHARDING_ADMIN_TOKEN", "12345678901234567890123456789012")

		app := fiber.New()
		app.Use(ShardingControlPlaneMiddleware())
		app.Get("/v1/sharding/rebalance/status", func(c *fiber.Ctx) error {
			return c.SendStatus(fiber.StatusOK)
		})

		t.Run("missing_header_returns_401", func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/sharding/rebalance/status", http.NoBody))
			require.NoError(t, err)
			assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

			resp.Body.Close()
		})

		t.Run("invalid_header_returns_401", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/sharding/rebalance/status", http.NoBody)
			req.Header.Set(ShardingControlTokenHeader, "wrong-token")

			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

			resp.Body.Close()
		})
	})

	t.Run("configured_token_allows_valid_header", func(t *testing.T) {
		shardingLimiterState = newSwaggerRateLimiterState()

		t.Setenv("ENV_NAME", "production")
		t.Setenv("SHARDING_ADMIN_TOKEN", "12345678901234567890123456789012")

		app := fiber.New()
		app.Use(ShardingControlPlaneMiddleware())
		app.Get("/v1/sharding/rebalance/status", func(c *fiber.Ctx) error {
			return c.SendStatus(fiber.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/sharding/rebalance/status", http.NoBody)
		req.Header.Set(ShardingControlTokenHeader, "12345678901234567890123456789012")

		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		resp.Body.Close()
	})

	t.Run("dev_env_without_token_is_also_blocked", func(t *testing.T) {
		shardingLimiterState = newSwaggerRateLimiterState()

		t.Setenv("ENV_NAME", "dev")
		t.Setenv("SHARDING_ADMIN_TOKEN", "")

		app := fiber.New()
		app.Use(ShardingControlPlaneMiddleware())
		app.Get("/v1/sharding/rebalance/status", func(c *fiber.Ctx) error {
			return c.SendStatus(fiber.StatusOK)
		})

		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/sharding/rebalance/status", http.NoBody))
		require.NoError(t, err)
		assert.Equal(t, fiber.StatusServiceUnavailable, resp.StatusCode)

		resp.Body.Close()
	})

	t.Run("token_below_minimum_length_is_blocked", func(t *testing.T) {
		shardingLimiterState = newSwaggerRateLimiterState()

		t.Setenv("ENV_NAME", "production")
		t.Setenv("SHARDING_ADMIN_TOKEN", "short-token")

		app := fiber.New()
		app.Use(ShardingControlPlaneMiddleware())
		app.Get("/v1/sharding/rebalance/status", func(c *fiber.Ctx) error {
			return c.SendStatus(fiber.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/sharding/rebalance/status", http.NoBody)
		req.Header.Set(ShardingControlTokenHeader, "short-token")

		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, fiber.StatusServiceUnavailable, resp.StatusCode)

		resp.Body.Close()
	})

	t.Run("placeholder_token_is_blocked", func(t *testing.T) {
		shardingLimiterState = newSwaggerRateLimiterState()

		t.Setenv("ENV_NAME", "production")
		t.Setenv("SHARDING_ADMIN_TOKEN", knownPlaceholderToken)

		app := fiber.New()
		app.Use(ShardingControlPlaneMiddleware())
		app.Get("/v1/sharding/rebalance/status", func(c *fiber.Ctx) error {
			return c.SendStatus(fiber.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/sharding/rebalance/status", http.NoBody)
		req.Header.Set(ShardingControlTokenHeader, knownPlaceholderToken)

		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, fiber.StatusServiceUnavailable, resp.StatusCode)

		resp.Body.Close()
	})

	t.Run("configured_token_rate_limits_requests", func(t *testing.T) {
		shardingLimiterState = newSwaggerRateLimiterState()

		t.Setenv("ENV_NAME", "production")
		t.Setenv("SHARDING_ADMIN_TOKEN", "12345678901234567890123456789012")
		t.Setenv("SHARDING_ADMIN_RATE_LIMIT_MAX", "1")
		t.Setenv("SHARDING_ADMIN_RATE_LIMIT_WINDOW_SECONDS", "60")

		app := fiber.New()
		app.Use(ShardingControlPlaneMiddleware())
		app.Get("/v1/sharding/rebalance/status", func(c *fiber.Ctx) error {
			return c.SendStatus(fiber.StatusOK)
		})

		req1 := httptest.NewRequest(http.MethodGet, "/v1/sharding/rebalance/status", http.NoBody)
		req1.Header.Set(ShardingControlTokenHeader, "12345678901234567890123456789012")
		req1.RemoteAddr = "10.0.0.1:12345"

		resp, err := app.Test(req1)
		require.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		resp.Body.Close()

		req2 := httptest.NewRequest(http.MethodGet, "/v1/sharding/rebalance/status", http.NoBody)
		req2.Header.Set(ShardingControlTokenHeader, "12345678901234567890123456789012")
		req2.RemoteAddr = "10.0.0.1:12345"

		resp, err = app.Test(req2)
		require.NoError(t, err)
		assert.Equal(t, fiber.StatusTooManyRequests, resp.StatusCode)

		resp.Body.Close()
	})
}
