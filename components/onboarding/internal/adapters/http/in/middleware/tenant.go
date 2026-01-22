// Package middleware provides HTTP middleware functions for the onboarding service.
package middleware

import (
	"context"
	"errors"
	"net/http"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	poolmanager "github.com/LerianStudio/lib-commons/v2/commons/pool-manager"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/gofiber/fiber/v2"
	jwt "github.com/golang-jwt/jwt/v5"
)

// TenantContextKey is the context key for storing tenant-specific database connection.
type TenantContextKey string

const (
	// TenantDBConnectionKey is the key for storing tenant database connection in context.
	TenantDBConnectionKey TenantContextKey = "tenant_db_connection"
	// TenantIDKey is the key for storing tenant ID in context.
	TenantIDKey TenantContextKey = "tenant_id"
)

// TenantMiddleware provides middleware for multi-tenant database connection handling.
type TenantMiddleware struct {
	pool *poolmanager.TenantConnectionPool
}

// NewTenantMiddleware creates a new TenantMiddleware with the given connection pool.
// If pool is nil, the middleware will pass through without doing anything (single-tenant mode).
func NewTenantMiddleware(pool *poolmanager.TenantConnectionPool) *TenantMiddleware {
	return &TenantMiddleware{
		pool: pool,
	}
}

// ExtractAndSetConnection is a Fiber middleware that extracts the tenant ID from the JWT token,
// retrieves the tenant-specific database connection from the pool, and stores it in the context.
//
// The middleware supports two modes:
// 1. Multi-tenant mode (when pool is set): Extracts tenant ID from JWT "owner" claim and gets tenant-specific connection
// 2. Single-tenant mode (when pool is nil): Passes through without modification
//
// The tenant ID is expected to be in the JWT token's "owner" claim (set by lib-auth).
func (m *TenantMiddleware) ExtractAndSetConnection() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Single-tenant mode: pass through if no pool configured
		if m.pool == nil || !m.pool.IsMultiTenant() {
			return c.Next()
		}

		ctx := libOpentelemetry.ExtractHTTPContext(c)
		logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

		ctx, span := tracer.Start(ctx, "middleware.tenant.extract_and_set_connection")
		defer span.End()

		// Extract tenant ID from JWT token
		tenantID, err := extractTenantIDFromToken(c)
		if err != nil {
			logger.Warnf("Failed to extract tenant ID from token: %v", err)
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to extract tenant ID", err)

			// For missing/invalid tenant ID, use default connection as fallback
			defaultConn := m.pool.GetDefaultConnection()
			if defaultConn != nil {
				logger.Info("Using default connection (no tenant ID in token)")
				ctx = context.WithValue(ctx, TenantDBConnectionKey, defaultConn)
				c.SetUserContext(ctx)
				return c.Next()
			}

			return libHTTP.Unauthorized(c, "TENANT_ID_REQUIRED", "Unauthorized", "tenant ID is required for multi-tenant mode")
		}

		logger.Infof("Extracted tenant ID: %s", tenantID)

		// Store tenant ID in context
		ctx = context.WithValue(ctx, TenantIDKey, tenantID)

		// Get tenant-specific connection from pool
		conn, err := m.pool.GetConnection(ctx, tenantID)
		if err != nil {
			logger.Errorf("Failed to get connection for tenant %s: %v", tenantID, err)
			libOpentelemetry.HandleSpanError(&span, "Failed to get tenant connection", err)

			// Check for specific errors
			if errors.Is(err, poolmanager.ErrTenantNotFound) {
				return libHTTP.NotFound(c, "TENANT_NOT_FOUND", "Not Found", "tenant not found")
			}

			if errors.Is(err, poolmanager.ErrPoolClosed) {
				return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
					"code":    "SERVICE_UNAVAILABLE",
					"message": "Service temporarily unavailable",
				})
			}

			return libHTTP.InternalServerError(c, "CONNECTION_ERROR", "Internal Server Error", "failed to establish database connection")
		}

		// Store connection in context
		ctx = context.WithValue(ctx, TenantDBConnectionKey, conn)
		c.SetUserContext(ctx)

		logger.Infof("Set tenant connection for tenant: %s", tenantID)

		return c.Next()
	}
}

// extractTenantIDFromToken extracts the tenant ID from the JWT token's "owner" claim.
// The tenant ID is typically set by lib-auth when validating the token.
func extractTenantIDFromToken(c *fiber.Ctx) (string, error) {
	accessToken := libHTTP.ExtractTokenFromHeader(c)
	if accessToken == "" {
		return "", errors.New("no authorization token provided")
	}

	// Parse token without validation (validation is done by auth middleware)
	token, _, err := new(jwt.Parser).ParseUnverified(accessToken, jwt.MapClaims{})
	if err != nil {
		return "", err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid token claims format")
	}

	// Try to get tenant ID from "owner" claim (set by lib-auth for normal users)
	if owner, ok := claims["owner"].(string); ok && owner != "" {
		return owner, nil
	}

	// Try to get from "tenant_id" claim (alternative claim name)
	if tenantID, ok := claims["tenant_id"].(string); ok && tenantID != "" {
		return tenantID, nil
	}

	// Try to get from "organization_id" claim (another alternative)
	if orgID, ok := claims["organization_id"].(string); ok && orgID != "" {
		return orgID, nil
	}

	return "", errors.New("tenant ID not found in token claims")
}

// GetTenantConnection retrieves the tenant database connection from the context.
// Returns nil if not in multi-tenant mode or no connection is set.
func GetTenantConnection(ctx context.Context) *libPostgres.PostgresConnection {
	if conn, ok := ctx.Value(TenantDBConnectionKey).(*libPostgres.PostgresConnection); ok {
		return conn
	}
	return nil
}

// GetTenantID retrieves the tenant ID from the context.
// Returns empty string if no tenant ID is set.
func GetTenantID(ctx context.Context) string {
	if tenantID, ok := ctx.Value(TenantIDKey).(string); ok {
		return tenantID
	}
	return ""
}

// RequireTenant is a middleware that ensures a tenant ID is present in the context.
// Use this for routes that must have a tenant context.
func (m *TenantMiddleware) RequireTenant() fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		tenantID := GetTenantID(ctx)

		if tenantID == "" {
			ctx := libOpentelemetry.ExtractHTTPContext(c)
			logger, _, _, _ := libCommons.NewTrackingFromContext(ctx)
			logger.Warn("Tenant ID required but not found in context")

			return libHTTP.BadRequest(c, "tenant ID is required")
		}

		return c.Next()
	}
}
