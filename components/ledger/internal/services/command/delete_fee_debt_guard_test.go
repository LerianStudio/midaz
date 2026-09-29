// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
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

// owedFeeDebts is the FeeDebts boundary of the creditor guard: it answers
// HasOpenCreditor with owed and records the refs it was asked for.
type owedFeeDebts struct {
	FeeDebtRecorder
	owed bool
	err  error
	refs []string
}

func (f *owedFeeDebts) HasOpenCreditor(_ context.Context, _, _ uuid.UUID, refs []string) (bool, error) {
	f.refs = refs

	return f.owed, f.err
}

// allowEmptyEngineRecovery answers every engine recovery walk with one empty terminal page.
func allowEmptyEngineRecovery(mock *redis.MockRedisRepository) {
	mock.EXPECT().ScanRecoveryMessages(gomock.Any(), redis.RecoveryQueueSourceEngineRecover, uint64(0), gomock.Any()).
		Return(redis.RecoveryScanPage{}, nil).AnyTimes()
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

	t.Run("an unreadable debt list answers the retryable 0520 and releases the marker", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		bal := zeroBalance()
		uc.TransactionReader = &feeDebtReader{err: errors.New("redis down")}

		mockBalanceRepo.EXPECT().Find(gomock.Any(), organizationID, ledgerID, balanceID).Return(bal, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
		mockRedisRepo.EXPECT().Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).Return("", nil)
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, bal)

		requireClosingCode(t, uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID), constant.ErrAccountClosingProtectionIndeterminate)
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

// owedRecoveryPage is one terminal engine recovery page whose record, of the recovery
// fixture scope, changes a debt owed to creditRef.
func owedRecoveryPage(t *testing.T, cursor uint64, creditRef string) redis.RecoveryScanPage {
	t.Helper()

	envelope, err := DecodeTransactionWriteBehindEnvelope([]byte(closingWriteBehindRecoveryEnvelope(t, constant.ActionDirect)))
	require.NoError(t, err)

	envelope.Record.Result.FeeDebt = []accounting.FeeDebtChange{{
		TransactionID: envelope.Record.TransactionID, Kind: accounting.FeeDebtOpened, DebtorRef: "@payer#default", CreditRef: creditRef,
	}}
	raw, err := json.Marshal(envelope)
	require.NoError(t, err)

	return closingPage(redis.RecoveryQueueSourceEngineRecover, cursor, string(raw))
}

func TestRefuseOpenFeeDebt_OwedToTheBalance(t *testing.T) {
	ctx := context.Background()
	fees := []*mmodel.Balance{{Alias: "@fees", Key: "default"}}

	newUseCase := func(t *testing.T, debts *owedFeeDebts, pages ...redis.RecoveryScanPage) *UseCase {
		mock := redis.NewMockRedisRepository(gomock.NewController(t))
		cursor := uint64(0)

		for _, page := range pages {
			mock.EXPECT().ScanRecoveryMessages(gomock.Any(), redis.RecoveryQueueSourceEngineRecover, cursor, gomock.Any()).Return(page, nil)
			cursor = page.Cursor
		}

		return &UseCase{TransactionRedisRepo: mock, TransactionReader: &feeDebtReader{}, FeeDebts: debts}
	}
	refuse := func(uc *UseCase, ledgerID uuid.UUID) error {
		return uc.refuseOpenFeeDebt(ctx, recoveryScopeOrgID, ledgerID, fees, engineRecoverySources, nil)
	}
	requireOwed := func(t *testing.T, err error) {
		var unprocessable midazpkg.UnprocessableOperationError
		require.ErrorAs(t, err, &unprocessable)
		assert.Equal(t, constant.ErrBalanceOwedFeeDebt.Error(), unprocessable.Code)
	}

	t.Run("a change still in recovery refuses before the projection is read", func(t *testing.T) {
		debts := &owedFeeDebts{}
		uc := newUseCase(t, debts, closingPage(redis.RecoveryQueueSourceEngineRecover, 7), owedRecoveryPage(t, 0, "@fees#default"))

		requireOwed(t, refuse(uc, recoveryScopeLedgerID))
		assert.Nil(t, debts.refs)
	})

	t.Run("a change owed elsewhere leaves the answer to the projection", func(t *testing.T) {
		for name, scope := range map[string]struct {
			ledgerID uuid.UUID
			creditor string
		}{
			"another creditor": {recoveryScopeLedgerID, "@other#default"},
			"another ledger":   {uuid.New(), "@fees#default"},
		} {
			debts := &owedFeeDebts{}
			uc := newUseCase(t, debts, owedRecoveryPage(t, 0, scope.creditor))

			require.NoError(t, refuse(uc, scope.ledgerID), name)
			assert.Equal(t, []string{"@fees#default"}, debts.refs, name)
		}
	})

	t.Run("a debt the projection holds refuses", func(t *testing.T) {
		uc := newUseCase(t, &owedFeeDebts{owed: true}, redis.RecoveryScanPage{})

		requireOwed(t, refuse(uc, recoveryScopeLedgerID))
	})

	t.Run("an unproven walk or a failed projection read refuses as indeterminate", func(t *testing.T) {
		uc := newUseCase(t, &owedFeeDebts{err: errors.New("mongo down")}, redis.RecoveryScanPage{})
		requireClosingCode(t, refuse(uc, recoveryScopeLedgerID), constant.ErrAccountClosingProtectionIndeterminate)

		uc = newUseCase(t, &owedFeeDebts{}, redis.RecoveryScanPage{Cursor: 7, Bytes: maxRecoveryWalkBytes + 1})
		requireClosingCode(t, refuse(uc, recoveryScopeLedgerID), constant.ErrAccountClosingProtectionIndeterminate)

		mock := redis.NewMockRedisRepository(gomock.NewController(t))
		mock.EXPECT().ScanRecoveryMessages(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(redis.RecoveryScanPage{}, errors.New("redis down"))
		uc = &UseCase{TransactionRedisRepo: mock, TransactionReader: &feeDebtReader{}, FeeDebts: &owedFeeDebts{}}
		requireClosingCode(t, refuse(uc, recoveryScopeLedgerID), constant.ErrAccountClosingProtectionIndeterminate)
	})
}

func TestDeleteBalance_RefusesAFeeDebtOwedToIt(t *testing.T) {
	organizationID, ledgerID, balanceID := uuid.New(), uuid.New(), uuid.New()
	uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
	uc.FeeDebts = &owedFeeDebts{owed: true}
	bal := &mmodel.Balance{ID: balanceID.String(), Alias: "@fees", Key: "default"}

	mockBalanceRepo.EXPECT().Find(gomock.Any(), organizationID, ledgerID, balanceID).Return(bal, nil)
	expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
	mockRedisRepo.EXPECT().Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).Return("", nil)
	expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, bal)

	err := uc.DeleteBalance(context.Background(), organizationID, ledgerID, balanceID)

	var unprocessable midazpkg.UnprocessableOperationError
	require.ErrorAs(t, err, &unprocessable)
	assert.Equal(t, constant.ErrBalanceOwedFeeDebt.Error(), unprocessable.Code)
}

func TestDeleteAccountByID_RefusesABalanceOwedAFeeDebt(t *testing.T) {
	m := newProtectionMocks(t)
	m.expectOwnershipRoundTrip()
	m.uc.FeeDebts = &owedFeeDebts{owed: true}
	creditor := &mmodel.Balance{
		ID: uuid.NewString(), Alias: "@fees", Key: constant.DefaultBalanceKey, AccountID: protectionAccountID.String(),
		Available: decimal.Zero, OnHold: decimal.Zero, OverdraftUsed: decimal.Zero,
	}

	m.account.EXPECT().Find(gomock.Any(), protectionOrgID, protectionLedgerID, nil, protectionAccountID, mmodel.HolderOffV1).
		Return(&mmodel.Account{ID: protectionAccountID.String(), Type: "deposit"}, nil)
	m.balance.EXPECT().ListByAccountID(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return([]*mmodel.Balance{creditor}, nil)
	expectDeleteMarkerPlant(m.redis, protectionOrgID, protectionLedgerID, creditor)
	m.redis.EXPECT().ListBalanceByKey(gomock.Any(), protectionOrgID, protectionLedgerID, gomock.Any()).Return(nil, nil)
	expectDeleteMarkerRelease(m.redis, protectionOrgID, protectionLedgerID, creditor)

	err := m.uc.DeleteAccountByID(context.Background(), protectionOrgID, protectionLedgerID, nil, protectionAccountID, "token")

	var unprocessable midazpkg.UnprocessableOperationError
	require.ErrorAs(t, err, &unprocessable)
	assert.Equal(t, constant.ErrBalanceOwedFeeDebt.Error(), unprocessable.Code)
}

// TestDeleteAccountByID_AnswersAnIndeterminateStateAsRetryable proves an account deletion
// whose balances could not be proven deletable answers the retryable 0520, not 0012.
func TestDeleteAccountByID_AnswersAnIndeterminateStateAsRetryable(t *testing.T) {
	m := newProtectionMocks(t)

	m.account.EXPECT().Find(gomock.Any(), protectionOrgID, protectionLedgerID, nil, protectionAccountID, mmodel.HolderOffV1).
		Return(&mmodel.Account{ID: protectionAccountID.String(), Type: "deposit"}, nil)
	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return("", false, nil)
	m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID, gomock.Any()).
		Return(false, errors.New("cache unavailable"))

	err := m.uc.DeleteAccountByID(context.Background(), protectionOrgID, protectionLedgerID, nil, protectionAccountID, "token")

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
}
