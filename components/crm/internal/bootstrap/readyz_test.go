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
