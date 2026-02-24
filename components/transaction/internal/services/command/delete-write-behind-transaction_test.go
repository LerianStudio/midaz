// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"go.uber.org/mock/gomock"
)

func TestDeleteWriteBehindTransaction_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7().String()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
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

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7().String()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	expectedKey := utils.WriteBehindTransactionKey(organizationID, ledgerID, transactionID)

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), expectedKey).
		Return(errors.New("redis connection error")).
		Times(1)

	// Should not panic on error
	uc.DeleteWriteBehindTransaction(context.Background(), organizationID, ledgerID, transactionID)
}
