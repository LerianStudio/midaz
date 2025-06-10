package command

import (
	"context"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/rabbitmq"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/mock/gomock"
	"os"
	"testing"
)

func TestSendLogTransactionAuditQueue(t *testing.T) {
	// Save original env vars to restore after test
	originalExchange := os.Getenv("RABBITMQ_TRANSACTION_AUDIT_EXCHANGE")
	originalKey := os.Getenv("RABBITMQ_TRANSACTION_AUDIT_KEY")
	originalAuditEnabled := os.Getenv("AUDIT_LOG_ENABLED")

	// Set test env vars
	os.Setenv("RABBITMQ_TRANSACTION_AUDIT_EXCHANGE", "test-exchange")
	os.Setenv("RABBITMQ_TRANSACTION_AUDIT_KEY", "test-key")
	os.Setenv("AUDIT_LOG_ENABLED", "true")

	// Restore env vars after test
	defer func() {
		os.Setenv("RABBITMQ_TRANSACTION_AUDIT_EXCHANGE", originalExchange)
		os.Setenv("RABBITMQ_TRANSACTION_AUDIT_KEY", originalKey)
		os.Setenv("AUDIT_LOG_ENABLED", originalAuditEnabled)
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
	transactionID := uuid.New()

	// Create int64 values for Amount and Scale
	var amountValue = decimal.NewFromInt(50)

	// Create operations
	operations := []*operation.Operation{
		{
			ID:             uuid.New().String(),
			TransactionID:  transactionID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			AccountAlias:   "alias1",
			Type:           "debit",
			AssetCode:      "USD",
			Amount: operation.Amount{
				Value: &amountValue,
			},
			Metadata: map[string]interface{}{"key": "value"},
		},
		{
			ID:             uuid.New().String(),
			TransactionID:  transactionID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			AccountAlias:   "alias2",
			Type:           "credit",
			AssetCode:      "EUR",
			Amount: operation.Amount{
				Value: &amountValue,
			},
			Metadata: nil,
		},
	}

	t.Run("success with audit enabled", func(t *testing.T) {
		// Set environment variables for the test
		os.Setenv("RABBITMQ_AUDIT_EXCHANGE", "test-exchange")
		os.Setenv("RABBITMQ_AUDIT_KEY", "test-key")
		os.Setenv("AUDIT_LOG_ENABLED", "true")
		// Ensure we clean up after the test
		defer func() {
			os.Unsetenv("RABBITMQ_AUDIT_EXCHANGE")
			os.Unsetenv("RABBITMQ_AUDIT_KEY")
			os.Unsetenv("AUDIT_LOG_ENABLED")
		}()

		// Mock RabbitMQRepo.ProducerDefault with the environment variable values
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
			Return(nil, nil).
			Times(1)

		// Call the method
		uc.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, transactionID)

		// No assertions needed as the function doesn't return anything
		// The test passes if the mock expectations are met
	})

	t.Run("audit disabled", func(t *testing.T) {
		// Disable audit logging
		os.Setenv("AUDIT_LOG_ENABLED", "false")

		// No expectations for RabbitMQRepo as it shouldn't be called

		// Call the method
		uc.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, transactionID)

		// No assertions needed as the function doesn't return anything
		// The test passes if no mock expectations are called
	})

	t.Run("audit enabled by default", func(t *testing.T) {
		// Unset AUDIT_LOG_ENABLED to test default behavior
		os.Unsetenv("AUDIT_LOG_ENABLED")

		// Set exchange and key environment variables
		os.Setenv("RABBITMQ_AUDIT_EXCHANGE", "test-exchange")
		os.Setenv("RABBITMQ_AUDIT_KEY", "test-key")
		// Ensure we clean up after the test
		defer func() {
			os.Unsetenv("RABBITMQ_AUDIT_EXCHANGE")
			os.Unsetenv("RABBITMQ_AUDIT_KEY")
		}()

		// Mock RabbitMQRepo.ProducerDefault
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
			Return(nil, nil).
			Times(1)

		// Call the method
		uc.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, transactionID)

		// No assertions needed as the function doesn't return anything
		// The test passes if the mock expectations are met
	})
}
