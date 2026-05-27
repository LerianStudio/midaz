// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockChecker implements DependencyChecker for testing.
type mockChecker struct {
	name       string
	tlsEnabled bool
	checkFn    func(ctx context.Context) DependencyCheck
}

func newMockChecker(name string, tlsEnabled bool, status DependencyStatus) *mockChecker {
	return &mockChecker{
		name:       name,
		tlsEnabled: tlsEnabled,
		checkFn: func(_ context.Context) DependencyCheck {
			latency := int64(1)
			return DependencyCheck{
				Status:    status,
				LatencyMs: &latency,
			}
		},
	}
}

func newMockCheckerWithError(name string, tlsEnabled bool, status DependencyStatus, errMsg string) *mockChecker {
	return &mockChecker{
		name:       name,
		tlsEnabled: tlsEnabled,
		checkFn: func(_ context.Context) DependencyCheck {
			latency := int64(1)
			return DependencyCheck{
				Status:    status,
				LatencyMs: &latency,
				Error:     errMsg,
			}
		},
	}
}

func newMockCheckerWithReason(name string, tlsEnabled bool, status DependencyStatus, reason string) *mockChecker {
	return &mockChecker{
		name:       name,
		tlsEnabled: tlsEnabled,
		checkFn: func(_ context.Context) DependencyCheck {
			return DependencyCheck{
				Status: status,
				Reason: reason,
			}
		},
	}
}

func (m *mockChecker) Name() string {
	return m.name
}

func (m *mockChecker) TLSEnabled() bool {
	return m.tlsEnabled
}

func (m *mockChecker) Check(ctx context.Context) DependencyCheck {
	return m.checkFn(ctx)
}

// createTestApp creates a Fiber app with the readyz handler for testing.
func createTestApp(handler *ReadyzHandler) *fiber.App {
	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	return app
}

func TestReadyzHandler_HandleReadyz_AllHealthy(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "local",
		Checkers: []DependencyChecker{
			newMockChecker("mongo", true, StatusUp),
		},
	})
	handler.SetServerReady()

	app := createTestApp(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, "1.0.0", response.Version)
	assert.Equal(t, "local", response.DeploymentMode)
	assert.Contains(t, response.Checks, "mongo")
	assert.Equal(t, StatusUp, response.Checks["mongo"].Status)
}

func TestReadyzHandler_HandleReadyz_OneDown(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "local",
		Checkers: []DependencyChecker{
			newMockChecker("mongo", true, StatusDown),
		},
	})
	handler.SetServerReady()

	app := createTestApp(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "unhealthy", response.Status)
	assert.Equal(t, StatusDown, response.Checks["mongo"].Status)
}

func TestReadyzHandler_HandleReadyz_OneDegraded(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "local",
		Checkers: []DependencyChecker{
			newMockChecker("mongo", true, StatusDegraded),
		},
	})
	handler.SetServerReady()

	app := createTestApp(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "unhealthy", response.Status)
	assert.Equal(t, StatusDegraded, response.Checks["mongo"].Status)
}

func TestReadyzHandler_HandleReadyz_SkippedCountsAsHealthy(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "local",
		Checkers: []DependencyChecker{
			newMockCheckerWithReason("mongo", true, StatusSkipped, "MONGO_ENABLED=false"),
		},
	})
	handler.SetServerReady()

	app := createTestApp(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, StatusSkipped, response.Checks["mongo"].Status)
	assert.Equal(t, "MONGO_ENABLED=false", response.Checks["mongo"].Reason)
}

func TestReadyzHandler_HandleReadyz_NACountsAsHealthy(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "local",
		Checkers: []DependencyChecker{
			newMockCheckerWithReason("mongo", true, StatusNA, "tenant-scoped; use /readyz/tenant/:id"),
		},
	})
	handler.SetServerReady()

	app := createTestApp(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, StatusNA, response.Checks["mongo"].Status)
}

func TestReadyzHandler_HandleReadyz_NoCheckers(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "local",
		Checkers:       []DependencyChecker{},
	})
	handler.SetServerReady()

	app := createTestApp(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.Empty(t, response.Checks)
}

func TestReadyzHandler_LifecycleState_ServerNotReady(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "local",
		Checkers: []DependencyChecker{
			newMockChecker("mongo", true, StatusUp),
		},
	})
	// Do NOT call SetServerReady() - server is not ready

	app := createTestApp(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "unhealthy", response.Status)
	assert.Contains(t, response.Reason, "server not ready")
}

func TestReadyzHandler_LifecycleState_Draining(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "local",
		Checkers: []DependencyChecker{
			newMockChecker("mongo", true, StatusUp),
		},
	})
	handler.SetServerReady()
	handler.StartDrain()

	app := createTestApp(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "unhealthy", response.Status)
	assert.Contains(t, response.Reason, "draining")
}

func TestReadyzHandler_ErrorSanitization_LocalMode(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "local",
		Checkers: []DependencyChecker{
			newMockCheckerWithError("mongo", true, StatusDown, "connection refused to 192.168.1.100:27017"),
		},
	})
	handler.SetServerReady()

	app := createTestApp(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	// In local mode, full error is exposed
	assert.Contains(t, response.Checks["mongo"].Error, "192.168.1.100")
}

func TestReadyzHandler_ErrorSanitization_SaaSMode(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "saas",
		Checkers: []DependencyChecker{
			newMockCheckerWithError("mongo", true, StatusDown, "connection refused to 192.168.1.100:27017"),
		},
	})
	handler.SetServerReady()

	app := createTestApp(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	// In saas mode, error is sanitized (should not contain internal IP)
	assert.NotContains(t, response.Checks["mongo"].Error, "192.168.1.100")
	assert.Contains(t, response.Checks["mongo"].Error, "mongo check failed")
}

func TestReadyzHandler_ErrorSanitization_BYOCMode(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "byoc",
		Checkers: []DependencyChecker{
			newMockCheckerWithError("mongo", true, StatusDown, "connection refused to 192.168.1.100:27017"),
		},
	})
	handler.SetServerReady()

	app := createTestApp(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	// In byoc mode, error is sanitized (should not contain internal IP)
	assert.NotContains(t, response.Checks["mongo"].Error, "192.168.1.100")
	assert.Contains(t, response.Checks["mongo"].Error, "mongo check failed")
}

func TestReadyzHandler_TLSFieldSet(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "local",
		Checkers: []DependencyChecker{
			newMockChecker("mongo_tls", true, StatusUp),
			newMockChecker("mongo_notls", false, StatusUp),
		},
	})
	handler.SetServerReady()

	app := createTestApp(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	// Check TLS field is set correctly
	require.NotNil(t, response.Checks["mongo_tls"].TLS)
	assert.True(t, *response.Checks["mongo_tls"].TLS)

	require.NotNil(t, response.Checks["mongo_notls"].TLS)
	assert.False(t, *response.Checks["mongo_notls"].TLS)
}

func TestReadyzHandler_SetServerReady(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "local",
	})

	assert.False(t, handler.IsServerReady())

	handler.SetServerReady()

	assert.True(t, handler.IsServerReady())
}

func TestReadyzHandler_StartDrain(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "local",
	})

	assert.False(t, handler.IsDraining())

	handler.StartDrain()

	assert.True(t, handler.IsDraining())
}

func TestReadyzHandler_MultipleCheckers(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "local",
		Checkers: []DependencyChecker{
			newMockChecker("mongo", true, StatusUp),
			newMockChecker("upstream_api", true, StatusUp),
		},
	})
	handler.SetServerReady()

	app := createTestApp(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.Len(t, response.Checks, 2)
	assert.Contains(t, response.Checks, "mongo")
	assert.Contains(t, response.Checks, "upstream_api")
}

func TestReadyzHandler_MixedStatusWithDownFails(t *testing.T) {
	t.Parallel()

	handler := NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Version:        "1.0.0",
		DeploymentMode: "local",
		Checkers: []DependencyChecker{
			newMockChecker("mongo", true, StatusUp),
			newMockChecker("upstream_api", true, StatusDown),
		},
	})
	handler.SetServerReady()

	app := createTestApp(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "unhealthy", response.Status)
}

func TestNAChecker(t *testing.T) {
	t.Parallel()

	checker := NewNAChecker("mongo", "tenant-scoped; use /readyz/tenant/:id", true)

	assert.Equal(t, "mongo", checker.Name())
	assert.True(t, checker.TLSEnabled())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	check := checker.Check(ctx)

	assert.Equal(t, StatusNA, check.Status)
	assert.Equal(t, "tenant-scoped; use /readyz/tenant/:id", check.Reason)
}

func TestNAChecker_TLSDisabled(t *testing.T) {
	t.Parallel()

	checker := NewNAChecker("mongo", "some reason", false)

	assert.Equal(t, "mongo", checker.Name())
	assert.False(t, checker.TLSEnabled())

	ctx := context.Background()
	check := checker.Check(ctx)

	assert.Equal(t, StatusNA, check.Status)
	assert.Equal(t, "some reason", check.Reason)
}

// ============================================================================
// VaultChecker Tests (ST-003-02)
// ============================================================================

func TestVaultChecker_Name(t *testing.T) {
	t.Parallel()

	checker := NewVaultChecker("vault", nil, "https://vault.example.com:8200")

	assert.Equal(t, "vault", checker.Name())
}

func TestVaultChecker_TLSEnabled_HTTPS(t *testing.T) {
	t.Parallel()

	checker := NewVaultChecker("vault", nil, "https://vault.example.com:8200")

	assert.True(t, checker.TLSEnabled(),
		"VaultChecker must return TLSEnabled=true for HTTPS address")
}

func TestVaultChecker_TLSEnabled_HTTP(t *testing.T) {
	t.Parallel()

	checker := NewVaultChecker("vault", nil, "http://vault.example.com:8200")

	assert.False(t, checker.TLSEnabled(),
		"VaultChecker must return TLSEnabled=false for HTTP address")
}

func TestVaultChecker_Check_NilClient(t *testing.T) {
	t.Parallel()

	checker := NewVaultChecker("vault", nil, "https://vault.example.com:8200")

	ctx := context.Background()
	check := checker.Check(ctx)

	assert.Equal(t, StatusSkipped, check.Status,
		"VaultChecker must return StatusSkipped when client is nil")
	assert.Contains(t, check.Reason, "not configured",
		"Reason must indicate client not configured")
}

func TestVaultChecker_Check_ClientAuthenticated(t *testing.T) {
	t.Parallel()

	// Create a mock client that reports as authenticated
	mockClient := &mockVaultClient{authenticated: true}
	checker := NewVaultCheckerWithClient("vault", mockClient, "https://vault.example.com:8200")

	ctx := context.Background()
	check := checker.Check(ctx)

	assert.Equal(t, StatusUp, check.Status,
		"VaultChecker must return StatusUp when client is authenticated")
	assert.Nil(t, check.LatencyMs,
		"LatencyMs must be nil for simple flag check (no network call)")
}

func TestVaultChecker_Check_ClientNotAuthenticated(t *testing.T) {
	t.Parallel()

	// Create a mock client that reports as NOT authenticated
	mockClient := &mockVaultClient{authenticated: false}
	checker := NewVaultCheckerWithClient("vault", mockClient, "https://vault.example.com:8200")

	ctx := context.Background()
	check := checker.Check(ctx)

	assert.Equal(t, StatusDown, check.Status,
		"VaultChecker must return StatusDown when client is not authenticated")
	assert.Contains(t, check.Error, "not authenticated",
		"Error must indicate authentication failure")
}

func TestVaultChecker_TLSDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		addr    string
		wantTLS bool
	}{
		{
			name:    "https scheme",
			addr:    "https://vault.example.com:8200",
			wantTLS: true,
		},
		{
			name:    "http scheme",
			addr:    "http://vault.example.com:8200",
			wantTLS: false,
		},
		{
			name:    "HTTPS uppercase",
			addr:    "HTTPS://vault.example.com:8200",
			wantTLS: true,
		},
		{
			name:    "no scheme defaults to false",
			addr:    "vault.example.com:8200",
			wantTLS: false,
		},
		{
			name:    "empty address",
			addr:    "",
			wantTLS: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			checker := NewVaultChecker("vault", nil, tt.addr)

			assert.Equal(t, tt.wantTLS, checker.TLSEnabled(),
				"TLSEnabled() must match expected value for address: %s", tt.addr)
		})
	}
}

// mockVaultClient is a test double for vault authentication checking.
type mockVaultClient struct {
	authenticated bool
}

func (m *mockVaultClient) IsAuthenticated() bool {
	return m.authenticated
}
