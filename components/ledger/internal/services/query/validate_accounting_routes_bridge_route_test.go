// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// A transaction route that also links the cross-ledger bridge route validates a
// single-ledger transaction exactly as it would without it: the bridge route is
// never part of an action's template.
func TestValidateAccountingRules_BridgeRouteDoesNotChangeSingleLedgerValidation(t *testing.T) {
	t.Parallel()

	sourceID := uuid.New()
	destinationID := uuid.New()
	rubric := &mmodel.AccountingRubric{Code: "1000", Description: "Rubric"}
	client := []mmodel.OperationRoute{
		{ID: sourceID, OperationType: constant.OperationRouteTypeSource, AccountingEntries: &mmodel.AccountingEntries{
			Direct: &mmodel.AccountingEntry{Debit: rubric},
			Hold:   &mmodel.AccountingEntry{Debit: rubric, Credit: rubric},
			Commit: &mmodel.AccountingEntry{Debit: rubric},
			Cancel: &mmodel.AccountingEntry{Debit: rubric, Credit: rubric},
		}},
		{ID: destinationID, OperationType: constant.OperationRouteTypeDestination, AccountingEntries: &mmodel.AccountingEntries{
			Direct: &mmodel.AccountingEntry{Credit: rubric},
			Commit: &mmodel.AccountingEntry{Credit: rubric},
		}},
	}
	bridge := mmodel.OperationRoute{ID: uuid.New(), OperationType: constant.OperationRouteTypeBidirectional, AccountingEntries: &mmodel.AccountingEntries{
		CrossLedger: &mmodel.AccountingEntry{Debit: rubric, Credit: rubric},
	}}

	validate := func(t *testing.T, operationRoutes []mmodel.OperationRoute, action, sourceOperation string) error {
		t.Helper()

		ctrl := gomock.NewController(t)
		ledgers := ledger.NewMockRepository(ctrl)
		ledgers.EXPECT().GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(map[string]any{"accounting": map[string]any{"validateRoutes": true}}, nil)

		transactionRouteID := uuid.New()
		cache, err := (&mmodel.TransactionRoute{ID: transactionRouteID, OperationRoutes: operationRoutes}).ToCache().ToMsgpack()
		require.NoError(t, err)

		cacheRepo := redis.NewMockRedisRepository(ctrl)
		cacheRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(cache, nil)

		routeID := transactionRouteID.String()
		source := mtransaction.Amount{Operation: sourceOperation, Direction: constant.DirectionDebit}
		destination := mtransaction.Amount{Operation: constant.CREDIT, Direction: constant.DirectionCredit}

		_, err = (&UseCase{TransactionRedisRepo: cacheRepo, LedgerRepo: ledgers}).ValidateAccountingRules(context.Background(), uuid.New(), uuid.New(),
			[]mmodel.BalanceOperation{
				{Alias: "0#@sender#default", Balance: &mmodel.Balance{AccountType: "deposit"}, Amount: source},
				{Alias: "0#@receiver#default", Balance: &mmodel.Balance{AccountType: "deposit"}, Amount: destination},
			},
			&mtransaction.Responses{
				TransactionRouteID:  &routeID,
				From:                map[string]mtransaction.Amount{"0#@sender#default": source},
				To:                  map[string]mtransaction.Amount{"0#@receiver#default": destination},
				OperationRoutesFrom: map[string]string{"0#@sender#default": sourceID.String()},
				OperationRoutesTo:   map[string]string{"0#@receiver#default": destinationID.String()},
			},
			action)

		return err
	}

	for _, tc := range []struct{ action, sourceOperation string }{
		{constant.ActionDirect, constant.DEBIT},
		{constant.ActionHold, constant.ONHOLD},
		{constant.ActionCommit, constant.DEBIT},
	} {
		t.Run(tc.action, func(t *testing.T) {
			t.Parallel()

			require.NoError(t, validate(t, client, tc.action, tc.sourceOperation), "without the bridge route")
			require.NoError(t, validate(t, append(append([]mmodel.OperationRoute(nil), client...), bridge), tc.action, tc.sourceOperation), "with the bridge route")
		})
	}
}
