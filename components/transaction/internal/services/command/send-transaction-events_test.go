package command

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/shopspring/decimal"
	"go.uber.org/mock/gomock"
	"os"
	"testing"
	"time"
)

func TestSendTransactionEvents(t *testing.T) {
	// Save original env vars to restore after test
	originalExchange := os.Getenv("RABBITMQ_TRANSACTION_EVENTS_EXCHANGE")
	originalEventsEnabled := os.Getenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED")
	originalVersion := os.Getenv("VERSION")

	// Set test env vars
	os.Setenv("RABBITMQ_TRANSACTION_EVENTS_EXCHANGE", "test-events-exchange")
	os.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "true")
	os.Setenv("VERSION", "1.0.0")

	// Restore env vars after test
	defer func() {
		os.Setenv("RABBITMQ_TRANSACTION_EVENTS_EXCHANGE", originalExchange)
		os.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", originalEventsEnabled)
		os.Setenv("VERSION", originalVersion)
	}()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock repositories
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)

	// Create the UseCase instance
	uc := &UseCase{
		RabbitMQRepo: mockRabbitMQRepo,
	}

	// Test data
	ctx := context.Background()

	description := constant.APPROVED
	status := transaction.Status{
		Code:        description,
		Description: &description,
	}

	assetCode := "BRL"

	parentTransactionID := libCommons.GenerateUUIDv7().String()

	amount := decimal.NewFromInt(100)

	chartOfAccountsGroupName := "ChartOfAccountsGroupName"

	tran := &transaction.Transaction{
		ID:                       libCommons.GenerateUUIDv7().String(),
		ParentTransactionID:      &parentTransactionID,
		OrganizationID:           libCommons.GenerateUUIDv7().String(),
		LedgerID:                 libCommons.GenerateUUIDv7().String(),
		Description:              description,
		Status:                   status,
		Amount:                   &amount,
		AssetCode:                assetCode,
		ChartOfAccountsGroupName: chartOfAccountsGroupName,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}

	t.Run("success with events approved", func(t *testing.T) {
		// Set environment variables for the test
		os.Setenv("RABBITMQ_TRANSACTION_EVENTS_EXCHANGE", "test-events-exchange")
		os.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "true")
		os.Setenv("VERSION", "1.0.0")
		// Ensure we clean up after the test
		defer func() {
			os.Unsetenv("RABBITMQ_TRANSACTION_EVENTS_EXCHANGE")
			os.Unsetenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED")
			os.Unsetenv("VERSION")
		}()

		// Mock RabbitMQRepo.ProducerDefault with the environment variable values
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-events-exchange", "midaz.transaction.APPROVED", gomock.Any()).
			Return(nil, nil).
			Times(1)

		// Call the method
		uc.SendTransactionEvents(ctx, tran)

		// No assertions needed as the function doesn't return anything
		// The test passes if the mock expectations are met
	})

	t.Run("events disabled", func(t *testing.T) {
		// Disable transaction events
		os.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "false")

		// No expectations for RabbitMQRepo as it shouldn't be called

		// Call the method
		uc.SendTransactionEvents(ctx, tran)

		// No assertions needed as the function doesn't return anything
		// The test passes if no mock expectations are called
	})

	t.Run("events enabled by default", func(t *testing.T) {
		// Unset RABBITMQ_TRANSACTION_EVENTS_ENABLED to test default behavior
		os.Unsetenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED")

		// Set exchange environment variable
		os.Setenv("RABBITMQ_TRANSACTION_EVENTS_EXCHANGE", "test-events-exchange")
		os.Setenv("VERSION", "1.0.0")
		// Ensure we clean up after the test
		defer func() {
			os.Unsetenv("RABBITMQ_TRANSACTION_EVENTS_EXCHANGE")
			os.Unsetenv("VERSION")
		}()

		// Mock RabbitMQRepo.ProducerDefault
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-events-exchange", "midaz.transaction.APPROVED", gomock.Any()).
			Return(nil, nil).
			Times(1)

		// Call the method
		uc.SendTransactionEvents(ctx, tran)

		// No assertions needed as the function doesn't return anything
		// The test passes if the mock expectations are met
	})

	t.Run("with different action", func(t *testing.T) {
		// Set environment variables for the test
		os.Setenv("RABBITMQ_TRANSACTION_EVENTS_EXCHANGE", "test-events-exchange")
		os.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "true")
		os.Setenv("VERSION", "1.0.0")
		// Ensure we clean up after the test
		defer func() {
			os.Unsetenv("RABBITMQ_TRANSACTION_EVENTS_EXCHANGE")
			os.Unsetenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED")
			os.Unsetenv("VERSION")
		}()

		description = constant.CANCELED
		status = transaction.Status{
			Code:        description,
			Description: &description,
		}

		tran.Status = status

		// Mock RabbitMQRepo.ProducerDefault with different action
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-events-exchange", "midaz.transaction.CANCELED", gomock.Any()).
			Return(nil, nil).
			Times(1)

		// Call the method with different action
		uc.SendTransactionEvents(ctx, tran)

		// No assertions needed as the function doesn't return anything
		// The test passes if the mock expectations are met
	})
}
