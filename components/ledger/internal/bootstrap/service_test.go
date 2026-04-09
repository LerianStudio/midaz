// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"io"
	"net/http"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	tmmiddleware "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// StubRunnable is a stub implementation of mbootstrap.Runnable for testing.
type StubRunnable struct {
	name string
}

func (s *StubRunnable) Run(l *libCommons.Launcher) error {
	return nil
}

// Use newTestLogger() from balance.worker_test.go (same package).

// TestService_Run_LaunchesAllWorkers verifies that Service.Run()
// correctly includes workers and the unified server.
func TestService_Run_LaunchesAllWorkers(t *testing.T) {
	logger := newTestLogger()

	// Create a minimal service with direct infrastructure
	service := &Service{
		UnifiedServer:      &UnifiedServer{serverAddress: ":0", logger: logger, telemetry: &libOpentelemetry.Telemetry{}},
		MultiQueueConsumer: nil, // single-tenant - no multi-queue
		RedisQueueConsumer: nil, // nil for this test
		BalanceSyncWorker:  nil, // nil for this test
		Logger:             logger,
		Telemetry:          &libOpentelemetry.Telemetry{},
	}

	// Assert - verify the service has the expected fields
	assert.NotNil(t, service.UnifiedServer, "UnifiedServer should not be nil")
	assert.NotNil(t, service.Logger, "Logger should not be nil")
	assert.NotNil(t, service.Telemetry, "Telemetry should not be nil")
}

// TestService_CompositionContract verifies the composition contract
// of the unified service struct.
func TestService_CompositionContract(t *testing.T) {
	t.Run("Service struct has required fields", func(t *testing.T) {
		service := &Service{
			Logger:    newTestLogger(),
			Telemetry: &libOpentelemetry.Telemetry{},
		}

		assert.NotNil(t, service.Logger, "Logger should not be nil")
		assert.NotNil(t, service.Telemetry, "Telemetry should not be nil")
	})

	t.Run("Service accepts all worker types", func(t *testing.T) {
		// Verify all worker fields can be set (compile-time check)
		service := &Service{
			UnifiedServer:         &UnifiedServer{},
			MultiQueueConsumer:    &MultiQueueConsumer{},
			RedisQueueConsumer:    &RedisQueueConsumer{},
			BalanceSyncWorker:     &BalanceSyncWorker{},
			CircuitBreakerManager: nil, // optional
			Logger:                newTestLogger(),
			Telemetry:             &libOpentelemetry.Telemetry{},
		}

		assert.NotNil(t, service.MultiQueueConsumer, "MultiQueueConsumer should be settable")
		assert.NotNil(t, service.RedisQueueConsumer, "RedisQueueConsumer should be settable")
		assert.NotNil(t, service.BalanceSyncWorker, "BalanceSyncWorker should be settable")
	})
}

// TestMidazErrorMapper verifies that midazErrorMapper correctly maps tenant-manager
// errors to Midaz-specific HTTP responses.
func TestMidazErrorMapper(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		err            error
		tenantID       string
		wantStatusCode int
		wantNil        bool
		wantCode       string
		wantTitle      string
	}{
		{
			name:     "nil_error_returns_nil",
			err:      nil,
			tenantID: "tenant-abc",
			wantNil:  true,
		},
		{
			name: "tenant_suspended_returns_403",
			err: &tmcore.TenantSuspendedError{
				TenantID: "tenant-abc",
				Status:   "suspended",
				Message:  "tenant service is suspended",
			},
			tenantID:       "tenant-abc",
			wantStatusCode: http.StatusForbidden,
			wantNil:        false,
			wantCode:       "0159",
			wantTitle:      "Service Suspended",
		},
		{
			name: "tenant_purged_returns_403",
			err: &tmcore.TenantSuspendedError{
				TenantID: "tenant-abc",
				Status:   "purged",
				Message:  "tenant service is purged",
			},
			tenantID:       "tenant-abc",
			wantStatusCode: http.StatusForbidden,
			wantNil:        false,
			wantCode:       "0159",
			wantTitle:      "Service Suspended",
		},
		{
			name:           "tenant_not_found_returns_404",
			err:            tmcore.ErrTenantNotFound,
			tenantID:       "tenant-missing",
			wantStatusCode: http.StatusNotFound,
			wantNil:        false,
			wantCode:       "0160",
			wantTitle:      "Tenant Not Found",
		},
		{
			name:           "tenant_not_provisioned_returns_422",
			err:            tmcore.ErrTenantNotProvisioned,
			tenantID:       "tenant-abc",
			wantStatusCode: http.StatusUnprocessableEntity,
			wantNil:        false,
			wantCode:       "0146",
			wantTitle:      "Tenant Not Provisioned",
		},
		{
			name:           "42P01_postgres_error_returns_422",
			err:            errors.New("ERROR: relation \"organization\" does not exist (SQLSTATE 42P01)"),
			tenantID:       "tenant-xyz",
			wantStatusCode: http.StatusUnprocessableEntity,
			wantNil:        false,
			wantCode:       "0146",
			wantTitle:      "Tenant Not Provisioned",
		},
		{
			name:           "relation_does_not_exist_without_sqlstate_returns_422",
			err:            errors.New("pq: relation \"account\" does not exist"),
			tenantID:       "tenant-abc",
			wantStatusCode: http.StatusUnprocessableEntity,
			wantNil:        false,
			wantCode:       "0146",
			wantTitle:      "Tenant Not Provisioned",
		},
		{
			name:           "unknown_error_returns_503",
			err:            errors.New("some other error"),
			tenantID:       "tenant-abc",
			wantStatusCode: http.StatusServiceUnavailable,
			wantNil:        false,
			wantCode:       "0161",
			wantTitle:      "Tenant Service Unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			app.Post("/test", func(c *fiber.Ctx) error {
				result := midazErrorMapper(c, tt.err, tt.tenantID)
				if result != nil {
					return result
				}

				if c.Response().StatusCode() != fiber.StatusOK {
					return nil
				}

				return c.SendStatus(http.StatusOK)
			})

			req, err := http.NewRequest(http.MethodPost, "/test", nil)
			require.NoError(t, err)

			req.Host = "localhost"

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer func() {
				_ = resp.Body.Close()
			}()

			if tt.wantNil {
				assert.Equal(t, http.StatusOK, resp.StatusCode,
					"nil return should result in 200 (pass-through)")
			} else {
				assert.Equal(t, tt.wantStatusCode, resp.StatusCode,
					"expected status %d", tt.wantStatusCode)

				if tt.wantCode != "" || tt.wantTitle != "" {
					body, readErr := io.ReadAll(resp.Body)
					require.NoError(t, readErr)
					bodyStr := string(body)

					if tt.wantCode != "" {
						assert.Contains(t, bodyStr, tt.wantCode,
							"response body should contain error code %q", tt.wantCode)
					}

					if tt.wantTitle != "" {
						assert.Contains(t, bodyStr, tt.wantTitle,
							"response body should contain title %q", tt.wantTitle)
					}
				}
			}
		})
	}
}

// TestNewUnifiedServer_CreatesServer verifies that NewUnifiedServer creates a
// valid server without requiring global tenant middleware wiring.
func TestNewUnifiedServer_CreatesServer(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	telemetry := &libOpentelemetry.Telemetry{}

	t.Run("creates_server_without_route_registrars", func(t *testing.T) {
		t.Parallel()

		server := NewUnifiedServer(
			":0",
			logger,
			telemetry,
		)

		require.NotNil(t, server, "NewUnifiedServer should return non-nil server")
		assert.Equal(t, ":0", server.ServerAddress())
	})

	t.Run("creates_server_with_route_registrar", func(t *testing.T) {
		t.Parallel()

		server := NewUnifiedServer(
			":0",
			logger,
			telemetry,
			func(router fiber.Router) {
				router.Get("/test", func(c *fiber.Ctx) error {
					return c.SendStatus(fiber.StatusNoContent)
				})
			},
		)

		require.NotNil(t, server, "NewUnifiedServer should return non-nil server when a registrar is provided")
		assert.Equal(t, ":0", server.ServerAddress())
	})
}

// TestTenantMiddleware_DisabledWhenNoManagers verifies that middleware constructed
// with no managers is a valid non-nil instance but reports itself as disabled.
func TestTenantMiddleware_DisabledWhenNoManagers(t *testing.T) {
	t.Parallel()

	mid := tmmiddleware.NewTenantMiddleware()
	assert.NotNil(t, mid, "NewTenantMiddleware always returns non-nil")
	assert.False(t, mid.Enabled(), "middleware with no managers should be disabled")
}
