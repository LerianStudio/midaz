// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestCreateTransactionBalanceEngineLeavesAnnotationsOnLegacyPath(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			uc, reader, redisRepo := newVersionUseCase(t, mmodel.LedgerSettings{})
			redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			redisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			executor := &scriptedBalanceEngine{}
			uc.BalanceEngine = executor
			input := skippingTransaction()
			input.Skip = nil
			date := time.Date(2026, time.July, 1, 12, 0, 0, 0, time.UTC)
			input.TransactionDate = (*mtransaction.TransactionDate)(&date)
			organizationID := uuid.MustParse("91111111-1111-4111-8111-111111111111")
			ledgerID := uuid.MustParse("92222222-2222-4222-8222-222222222222")
			var err error
			if version == "v1" {
				_, _, err = uc.CreateTransactionV1(context.Background(), CreateTransactionV1Input{
					OrganizationID: organizationID, LedgerID: ledgerID, Transaction: input,
					TransactionStatus: constant.NOTED, IdempotencyTTL: time.Minute,
				})
			} else {
				_, _, err = uc.CreateTransactionV2(context.Background(), CreateTransactionV2Input{
					OrganizationID: organizationID, LedgerID: ledgerID, Transaction: input,
					TransactionStatus: constant.NOTED, IdempotencyTTL: time.Minute,
				})
			}
			require.ErrorIs(t, err, errBalancesUnavailable)
			assert.Equal(t, 1, reader.getBalancesCalls)
			assert.Empty(t, executor.requests)
		})
	}
}

func TestCreateTransactionBalanceEngineRequiresFinalizationBeforePreparation(t *testing.T) {
	uc, reader, redisRepo := newVersionUseCase(t, mmodel.LedgerSettings{})
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	executor := &scriptedBalanceEngine{}
	uc.BalanceEngine = executor

	_, _, err := uc.CreateTransactionV1(context.Background(), CreateTransactionV1Input{
		OrganizationID: uuid.MustParse("91111111-1111-4111-8111-111111111111"),
		LedgerID:       uuid.MustParse("92222222-2222-4222-8222-222222222222"),
		Transaction:    skippingTransaction(), TransactionStatus: constant.CREATED, IdempotencyTTL: time.Minute,
	})
	require.ErrorContains(t, err, "finalizer is not configured")
	assert.Zero(t, reader.getBalancesCalls)
	assert.Empty(t, executor.requests)
}
