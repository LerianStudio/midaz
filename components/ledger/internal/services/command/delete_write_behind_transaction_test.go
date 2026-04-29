// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestDeleteWriteBehindTransaction_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	expectedKey := utils.WriteBehindTransactionKey(organizationID, ledgerID, transactionID)

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), expectedKey).
		Return(nil).
		Times(1)

	uc.DeleteWriteBehindTransaction(context.Background(), organizationID, ledgerID, transactionID)
}

func TestDeleteWriteBehindTransaction_DelError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	expectedKey := utils.WriteBehindTransactionKey(organizationID, ledgerID, transactionID)

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), expectedKey).
		Return(errors.New("redis connection error")).
		Times(1)

	// Should not panic on error
	uc.DeleteWriteBehindTransaction(context.Background(), organizationID, ledgerID, transactionID)
}
