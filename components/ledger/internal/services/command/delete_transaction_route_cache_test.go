// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func TestDeleteTransactionRouteCache_DeletesOrganizationAndLedgerKeys(t *testing.T) {
	ctrl := gomock.NewController(t)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	route := &mmodel.TransactionRoute{ID: uuid.Must(libCommons.GenerateUUIDv7()), OrganizationID: organizationID, LedgerID: &ledgerID}

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{TransactionRedisRepo: mockRedisRepo}

	mockRedisRepo.EXPECT().Del(gomock.Any(), utils.AccountingRoutesInternalKey(organizationID, route.ID)).Return(nil).Times(1)
	mockRedisRepo.EXPECT().Del(gomock.Any(), utils.LedgerAccountingRoutesInternalKey(organizationID, ledgerID, route.ID)).Return(nil).Times(1)

	assert.NoError(t, uc.DeleteTransactionRouteCache(context.Background(), route))
}

func TestDeleteTransactionRouteCache_RouteWithoutLedgerDeletesOnlyOrganizationKey(t *testing.T) {
	ctrl := gomock.NewController(t)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	route := &mmodel.TransactionRoute{ID: uuid.Must(libCommons.GenerateUUIDv7()), OrganizationID: organizationID}

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{TransactionRedisRepo: mockRedisRepo}

	mockRedisRepo.EXPECT().Del(gomock.Any(), utils.AccountingRoutesInternalKey(organizationID, route.ID)).Return(nil).Times(1)

	assert.NoError(t, uc.DeleteTransactionRouteCache(context.Background(), route))
}

// A failed organization-key delete must not skip the ledger-key delete: that key
// is the one pods resolving routes by ledger keep serving forever.
func TestDeleteTransactionRouteCache_OrganizationKeyFailureStillDeletesLedgerKey(t *testing.T) {
	ctrl := gomock.NewController(t)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	route := &mmodel.TransactionRoute{ID: uuid.Must(libCommons.GenerateUUIDv7()), OrganizationID: organizationID, LedgerID: &ledgerID}

	redisError := errors.New("redis connection error")
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{TransactionRedisRepo: mockRedisRepo}

	mockRedisRepo.EXPECT().Del(gomock.Any(), utils.AccountingRoutesInternalKey(organizationID, route.ID)).Return(redisError).Times(1)
	mockRedisRepo.EXPECT().Del(gomock.Any(), utils.LedgerAccountingRoutesInternalKey(organizationID, ledgerID, route.ID)).Return(nil).Times(1)

	err := uc.DeleteTransactionRouteCache(context.Background(), route)

	assert.ErrorIs(t, err, redisError)
}

func TestDeleteTransactionRouteCache_LedgerKeyFailureIsReturned(t *testing.T) {
	ctrl := gomock.NewController(t)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	route := &mmodel.TransactionRoute{ID: uuid.Must(libCommons.GenerateUUIDv7()), OrganizationID: organizationID, LedgerID: &ledgerID}

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{TransactionRedisRepo: mockRedisRepo}

	mockRedisRepo.EXPECT().Del(gomock.Any(), utils.AccountingRoutesInternalKey(organizationID, route.ID)).Return(nil).Times(1)
	mockRedisRepo.EXPECT().Del(gomock.Any(), utils.LedgerAccountingRoutesInternalKey(organizationID, ledgerID, route.ID)).Return(context.Canceled).Times(1)

	err := uc.DeleteTransactionRouteCache(context.Background(), route)

	assert.ErrorIs(t, err, context.Canceled)
}
