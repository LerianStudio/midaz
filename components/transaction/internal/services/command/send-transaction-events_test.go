package command

import (
	"context"
	"os"
	"testing"

	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/rabbitmq"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/mock/gomock"
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
	organizationID := uuid.New()
	ledgerID := uuid.New()
	action := "created"

	// Create transaction data
	amountValue := decimal.NewFromInt(100)
	transaction := &libTransaction.Transaction{
		ChartOfAccountsGroupName: "ASSETS",
		Description:              "Test transaction",
		Code:                     "TXN001",
		Pending:                  false,
		Metadata:                 map[string]any{"test": "value"},
		Send: libTransaction.Send{
			Asset: "USD",
			Value: amountValue,
			Source: libTransaction.Source{
				From: []libTransaction.FromTo{
					{
						AccountAlias: "test-source-account",
						Amount:       &libTransaction.Amount{Value: amountValue},
						IsFrom:       true,
					},
				},
			},
			Distribute: libTransaction.Distribute{
				To: []libTransaction.FromTo{
					{
						AccountAlias: "test-destination-account",
						Amount:       &libTransaction.Amount{Value: amountValue},
						IsFrom:       false,
					},
				},
			},
		},
	}

	t.Run("success with events enabled", func(t *testing.T) {
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
			ProducerDefault(gomock.Any(), "test-events-exchange", "midaz.transaction.created", gomock.Any()).
			Return(nil, nil).
			Times(1)

		// Call the method
		uc.SendTransactionEvents(ctx, organizationID, ledgerID, action, transaction)

		// No assertions needed as the function doesn't return anything
		// The test passes if the mock expectations are met
	})

	t.Run("events disabled", func(t *testing.T) {
		// Disable transaction events
		os.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "false")

		// No expectations for RabbitMQRepo as it shouldn't be called

		// Call the method
		uc.SendTransactionEvents(ctx, organizationID, ledgerID, action, transaction)

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
			ProducerDefault(gomock.Any(), "test-events-exchange", "midaz.transaction.created", gomock.Any()).
			Return(nil, nil).
			Times(1)

		// Call the method
		uc.SendTransactionEvents(ctx, organizationID, ledgerID, action, transaction)

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

		updateAction := "updated"

		// Mock RabbitMQRepo.ProducerDefault with different action
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-events-exchange", "midaz.transaction.updated", gomock.Any()).
			Return(nil, nil).
			Times(1)

		// Call the method with different action
		uc.SendTransactionEvents(ctx, organizationID, ledgerID, updateAction, transaction)

		// No assertions needed as the function doesn't return anything
		// The test passes if the mock expectations are met
	})
}