package query

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	pkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetBalanceByID(t *testing.T) {
	ID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	balanceRes := &mmodel.Balance{
		ID:             ID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
	}

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(balanceRes, nil).
		Times(1)
	res, err := uc.BalanceRepo.Find(context.TODO(), organizationID, ledgerID, ID)

	assert.Equal(t, balanceRes, res)
	assert.Nil(t, err)
}

func TestGetBalanceIDError(t *testing.T) {
	errMSG := "err to get balance on database"
	ID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, ID).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.BalanceRepo.Find(context.TODO(), organizationID, ledgerID, ID)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}

func TestGetBalanceByIDUseCase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	id := libCommons.GenerateUUIDv7()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()
	now := time.Now()

	balanceData := &mmodel.Balance{
		ID:             id.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          "@user1",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromFloat(1000),
		OnHold:         decimal.NewFromFloat(200),
		Version:        1,
		AccountType:    "checking",
		AllowSending:   true,
		AllowReceiving: true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Create mocks
	balanceRepo := balance.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)

	// Create use case with mocks
	uc := UseCase{
		BalanceRepo: balanceRepo,
		RedisRepo:   redisRepo,
	}

	// Test cases
	tests := []struct {
		name           string
		setupMocks     func()
		expectedResult *mmodel.Balance
		expectedError  error
	}{
		{
			name: "success",
			setupMocks: func() {
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, id).
					Return(balanceData, nil)
				// No cache present
				internalKey := libCommons.BalanceInternalKey(orgID.String(), ledgerID.String(), balanceData.Alias+"#"+balanceData.Key)
				redisRepo.EXPECT().Get(gomock.Any(), internalKey).Return("", nil)
			},
			expectedResult: balanceData,
			expectedError:  nil,
		},
		{
			name: "error_finding_balance",
			setupMocks: func() {
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, id).
					Return(nil, errors.New("database error"))
				// Redis must NOT be called when DB fails
			},
			expectedResult: nil,
			expectedError:  errors.New("database error"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mocks for this test case
			tc.setupMocks()

			// Call the method being tested
			result, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

			// Assert results
			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestGetBalanceByID_RedisOverlayApplied(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	id := libCommons.GenerateUUIDv7()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	balFromDB := &mmodel.Balance{
		ID:             id.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      libCommons.GenerateUUIDv7().String(),
		Alias:          "@alice",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.RequireFromString("0"),
		OnHold:         decimal.RequireFromString("0"),
		Version:        1,
	}

	balanceRepo := balance.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)

	uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

	balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(balFromDB, nil)

	cached := mmodel.BalanceRedis{
		ID:             id.String(),
		Alias:          balFromDB.Alias,
		AccountID:      balFromDB.AccountID,
		AssetCode:      "USD",
		Available:      decimal.RequireFromString("123.45"),
		OnHold:         decimal.RequireFromString("6.78"),
		Version:        9,
		AccountType:    "checking",
		AllowSending:   1,
		AllowReceiving: 1,
	}
	b, _ := json.Marshal(cached)
	internalKey := libCommons.BalanceInternalKey(orgID.String(), ledgerID.String(), balFromDB.Alias+"#"+balFromDB.Key)
	redisRepo.EXPECT().Get(gomock.Any(), internalKey).Return(string(b), nil)

	out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)
	assert.NoError(t, err)
	assert.Equal(t, decimal.RequireFromString("123.45"), out.Available)
	assert.Equal(t, decimal.RequireFromString("6.78"), out.OnHold)
	assert.Equal(t, int64(9), out.Version)
}

func TestGetBalanceByID_RedisErrorSkipsOverlay(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	id := libCommons.GenerateUUIDv7()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	balFromDB := &mmodel.Balance{
		ID:             id.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      libCommons.GenerateUUIDv7().String(),
		Alias:          "@bob",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.RequireFromString("10"),
		OnHold:         decimal.RequireFromString("1"),
		Version:        2,
	}

	balanceRepo := balance.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

	balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(balFromDB, nil)
	internalKey := libCommons.BalanceInternalKey(orgID.String(), ledgerID.String(), balFromDB.Alias+"#"+balFromDB.Key)
	redisRepo.EXPECT().Get(gomock.Any(), internalKey).Return("", errors.New("redis down"))

	out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)
	assert.NoError(t, err)
	assert.True(t, out.Available.Equal(decimal.RequireFromString("10")))
	assert.True(t, out.OnHold.Equal(decimal.RequireFromString("1")))
	assert.Equal(t, int64(2), out.Version)
}

func TestGetBalanceByID_InvalidCacheSkipsOverlay(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	id := libCommons.GenerateUUIDv7()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	balFromDB := &mmodel.Balance{
		ID:             id.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      libCommons.GenerateUUIDv7().String(),
		Alias:          "@carol",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.RequireFromString("5"),
		OnHold:         decimal.RequireFromString("0"),
		Version:        1,
	}

	balanceRepo := balance.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

	balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(balFromDB, nil)
	internalKey := libCommons.BalanceInternalKey(orgID.String(), ledgerID.String(), balFromDB.Alias+"#"+balFromDB.Key)
	redisRepo.EXPECT().Get(gomock.Any(), internalKey).Return("{not-json}", nil)

	out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)
	assert.NoError(t, err)
	assert.True(t, out.Available.Equal(decimal.RequireFromString("5")))
	assert.True(t, out.OnHold.Equal(decimal.RequireFromString("0")))
	assert.Equal(t, int64(1), out.Version)
}

func TestGetBalanceByID_NotFoundReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	id := libCommons.GenerateUUIDv7()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	balanceRepo := balance.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

	notFoundErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())
	balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(nil, notFoundErr)

	out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)
	assert.Error(t, err)
	var nf pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &nf))
	assert.Nil(t, out)
}

func TestGetBalanceByID_RepoErrorPreventsRedisCall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	id := libCommons.GenerateUUIDv7()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	balanceRepo := balance.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	uc := UseCase{BalanceRepo: balanceRepo, RedisRepo: redisRepo}

	balanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, id).Return(nil, context.Canceled)

	out, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)
	assert.Error(t, err)
	assert.Nil(t, out)
}
