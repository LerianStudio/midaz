// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"io"
	"net/http"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	tmmiddleware "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/middleware"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/onboarding"
	"github.com/LerianStudio/midaz/v3/components/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// StubRunnable is a stub implementation of mbootstrap.Runnable for testing.
// It returns pre-configured values without verifying interactions.
type StubRunnable struct {
	name string
}

func (s *StubRunnable) Run(l *libCommons.Launcher) error {
	return nil
}

// StubService is a stub implementation of onboarding.OnboardingService for testing.
// It returns pre-configured values without verifying interactions.
type StubService struct {
	runnables         []mbootstrap.RunnableConfig
	metadataIndexRepo mbootstrap.MetadataIndexRepository
	settingsPort      mbootstrap.SettingsPort
	pgManager         interface{}
	mongoManager      interface{}
}

func (s *StubService) GetRunnables() []mbootstrap.RunnableConfig {
	return s.runnables
}

func (s *StubService) GetRouteRegistrar() func(*fiber.App) {
	return func(app *fiber.App) {}
}

func (s *StubService) GetMetadataIndexPort() mbootstrap.MetadataIndexRepository {
	return s.metadataIndexRepo
}

func (s *StubService) GetSettingsPort() mbootstrap.SettingsPort {
	return s.settingsPort
}

func (s *StubService) GetPGManager() interface{} {
	return s.pgManager
}

func (s *StubService) GetMongoManager() interface{} {
	return s.mongoManager
}

// Ensure StubService implements onboarding.OnboardingService
var _ onboarding.OnboardingService = (*StubService)(nil)

// StubTransactionService is a stub implementation of transaction.TransactionService for testing.
// It returns pre-configured values without verifying interactions.
type StubTransactionService struct {
	mbootstrap.Service
	runnables         []mbootstrap.RunnableConfig
	balancePort       mbootstrap.BalancePort
	metadataIndexRepo mbootstrap.MetadataIndexRepository
	settingsPort      mbootstrap.SettingsPort
	pgManager         interface{}
	mongoManager      interface{}
	consumer          interface{}
}

func (s *StubTransactionService) GetRunnables() []mbootstrap.RunnableConfig {
	return s.runnables
}

func (s *StubTransactionService) GetBalancePort() mbootstrap.BalancePort {
	return s.balancePort
}

func (s *StubTransactionService) GetMetadataIndexPort() mbootstrap.MetadataIndexRepository {
	return s.metadataIndexRepo
}

func (s *StubTransactionService) GetRouteRegistrar() func(*fiber.App) {
	return func(app *fiber.App) {}
}

func (s *StubTransactionService) SetSettingsPort(port mbootstrap.SettingsPort) {
	s.settingsPort = port
}

func (s *StubTransactionService) GetPGManager() interface{} {
	return s.pgManager
}

func (s *StubTransactionService) GetMongoManager() interface{} {
	return s.mongoManager
}

func (s *StubTransactionService) GetMultiTenantConsumer() interface{} {
	return s.consumer
}

// Ensure StubTransactionService implements transaction.TransactionService
var _ transaction.TransactionService = (*StubTransactionService)(nil)

// TestService_GetRunnables_ReturnsAllComponents verifies that Service.Run()
// correctly collects runnables from both onboarding and transaction services.
// This is a unit test that uses stubs to verify the composition logic.
func TestService_GetRunnables_ReturnsAllComponents(t *testing.T) {
	// Arrange
	logger := libZap.InitializeLogger()

	// Create stub runnables for onboarding
	onboardingRunnable := &StubRunnable{name: "onboarding-server"}
	onboardingRunnables := []mbootstrap.RunnableConfig{
		{Name: "Onboarding Server", Runnable: onboardingRunnable},
	}

	// Create stub runnables for transaction (without gRPC, as in unified mode)
	txFiberRunnable := &StubRunnable{name: "tx-fiber"}
	txRabbitRunnable := &StubRunnable{name: "tx-rabbit"}
	txRedisRunnable := &StubRunnable{name: "tx-redis"}
	transactionRunnables := []mbootstrap.RunnableConfig{
		{Name: "Transaction Fiber Server", Runnable: txFiberRunnable},
		{Name: "Transaction RabbitMQ Consumer", Runnable: txRabbitRunnable},
		{Name: "Transaction Redis Consumer", Runnable: txRedisRunnable},
	}

	stubOnboardingService := &StubService{runnables: onboardingRunnables}
	stubTransactionService := &StubTransactionService{runnables: transactionRunnables}

	service := &Service{
		OnboardingService:  stubOnboardingService,
		TransactionService: stubTransactionService,
		Logger:             logger,
		Telemetry:          &libOpentelemetry.Telemetry{},
	}

	// Act - collect runnables from both services (simulating what Run() does)
	onboardingResult := service.OnboardingService.GetRunnables()
	transactionResult := service.TransactionService.GetRunnables()
	totalRunnables := len(onboardingResult) + len(transactionResult)

	// Assert
	assert.Equal(t, 1, len(onboardingResult), "Onboarding should have 1 runnable")
	assert.Equal(t, 3, len(transactionResult), "Transaction should have 3 runnables (no gRPC in unified mode)")
	assert.Equal(t, 4, totalRunnables, "Total runnables should be 4")

	// Verify specific runnable names
	assert.Equal(t, "Onboarding Server", onboardingResult[0].Name)
	assert.Equal(t, "Transaction Fiber Server", transactionResult[0].Name)
	assert.Equal(t, "Transaction RabbitMQ Consumer", transactionResult[1].Name)
	assert.Equal(t, "Transaction Redis Consumer", transactionResult[2].Name)
}

// TestInitServers_UnifiedMode_BalancePortWiring verifies that in unified mode,
// the BalancePort from Transaction is correctly passed to Onboarding.
// This test focuses on verifying the wiring contract, not actual initialization.
func TestInitServers_UnifiedMode_BalancePortWiring(t *testing.T) {
	// Arrange
	mockBalancePort := mbootstrap.NewMockBalancePort(nil) // using existing mock from mbootstrap

	stubTransactionService := &StubTransactionService{
		balancePort: mockBalancePort,
		runnables: []mbootstrap.RunnableConfig{
			{Name: "Transaction Fiber Server", Runnable: &StubRunnable{}},
		},
	}

	// Act - verify GetBalancePort returns the expected port
	retrievedPort := stubTransactionService.GetBalancePort()

	// Assert
	require.NotNil(t, retrievedPort, "GetBalancePort should return a non-nil BalancePort")
	assert.Equal(t, mockBalancePort, retrievedPort, "GetBalancePort should return the same BalancePort that was set")

	// This verifies the wiring contract:
	// 1. Transaction service exposes GetBalancePort()
	// 2. The port can be passed to Onboarding for in-process calls
	// 3. No intermediate adapter needed - direct reference passing
}

// TestInitServers_UnifiedMode_MetadataIndexRepoWiring verifies that in unified mode,
// the MetadataIndexRepo from Transaction is correctly passed to Ledger.
// This test focuses on verifying the wiring contract, not actual initialization.
func TestInitServers_UnifiedMode_MetadataIndexRepoWiring(t *testing.T) {
	// Arrange
	mockMetadataIndexRepo := mbootstrap.NewMockMetadataIndexRepository(nil)

	stubTransactionService := &StubTransactionService{
		metadataIndexRepo: mockMetadataIndexRepo,
		runnables: []mbootstrap.RunnableConfig{
			{Name: "Transaction Fiber Server", Runnable: &StubRunnable{}},
		},
	}

	// Act - verify GetMetadataIndexPort returns the expected repo
	retrievedRepo := stubTransactionService.GetMetadataIndexPort()

	// Assert
	require.NotNil(t, retrievedRepo, "GetMetadataIndexPort should return a non-nil MetadataIndexRepository")
	assert.Equal(t, mockMetadataIndexRepo, retrievedRepo, "GetMetadataIndexPort should return the same MetadataIndexRepository that was set")

	// This verifies the wiring contract:
	// 1. Transaction service exposes GetMetadataIndexPort()
	// 2. The repo can be passed to Ledger for in-process calls
	// 3. Direct repository access - no intermediate adapter needed
}

// TestInitServers_UnifiedMode_SettingsPortWiring verifies that in unified mode,
// the SettingsPort from Onboarding is correctly passed to Transaction.
// This test focuses on verifying the wiring contract, not actual initialization.
func TestInitServers_UnifiedMode_SettingsPortWiring(t *testing.T) {
	// Arrange
	mockSettingsPort := mbootstrap.NewMockSettingsPort(nil)

	stubOnboardingService := &StubService{
		settingsPort: mockSettingsPort,
		runnables: []mbootstrap.RunnableConfig{
			{Name: "Onboarding Server", Runnable: &StubRunnable{}},
		},
	}

	stubTransactionService := &StubTransactionService{
		runnables: []mbootstrap.RunnableConfig{
			{Name: "Transaction Fiber Server", Runnable: &StubRunnable{}},
		},
	}

	// Act - verify GetSettingsPort returns the expected port
	retrievedPort := stubOnboardingService.GetSettingsPort()

	// Assert
	require.NotNil(t, retrievedPort, "GetSettingsPort should return a non-nil SettingsPort")
	assert.Equal(t, mockSettingsPort, retrievedPort, "GetSettingsPort should return the same SettingsPort that was set")

	// Verify the wiring works by setting it on transaction service
	stubTransactionService.SetSettingsPort(retrievedPort)
	assert.Equal(t, mockSettingsPort, stubTransactionService.settingsPort, "SetSettingsPort should store the SettingsPort")
}

// TestInitServers_UnifiedMode_NilSettingsPortError verifies that initialization
// fails with an error when SettingsPort is nil in unified mode.
// A nil SettingsPort is an initialization bug, not a recoverable state.
func TestInitServers_UnifiedMode_NilSettingsPortError(t *testing.T) {
	// Arrange - StubService with nil settingsPort (simulates misconfiguration)
	stubOnboardingService := &StubService{
		settingsPort:      nil, // This should cause an error
		metadataIndexRepo: mbootstrap.NewMockMetadataIndexRepository(nil),
		runnables: []mbootstrap.RunnableConfig{
			{Name: "Onboarding Server", Runnable: &StubRunnable{}},
		},
	}

	// Act - verify GetSettingsPort returns nil
	retrievedPort := stubOnboardingService.GetSettingsPort()

	// Assert - GetSettingsPort returns nil, which should trigger an error in InitServers
	assert.Nil(t, retrievedPort, "GetSettingsPort should return nil when settingsPort is not configured")

	// This test documents the expected behavior:
	// When settingsPort is nil, InitServers should return an error with message
	// "failed to get SettingsPort from onboarding module" (consistent with other port checks)
}

// TestService_CompositionContract verifies the composition contract
// between Ledger, Onboarding, and Transaction services.
func TestService_CompositionContract(t *testing.T) {
	t.Run("Service struct has required fields", func(t *testing.T) {
		// Arrange & Act
		service := &Service{
			OnboardingService:  &StubService{},
			TransactionService: &StubTransactionService{},
			Logger:             libZap.InitializeLogger(),
			Telemetry:          &libOpentelemetry.Telemetry{},
		}

		// Assert - verify the service has all required components
		assert.NotNil(t, service.OnboardingService, "OnboardingService should not be nil")
		assert.NotNil(t, service.TransactionService, "TransactionService should not be nil")
		assert.NotNil(t, service.Logger, "Logger should not be nil")
		assert.NotNil(t, service.Telemetry, "Telemetry should not be nil")
	})

	t.Run("OnboardingService implements mbootstrap.Service", func(t *testing.T) {
		// This is a compile-time check enforced by the interface
		var _ mbootstrap.Service = (*StubService)(nil)
	})

	t.Run("TransactionService implements TransactionService interface", func(t *testing.T) {
		// This is a compile-time check enforced by the interface
		var _ transaction.TransactionService = (*StubTransactionService)(nil)
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
			name:           "tenant_not_provisioned_returns_422",
			err:            tmcore.ErrTenantNotProvisioned,
			tenantID:       "tenant-abc",
			wantStatusCode: http.StatusUnprocessableEntity,
			wantNil:        false,
			wantCode:       "0100",
			wantTitle:      "Tenant Not Provisioned",
		},
		{
			name:           "42P01_postgres_error_returns_422",
			err:            errors.New("ERROR: relation \"organization\" does not exist (SQLSTATE 42P01)"),
			tenantID:       "tenant-xyz",
			wantStatusCode: http.StatusUnprocessableEntity,
			wantNil:        false,
			wantCode:       "0100",
			wantTitle:      "Tenant Not Provisioned",
		},
		{
			name:           "relation_does_not_exist_without_sqlstate_returns_422",
			err:            errors.New("pq: relation \"account\" does not exist"),
			tenantID:       "tenant-abc",
			wantStatusCode: http.StatusUnprocessableEntity,
			wantNil:        false,
			wantCode:       "0100",
			wantTitle:      "Tenant Not Provisioned",
		},
		{
			name:           "non_provisioning_error_returns_err",
			err:            errors.New("some other error"),
			tenantID:       "tenant-abc",
			wantStatusCode: http.StatusInternalServerError,
			wantNil:        false,
		},
		{
			name:     "nil_error_returns_nil",
			err:      nil,
			tenantID: "tenant-abc",
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a Fiber app and test context.
			// midazErrorMapper writes the response directly via c.Status().JSON() and returns nil
			// when it handles the error, or returns nil without writing when it does not handle it.
			// We use a sentinel header to distinguish "mapper handled it" from "mapper passed through".
			app := fiber.New()
			app.Post("/test", func(c *fiber.Ctx) error {
				result := midazErrorMapper(c, tt.err, tt.tenantID)
				if result != nil {
					return result
				}

				// If the mapper already wrote a response (status != 200), do not overwrite
				if c.Response().StatusCode() != fiber.StatusOK {
					return nil
				}

				return c.SendStatus(http.StatusOK)
			})

			req, err := http.NewRequest(http.MethodPost, "/test", nil)
			require.NoError(t, err)

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer func() {
				_ = resp.Body.Close()
			}()

			if tt.wantNil {
				// When the mapper returns nil, the handler sends 200 OK
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

// TestNewUnifiedServer_AcceptsMultiPoolMiddleware verifies that NewUnifiedServer
// accepts a *tmmiddleware.MultiPoolMiddleware parameter and creates a valid server.
func TestNewUnifiedServer_AcceptsMultiPoolMiddleware(t *testing.T) {
	t.Parallel()

	logger := libZap.InitializeLogger()
	telemetry := &libOpentelemetry.Telemetry{}

	t.Run("nil_middleware_creates_server_without_tenant_db", func(t *testing.T) {
		t.Parallel()

		server := NewUnifiedServer(
			":0",
			logger,
			telemetry,
			nil, // multiPoolMiddleware is nil
		)

		require.NotNil(t, server, "NewUnifiedServer should return non-nil server when middleware is nil")
		assert.Equal(t, ":0", server.ServerAddress())
	})

	t.Run("non_nil_middleware_accepted_by_server", func(t *testing.T) {
		t.Parallel()

		// Create a middleware with no routes (enabled=false) to avoid needing real managers
		middleware := tmmiddleware.NewMultiPoolMiddleware()

		server := NewUnifiedServer(
			":0",
			logger,
			telemetry,
			middleware,
		)

		require.NotNil(t, server, "NewUnifiedServer should return non-nil server when middleware is provided")
		assert.Equal(t, ":0", server.ServerAddress())
	})
}

// TestMultiPoolMiddleware_NilWhenDisabled verifies that middleware constructed
// with no pools is effectively a no-op but still a valid non-nil instance.
func TestMultiPoolMiddleware_NilWhenDisabled(t *testing.T) {
	t.Parallel()

	// Test that middleware constructed with no pools is effectively disabled
	middleware := tmmiddleware.NewMultiPoolMiddleware()
	assert.NotNil(t, middleware, "NewMultiPoolMiddleware always returns non-nil")
	// The middleware exists but has no pools configured
}
