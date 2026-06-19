//go:build integration

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

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	vaulttestutil "github.com/LerianStudio/midaz/v3/tests/utils/vault"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// initKMS Integration Tests
// ============================================================================

func TestIntegration_InitKMS_LegacyMode(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := &Config{
		KMSVendor:      "none",
		DeploymentMode: "local",
	}
	logger := libLog.NewNop()
	ctx := context.Background()

	// Act
	result, err := initKMS(ctx, cfg, logger)

	// Assert
	require.NoError(t, err, "initKMS should not return error for legacy mode")
	require.NotNil(t, result)
	assert.True(t, result.Mode.IsLegacy(), "mode should be legacy")
	assert.Nil(t, result.VaultClient, "VaultClient should be nil in legacy mode")
}

func TestIntegration_InitKMS_LegacyMode_EmptyVendor(t *testing.T) {
	t.Parallel()

	// Arrange - empty vendor should default to legacy
	cfg := &Config{
		KMSVendor:      "",
		DeploymentMode: "local",
	}
	logger := libLog.NewNop()
	ctx := context.Background()

	// Act
	result, err := initKMS(ctx, cfg, logger)

	// Assert
	require.NoError(t, err, "initKMS should not return error for empty vendor")
	require.NotNil(t, result)
	assert.True(t, result.Mode.IsLegacy(), "empty vendor should default to legacy mode")
}

func TestIntegration_InitKMS_InvalidVendor_FailsClosed(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := &Config{
		KMSVendor:      "invalid-vendor",
		DeploymentMode: "local",
	}
	logger := libLog.NewNop()
	ctx := context.Background()

	// Act
	result, err := initKMS(ctx, cfg, logger)

	// Assert - invalid vendor should fail closed
	require.Error(t, err, "initKMS should return error for invalid vendor")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unsupported KMS vendor")
}

func TestIntegration_InitKMS_EnvelopeMode_LocalDeployment(t *testing.T) {
	// Arrange - Start Vault container
	vaultContainer := vaulttestutil.SetupContainer(t)

	cfg := &Config{
		KMSVendor:      "hashicorp-vault",
		VaultAddr:      vaultContainer.Address,
		VaultMountPath: "transit",
		DeploymentMode: "local", // Local mode uses hardcoded root token
	}
	logger := libLog.NewNop()
	ctx := context.Background()

	// Act
	result, err := initKMS(ctx, cfg, logger)

	// Assert
	require.NoError(t, err, "initKMS should not return error with valid Vault config")
	require.NotNil(t, result)
	assert.True(t, result.Mode.IsEnvelope(), "mode should be envelope")
	assert.NotNil(t, result.VaultClient, "VaultClient should be initialized")
	assert.True(t, result.VaultClient.IsAuthenticated(), "VaultClient should be authenticated")
}

func TestIntegration_InitKMS_EnvelopeMode_ProductionWithoutCredentials(t *testing.T) {
	t.Parallel()

	// Arrange - Production mode without AppRole credentials
	cfg := &Config{
		KMSVendor:      "hashicorp-vault",
		VaultAddr:      "https://vault.example.com:8200",
		VaultMountPath: "transit",
		DeploymentMode: "saas",
		// No VaultRoleID or VaultSecretID
	}
	logger := libLog.NewNop()
	ctx := context.Background()

	// Act
	result, err := initKMS(ctx, cfg, logger)

	// Assert - should fail because production requires AppRole credentials
	require.Error(t, err, "initKMS should return error for production mode without credentials")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "vault config", "error should mention vault configuration")
}

// TestIntegration_InitKMS_EnvelopeMode_VaultUnreachable_AppRoleAuth tests that
// initKMS fails gracefully when Vault is unreachable in production mode (AppRole auth).
//
// Note: In local mode (token auth), Login() does NOT make network calls - it simply
// sets the token. Network issues would only be detected on first Vault operation.
// This test uses production mode (AppRole) which DOES make a network call during login.
func TestIntegration_InitKMS_EnvelopeMode_VaultUnreachable_AppRoleAuth(t *testing.T) {
	t.Parallel()

	// Arrange - Configure envelope mode with unreachable Vault address
	// Use production mode (saas) with AppRole credentials to force network call during login
	cfg := &Config{
		KMSVendor:      "hashicorp-vault",
		VaultAddr:      "http://unreachable-vault-host:8200",
		VaultRoleID:    "fake-role-id",
		VaultSecretID:  "fake-secret-id",
		VaultMountPath: "transit",
		DeploymentMode: "saas", // Production mode uses AppRole which makes network calls
	}
	logger := libLog.NewNop()

	// Use a short timeout to avoid long test execution
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Act
	result, err := initKMS(ctx, cfg, logger)

	// Assert - should fail because Vault is unreachable during AppRole login
	require.Error(t, err, "initKMS should return error when Vault is unreachable")
	assert.Nil(t, result, "result should be nil when Vault is unreachable")
	assert.Contains(t, err.Error(), "authenticate",
		"error should indicate authentication failure")
}

// TestIntegration_InitKMS_TokenAuth_DeferredValidation documents that token auth
// defers network validation to the first Vault operation.
//
// This is intentional behavior: In local/dev mode, we want fast startup even if
// Vault is temporarily unavailable. Errors will surface on first encrypt/decrypt.
func TestIntegration_InitKMS_TokenAuth_DeferredValidation(t *testing.T) {
	t.Parallel()

	// Arrange - Configure with unreachable address but local mode (token auth)
	cfg := &Config{
		KMSVendor:      "hashicorp-vault",
		VaultAddr:      "http://unreachable-vault-host:8200",
		VaultMountPath: "transit",
		DeploymentMode: "local", // Token auth does NOT make network calls during Login()
	}
	logger := libLog.NewNop()
	ctx := context.Background()

	// Act
	result, err := initKMS(ctx, cfg, logger)

	// Assert - Token auth succeeds at login time without network validation
	// This is documented behavior: token is only validated on first operation
	require.NoError(t, err,
		"Token auth login should succeed without network validation (by design)")
	require.NotNil(t, result)
	assert.True(t, result.Mode.IsEnvelope())
	assert.NotNil(t, result.VaultClient)
	assert.True(t, result.VaultClient.IsAuthenticated(),
		"Client reports authenticated (token set, not network-validated)")
}

// TestIntegration_InitKMS_ContextCancellation_AppRoleAuth tests that context
// cancellation is properly handled during AppRole authentication.
//
// Note: Token auth does not make network calls during Login(), so context
// cancellation has no effect. This test uses AppRole to verify proper handling.
func TestIntegration_InitKMS_ContextCancellation_AppRoleAuth(t *testing.T) {
	t.Parallel()

	// Arrange - Configure with AppRole auth to force network call
	cfg := &Config{
		KMSVendor:      "hashicorp-vault",
		VaultAddr:      "http://10.255.255.1:8200", // Non-routable IP
		VaultRoleID:    "fake-role-id",
		VaultSecretID:  "fake-secret-id",
		VaultMountPath: "transit",
		DeploymentMode: "saas", // AppRole makes network calls
	}
	logger := libLog.NewNop()

	// Create a context that is already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Act
	result, err := initKMS(ctx, cfg, logger)

	// Assert - should fail due to cancelled context during AppRole login
	require.Error(t, err, "initKMS should return error when context is cancelled")
	assert.Nil(t, result, "result should be nil when context is cancelled")
}

// TestIntegration_InitKMS_ContextTimeout_AppRoleAuth tests that context timeout
// is properly handled during AppRole authentication.
func TestIntegration_InitKMS_ContextTimeout_AppRoleAuth(t *testing.T) {
	t.Parallel()

	// Arrange - Configure with AppRole auth to force network call
	cfg := &Config{
		KMSVendor:      "hashicorp-vault",
		VaultAddr:      "http://10.255.255.1:8200", // Non-routable IP
		VaultRoleID:    "fake-role-id",
		VaultSecretID:  "fake-secret-id",
		VaultMountPath: "transit",
		DeploymentMode: "saas", // AppRole makes network calls
	}
	logger := libLog.NewNop()

	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Act
	result, err := initKMS(ctx, cfg, logger)

	// Assert - should fail due to timeout during AppRole login
	require.Error(t, err, "initKMS should return error when context times out")
	assert.Nil(t, result, "result should be nil when context times out")
}

// ============================================================================
// buildVaultConfig Integration Tests
// ============================================================================

func TestIntegration_BuildVaultConfig_LocalMode_UsesHardcodedToken(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := &Config{
		VaultAddr:      "http://localhost:8200",
		VaultMountPath: "transit",
		DeploymentMode: "local",
	}

	// Act
	vaultCfg := buildVaultConfig(cfg)

	// Assert
	assert.Equal(t, DefaultVaultDevToken, vaultCfg.Token, "local mode should use hardcoded token")
	assert.Equal(t, "token", string(vaultCfg.AuthMethod), "local mode should use token auth")
}

func TestIntegration_BuildVaultConfig_LocalMode_IgnoresAppRoleCredentials(t *testing.T) {
	t.Parallel()

	// Arrange - AppRole credentials provided but should be ignored in local mode
	cfg := &Config{
		VaultAddr:      "http://localhost:8200",
		VaultRoleID:    "role-123",
		VaultSecretID:  "secret-456",
		VaultMountPath: "transit",
		DeploymentMode: "local",
	}

	// Act
	vaultCfg := buildVaultConfig(cfg)

	// Assert - should still use token auth
	assert.Equal(t, DefaultVaultDevToken, vaultCfg.Token, "local mode should use hardcoded token")
	assert.Equal(t, "token", string(vaultCfg.AuthMethod), "local mode should ignore AppRole")
}

func TestIntegration_BuildVaultConfig_ProductionMode_UsesAppRole(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := &Config{
		VaultAddr:      "https://vault.example.com:8200",
		VaultRoleID:    "role-123",
		VaultSecretID:  "secret-456",
		VaultMountPath: "transit",
		DeploymentMode: "saas",
	}

	// Act
	vaultCfg := buildVaultConfig(cfg)

	// Assert
	assert.Empty(t, vaultCfg.Token, "production mode should not set token")
	assert.Equal(t, "approle", string(vaultCfg.AuthMethod), "production mode should use AppRole")
	assert.Equal(t, cfg.VaultRoleID, vaultCfg.RoleID)
	assert.Equal(t, cfg.VaultSecretID, vaultCfg.SecretID)
}

func TestIntegration_BuildVaultConfig_BYOCMode_UsesAppRole(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := &Config{
		VaultAddr:      "https://vault.customer.com:8200",
		VaultRoleID:    "byoc-role",
		VaultSecretID:  "byoc-secret",
		VaultMountPath: "crm-transit",
		DeploymentMode: "byoc",
	}

	// Act
	vaultCfg := buildVaultConfig(cfg)

	// Assert
	assert.Empty(t, vaultCfg.Token, "byoc mode should not set token")
	assert.Equal(t, "approle", string(vaultCfg.AuthMethod), "byoc mode should use AppRole")
}

// ============================================================================
// VaultClient Integration Tests
// ============================================================================

func TestIntegration_VaultClient_Authentication(t *testing.T) {
	// Arrange
	vaultContainer := vaulttestutil.SetupContainer(t)
	client := vaulttestutil.CreateClient(t, vaultContainer)

	// Assert
	assert.True(t, client.IsAuthenticated(), "client should be authenticated after login")
}

// TestIntegration_VaultClient_IsAuthenticated_AfterLogin verifies that the Vault client
// is properly authenticated after initialization using the local dev token.
//
// Note: This test uses a Vault dev container which starts with the root token.
// The test does NOT require the Transit secrets engine to be enabled because
// authentication (token validation) is independent of Transit engine availability.
// Transit engine is only needed for actual encrypt/decrypt operations.
func TestIntegration_VaultClient_IsAuthenticated_AfterLogin(t *testing.T) {
	// Arrange
	vaultContainer := vaulttestutil.SetupContainer(t)

	cfg := &Config{
		VaultAddr:      vaultContainer.Address,
		VaultMountPath: "transit", // Mount path is required but Transit engine isn't used for auth
		DeploymentMode: "local",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Act
	client, err := initVaultClient(ctx, cfg, libLog.NewNop())

	// Assert - Token authentication must succeed with Vault dev container
	// The dev container uses the same root token (DefaultVaultDevToken) that
	// we configure in local deployment mode.
	require.NoError(t, err, "initVaultClient must succeed with Vault dev container")
	require.NotNil(t, client, "client must not be nil after successful initialization")
	assert.True(t, client.IsAuthenticated(),
		"client must be authenticated after successful login")
}

// ============================================================================
// VaultChecker Integration Tests
// ============================================================================

func TestIntegration_VaultChecker_WithRealClient(t *testing.T) {
	// Arrange
	vaultContainer := vaulttestutil.SetupContainer(t)
	client := vaulttestutil.CreateClient(t, vaultContainer)

	checker := NewVaultChecker("vault", client, vaultContainer.Address)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Act
	check := checker.Check(ctx)

	// Assert
	assert.Equal(t, StatusUp, check.Status, "checker should return up for healthy Vault")
	assert.NotNil(t, check.LatencyMs, "latency should be measured for health check")
}

func TestIntegration_VaultChecker_TLSDetection_HTTP(t *testing.T) {
	// Arrange
	vaultContainer := vaulttestutil.SetupContainer(t)

	checker := NewVaultChecker("vault", nil, vaultContainer.Address)

	// Assert - test container uses HTTP
	assert.False(t, checker.TLSEnabled(), "test container should use HTTP (no TLS)")
}

func TestIntegration_VaultChecker_TLSDetection_HTTPS(t *testing.T) {
	t.Parallel()

	// Arrange - mock HTTPS address
	checker := NewVaultChecker("vault", nil, "https://vault.example.com:8200")

	// Assert
	assert.True(t, checker.TLSEnabled(), "HTTPS address should report TLS enabled")
}

// ============================================================================
// Readyz Integration with VaultChecker
// ============================================================================

func TestIntegration_Readyz_WithVaultChecker(t *testing.T) {
	// Arrange
	vaultContainer := vaulttestutil.SetupContainer(t)
	client := vaulttestutil.CreateClient(t, vaultContainer)

	checkers := []DependencyChecker{
		NewVaultChecker("vault", client, vaultContainer.Address),
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
		Version:        "1.0.0-test",
		DeploymentMode: "local",
	})

	// Create Fiber app for testing
	app := setupFiberApp(handler)

	// Act
	resp := performReadyzRequest(t, app)

	// Assert
	assert.Equal(t, 200, resp.StatusCode)

	response := parseReadyzResponse(t, resp)
	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, StatusUp, response.Checks["vault"].Status)
}

func TestIntegration_Readyz_MixedCheckers_MongoAndVault(t *testing.T) {
	// Arrange
	vaultContainer := vaulttestutil.SetupContainer(t)
	client := vaulttestutil.CreateClient(t, vaultContainer)

	// Mix of VaultChecker (up) and NAChecker (n/a for mongo)
	checkers := []DependencyChecker{
		NewVaultChecker("vault", client, vaultContainer.Address),
		NewNAChecker("mongo", "MongoDB not configured in test", false),
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
		Version:        "1.0.0-test",
		DeploymentMode: "local",
	})

	app := setupFiberApp(handler)

	// Act
	resp := performReadyzRequest(t, app)

	// Assert - should be healthy (n/a doesn't count as unhealthy)
	assert.Equal(t, 200, resp.StatusCode)

	response := parseReadyzResponse(t, resp)
	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, StatusUp, response.Checks["vault"].Status)
	assert.Equal(t, StatusNA, response.Checks["mongo"].Status)
}

func TestIntegration_Readyz_VaultChecker_LegacyModeShowsNA(t *testing.T) {
	t.Parallel()

	// Arrange - Simulate legacy mode where Vault is not used
	checkers := []DependencyChecker{
		NewNAChecker("vault", "Legacy encryption mode (Vault not used)", false),
	}

	handler := newReadyHandler(ReadyzHandlerConfig{
		Logger:         libLog.NewNop(),
		Checkers:       checkers,
		Version:        "1.0.0-test",
		DeploymentMode: "local",
	})

	app := setupFiberApp(handler)

	// Act
	resp := performReadyzRequest(t, app)

	// Assert
	assert.Equal(t, 200, resp.StatusCode)

	response := parseReadyzResponse(t, resp)
	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, StatusNA, response.Checks["vault"].Status)
	assert.Equal(t, "Legacy encryption mode (Vault not used)", response.Checks["vault"].Reason)
}

// ============================================================================
// resolveVaultAuth Integration Tests
// ============================================================================

func TestIntegration_ResolveVaultAuth_DeploymentModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		deploymentMode string
		hasAppRole     bool
		wantMethod     string
		wantToken      string
	}{
		{
			name:           "local mode uses hardcoded token",
			deploymentMode: "local",
			hasAppRole:     false,
			wantMethod:     "token",
			wantToken:      DefaultVaultDevToken,
		},
		{
			name:           "local mode ignores AppRole",
			deploymentMode: "local",
			hasAppRole:     true,
			wantMethod:     "token",
			wantToken:      DefaultVaultDevToken,
		},
		{
			name:           "empty mode defaults to local",
			deploymentMode: "",
			hasAppRole:     true,
			wantMethod:     "token",
			wantToken:      DefaultVaultDevToken,
		},
		{
			name:           "saas mode uses AppRole",
			deploymentMode: "saas",
			hasAppRole:     true,
			wantMethod:     "approle",
			wantToken:      "",
		},
		{
			name:           "saas mode without AppRole returns empty",
			deploymentMode: "saas",
			hasAppRole:     false,
			wantMethod:     "",
			wantToken:      "",
		},
		{
			name:           "byoc mode uses AppRole",
			deploymentMode: "byoc",
			hasAppRole:     true,
			wantMethod:     "approle",
			wantToken:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			method, token := resolveVaultAuth(tt.deploymentMode, tt.hasAppRole)

			// Assert
			assert.Equal(t, tt.wantMethod, string(method))
			assert.Equal(t, tt.wantToken, token)
		})
	}
}

// ============================================================================
// Encryption Mode Resolution Integration Tests
// ============================================================================

func TestIntegration_ResolveEncryptionMode_ValidVendors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		vendor   string
		wantMode crypto.EncryptionMode
	}{
		{"empty defaults to legacy", "", crypto.EncryptionModeLegacy},
		{"none is legacy", "none", crypto.EncryptionModeLegacy},
		{"NONE uppercase is legacy", "NONE", crypto.EncryptionModeLegacy},
		{"hashicorp-vault is envelope", "hashicorp-vault", crypto.EncryptionModeEnvelope},
		{"HASHICORP-VAULT uppercase is envelope", "HASHICORP-VAULT", crypto.EncryptionModeEnvelope},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{KMSVendor: tt.vendor}

			mode, err := resolveEncryptionMode(cfg)

			require.NoError(t, err)
			assert.Equal(t, tt.wantMode, mode)
		})
	}
}

// ============================================================================
// Test Helpers
// ============================================================================

func setupFiberApp(handler *ReadyzHandler) *fiber.App {
	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	return app
}

type testResponse struct {
	StatusCode int
	Body       io.ReadCloser
}

func performReadyzRequest(t *testing.T, app *fiber.App) *testResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req, 10000)
	require.NoError(t, err)

	return &testResponse{
		StatusCode: resp.StatusCode,
		Body:       resp.Body,
	}
}

func parseReadyzResponse(t *testing.T, resp *testResponse) ReadyzResponse {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response ReadyzResponse
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	return response
}
