package bootstrap

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

// createMockJWT creates a mock JWT token with the given claims for testing.
func createMockJWT(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signature := base64.RawURLEncoding.EncodeToString([]byte("signature"))
	return header + "." + payload + "." + signature
}

func TestDualPoolMiddleware_SelectPool(t *testing.T) {
	logger := libZap.InitializeLogger()

	// Create pools with default connections (single-tenant mode)
	onboardingPool := poolmanager.NewTenantConnectionPool(nil, "ledger", "onboarding", logger)
	transactionPool := poolmanager.NewTenantConnectionPool(nil, "ledger", "transaction", logger)

	middleware := &DualPoolMiddleware{
		onboardingPool:  onboardingPool,
		transactionPool: transactionPool,
		logger:          logger,
	}

	tests := []struct {
		name         string
		path         string
		expectedPool string
	}{
		// Onboarding paths
		{
			name:         "organizations path routes to onboarding",
			path:         "/v1/organizations",
			expectedPool: "onboarding",
		},
		{
			name:         "organizations with ID routes to onboarding",
			path:         "/v1/organizations/123",
			expectedPool: "onboarding",
		},
		{
			name:         "ledgers path routes to onboarding",
			path:         "/v1/organizations/123/ledgers",
			expectedPool: "onboarding",
		},
		{
			name:         "ledgers with ID routes to onboarding",
			path:         "/v1/organizations/123/ledgers/456",
			expectedPool: "onboarding",
		},
		{
			name:         "accounts path routes to onboarding",
			path:         "/v1/organizations/123/ledgers/456/accounts",
			expectedPool: "onboarding",
		},
		{
			name:         "assets path routes to onboarding",
			path:         "/v1/organizations/123/ledgers/456/assets",
			expectedPool: "onboarding",
		},
		{
			name:         "portfolios path routes to onboarding",
			path:         "/v1/organizations/123/ledgers/456/portfolios",
			expectedPool: "onboarding",
		},
		{
			name:         "segments path routes to onboarding",
			path:         "/v1/organizations/123/ledgers/456/segments",
			expectedPool: "onboarding",
		},
		{
			name:         "account-types path routes to onboarding",
			path:         "/v1/organizations/123/ledgers/456/account-types",
			expectedPool: "onboarding",
		},

		// Transaction paths
		{
			name:         "transactions path routes to transaction",
			path:         "/v1/organizations/123/ledgers/456/transactions",
			expectedPool: "transaction",
		},
		{
			name:         "transactions DSL path routes to transaction",
			path:         "/v1/organizations/123/ledgers/456/transactions/dsl",
			expectedPool: "transaction",
		},
		{
			name:         "transactions JSON path routes to transaction",
			path:         "/v1/organizations/123/ledgers/456/transactions/json",
			expectedPool: "transaction",
		},
		{
			name:         "operations path routes to transaction",
			path:         "/v1/organizations/123/ledgers/456/accounts/789/operations",
			expectedPool: "transaction",
		},
		{
			name:         "balances path routes to transaction",
			path:         "/v1/organizations/123/ledgers/456/balances",
			expectedPool: "transaction",
		},
		{
			name:         "balances by account routes to transaction",
			path:         "/v1/organizations/123/ledgers/456/accounts/789/balances",
			expectedPool: "transaction",
		},
		{
			name:         "asset-rates path routes to transaction",
			path:         "/v1/organizations/123/ledgers/456/asset-rates",
			expectedPool: "transaction",
		},
		{
			name:         "operation-routes path routes to transaction",
			path:         "/v1/organizations/123/ledgers/456/operation-routes",
			expectedPool: "transaction",
		},
		{
			name:         "transaction-routes path routes to transaction",
			path:         "/v1/organizations/123/ledgers/456/transaction-routes",
			expectedPool: "transaction",
		},

		// Health/utility paths default to onboarding
		{
			name:         "health path routes to onboarding (default)",
			path:         "/health",
			expectedPool: "onboarding",
		},
		{
			name:         "version path routes to onboarding (default)",
			path:         "/version",
			expectedPool: "onboarding",
		},
		{
			name:         "swagger path routes to onboarding (default)",
			path:         "/swagger/index.html",
			expectedPool: "onboarding",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool := middleware.selectPool(tc.path)
			poolName := middleware.getPoolName(tc.path)

			assert.NotNil(t, pool, "Pool should not be nil")
			assert.Equal(t, tc.expectedPool, poolName, "Pool name should match expected")
		})
	}
}

func TestDualPoolMiddleware_IsTransactionPath(t *testing.T) {
	logger := libZap.InitializeLogger()

	middleware := &DualPoolMiddleware{
		logger: logger,
	}

	tests := []struct {
		name           string
		path           string
		isTransaction  bool
	}{
		// Transaction paths
		{"transactions", "/v1/organizations/123/ledgers/456/transactions", true},
		{"transactions with ID", "/v1/organizations/123/ledgers/456/transactions/789", true},
		{"operations", "/v1/organizations/123/ledgers/456/accounts/789/operations", true},
		{"operations with ID", "/v1/organizations/123/ledgers/456/accounts/789/operations/012", true},
		{"balances", "/v1/organizations/123/ledgers/456/balances", true},
		{"balances by account", "/v1/organizations/123/ledgers/456/accounts/789/balances", true},
		{"asset-rates", "/v1/organizations/123/ledgers/456/asset-rates", true},
		{"operation-routes", "/v1/organizations/123/ledgers/456/operation-routes", true},
		{"transaction-routes", "/v1/organizations/123/ledgers/456/transaction-routes", true},

		// Non-transaction paths
		{"organizations", "/v1/organizations", false},
		{"ledgers", "/v1/organizations/123/ledgers", false},
		{"accounts", "/v1/organizations/123/ledgers/456/accounts", false},
		{"assets", "/v1/organizations/123/ledgers/456/assets", false},
		{"portfolios", "/v1/organizations/123/ledgers/456/portfolios", false},
		{"segments", "/v1/organizations/123/ledgers/456/segments", false},
		{"account-types", "/v1/organizations/123/ledgers/456/account-types", false},
		{"health", "/health", false},
		{"version", "/version", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := middleware.isTransactionPath(tc.path)
			assert.Equal(t, tc.isTransaction, result, "isTransactionPath should return expected value for path: %s", tc.path)
		})
	}
}

func TestDualPoolMiddleware_SingleTenantMode(t *testing.T) {
	logger := libZap.InitializeLogger()

	// Create pools without Pool Manager client (single-tenant mode)
	onboardingPool := poolmanager.NewTenantConnectionPool(nil, "ledger", "onboarding", logger)
	transactionPool := poolmanager.NewTenantConnectionPool(nil, "ledger", "transaction", logger)

	pools := &MultiTenantPools{
		OnboardingPool:  onboardingPool,
		TransactionPool: transactionPool,
	}

	middleware := NewDualPoolMiddleware(pools, logger)

	app := fiber.New()
	app.Use(middleware.WithTenantDB)
	app.Get("/v1/organizations", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})
	app.Get("/v1/organizations/:org/ledgers/:ledger/transactions", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	t.Run("onboarding path passes through in single-tenant mode", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/organizations", nil)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("transaction path passes through in single-tenant mode", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/organizations/123/ledgers/456/transactions", nil)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})
}

func TestDualPoolMiddleware_WithDefaultConnection(t *testing.T) {
	logger := libZap.InitializeLogger()

	// Create pools with default connections
	onboardingPool := poolmanager.NewTenantConnectionPool(nil, "ledger", "onboarding", logger)
	onboardingDefaultConn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: "host=onboarding-db",
		Logger:                  logger,
	}
	onboardingPool.WithDefaultConnection(onboardingDefaultConn)

	transactionPool := poolmanager.NewTenantConnectionPool(nil, "ledger", "transaction", logger)
	transactionDefaultConn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: "host=transaction-db",
		Logger:                  logger,
	}
	transactionPool.WithDefaultConnection(transactionDefaultConn)

	pools := &MultiTenantPools{
		OnboardingPool:  onboardingPool,
		TransactionPool: transactionPool,
	}

	middleware := NewDualPoolMiddleware(pools, logger)

	app := fiber.New()
	app.Use(middleware.WithTenantDB)
	app.Get("/v1/organizations", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	t.Run("passes through with default connection when pool not multi-tenant", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/organizations", nil)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})
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

func TestNewDualPoolMiddleware(t *testing.T) {
	logger := libZap.InitializeLogger()

	onboardingPool := poolmanager.NewTenantConnectionPool(nil, "ledger", "onboarding", logger)
	transactionPool := poolmanager.NewTenantConnectionPool(nil, "ledger", "transaction", logger)

	pools := &MultiTenantPools{
		OnboardingPool:  onboardingPool,
		TransactionPool: transactionPool,
	}

	middleware := NewDualPoolMiddleware(pools, logger)

	assert.NotNil(t, middleware)
	assert.Equal(t, onboardingPool, middleware.onboardingPool)
	assert.Equal(t, transactionPool, middleware.transactionPool)
	assert.NotNil(t, middleware.logger)
}

func TestMultiTenantPools_Structure(t *testing.T) {
	logger := libZap.InitializeLogger()

	onboardingPool := poolmanager.NewTenantConnectionPool(nil, "ledger", "onboarding", logger)
	transactionPool := poolmanager.NewTenantConnectionPool(nil, "ledger", "transaction", logger)

	pools := &MultiTenantPools{
		OnboardingPool:  onboardingPool,
		TransactionPool: transactionPool,
	}

	t.Run("contains both pools", func(t *testing.T) {
		assert.NotNil(t, pools.OnboardingPool)
		assert.NotNil(t, pools.TransactionPool)
	})

	t.Run("pools are independent", func(t *testing.T) {
		assert.NotEqual(t, pools.OnboardingPool, pools.TransactionPool)
	})
}
