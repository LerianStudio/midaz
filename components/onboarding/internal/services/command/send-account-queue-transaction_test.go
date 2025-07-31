package command

import (
	"context"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	// "github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"os"
	"testing"
	"time"
)

func TestSendAccountQueueTransaction(t *testing.T) {
	// Save original env vars to restore after test
	originalExchange := os.Getenv("RABBITMQ_EXCHANGE")
	originalKey := os.Getenv("RABBITMQ_KEY")

	// Set test env vars
	os.Setenv("RABBITMQ_EXCHANGE", "test-exchange")
	os.Setenv("RABBITMQ_KEY", "test-key")

	// Restore env vars after test
	defer func() {
		os.Setenv("RABBITMQ_EXCHANGE", originalExchange)
		os.Setenv("RABBITMQ_KEY", originalKey)
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
	accountID := uuid.New()

	account := mmodel.Account{
		ID:        accountID.String(),
		Name:      "Test Account",
		Type:      "deposit",
		AssetCode: "USD",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	t.Run("success", func(t *testing.T) {
		// Setup expectations
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(
				gomock.Any(),
				"test-exchange",
				"test-key",
				gomock.Any(), // Using Any() for the queue message as it contains marshaled data
			).
			Return(nil, nil).
			Times(1)

		// Call the function
		uc.SendAccountQueueTransaction(ctx, organizationID, ledgerID, account)

		// No assertions needed as the function doesn't return anything
		// The test passes if the mock expectations are met
	})
}
