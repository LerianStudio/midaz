package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestSyncBalance_SuccessSynced verifies that when the repository sync succeeds
// and returns true, the use case returns true and no error.
func TestSyncBalance_SuccessSynced(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceRedis := mmodel.BalanceRedis{ID: libCommons.GenerateUUIDv7().String(), Alias: "@alias"}

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Sync(gomock.Any(), organizationID, ledgerID, balanceRedis).
		Return(true, nil).
		Times(1)

	res, err := uc.SyncBalance(context.TODO(), organizationID, ledgerID, balanceRedis)

	assert.True(t, res)
	assert.Nil(t, err)
}

// TestSyncBalance_SuccessSkipped verifies that when the repository indicates the
// balance is newer (no sync performed), the use case returns false and no error.
func TestSyncBalance_SuccessSkipped(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceRedis := mmodel.BalanceRedis{ID: libCommons.GenerateUUIDv7().String(), Alias: "@alias2"}

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Sync(gomock.Any(), organizationID, ledgerID, balanceRedis).
		Return(false, nil).
		Times(1)

	res, err := uc.SyncBalance(context.TODO(), organizationID, ledgerID, balanceRedis)

	assert.False(t, res)
	assert.Nil(t, err)
}

// TestSyncBalance_Error verifies that when the repository sync returns an error,
// the use case returns false and propagates the error.
func TestSyncBalance_Error(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceRedis := mmodel.BalanceRedis{ID: libCommons.GenerateUUIDv7().String(), Alias: "@alias3"}

	errMSG := "err syncing balance from redis"

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Sync(gomock.Any(), organizationID, ledgerID, balanceRedis).
		Return(false, errors.New(errMSG)).
		Times(1)

	res, err := uc.SyncBalance(context.TODO(), organizationID, ledgerID, balanceRedis)

	assert.False(t, res)
	assert.NotEmpty(t, err)
	assert.Equal(t, errMSG, err.Error())
}
