package command

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateAccountingRouteCache_Success tests successful cache creation with operation routes
func TestCreateAccountingRouteCache_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()
	operationRouteID := libCommons.GenerateUUIDv7()

	route := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		Description:    "Test transaction route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:             operationRouteID,
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Type:           "debit",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@cash_account",
				},
			},
		},
	}

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	expectedCacheData := `{"` + operationRouteID.String() + `":{"account":{"ruleType":"alias","validIf":"@cash_account"},"type":"debit"}}`

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), expectedCacheData, time.Duration(0)).
		Return(nil).
		Times(1)

	err := uc.CreateAccountingRouteCache(context.Background(), route)

	assert.NoError(t, err)
}

// TestCreateAccountingRouteCache_SuccessWithoutAccountRule tests successful cache creation without account rules
func TestCreateAccountingRouteCache_SuccessWithoutAccountRule(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()
	operationRouteID := libCommons.GenerateUUIDv7()

	route := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		Description:    "Test transaction route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:             operationRouteID,
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Type:           "credit",
				Account:        nil,
			},
		},
	}

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	expectedCacheData := `{"` + operationRouteID.String() + `":{"type":"credit"}}`

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), expectedCacheData, time.Duration(0)).
		Return(nil).
		Times(1)

	err := uc.CreateAccountingRouteCache(context.Background(), route)

	assert.NoError(t, err)
}

// TestCreateAccountingRouteCache_SuccessWithEmptyOperationRoutes tests successful cache creation with empty operation routes
func TestCreateAccountingRouteCache_SuccessWithEmptyOperationRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()

	route := &mmodel.TransactionRoute{
		ID:              routeID,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Title:           "Test Route",
		Description:     "Test transaction route",
		OperationRoutes: []mmodel.OperationRoute{},
	}

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	expectedCacheData := `{}`

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), expectedCacheData, time.Duration(0)).
		Return(nil).
		Times(1)

	err := uc.CreateAccountingRouteCache(context.Background(), route)

	assert.NoError(t, err)
}

// TestCreateAccountingRouteCache_SuccessWithMultipleOperationRoutes tests successful cache creation with multiple operation routes
func TestCreateAccountingRouteCache_SuccessWithMultipleOperationRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()
	operationRouteID1 := libCommons.GenerateUUIDv7()
	operationRouteID2 := libCommons.GenerateUUIDv7()

	route := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		Description:    "Test transaction route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:             operationRouteID1,
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Type:           "debit",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@cash_account",
				},
			},
			{
				ID:             operationRouteID2,
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Type:           "credit",
				Account: &mmodel.AccountRule{
					RuleType: "account_type",
					ValidIf:  []string{"liability", "asset"},
				},
			},
		},
	}

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(0)).
		Return(nil).
		Times(1)

	err := uc.CreateAccountingRouteCache(context.Background(), route)

	assert.NoError(t, err)
}

// TestCreateAccountingRouteCache_ToCacheDataError tests error handling when ToCacheData fails
func TestCreateAccountingRouteCache_ToCacheDataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()

	// Create a route with an operation route that has an invalid UUID to force ToCacheData error
	route := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		Description:    "Test transaction route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:             uuid.UUID{}, // Invalid UUID
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Type:           "debit",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  make(chan int), // Invalid data type that will cause JSON marshal error
				},
			},
		},
	}

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	err := uc.CreateAccountingRouteCache(context.Background(), route)

	assert.Error(t, err)
}

// TestCreateAccountingRouteCache_RedisSetError tests error handling when Redis Set fails
func TestCreateAccountingRouteCache_RedisSetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()
	operationRouteID := libCommons.GenerateUUIDv7()

	route := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		Description:    "Test transaction route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:             operationRouteID,
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Type:           "debit",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@cash_account",
				},
			},
		},
	}

	redisError := errors.New("redis connection error")
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(0)).
		Return(redisError).
		Times(1)

	err := uc.CreateAccountingRouteCache(context.Background(), route)

	assert.Error(t, err)
	assert.Equal(t, redisError, err)
}

// TestCreateAccountingRouteCache_ContextCancelled tests error handling when context is cancelled
func TestCreateAccountingRouteCache_ContextCancelled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()
	operationRouteID := libCommons.GenerateUUIDv7()

	route := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		Description:    "Test transaction route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:             operationRouteID,
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Type:           "debit",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@cash_account",
				},
			},
		},
	}

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(0)).
		Return(context.Canceled).
		Times(1)

	err := uc.CreateAccountingRouteCache(ctx, route)

	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}
