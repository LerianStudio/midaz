// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"

	pkgHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
)

func intPtr(v int) *int {
	return &v
}

func TestMigrateAccountShardReturnsBadRequestWhenContextIDsMissing(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	handler := &TransactionHandler{}

	app.Post("/migrate", func(c *fiber.Ctx) error {
		return handler.MigrateAccountShard(&migrateAccountShardInput{Alias: "@alice", TargetShard: intPtr(1)}, c)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/migrate", http.NoBody)
	resp, err := app.Test(req)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, string(body), "organization_id is required")
}

func TestMigrateAccountShardReturnsBadRequestOnInvalidPayload(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	handler := &TransactionHandler{}

	app.Post("/migrate", func(c *fiber.Ctx) error {
		c.Locals("organization_id", uuid.New())
		c.Locals("ledger_id", uuid.New())

		return handler.MigrateAccountShard(nil, c)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/migrate", http.NoBody)
	resp, err := app.Test(req)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, string(body), "invalid migration payload")
}

func TestMigrateAccountShardReturnsBadRequestWhenLedgerIDMissing(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	handler := &TransactionHandler{}

	app.Post("/migrate", func(c *fiber.Ctx) error {
		c.Locals("organization_id", uuid.New())

		return handler.MigrateAccountShard(&migrateAccountShardInput{Alias: "@alice", TargetShard: intPtr(0)}, c)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/migrate", http.NoBody)
	resp, err := app.Test(req)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, string(body), "ledger_id is required")
}

func TestMigrateAccountShardInputAcceptsTargetShardZero(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Post("/migrate", pkgHTTP.WithBody(new(migrateAccountShardInput), func(_ any, c *fiber.Ctx) error {
		return c.SendStatus(http.StatusNoContent)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/migrate", strings.NewReader(`{"alias":"@alice","targetShard":0}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestMigrateAccountShardReturnsBadRequestWhenTargetShardMissing(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	handler := &TransactionHandler{}

	app.Post("/migrate", func(c *fiber.Ctx) error {
		c.Locals("organization_id", uuid.New())
		c.Locals("ledger_id", uuid.New())

		return handler.MigrateAccountShard(&migrateAccountShardInput{Alias: "@alice", TargetShard: nil}, c)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/migrate", http.NoBody)
	resp, err := app.Test(req)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, string(body), "targetShard is required")
}

func TestMigrateAccountShardReturnsServiceUnavailableWhenCommandMissing(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	handler := &TransactionHandler{}

	app.Post("/migrate", func(c *fiber.Ctx) error {
		c.Locals("organization_id", uuid.New())
		c.Locals("ledger_id", uuid.New())

		return handler.MigrateAccountShard(&migrateAccountShardInput{Alias: "@alice", TargetShard: intPtr(1)}, c)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/migrate", http.NoBody)
	resp, err := app.Test(req)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Contains(t, string(body), "transaction command service is unavailable")
}

func TestShardingRebalanceRoutesRequireControlPlaneToken(t *testing.T) {
	t.Setenv("ENV_NAME", "production")
	t.Setenv("SHARDING_ADMIN_TOKEN", "")

	app := fiber.New()
	authClient := middleware.NewAuthClient("", false, nil)

	RegisterRoutesToApp(
		app,
		authClient,
		&TransactionHandler{},
		&OperationHandler{},
		&AssetRateHandler{},
		&BalanceHandler{},
		&OperationRouteHandler{},
		&TransactionRouteHandler{},
	)

	testCases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "status", method: http.MethodGet, path: "/v1/sharding/rebalance/status"},
		{name: "pause", method: http.MethodPost, path: "/v1/sharding/rebalance/pause"},
		{name: "resume", method: http.MethodPost, path: "/v1/sharding/rebalance/resume"},
		{
			name:   "migrations",
			method: http.MethodPost,
			path:   "/v1/organizations/" + uuid.New().String() + "/ledgers/" + uuid.New().String() + "/sharding/migrations",
			body:   `{"alias":"@alice","targetShard":0}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}

			resp, err := app.Test(req)
			require.NoError(t, err)

			body, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)
			require.NoError(t, resp.Body.Close())

			assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

			var payload map[string]any
			require.NoError(t, json.Unmarshal(body, &payload))
			assert.Contains(t, payload["error"], "token is not configured")
		})
	}
}

func TestShardingRebalanceRoutesValidateControlPlaneToken(t *testing.T) {
	t.Setenv("ENV_NAME", "production")
	t.Setenv("SHARDING_ADMIN_TOKEN", "12345678901234567890123456789012")

	app := fiber.New()
	authClient := middleware.NewAuthClient("", false, nil)

	RegisterRoutesToApp(
		app,
		authClient,
		&TransactionHandler{},
		&OperationHandler{},
		&AssetRateHandler{},
		&BalanceHandler{},
		&OperationRouteHandler{},
		&TransactionRouteHandler{},
	)

	testCases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "status", method: http.MethodGet, path: "/v1/sharding/rebalance/status"},
		{name: "pause", method: http.MethodPost, path: "/v1/sharding/rebalance/pause"},
		{name: "resume", method: http.MethodPost, path: "/v1/sharding/rebalance/resume"},
		{
			name:   "migrations",
			method: http.MethodPost,
			path:   "/v1/organizations/" + uuid.New().String() + "/ledgers/" + uuid.New().String() + "/sharding/migrations",
			body:   `{"alias":"@alice","targetShard":0}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			unauthorizedReq := httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				unauthorizedReq.Header.Set("Content-Type", "application/json")
			}

			unauthorizedReq.Header.Set("X-Sharding-Token", "wrong-token")

			resp, err := app.Test(unauthorizedReq)
			require.NoError(t, err)

			unauthorizedBody, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)
			require.NoError(t, resp.Body.Close())
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			assert.Contains(t, strings.TrimSpace(string(unauthorizedBody)), "Unauthorized")

			authorizedReq := httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				authorizedReq.Header.Set("Content-Type", "application/json")
			}

			authorizedReq.Header.Set("X-Sharding-Token", "12345678901234567890123456789012")

			resp, err = app.Test(authorizedReq)
			require.NoError(t, err)

			authorizedBody, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)
			require.NoError(t, resp.Body.Close())
			assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
			assert.Contains(t, string(authorizedBody), "transaction command service is unavailable")
		})
	}
}
