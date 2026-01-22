package middleware

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"testing"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	poolmanager "github.com/LerianStudio/lib-commons/v2/commons/pool-manager"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createMockJWT creates a mock JWT token with the given claims
func createMockJWT(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signature := base64.RawURLEncoding.EncodeToString([]byte("signature"))
	return header + "." + payload + "." + signature
}

func TestNewTenantMiddleware(t *testing.T) {
	t.Run("creates middleware with pool", func(t *testing.T) {
		logger := libZap.InitializeLogger()
		client := poolmanager.NewClient("http://localhost:8080", logger)
		pool := poolmanager.NewTenantConnectionPool(client, "ledger", "onboarding", logger)

		middleware := NewTenantMiddleware(pool)

		assert.NotNil(t, middleware)
		assert.Equal(t, pool, middleware.pool)
	})

	t.Run("creates middleware with nil pool", func(t *testing.T) {
		middleware := NewTenantMiddleware(nil)

		assert.NotNil(t, middleware)
		assert.Nil(t, middleware.pool)
	})
}

func TestTenantMiddleware_ExtractAndSetConnection_SingleTenantMode(t *testing.T) {
	// Single-tenant mode: pool is nil
	middleware := NewTenantMiddleware(nil)

	app := fiber.New()
	app.Use(middleware.ExtractAndSetConnection())
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestTenantMiddleware_ExtractAndSetConnection_WithDefaultConnection(t *testing.T) {
	logger := libZap.InitializeLogger()

	// Create pool with default connection but no client (simulating single-tenant with pool structure)
	pool := poolmanager.NewTenantConnectionPool(nil, "ledger", "onboarding", logger)
	defaultConn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: "host=default-db",
		Logger:                  logger,
	}
	pool.WithDefaultConnection(defaultConn)

	middleware := NewTenantMiddleware(pool)

	app := fiber.New()
	app.Use(middleware.ExtractAndSetConnection())
	app.Get("/test", func(c *fiber.Ctx) error {
		// Pool is not multi-tenant (client is nil), so it should pass through
		return c.SendString("OK")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetTenantConnection(t *testing.T) {
	t.Run("returns connection when set", func(t *testing.T) {
		conn := &libPostgres.PostgresConnection{
			ConnectionStringPrimary: "host=test-db",
		}
		ctx := context.WithValue(context.Background(), TenantDBConnectionKey, conn)

		result := GetTenantConnection(ctx)

		assert.Equal(t, conn, result)
	})

	t.Run("returns nil when not set", func(t *testing.T) {
		ctx := context.Background()

		result := GetTenantConnection(ctx)

		assert.Nil(t, result)
	})

	t.Run("returns nil for wrong type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), TenantDBConnectionKey, "wrong-type")

		result := GetTenantConnection(ctx)

		assert.Nil(t, result)
	})
}

func TestGetTenantID(t *testing.T) {
	t.Run("returns tenant ID when set", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), TenantIDKey, "tenant-123")

		result := GetTenantID(ctx)

		assert.Equal(t, "tenant-123", result)
	})

	t.Run("returns empty string when not set", func(t *testing.T) {
		ctx := context.Background()

		result := GetTenantID(ctx)

		assert.Equal(t, "", result)
	})

	t.Run("returns empty string for wrong type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), TenantIDKey, 123)

		result := GetTenantID(ctx)

		assert.Equal(t, "", result)
	})
}

func TestTenantMiddleware_RequireTenant(t *testing.T) {
	middleware := NewTenantMiddleware(nil)

	t.Run("passes when tenant ID is set", func(t *testing.T) {
		app := fiber.New()
		app.Use(func(c *fiber.Ctx) error {
			ctx := context.WithValue(c.UserContext(), TenantIDKey, "tenant-123")
			c.SetUserContext(ctx)
			return c.Next()
		})
		app.Use(middleware.RequireTenant())
		app.Get("/test", func(c *fiber.Ctx) error {
			return c.SendString("OK")
		})

		req := httptest.NewRequest("GET", "/test", nil)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("returns error when tenant ID is not set", func(t *testing.T) {
		app := fiber.New()
		app.Use(middleware.RequireTenant())
		app.Get("/test", func(c *fiber.Ctx) error {
			return c.SendString("OK")
		})

		req := httptest.NewRequest("GET", "/test", nil)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
	})
}

func TestExtractTenantIDFromToken(t *testing.T) {
	t.Run("extracts tenant ID from owner claim", func(t *testing.T) {
		token := createMockJWT(map[string]interface{}{
			"sub":   "user-123",
			"owner": "tenant-456",
		})

		app := fiber.New()
		app.Get("/test", func(c *fiber.Ctx) error {
			tenantID, err := extractTenantIDFromToken(c)
			if err != nil {
				return c.Status(400).SendString(err.Error())
			}
			return c.SendString(tenantID)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("extracts tenant ID from tenant_id claim", func(t *testing.T) {
		token := createMockJWT(map[string]interface{}{
			"sub":       "user-123",
			"tenant_id": "tenant-789",
		})

		app := fiber.New()
		app.Get("/test", func(c *fiber.Ctx) error {
			tenantID, err := extractTenantIDFromToken(c)
			if err != nil {
				return c.Status(400).SendString(err.Error())
			}
			return c.SendString(tenantID)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("extracts tenant ID from organization_id claim", func(t *testing.T) {
		token := createMockJWT(map[string]interface{}{
			"sub":             "user-123",
			"organization_id": "org-123",
		})

		app := fiber.New()
		app.Get("/test", func(c *fiber.Ctx) error {
			tenantID, err := extractTenantIDFromToken(c)
			if err != nil {
				return c.Status(400).SendString(err.Error())
			}
			return c.SendString(tenantID)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("returns error when no tenant ID claims", func(t *testing.T) {
		token := createMockJWT(map[string]interface{}{
			"sub": "user-123",
		})

		app := fiber.New()
		app.Get("/test", func(c *fiber.Ctx) error {
			_, err := extractTenantIDFromToken(c)
			if err != nil {
				return c.Status(400).SendString(err.Error())
			}
			return c.SendString("OK")
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
	})

	t.Run("returns error when no token provided", func(t *testing.T) {
		app := fiber.New()
		app.Get("/test", func(c *fiber.Ctx) error {
			_, err := extractTenantIDFromToken(c)
			if err != nil {
				return c.Status(401).SendString(err.Error())
			}
			return c.SendString("OK")
		})

		req := httptest.NewRequest("GET", "/test", nil)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, 401, resp.StatusCode)
	})
}
