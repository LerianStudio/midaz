// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"
)

func TestUpdateWriteBehindTransaction_Success(t *testing.T) {
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
		Status: transaction.Status{
			Code: "CANCELED",
		},
	}

	expectedKey := utils.WriteBehindTransactionKey(organizationID, ledgerID, tran.ID)

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	uc.UpdateWriteBehindTransaction(context.Background(), organizationID, ledgerID, tran)
}

func TestUpdateWriteBehindTransaction_SetBytesError(t *testing.T) {
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

	expectedKey := utils.WriteBehindTransactionKey(organizationID, ledgerID, tran.ID)

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, gomock.Any(), gomock.Any()).
		Return(errors.New("redis connection error")).
		Times(1)

	// Should not panic on error
	uc.UpdateWriteBehindTransaction(context.Background(), organizationID, ledgerID, tran)
}

func TestUpdateWriteBehindTransaction_StatusUpdated(t *testing.T) {
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
		Status: transaction.Status{
			Code: "APPROVED",
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

	uc.UpdateWriteBehindTransaction(context.Background(), organizationID, ledgerID, tran)

	// Verify that updated status is serialized
	var decoded transaction.Transaction
	err := msgpack.Unmarshal(capturedData, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, "APPROVED", decoded.Status.Code)
}
