package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"
)

func TestCreateWriteBehindTransaction_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	tran := &transaction.Transaction{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AssetCode:      "BRL",
	}

	parserDSL := libTransaction.Transaction{
		Description: "Test transaction",
		Send: libTransaction.Send{
			Asset: "BRL",
		},
	}

	expectedKey := utils.WriteBehindTransactionKey(organizationID, ledgerID, tran.ID)

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	uc.CreateWriteBehindTransaction(context.Background(), organizationID, ledgerID, tran, parserDSL)
}

func TestCreateWriteBehindTransaction_SetBytesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	tran := &transaction.Transaction{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
	}

	parserDSL := libTransaction.Transaction{
		Description: "Test transaction",
	}

	expectedKey := utils.WriteBehindTransactionKey(organizationID, ledgerID, tran.ID)

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, gomock.Any(), gomock.Any()).
		Return(errors.New("redis connection error")).
		Times(1)

	// Should not panic on error
	uc.CreateWriteBehindTransaction(context.Background(), organizationID, ledgerID, tran, parserDSL)
}

func TestCreateWriteBehindTransaction_MsgpackIncludesBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	tran := &transaction.Transaction{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AssetCode:      "BRL",
	}

	parserDSL := libTransaction.Transaction{
		Description: "DSL body content",
		Send: libTransaction.Send{
			Asset: "BRL",
		},
	}

	var capturedData []byte

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, data []byte, _ any) error {
			capturedData = data
			return nil
		}).
		Times(1)

	uc.CreateWriteBehindTransaction(context.Background(), organizationID, ledgerID, tran, parserDSL)

	// Verify that msgpack data includes Body
	var decoded transaction.Transaction
	err := msgpack.Unmarshal(capturedData, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, "DSL body content", decoded.Body.Description)
	assert.Equal(t, "BRL", decoded.Body.Send.Asset)
	assert.Equal(t, tran.ID, decoded.ID)
}
