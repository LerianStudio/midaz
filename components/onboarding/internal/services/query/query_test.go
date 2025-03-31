package query

import (
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/redis"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestNewUseCase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock repositories
	mockOrgRepo := organization.NewMockRepository(ctrl)
	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockSegmentRepo := segment.NewMockRepository(ctrl)
	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Create the UseCase instance
	uc := &UseCase{
		OrganizationRepo: mockOrgRepo,
		LedgerRepo:       mockLedgerRepo,
		SegmentRepo:      mockSegmentRepo,
		PortfolioRepo:    mockPortfolioRepo,
		AccountRepo:      mockAccountRepo,
		AssetRepo:        mockAssetRepo,
		MetadataRepo:     mockMetadataRepo,
		RedisRepo:        mockRedisRepo,
	}

	// Verify that the UseCase was created correctly with all repositories
	assert.NotNil(t, uc)
	assert.Equal(t, mockOrgRepo, uc.OrganizationRepo)
	assert.Equal(t, mockLedgerRepo, uc.LedgerRepo)
	assert.Equal(t, mockSegmentRepo, uc.SegmentRepo)
	assert.Equal(t, mockPortfolioRepo, uc.PortfolioRepo)
	assert.Equal(t, mockAccountRepo, uc.AccountRepo)
	assert.Equal(t, mockAssetRepo, uc.AssetRepo)
	assert.Equal(t, mockMetadataRepo, uc.MetadataRepo)
	assert.Equal(t, mockRedisRepo, uc.RedisRepo)
}
