package command

import (
	"context"
	"os"
	"testing"

	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/mock/gomock"
)

func TestSendBTOExecuteAsync(t *testing.T) {
	// Save original env vars to restore after test
	originalExchange := os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE")
	originalKey := os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY")

	// Set test env vars
	os.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE", "test-exchange")
	os.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY", "test-key")

	// Restore env vars after test
	defer func() {
		os.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE", originalExchange)
		os.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY", originalKey)
	}()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock repositories
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Create the UseCase instance
	uc := &UseCase{
		RabbitMQRepo: mockRabbitMQRepo,
		RedisRepo:    mockRedisRepo,
	}

	// Test data
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

	// Create test data
	// Using the correct struct for transaction data
	parseDSL := &libTransaction.Transaction{}

	validate := &libTransaction.Responses{
		Aliases: []string{"alias1", "alias2"},
		From: map[string]libTransaction.Amount{
			"alias1": {
				Asset: "USD",
				Value: decimal.NewFromInt(50), // Value should be an int64
			},
		},
		To: map[string]libTransaction.Amount{
			"alias2": {
				Asset: "EUR",
				Value: decimal.NewFromInt(40), // Value should be an int64
			},
		},
	}

	balances := []*mmodel.Balance{
		{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          "alias1",
			Available:      decimal.NewFromInt(100),
			OnHold:         decimal.NewFromInt(0),
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			AssetCode:      "USD",
		},
		{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          "alias2",
			Available:      decimal.NewFromInt(200),
			OnHold:         decimal.NewFromInt(0),
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			AssetCode:      "EUR",
		},
	}

	tran := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
	}

	// Mock RabbitMQRepo.ProducerDefault
	mockRabbitMQRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)

	// Call the method with the correct parameters
	err := uc.SendBTOExecuteAsync(ctx, organizationID, ledgerID, parseDSL, validate, balances, tran)

	// Assert that no error occurred
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}
