// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	midazpkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// feeDebtReader is the TransactionReader boundary of the delete guards: it answers
// GetFeeDebtSeeds from debts and records the refs it was asked for. onRead, when set,
// runs at the read so a test can assert what already happened by then.
type feeDebtReader struct {
	TransactionReader
	debts  map[string][]accounting.FeeDebtItem
	err    error
	refs   []string
	onRead func()
}

func (r *feeDebtReader) GetFeeDebtSeeds(_ context.Context, _, _ uuid.UUID, refs []string) (map[string][]accounting.FeeDebtItem, error) {
	r.refs = refs

	if r.onRead != nil {
		r.onRead()
	}

	return r.debts, r.err
}

func openFeeDebt(ref string) map[string][]accounting.FeeDebtItem {
	return map[string][]accounting.FeeDebtItem{ref: {{ID: uuid.NewString() + ":0", CreditRef: "@fees#default"}}}
}

func TestDeleteBalance_FeeDebtGuard(t *testing.T) {
	ctx := context.Background()
	organizationID, ledgerID, balanceID := uuid.New(), uuid.New(), uuid.New()

	zeroBalance := func() *mmodel.Balance {
		return &mmodel.Balance{ID: balanceID.String(), Alias: "@payer", Key: "default"}
	}

	t.Run("open debt refuses the delete after the marker and releases it", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		bal := zeroBalance()
		planted := false
		reader := &feeDebtReader{
			debts:  openFeeDebt("@payer#default"),
			onRead: func() { assert.True(t, planted, "fee debt read before the delete marker was planted") },
		}
		uc.TransactionReader = reader

		mockBalanceRepo.EXPECT().Find(gomock.Any(), organizationID, ledgerID, balanceID).Return(bal, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal).
			Do(func(context.Context, string, string, any) { planted = true })
		mockRedisRepo.EXPECT().Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).Return("", nil)
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, bal)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		var unprocessable midazpkg.UnprocessableOperationError
		require.ErrorAs(t, err, &unprocessable)
		assert.Equal(t, constant.ErrBalanceHasOpenFeeDebt.Error(), unprocessable.Code)
		assert.Equal(t, []string{"@payer#default"}, reader.refs)
	})

	t.Run("an empty debt list deletes as before", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		bal := zeroBalance()
		uc.TransactionReader = &feeDebtReader{debts: map[string][]accounting.FeeDebtItem{"@payer#default": {}}}

		mockBalanceRepo.EXPECT().Find(gomock.Any(), organizationID, ledgerID, balanceID).Return(bal, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
		mockRedisRepo.EXPECT().Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).Return("", nil)
		mockBalanceRepo.EXPECT().Delete(gomock.Any(), organizationID, ledgerID, balanceID).Return(nil)
		expectCacheEvict(mockRedisRepo, organizationID, ledgerID, bal)

		require.NoError(t, uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID))
	})

	t.Run("an unreadable debt list fails closed and releases the marker", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		bal := zeroBalance()
		readErr := errors.New("redis down")
		uc.TransactionReader = &feeDebtReader{err: readErr}

		mockBalanceRepo.EXPECT().Find(gomock.Any(), organizationID, ledgerID, balanceID).Return(bal, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
		mockRedisRepo.EXPECT().Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).Return("", nil)
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, bal)

		require.ErrorIs(t, uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID), readErr)
	})
}

func TestDeleteAccountByID_RefusesABalanceWithOpenFeeDebt(t *testing.T) {
	m := newProtectionMocks(t)
	m.expectOwnershipRoundTrip()

	newBalance := func(key string) *mmodel.Balance {
		return &mmodel.Balance{
			ID: uuid.NewString(), Alias: "@payer", Key: key, AccountID: protectionAccountID.String(),
			Available: decimal.Zero, OnHold: decimal.Zero, OverdraftUsed: decimal.Zero,
		}
	}
	clean, indebted := newBalance(constant.DefaultBalanceKey), newBalance("savings")
	reader := &feeDebtReader{debts: openFeeDebt("@payer#savings")}
	m.uc.TransactionReader = reader

	m.account.EXPECT().Find(gomock.Any(), protectionOrgID, protectionLedgerID, nil, protectionAccountID, mmodel.HolderOffV1).
		Return(&mmodel.Account{ID: protectionAccountID.String(), Type: "deposit"}, nil)
	m.balance.EXPECT().ListByAccountID(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return([]*mmodel.Balance{clean, indebted}, nil)
	expectDeleteMarkerPlant(m.redis, protectionOrgID, protectionLedgerID, clean)
	expectDeleteMarkerPlant(m.redis, protectionOrgID, protectionLedgerID, indebted)
	m.redis.EXPECT().ListBalanceByKey(gomock.Any(), protectionOrgID, protectionLedgerID, gomock.Any()).Return(nil, nil).Times(2)
	expectDeleteMarkerRelease(m.redis, protectionOrgID, protectionLedgerID, clean)
	expectDeleteMarkerRelease(m.redis, protectionOrgID, protectionLedgerID, indebted)

	err := m.uc.DeleteAccountByID(context.Background(), protectionOrgID, protectionLedgerID, nil, protectionAccountID, "token")

	var unprocessable midazpkg.UnprocessableOperationError
	require.ErrorAs(t, err, &unprocessable)
	assert.Equal(t, constant.ErrBalanceHasOpenFeeDebt.Error(), unprocessable.Code)
	assert.Equal(t, []string{"@payer#default", "@payer#savings"}, reader.refs)
}
