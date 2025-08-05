package command

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
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
				OperationType:  "source",
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

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(0)).
		Return(nil).
		Times(1)

	err := uc.CreateAccountingRouteCache(context.Background(), route)

	assert.NoError(t, err)
}

// TestCreateAccountingRouteCache_SuccessWithoutAccountRule tests successful cache creation without account rule
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
				OperationType:  "source",
				Account:        nil, // No account rule
			},
		},
	}

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(0)).
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
		OperationRoutes: []mmodel.OperationRoute{}, // Empty operation routes
	}

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(0)).
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
				OperationType:  "source",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@cash_account",
				},
			},
			{
				ID:             operationRouteID2,
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				OperationType:  "destination",
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
		SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(0)).
		Return(nil).
		Times(1)

	err := uc.CreateAccountingRouteCache(context.Background(), route)

	assert.NoError(t, err)
}

// TestCreateAccountingRouteCache_ToMsgpackError tests error handling when ToMsgpack fails
func TestCreateAccountingRouteCache_ToMsgpackError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()

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
				OperationType:  "source",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  make(chan int), // Invalid data type that will cause msgpack encode error
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

// TestCreateAccountingRouteCache_RedisSetError tests error handling when Redis SetBytes fails
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
				OperationType:  "source",
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
		SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(0)).
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
				OperationType:  "source",
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
		SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(0)).
		Return(context.Canceled).
		Times(1)

	err := uc.CreateAccountingRouteCache(ctx, route)

	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}
