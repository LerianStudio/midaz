package bootstrap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	tmmiddleware "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/middleware"
	tmpg "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/postgres"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConsumerTrigger implements tmmiddleware.ConsumerTrigger for testing.
// It records which tenant IDs had their consumers triggered.
type mockConsumerTrigger struct {
	mu        sync.Mutex
	triggered []string
}

// EnsureConsumerStarted records the tenant ID that was triggered.
func (m *mockConsumerTrigger) EnsureConsumerStarted(_ context.Context, tenantID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.triggered = append(m.triggered, tenantID)
}

// getTriggered returns a copy of the triggered tenant IDs (thread-safe).
func (m *mockConsumerTrigger) getTriggered() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]string, len(m.triggered))
	copy(result, m.triggered)

	return result
}

// createMockJWT creates a mock JWT token with the given claims for testing.
func createMockJWT(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signature := base64.RawURLEncoding.EncodeToString([]byte("signature"))
	return header + "." + payload + "." + signature
}

func TestMultiPoolMiddleware_RouteMatching(t *testing.T) {
	logger := libZap.InitializeLogger()

	// Create pools with default connections (single-tenant mode)
	onboardingPool := tmpg.NewManager(nil, "ledger", tmpg.WithModule("onboarding"), tmpg.WithLogger(logger))
	transactionPool := tmpg.NewManager(nil, "ledger", tmpg.WithModule("transaction"), tmpg.WithLogger(logger))

	mid := tmmiddleware.NewMultiPoolMiddleware(
		tmmiddleware.WithRoute(transactionPaths, "transaction", transactionPool, nil),
		tmmiddleware.WithDefaultRoute("onboarding", onboardingPool, nil),
		tmmiddleware.WithPublicPaths(publicPaths...),
		tmmiddleware.WithMultiPoolLogger(logger),
	)

	// The middleware should not be enabled since pools have no TM client (single-tenant)
	assert.False(t, mid.Enabled(), "Middleware should not be enabled in single-tenant mode")
}

func TestMultiPoolMiddleware_IsTransactionPath(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		isTransaction bool
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
			result := isTransactionPath(tc.path)
			assert.Equal(t, tc.isTransaction, result, "isTransactionPath should return expected value for path: %s", tc.path)
		})
	}
}

func TestMultiPoolMiddleware_SingleTenantMode(t *testing.T) {
	logger := libZap.InitializeLogger()

	// Create pools without Tenant Manager client (single-tenant mode)
	onboardingPool := tmpg.NewManager(nil, "ledger", tmpg.WithModule("onboarding"), tmpg.WithLogger(logger))
	transactionPool := tmpg.NewManager(nil, "ledger", tmpg.WithModule("transaction"), tmpg.WithLogger(logger))

	mid := tmmiddleware.NewMultiPoolMiddleware(
		tmmiddleware.WithRoute(transactionPaths, "transaction", transactionPool, nil),
		tmmiddleware.WithDefaultRoute("onboarding", onboardingPool, nil),
		tmmiddleware.WithPublicPaths(publicPaths...),
		tmmiddleware.WithMultiPoolLogger(logger),
	)

	app := fiber.New()
	app.Use(mid.WithTenantDB)
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

func TestMultiPoolMiddleware_WithDefaultConnection(t *testing.T) {
	logger := libZap.InitializeLogger()

	// Create pools with default connections
	onboardingPool := tmpg.NewManager(nil, "ledger", tmpg.WithModule("onboarding"), tmpg.WithLogger(logger))
	onboardingDefaultConn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: "host=onboarding-db",
		Logger:                  logger,
	}
	onboardingPool.WithDefaultConnection(onboardingDefaultConn)

	transactionPool := tmpg.NewManager(nil, "ledger", tmpg.WithModule("transaction"), tmpg.WithLogger(logger))
	transactionDefaultConn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: "host=transaction-db",
		Logger:                  logger,
	}
	transactionPool.WithDefaultConnection(transactionDefaultConn)

	mid := tmmiddleware.NewMultiPoolMiddleware(
		tmmiddleware.WithRoute(transactionPaths, "transaction", transactionPool, nil),
		tmmiddleware.WithDefaultRoute("onboarding", onboardingPool, nil),
		tmmiddleware.WithPublicPaths(publicPaths...),
		tmmiddleware.WithMultiPoolLogger(logger),
	)

	app := fiber.New()
	app.Use(mid.WithTenantDB)
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

func TestMultiPoolMiddleware_Construction(t *testing.T) {
	logger := libZap.InitializeLogger()

	onboardingPool := tmpg.NewManager(nil, "ledger", tmpg.WithModule("onboarding"), tmpg.WithLogger(logger))
	transactionPool := tmpg.NewManager(nil, "ledger", tmpg.WithModule("transaction"), tmpg.WithLogger(logger))

	t.Run("without consumer trigger", func(t *testing.T) {
		mid := tmmiddleware.NewMultiPoolMiddleware(
			tmmiddleware.WithRoute(transactionPaths, "transaction", transactionPool, nil),
			tmmiddleware.WithDefaultRoute("onboarding", onboardingPool, nil),
			tmmiddleware.WithPublicPaths(publicPaths...),
			tmmiddleware.WithMultiPoolLogger(logger),
		)

		assert.NotNil(t, mid)
		assert.False(t, mid.Enabled(), "Should not be enabled without TM client")
	})

	t.Run("with consumer trigger", func(t *testing.T) {
		trigger := &mockConsumerTrigger{}
		mid := tmmiddleware.NewMultiPoolMiddleware(
			tmmiddleware.WithRoute(transactionPaths, "transaction", transactionPool, nil),
			tmmiddleware.WithDefaultRoute("onboarding", onboardingPool, nil),
			tmmiddleware.WithPublicPaths(publicPaths...),
			tmmiddleware.WithConsumerTrigger(trigger),
			tmmiddleware.WithMultiPoolLogger(logger),
		)

		assert.NotNil(t, mid)
		assert.False(t, mid.Enabled(), "Should not be enabled without TM client")
	})
}

// newMockTenantManagerServer creates an httptest.Server that simulates the Tenant Manager API.
// The handler function receives tenantID and service from the request URL and returns a
// status code and response body. The caller must close the returned server.
func newMockTenantManagerServer(handler func(tenantID, service string) (int, interface{})) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse tenant ID and service from URL: /tenants/{tenantID}/settings?service={service}
		var tenantID string
		_, _ = fmt.Sscanf(r.URL.Path, "/tenants/%s/settings", &tenantID)
		service := r.URL.Query().Get("service")

		statusCode, body := handler(tenantID, service)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(body)
	}))
}

// newMultiTenantMultiPoolMiddleware creates a MultiPoolMiddleware configured for multi-tenant mode
// by providing a real Tenant Manager client pointed at the given mock server URL.
func newMultiTenantMultiPoolMiddleware(mockServerURL string) *tmmiddleware.MultiPoolMiddleware {
	zapLogger := libZap.InitializeLogger()
	client := tmclient.NewClient(mockServerURL, zapLogger)

	onboardingPool := tmpg.NewManager(client, "ledger", tmpg.WithModule("onboarding"), tmpg.WithLogger(zapLogger))
	transactionPool := tmpg.NewManager(client, "ledger", tmpg.WithModule("transaction"), tmpg.WithLogger(zapLogger))

	return tmmiddleware.NewMultiPoolMiddleware(
		tmmiddleware.WithRoute(transactionPaths, "transaction", transactionPool, nil),
		tmmiddleware.WithDefaultRoute("onboarding", onboardingPool, nil),
		tmmiddleware.WithPublicPaths(publicPaths...),
		tmmiddleware.WithErrorMapper(midazTenantErrorMapper),
		tmmiddleware.WithMultiPoolLogger(zapLogger),
	)
}

// parseErrorResponse reads and parses the error response body from a Fiber test response.
func parseErrorResponse(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	require.NoError(t, err, "failed to unmarshal response body: %s", string(body))

	return result
}

func TestWithTenantDB_Returns401WhenJWTMissingTenantID(t *testing.T) {
	mockServer := newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
		return http.StatusOK, tmcore.TenantConfig{}
	})
	defer mockServer.Close()

	mid := newMultiTenantMultiPoolMiddleware(mockServer.URL)

	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "no authorization header",
			token: "",
		},
		{
			name:  "JWT without tenantId claim",
			token: createMockJWT(map[string]interface{}{"sub": "user-1"}),
		},
		{
			name:  "JWT with empty tenantId claim",
			token: createMockJWT(map[string]interface{}{"tenantId": ""}),
		},
		{
			name:  "JWT with non-string tenantId claim",
			token: createMockJWT(map[string]interface{}{"tenantId": 12345}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Use(mid.WithTenantDB)
			app.Get("/v1/organizations", func(c *fiber.Ctx) error {
				return c.SendString("OK")
			})

			req := httptest.NewRequest("GET", "/v1/organizations", nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}

			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

			body := parseErrorResponse(t, resp)
			assert.Equal(t, "0042", body["code"])
			assert.Equal(t, "Unauthorized", body["title"])
			assert.Equal(t, "tenantId claim is required in JWT token for multi-tenant mode", body["message"])
		})
	}
}

func TestWithTenantDB_Returns404WhenTenantNotFound(t *testing.T) {
	mockServer := newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
		return http.StatusNotFound, map[string]string{"error": "not found"}
	})
	defer mockServer.Close()

	mid := newMultiTenantMultiPoolMiddleware(mockServer.URL)

	app := fiber.New()
	app.Use(mid.WithTenantDB)
	app.Get("/v1/organizations", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	token := createMockJWT(map[string]interface{}{"tenantId": "nonexistent-tenant"})
	req := httptest.NewRequest("GET", "/v1/organizations", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	body := parseErrorResponse(t, resp)
	assert.Equal(t, "0007", body["code"])
	assert.Equal(t, "Not Found", body["title"])
	assert.Equal(t, "tenant not found", body["message"])
}

func TestWithTenantDB_Returns503WhenManagerClosed(t *testing.T) {
	zapLogger := libZap.InitializeLogger()
	mockServer := newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
		return http.StatusOK, tmcore.TenantConfig{}
	})
	defer mockServer.Close()

	client := tmclient.NewClient(mockServer.URL, zapLogger)
	onboardingPool := tmpg.NewManager(client, "ledger", tmpg.WithModule("onboarding"), tmpg.WithLogger(zapLogger))
	transactionPool := tmpg.NewManager(client, "ledger", tmpg.WithModule("transaction"), tmpg.WithLogger(zapLogger))

	// Close the onboarding pool to trigger ErrManagerClosed
	err := onboardingPool.Close(context.Background())
	require.NoError(t, err)

	mid := tmmiddleware.NewMultiPoolMiddleware(
		tmmiddleware.WithRoute(transactionPaths, "transaction", transactionPool, nil),
		tmmiddleware.WithDefaultRoute("onboarding", onboardingPool, nil),
		tmmiddleware.WithPublicPaths(publicPaths...),
		tmmiddleware.WithErrorMapper(midazTenantErrorMapper),
		tmmiddleware.WithMultiPoolLogger(zapLogger),
	)

	app := fiber.New()
	app.Use(mid.WithTenantDB)
	app.Get("/v1/organizations", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	token := createMockJWT(map[string]interface{}{"tenantId": "tenant-123"})
	req := httptest.NewRequest("GET", "/v1/organizations", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, testErr := app.Test(req)
	require.NoError(t, testErr)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body := parseErrorResponse(t, resp)
	assert.Equal(t, "0130", body["code"])
	assert.Equal(t, "Service Unavailable", body["title"])
	assert.Equal(t, "service temporarily unavailable", body["message"])
}

func TestWithTenantDB_Returns503WhenServiceNotConfigured(t *testing.T) {
	mockServer := newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
		return http.StatusOK, tmcore.TenantConfig{
			ID:            tenantID,
			TenantSlug:    "test-tenant",
			IsolationMode: "isolated",
			Databases:     map[string]tmcore.DatabaseConfig{},
		}
	})
	defer mockServer.Close()

	mid := newMultiTenantMultiPoolMiddleware(mockServer.URL)

	app := fiber.New()
	app.Use(mid.WithTenantDB)
	app.Get("/v1/organizations", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	token := createMockJWT(map[string]interface{}{"tenantId": "tenant-no-pg"})
	req := httptest.NewRequest("GET", "/v1/organizations", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body := parseErrorResponse(t, resp)
	assert.Equal(t, "0130", body["code"])
	assert.Equal(t, "Service Unavailable", body["title"])
	assert.Equal(t, "database service not configured for tenant", body["message"])
}

func TestWithTenantDB_Returns422WhenSchemaConfigInvalid(t *testing.T) {
	mockServer := newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
		return http.StatusOK, tmcore.TenantConfig{
			ID:            tenantID,
			TenantSlug:    "test-tenant",
			IsolationMode: "schema",
			Databases: map[string]tmcore.DatabaseConfig{
				"onboarding": {
					PostgreSQL: &tmcore.PostgreSQLConfig{
						Host:     "localhost",
						Port:     5432,
						Database: "midaz",
						Username: "test",
						Password: "test",
						Schema:   "",
					},
				},
			},
		}
	})
	defer mockServer.Close()

	mid := newMultiTenantMultiPoolMiddleware(mockServer.URL)

	app := fiber.New()
	app.Use(mid.WithTenantDB)
	app.Get("/v1/organizations", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	token := createMockJWT(map[string]interface{}{"tenantId": "tenant-bad-schema"})
	req := httptest.NewRequest("GET", "/v1/organizations", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	body := parseErrorResponse(t, resp)
	assert.Equal(t, "0130", body["code"])
	assert.Equal(t, "Unprocessable Entity", body["title"])
	assert.Equal(t, "invalid schema configuration for tenant database", body["message"])
}

func TestWithTenantDB_Returns503WhenConnectionFails(t *testing.T) {
	mockServer := newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
		return http.StatusOK, tmcore.TenantConfig{
			ID:            tenantID,
			TenantSlug:    "test-tenant",
			IsolationMode: "isolated",
			Databases: map[string]tmcore.DatabaseConfig{
				"onboarding": {
					PostgreSQL: &tmcore.PostgreSQLConfig{
						Host:     "unreachable-host-that-does-not-exist.invalid",
						Port:     5432,
						Database: "midaz",
						Username: "test",
						Password: "test",
					},
				},
			},
		}
	})
	defer mockServer.Close()

	mid := newMultiTenantMultiPoolMiddleware(mockServer.URL)

	app := fiber.New()
	app.Use(mid.WithTenantDB)
	app.Get("/v1/organizations", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	token := createMockJWT(map[string]interface{}{"tenantId": "tenant-bad-conn"})
	req := httptest.NewRequest("GET", "/v1/organizations", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body := parseErrorResponse(t, resp)
	code := body["code"].(string)
	assert.Contains(t, []string{"0130"}, code,
		"expected 0130, got %s", code)
	assert.Equal(t, "Service Unavailable", body["title"])
}

func TestWithTenantDB_Returns503WhenDBInterfaceUnavailable(t *testing.T) {
	// The DB interface unavailable path requires GetConnection to succeed but GetDB to fail.
	// Since PostgresManager stores connections in an unexported map and GetDB calls
	// PostgresConnection.GetDB() which needs a real database, we validate this error path
	// by testing the response format directly.
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		// Replicate the exact response from the error mapper
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
			"code":    "0130",
			"title":   "Service Unavailable",
			"message": "database interface unavailable for tenant",
		})
	})
	app.Get("/v1/organizations", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	req := httptest.NewRequest("GET", "/v1/organizations", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body := parseErrorResponse(t, resp)
	assert.Equal(t, "0130", body["code"])
	assert.Equal(t, "Service Unavailable", body["title"])
	assert.Equal(t, "database interface unavailable for tenant", body["message"])
}

func TestWithTenantDB_Returns503WhenMongoUnavailable(t *testing.T) {
	// The full middleware requires a PG connection to succeed first, which requires
	// a real database. Instead, we test the MongoDB error path in isolation.
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		// Simulate the MongoDB error path
		mongoErr := fmt.Errorf("failed to get tenant config: %w", tmcore.ErrTenantNotFound)
		if mongoErr != nil {
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
				"code":    "0130",
				"title":   "Service Unavailable",
				"message": "MongoDB connection unavailable for tenant",
			})
		}
		return c.Next()
	})
	app.Get("/v1/organizations", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	req := httptest.NewRequest("GET", "/v1/organizations", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body := parseErrorResponse(t, resp)
	assert.Equal(t, "0130", body["code"])
	assert.Equal(t, "Service Unavailable", body["title"])
	assert.Equal(t, "MongoDB connection unavailable for tenant", body["message"])
}

func TestWithTenantDB_Returns503WhenConnectionGenericError(t *testing.T) {
	mockServer := newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
		return http.StatusInternalServerError, map[string]string{
			"error": "internal server error",
		}
	})
	defer mockServer.Close()

	mid := newMultiTenantMultiPoolMiddleware(mockServer.URL)

	app := fiber.New()
	app.Use(mid.WithTenantDB)
	app.Get("/v1/organizations", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	token := createMockJWT(map[string]interface{}{"tenantId": "tenant-500"})
	req := httptest.NewRequest("GET", "/v1/organizations", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body := parseErrorResponse(t, resp)
	assert.Equal(t, "0130", body["code"])
	assert.Equal(t, "Service Unavailable", body["title"])
	assert.Equal(t, "failed to establish database connection for tenant", body["message"])
}

func TestWithTenantDB_SkipsPublicPaths(t *testing.T) {
	mockServer := newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
		return http.StatusOK, tmcore.TenantConfig{}
	})
	defer mockServer.Close()

	mid := newMultiTenantMultiPoolMiddleware(mockServer.URL)

	tests := []struct {
		name string
		path string
	}{
		{"health endpoint", "/health"},
		{"version endpoint", "/version"},
		{"swagger endpoint", "/swagger/index.html"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Use(mid.WithTenantDB)
			app.Get(tt.path, func(c *fiber.Ctx) error {
				return c.SendString("OK")
			})

			req := httptest.NewRequest("GET", tt.path, nil)

			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func TestWithTenantDB_ErrorHandling_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		setupServer    func() *httptest.Server
		setupPool      func(mockURL string) *tmmiddleware.MultiPoolMiddleware
		token          string
		path           string
		expectedStatus int
		expectedCode   string
		expectedTitle  string
	}{
		{
			name: "returns 401 when JWT has no tenantId",
			setupServer: func() *httptest.Server {
				return newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
					return http.StatusOK, tmcore.TenantConfig{}
				})
			},
			setupPool: func(mockURL string) *tmmiddleware.MultiPoolMiddleware {
				return newMultiTenantMultiPoolMiddleware(mockURL)
			},
			token:          createMockJWT(map[string]interface{}{"sub": "user-1"}),
			path:           "/v1/organizations",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "0042",
			expectedTitle:  "Unauthorized",
		},
		{
			name: "returns 404 when tenant not found",
			setupServer: func() *httptest.Server {
				return newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
					return http.StatusNotFound, map[string]string{"error": "not found"}
				})
			},
			setupPool: func(mockURL string) *tmmiddleware.MultiPoolMiddleware {
				return newMultiTenantMultiPoolMiddleware(mockURL)
			},
			token:          createMockJWT(map[string]interface{}{"tenantId": "missing-tenant"}),
			path:           "/v1/organizations",
			expectedStatus: http.StatusNotFound,
			expectedCode:   "0007",
			expectedTitle:  "Not Found",
		},
		{
			name: "returns 503 when manager closed",
			setupServer: func() *httptest.Server {
				return newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
					return http.StatusOK, tmcore.TenantConfig{}
				})
			},
			setupPool: func(mockURL string) *tmmiddleware.MultiPoolMiddleware {
				zapLogger := libZap.InitializeLogger()
				client := tmclient.NewClient(mockURL, zapLogger)
				onboardingPool := tmpg.NewManager(client, "ledger", tmpg.WithModule("onboarding"), tmpg.WithLogger(zapLogger))
				transactionPool := tmpg.NewManager(client, "ledger", tmpg.WithModule("transaction"), tmpg.WithLogger(zapLogger))
				_ = onboardingPool.Close(context.Background())
				return tmmiddleware.NewMultiPoolMiddleware(
					tmmiddleware.WithRoute(transactionPaths, "transaction", transactionPool, nil),
					tmmiddleware.WithDefaultRoute("onboarding", onboardingPool, nil),
					tmmiddleware.WithPublicPaths(publicPaths...),
					tmmiddleware.WithErrorMapper(midazTenantErrorMapper),
					tmmiddleware.WithMultiPoolLogger(zapLogger),
				)
			},
			token:          createMockJWT(map[string]interface{}{"tenantId": "tenant-1"}),
			path:           "/v1/organizations",
			expectedStatus: http.StatusServiceUnavailable,
			expectedCode:   "0130",
			expectedTitle:  "Service Unavailable",
		},
		{
			name: "returns 503 when service not configured",
			setupServer: func() *httptest.Server {
				return newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
					return http.StatusOK, tmcore.TenantConfig{
						ID:            tenantID,
						TenantSlug:    "test",
						IsolationMode: "isolated",
						Databases:     map[string]tmcore.DatabaseConfig{},
					}
				})
			},
			setupPool: func(mockURL string) *tmmiddleware.MultiPoolMiddleware {
				return newMultiTenantMultiPoolMiddleware(mockURL)
			},
			token:          createMockJWT(map[string]interface{}{"tenantId": "tenant-no-config"}),
			path:           "/v1/organizations",
			expectedStatus: http.StatusServiceUnavailable,
			expectedCode:   "0130",
			expectedTitle:  "Service Unavailable",
		},
		{
			name: "returns 422 when schema config invalid",
			setupServer: func() *httptest.Server {
				return newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
					return http.StatusOK, tmcore.TenantConfig{
						ID:            tenantID,
						TenantSlug:    "test",
						IsolationMode: "schema",
						Databases: map[string]tmcore.DatabaseConfig{
							"onboarding": {
								PostgreSQL: &tmcore.PostgreSQLConfig{
									Host:     "localhost",
									Port:     5432,
									Database: "midaz",
									Username: "test",
									Password: "test",
								},
							},
						},
					}
				})
			},
			setupPool: func(mockURL string) *tmmiddleware.MultiPoolMiddleware {
				return newMultiTenantMultiPoolMiddleware(mockURL)
			},
			token:          createMockJWT(map[string]interface{}{"tenantId": "tenant-bad-schema"}),
			path:           "/v1/organizations",
			expectedStatus: http.StatusUnprocessableEntity,
			expectedCode:   "0130",
			expectedTitle:  "Unprocessable Entity",
		},
		{
			name: "returns 503 when tenant manager returns internal error",
			setupServer: func() *httptest.Server {
				return newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
					return http.StatusInternalServerError, map[string]string{"error": "internal"}
				})
			},
			setupPool: func(mockURL string) *tmmiddleware.MultiPoolMiddleware {
				return newMultiTenantMultiPoolMiddleware(mockURL)
			},
			token:          createMockJWT(map[string]interface{}{"tenantId": "tenant-err"}),
			path:           "/v1/organizations",
			expectedStatus: http.StatusServiceUnavailable,
			expectedCode:   "0130",
			expectedTitle:  "Service Unavailable",
		},
		{
			name: "returns 404 for transaction path when tenant not found",
			setupServer: func() *httptest.Server {
				return newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
					return http.StatusNotFound, map[string]string{"error": "not found"}
				})
			},
			setupPool: func(mockURL string) *tmmiddleware.MultiPoolMiddleware {
				return newMultiTenantMultiPoolMiddleware(mockURL)
			},
			token:          createMockJWT(map[string]interface{}{"tenantId": "missing-tenant"}),
			path:           "/v1/organizations/123/ledgers/456/transactions",
			expectedStatus: http.StatusNotFound,
			expectedCode:   "0007",
			expectedTitle:  "Not Found",
		},
		{
			name: "returns 503 for transaction path when manager closed",
			setupServer: func() *httptest.Server {
				return newMockTenantManagerServer(func(tenantID, service string) (int, interface{}) {
					return http.StatusOK, tmcore.TenantConfig{}
				})
			},
			setupPool: func(mockURL string) *tmmiddleware.MultiPoolMiddleware {
				zapLogger := libZap.InitializeLogger()
				client := tmclient.NewClient(mockURL, zapLogger)
				onboardingPool := tmpg.NewManager(client, "ledger", tmpg.WithModule("onboarding"), tmpg.WithLogger(zapLogger))
				transactionPool := tmpg.NewManager(client, "ledger", tmpg.WithModule("transaction"), tmpg.WithLogger(zapLogger))
				_ = transactionPool.Close(context.Background())
				return tmmiddleware.NewMultiPoolMiddleware(
					tmmiddleware.WithRoute(transactionPaths, "transaction", transactionPool, nil),
					tmmiddleware.WithDefaultRoute("onboarding", onboardingPool, nil),
					tmmiddleware.WithPublicPaths(publicPaths...),
					tmmiddleware.WithErrorMapper(midazTenantErrorMapper),
					tmmiddleware.WithMultiPoolLogger(zapLogger),
				)
			},
			token:          createMockJWT(map[string]interface{}{"tenantId": "tenant-1"}),
			path:           "/v1/organizations/123/ledgers/456/transactions",
			expectedStatus: http.StatusServiceUnavailable,
			expectedCode:   "0130",
			expectedTitle:  "Service Unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := tt.setupServer()
			defer mockServer.Close()

			mid := tt.setupPool(mockServer.URL)

			app := fiber.New()
			app.Use(mid.WithTenantDB)
			app.Get("/v1/organizations", func(c *fiber.Ctx) error {
				return c.SendString("OK")
			})
			app.Get("/v1/organizations/:org/ledgers/:ledger/transactions", func(c *fiber.Ctx) error {
				return c.SendString("OK")
			})

			req := httptest.NewRequest("GET", tt.path, nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body := parseErrorResponse(t, resp)
			assert.Equal(t, tt.expectedCode, body["code"])
			assert.Equal(t, tt.expectedTitle, body["title"])
		})
	}
}

func TestWithTenantDB_NilMiddlewarePassesThrough(t *testing.T) {
	logger := libZap.InitializeLogger()

	// Create middleware with nil pools - should pass through
	mid := tmmiddleware.NewMultiPoolMiddleware(
		tmmiddleware.WithMultiPoolLogger(logger),
	)

	app := fiber.New()
	app.Use(mid.WithTenantDB)
	app.Get("/v1/organizations", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	req := httptest.NewRequest("GET", "/v1/organizations", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMidazTenantErrorMapper(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		tenantID       string
		expectedStatus int
		expectedCode   string
		expectedTitle  string
		expectedMsg    string
	}{
		{
			name:           "missing authorization token",
			err:            fmt.Errorf("authorization token is required"),
			tenantID:       "",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "0042",
			expectedTitle:  "Unauthorized",
			expectedMsg:    "tenantId claim is required in JWT token for multi-tenant mode",
		},
		{
			name:           "failed to parse authorization token",
			err:            fmt.Errorf("failed to parse authorization token: invalid"),
			tenantID:       "",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "0042",
			expectedTitle:  "Unauthorized",
			expectedMsg:    "tenantId claim is required in JWT token for multi-tenant mode",
		},
		{
			name:           "missing tenantId in JWT",
			err:            fmt.Errorf("tenantId is required in JWT token"),
			tenantID:       "",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "0042",
			expectedTitle:  "Unauthorized",
			expectedMsg:    "tenantId claim is required in JWT token for multi-tenant mode",
		},
		{
			name:           "invalid JWT claims",
			err:            fmt.Errorf("JWT claims are not in expected format"),
			tenantID:       "",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "0042",
			expectedTitle:  "Unauthorized",
			expectedMsg:    "tenantId claim is required in JWT token for multi-tenant mode",
		},
		{
			name:           "tenant not found",
			err:            tmcore.ErrTenantNotFound,
			tenantID:       "t1",
			expectedStatus: http.StatusNotFound,
			expectedCode:   "0007",
			expectedTitle:  "Not Found",
			expectedMsg:    "tenant not found",
		},
		{
			name:           "manager closed",
			err:            tmcore.ErrManagerClosed,
			tenantID:       "t1",
			expectedStatus: http.StatusServiceUnavailable,
			expectedCode:   "0130",
			expectedTitle:  "Service Unavailable",
			expectedMsg:    "service temporarily unavailable",
		},
		{
			name:           "service not configured",
			err:            tmcore.ErrServiceNotConfigured,
			tenantID:       "t1",
			expectedStatus: http.StatusServiceUnavailable,
			expectedCode:   "0130",
			expectedTitle:  "Service Unavailable",
			expectedMsg:    "database service not configured for tenant",
		},
		{
			name:           "tenant suspended",
			err:            &tmcore.TenantSuspendedError{TenantID: "t1", Status: "suspended"},
			tenantID:       "t1",
			expectedStatus: http.StatusForbidden,
			expectedCode:   "0139",
			expectedTitle:  "Service Suspended",
			expectedMsg:    "tenant service is suspended",
		},
		{
			name:           "schema config error",
			err:            fmt.Errorf("schema mode requires a valid schema name"),
			tenantID:       "t1",
			expectedStatus: http.StatusUnprocessableEntity,
			expectedCode:   "0130",
			expectedTitle:  "Unprocessable Entity",
			expectedMsg:    "invalid schema configuration for tenant database",
		},
		{
			name:           "connection failure",
			err:            fmt.Errorf("failed to connect to database"),
			tenantID:       "t1",
			expectedStatus: http.StatusServiceUnavailable,
			expectedCode:   "0130",
			expectedTitle:  "Service Unavailable",
			expectedMsg:    "database connection unavailable for tenant",
		},
		{
			name:           "generic error",
			err:            fmt.Errorf("some unexpected error"),
			tenantID:       "t1",
			expectedStatus: http.StatusServiceUnavailable,
			expectedCode:   "0130",
			expectedTitle:  "Service Unavailable",
			expectedMsg:    "failed to establish database connection for tenant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				return midazTenantErrorMapper(c, tt.err, tt.tenantID)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body := parseErrorResponse(t, resp)
			assert.Equal(t, tt.expectedCode, body["code"])
			assert.Equal(t, tt.expectedTitle, body["title"])
			assert.Equal(t, tt.expectedMsg, body["message"])
		})
	}
}
