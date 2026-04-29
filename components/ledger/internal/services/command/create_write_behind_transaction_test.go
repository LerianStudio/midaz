// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"
)

func TestCreateWriteBehindTransaction_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	tran := &transaction.Transaction{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AssetCode:      "BRL",
	}

	transactionInput := mtransaction.Transaction{
		Description: "Test transaction",
		Send: mtransaction.Send{
			Asset: "BRL",
		},
	}

	expectedKey := utils.WriteBehindTransactionKey(organizationID, ledgerID, tran.ID)

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	uc.CreateWriteBehindTransaction(context.Background(), organizationID, ledgerID, tran, transactionInput)
}

func TestCreateWriteBehindTransaction_SetBytesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	tran := &transaction.Transaction{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
	}

	transactionInput := mtransaction.Transaction{
		Description: "Test transaction",
	}

	expectedKey := utils.WriteBehindTransactionKey(organizationID, ledgerID, tran.ID)

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, gomock.Any(), gomock.Any()).
		Return(errors.New("redis connection error")).
		Times(1)

	// Should not panic on error
	uc.CreateWriteBehindTransaction(context.Background(), organizationID, ledgerID, tran, transactionInput)
}

func TestCreateWriteBehindTransaction_MsgpackIncludesBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	tran := &transaction.Transaction{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AssetCode:      "BRL",
	}

	transactionInput := mtransaction.Transaction{
		Description: "Transaction body content",
		Send: mtransaction.Send{
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

	uc.CreateWriteBehindTransaction(context.Background(), organizationID, ledgerID, tran, transactionInput)

	// Verify that msgpack data includes Body
	var decoded transaction.Transaction
	err := msgpack.Unmarshal(capturedData, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, "Transaction body content", decoded.Body.Description)
	assert.Equal(t, "BRL", decoded.Body.Send.Asset)
	assert.Equal(t, tran.ID, decoded.ID)
}
