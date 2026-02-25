// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"
)

func TestGetWriteBehindTransaction_Hit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	original := &transaction.Transaction{
		ID:             transactionID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AssetCode:      "BRL",
		Body: pkgTransaction.Transaction{
			Description: "DSL body content",
			Send: pkgTransaction.Send{
				Asset: "BRL",
			},
		},
	}

	data, err := msgpack.Marshal(original)
	assert.NoError(t, err)

	expectedKey := utils.WriteBehindTransactionKey(organizationID, ledgerID, transactionID.String())

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return(data, nil).
		Times(1)

	tran, err := uc.GetWriteBehindTransaction(context.Background(), organizationID, ledgerID, transactionID)

	assert.NoError(t, err)
	assert.NotNil(t, tran)
	assert.Equal(t, transactionID.String(), tran.ID)
	assert.Equal(t, "BRL", tran.AssetCode)
	assert.Equal(t, "DSL body content", tran.Body.Description)
	assert.Equal(t, "BRL", tran.Body.Send.Asset)
}

func TestGetWriteBehindTransaction_Miss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	expectedKey := utils.WriteBehindTransactionKey(organizationID, ledgerID, transactionID.String())

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return(nil, errors.New("redis: nil")).
		Times(1)

	tran, err := uc.GetWriteBehindTransaction(context.Background(), organizationID, ledgerID, transactionID)

	assert.Error(t, err)
	assert.Nil(t, tran)
}

func TestGetWriteBehindTransaction_CorruptedData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	expectedKey := utils.WriteBehindTransactionKey(organizationID, ledgerID, transactionID.String())

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return([]byte("invalid-msgpack-data"), nil).
		Times(1)

	tran, err := uc.GetWriteBehindTransaction(context.Background(), organizationID, ledgerID, transactionID)

	assert.Error(t, err)
	assert.Nil(t, tran)
}
