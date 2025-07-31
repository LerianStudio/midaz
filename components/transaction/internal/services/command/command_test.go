package command

import (
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestNewUseCase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock repositories
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Create the UseCase instance
	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		OperationRepo:   mockOperationRepo,
		AssetRateRepo:   mockAssetRateRepo,
		BalanceRepo:     mockBalanceRepo,
		MetadataRepo:    mockMetadataRepo,
		RabbitMQRepo:    mockRabbitMQRepo,
		RedisRepo:       mockRedisRepo,
	}

	// Verify that the UseCase was created correctly with all repositories
	assert.NotNil(t, uc)
	assert.Equal(t, mockTransactionRepo, uc.TransactionRepo)
	assert.Equal(t, mockOperationRepo, uc.OperationRepo)
	assert.Equal(t, mockAssetRateRepo, uc.AssetRateRepo)
	assert.Equal(t, mockBalanceRepo, uc.BalanceRepo)
	assert.Equal(t, mockMetadataRepo, uc.MetadataRepo)
	assert.Equal(t, mockRabbitMQRepo, uc.RabbitMQRepo)
	assert.Equal(t, mockRedisRepo, uc.RedisRepo)
}
