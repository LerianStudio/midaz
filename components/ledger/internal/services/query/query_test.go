package query

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewUseCase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Onboarding repositories
	mockOrganizationRepo := organization.NewMockRepository(ctrl)
	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockSegmentRepo := segment.NewMockRepository(ctrl)
	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataOnboardingRepo := mongodb.NewMockRepository(ctrl)

	// Create mock repositories
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockMetadataTransactionRepo := mongodb.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	// Create the UseCase instance
	uc := &UseCase{
		OrganizationRepo:        mockOrganizationRepo,
		LedgerRepo:              mockLedgerRepo,
		SegmentRepo:             mockSegmentRepo,
		PortfolioRepo:           mockPortfolioRepo,
		AccountRepo:             mockAccountRepo,
		AssetRepo:               mockAssetRepo,
		AccountTypeRepo:         mockAccountTypeRepo,
		MetadataOnboardingRepo:  mockMetadataOnboardingRepo,
		TransactionRepo:         mockTransactionRepo,
		OperationRepo:           mockOperationRepo,
		AssetRateRepo:           mockAssetRateRepo,
		BalanceRepo:             mockBalanceRepo,
		MetadataTransactionRepo: mockMetadataTransactionRepo,
		RedisRepo:               mockRedisRepo,
		OperationRouteRepo:      mockOperationRouteRepo,
		TransactionRouteRepo:    mockTransactionRouteRepo,
	}

	// Verify that the UseCase was created correctly with all repositories
	assert.NotNil(t, uc)
	assert.Equal(t, mockOrganizationRepo, uc.OrganizationRepo)
	assert.Equal(t, mockLedgerRepo, uc.LedgerRepo)
	assert.Equal(t, mockSegmentRepo, uc.SegmentRepo)
	assert.Equal(t, mockPortfolioRepo, uc.PortfolioRepo)
	assert.Equal(t, mockAccountRepo, uc.AccountRepo)
	assert.Equal(t, mockAssetRepo, uc.AssetRepo)
	assert.Equal(t, mockAccountTypeRepo, uc.AccountTypeRepo)
	assert.Equal(t, mockMetadataOnboardingRepo, uc.MetadataOnboardingRepo)
	assert.Equal(t, mockTransactionRepo, uc.TransactionRepo)
	assert.Equal(t, mockOperationRepo, uc.OperationRepo)
	assert.Equal(t, mockAssetRateRepo, uc.AssetRateRepo)
	assert.Equal(t, mockBalanceRepo, uc.BalanceRepo)
	assert.Equal(t, mockMetadataTransactionRepo, uc.MetadataTransactionRepo)
	assert.Equal(t, mockRedisRepo, uc.RedisRepo)
	assert.Equal(t, mockOperationRouteRepo, uc.OperationRouteRepo)
	assert.Equal(t, mockTransactionRouteRepo, uc.TransactionRouteRepo)
	assert.Equal(t, mockMetadataOnboardingRepo, uc.MetadataOnboardingRepo)
}
