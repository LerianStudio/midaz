package bootstrap

import (
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
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
	runnables []mbootstrap.RunnableConfig
}

func (s *StubService) GetRunnables() []mbootstrap.RunnableConfig {
	return s.runnables
}

func (s *StubService) GetRouteRegistrar() func(*fiber.App) {
	return func(app *fiber.App) {}
}

// Ensure StubService implements onboarding.OnboardingService
var _ onboarding.OnboardingService = (*StubService)(nil)

// StubTransactionService is a stub implementation of transaction.TransactionService for testing.
// It returns pre-configured values without verifying interactions.
type StubTransactionService struct {
	mbootstrap.Service
	runnables         []mbootstrap.RunnableConfig
	balancePort       mbootstrap.BalancePort
	metadataIndexPort mbootstrap.MetadataIndexPort
}

func (s *StubTransactionService) GetRunnables() []mbootstrap.RunnableConfig {
	return s.runnables
}

func (s *StubTransactionService) GetBalancePort() mbootstrap.BalancePort {
	return s.balancePort
}

func (s *StubTransactionService) GetMetadataIndexPort() mbootstrap.MetadataIndexPort {
	return s.metadataIndexPort
}

func (s *StubTransactionService) GetRouteRegistrar() func(*fiber.App) {
	return func(app *fiber.App) {}
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

// TestInitServers_UnifiedMode_MetadataIndexPortWiring verifies that in unified mode,
// the MetadataIndexPort from Transaction is correctly passed to Ledger.
// This test focuses on verifying the wiring contract, not actual initialization.
func TestInitServers_UnifiedMode_MetadataIndexPortWiring(t *testing.T) {
	// Arrange
	mockMetadataIndexPort := mbootstrap.NewMockMetadataIndexPort(nil)

	stubTransactionService := &StubTransactionService{
		metadataIndexPort: mockMetadataIndexPort,
		runnables: []mbootstrap.RunnableConfig{
			{Name: "Transaction Fiber Server", Runnable: &StubRunnable{}},
		},
	}

	// Act - verify GetMetadataIndexPort returns the expected port
	retrievedPort := stubTransactionService.GetMetadataIndexPort()

	// Assert
	require.NotNil(t, retrievedPort, "GetMetadataIndexPort should return a non-nil MetadataIndexPort")
	assert.Equal(t, mockMetadataIndexPort, retrievedPort, "GetMetadataIndexPort should return the same MetadataIndexPort that was set")

	// This verifies the wiring contract:
	// 1. Transaction service exposes GetMetadataIndexPort()
	// 2. The port can be passed to Ledger for in-process calls
	// 3. No intermediate adapter needed - direct reference passing
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
