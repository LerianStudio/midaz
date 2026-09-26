// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	midazpkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

var (
	protectionOrgID     = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000001")
	protectionLedgerID  = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000002")
	protectionAccountID = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000003")
	protectionClosedAt  = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
)

type protectionMocks struct {
	uc      *UseCase
	balance *balance.MockRepository
	account *account.MockRepository
	redis   *redis.MockRedisRepository
}

func newProtectionMocks(t *testing.T) *protectionMocks {
	t.Helper()

	ctrl := gomock.NewController(t)

	mocks := &protectionMocks{
		balance: balance.NewMockRepository(ctrl),
		account: account.NewMockRepository(ctrl),
		redis:   redis.NewMockRedisRepository(ctrl),
	}

	mocks.uc = &UseCase{
		BalanceRepo:          mocks.balance,
		AccountRepo:          mocks.account,
		TransactionRedisRepo: mocks.redis,
	}

	return mocks
}

// expectOwnershipRoundTrip programs one successful acquisition and its release for
// the account under test.
func (m *protectionMocks) expectOwnershipRoundTrip() {
	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return("", false, nil)
	m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID, gomock.Any()).
		Return(true, nil)
	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID, gomock.Any()).
		Return(true, nil)
}

// TestCreateAdditionalBalance_RefusesAClosedAccount covers AS-05: a creation that
// arrives after the closing is refused, and no balance is persisted.
func TestCreateAdditionalBalance_RefusesAClosedAccount(t *testing.T) {
	m := newProtectionMocks(t)
	m.expectOwnershipRoundTrip()

	m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return(protectionClosedAt, true, nil)

	_, err := m.uc.CreateAdditionalBalance(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID,
		&mmodel.CreateAdditionalBalance{Key: "savings"})

	require.Error(t, err)

	var unprocessable midazpkg.UnprocessableOperationError
	require.True(t, errors.As(err, &unprocessable))
	assert.Equal(t, constant.ErrAccountClosed.Error(), unprocessable.Code)
}

// TestCreateAdditionalBalance_RefusesWhileAClosingOwnsTheAccount proves the
// creation and the closing coordinate through the same ownership, so a closing in
// flight validates a balance list that cannot grow under it.
func TestCreateAdditionalBalance_RefusesWhileAClosingOwnsTheAccount(t *testing.T) {
	m := newProtectionMocks(t)

	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return(uuid.NewString(), true, nil)

	_, err := m.uc.CreateAdditionalBalance(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID,
		&mmodel.CreateAdditionalBalance{Key: "savings"})

	require.Error(t, err)

	var conflict midazpkg.EntityConflictError
	require.True(t, errors.As(err, &conflict))
	assert.Equal(t, constant.ErrAccountClosingInProgress.Error(), conflict.Code)
}

// administrativeOperations are the exclusive callers of the account protection,
// each run the way its entrypoint is called, with the span it records under.
var administrativeOperations = []struct {
	name string
	span string
	run  func(ctx context.Context, uc *UseCase) error
}{
	{
		name: "default balance creation",
		span: "command.create_default_balance",
		run: func(ctx context.Context, uc *UseCase) error {
			_, err := uc.CreateDefaultBalance(ctx, mmodel.CreateBalanceInput{
				OrganizationID: protectionOrgID,
				LedgerID:       protectionLedgerID,
				AccountID:      protectionAccountID,
				Alias:          "wallet",
				AssetCode:      "BRL",
				AccountType:    "deposit",
			})

			return err
		},
	},
	{
		name: "additional balance creation",
		span: "command.create_additional_balance",
		run: func(ctx context.Context, uc *UseCase) error {
			_, err := uc.CreateAdditionalBalance(ctx, protectionOrgID, protectionLedgerID, protectionAccountID,
				&mmodel.CreateAdditionalBalance{Key: "savings"})

			return err
		},
	},
	{
		name: "balance deletion",
		span: "exec.delete_all_balances_by_account_id",
		run: func(ctx context.Context, uc *UseCase) error {
			return uc.DeleteAllBalancesByAccountID(ctx, protectionOrgID, protectionLedgerID, protectionAccountID, uuid.NewString())
		},
	},
}

// TestAdministrativeOperations_RefusedAsBusyWhileALoadHoldsTheAccount runs on
// every exclusive caller: while a cache-miss load holds a seed admission, the
// exclusive acquisition fails with no closing marker anywhere, so the operation is
// refused with the retryable busy code, not as a closing, writes nothing and keeps
// its span green. The strict mocks fail the test on any balance read or write past
// the refusal.
func TestAdministrativeOperations_RefusedAsBusyWhileALoadHoldsTheAccount(t *testing.T) {
	for _, op := range administrativeOperations {
		t.Run(op.name, func(t *testing.T) {
			m := newProtectionMocks(t)
			ctx, recorder := recordingContext()

			m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
				Return("", false, nil).Times(2)
			m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID, gomock.Any()).
				Return(false, nil)

			err := op.run(ctx, m.uc)

			require.Error(t, err)

			var conflict midazpkg.EntityConflictError
			require.True(t, errors.As(err, &conflict))
			assert.Equal(t, constant.ErrAccountAdministrativeOperationInProgress.Error(), conflict.Code)
			assert.Equal(t, codes.Unset, findSpan(t, recorder, op.span).Status().Code)
		})
	}
}

// TestAdministrativeOperations_RecordAnUnreadableProtectionAsTechnical is the
// other class of the same refusal: a cache that cannot answer the acquisition is a
// dependency failing, so the operation's span turns red instead of reading the
// failure as another holder.
func TestAdministrativeOperations_RecordAnUnreadableProtectionAsTechnical(t *testing.T) {
	for _, op := range administrativeOperations {
		t.Run(op.name, func(t *testing.T) {
			m := newProtectionMocks(t)
			ctx, recorder := recordingContext()

			m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
				Return("", false, nil)
			m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID, gomock.Any()).
				Return(false, errors.New("cache unavailable"))

			err := op.run(ctx, m.uc)

			require.Error(t, err)

			var unavailable midazpkg.ServiceUnavailableError
			require.True(t, errors.As(err, &unavailable))
			assert.Equal(t, constant.ErrAccountClosingProtectionIndeterminate.Error(), unavailable.Code)
			assert.Equal(t, codes.Error, findSpan(t, recorder, op.span).Status().Code)
		})
	}
}

// TestCreateAdditionalBalance_RejectedBeforeProtection proves the reserved-key
// guard still answers first: a payload that can never be valid costs no ownership.
func TestCreateAdditionalBalance_RejectedBeforeProtection(t *testing.T) {
	m := newProtectionMocks(t)

	_, err := m.uc.CreateAdditionalBalance(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID,
		&mmodel.CreateAdditionalBalance{Key: constant.OverdraftBalanceKey})

	require.Error(t, err)
}

// TestDeleteAllBalancesByAccountID_RefusedWhileAClosingOwnsTheAccount proves the
// delete takes the same ownership, so it cannot change the balance list a closing
// is validating.
func TestDeleteAllBalancesByAccountID_RefusedWhileAClosingOwnsTheAccount(t *testing.T) {
	m := newProtectionMocks(t)

	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return(uuid.NewString(), true, nil)

	err := m.uc.DeleteAllBalancesByAccountID(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID, uuid.NewString())

	require.Error(t, err)

	var conflict midazpkg.EntityConflictError
	require.True(t, errors.As(err, &conflict))
	assert.Equal(t, constant.ErrAccountClosingInProgress.Error(), conflict.Code)
}

// TestDeleteAllBalancesByAccountID_ReleasesOwnershipWhenThereIsNothingToDelete
// proves the ownership is given back as soon as the operation's result is known.
func TestDeleteAllBalancesByAccountID_ReleasesOwnershipWhenThereIsNothingToDelete(t *testing.T) {
	m := newProtectionMocks(t)
	m.expectOwnershipRoundTrip()

	m.balance.EXPECT().ListByAccountID(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return([]*mmodel.Balance{}, nil)

	require.NoError(t, m.uc.DeleteAllBalancesByAccountID(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID, uuid.NewString()))
}

// expectOwnershipHeld programs a successful acquisition and NO release, which is
// what an unresolved write outcome must leave behind: the mock controller fails the
// test if the ownership is given back.
func (m *protectionMocks) expectOwnershipHeld() {
	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return("", false, nil)
	m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID, gomock.Any()).
		Return(true, nil)
}

// expectAccountIsOpen programs the closing state of an account that was never
// closed: no negative cache entry and an authoritative row carrying no instant.
func (m *protectionMocks) expectAccountIsOpen() {
	m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return(time.Time{}, false, nil)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), protectionOrgID, protectionLedgerID, []uuid.UUID{protectionAccountID}).
		Return(map[uuid.UUID]*time.Time{protectionAccountID: nil}, nil)
}

// expectAdditionalBalanceReachesTheWrite programs the lookups CreateAdditionalBalance
// performs between the protection and its own INSERT.
func (m *protectionMocks) expectAdditionalBalanceReachesTheWrite() {
	m.balance.EXPECT().FindByAccountIDAndKey(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID, "savings").
		Return(nil, midazpkg.EntityNotFoundError{Code: constant.ErrDefaultBalanceNotFound.Error()})
	m.balance.EXPECT().FindByAccountIDAndKey(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID, constant.DefaultBalanceKey).
		Return(&mmodel.Balance{
			ID:             uuid.NewString(),
			Alias:          "wallet",
			OrganizationID: protectionOrgID.String(),
			LedgerID:       protectionLedgerID.String(),
			AccountID:      protectionAccountID.String(),
			AssetCode:      "BRL",
			AccountType:    "deposit",
		}, nil)
}

// TestCreateAdditionalBalance_KnownWriteRefusalReleasesTheOwnership proves a write
// the server rejected is a resolved outcome: nothing landed, so the account goes
// back to being available to a closing immediately.
func TestCreateAdditionalBalance_KnownWriteRefusalReleasesTheOwnership(t *testing.T) {
	m := newProtectionMocks(t)
	m.expectOwnershipRoundTrip()
	m.expectAccountIsOpen()
	m.expectAdditionalBalanceReachesTheWrite()

	m.balance.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(nil, &pgconn.PgError{Code: constant.UniqueViolationCode, ConstraintName: balanceAccountKeyUniqueIndex})

	_, err := m.uc.CreateAdditionalBalance(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID,
		&mmodel.CreateAdditionalBalance{Key: "savings"})

	require.Error(t, err)

	var conflict midazpkg.EntityConflictError
	require.True(t, errors.As(err, &conflict))
	assert.Equal(t, constant.ErrDuplicatedAliasKeyValue.Error(), conflict.Code)
}

// TestCreateAdditionalBalance_UnresolvedWriteKeepsTheOwnership covers D5: a write
// whose outcome cannot be established may still have landed, so the ownership stays
// for reconciliation instead of letting a closing validate a list that may grow.
func TestCreateAdditionalBalance_UnresolvedWriteKeepsTheOwnership(t *testing.T) {
	m := newProtectionMocks(t)
	m.expectOwnershipHeld()
	m.expectAccountIsOpen()
	m.expectAdditionalBalanceReachesTheWrite()

	m.balance.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil, context.DeadlineExceeded)

	_, err := m.uc.CreateAdditionalBalance(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID,
		&mmodel.CreateAdditionalBalance{Key: "savings"})

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestDeleteAllBalancesByAccountID_UnresolvedWriteKeepsTheOwnership pins the same
// classification on the deletion: the first write is the permission flip, and an
// unresolved answer to it leaves the account owned.
func TestDeleteAllBalancesByAccountID_UnresolvedWriteKeepsTheOwnership(t *testing.T) {
	m := newProtectionMocks(t)
	m.expectOwnershipHeld()

	deletable := &mmodel.Balance{
		ID:             uuid.NewString(),
		Alias:          "wallet",
		Key:            constant.DefaultBalanceKey,
		OrganizationID: protectionOrgID.String(),
		LedgerID:       protectionLedgerID.String(),
		AccountID:      protectionAccountID.String(),
		Available:      decimal.Zero,
		OnHold:         decimal.Zero,
		OverdraftUsed:  decimal.Zero,
	}

	m.balance.EXPECT().ListByAccountID(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return([]*mmodel.Balance{deletable}, nil)

	m.redis.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(2)
	m.redis.EXPECT().ListBalanceByKey(gomock.Any(), protectionOrgID, protectionLedgerID, gomock.Any()).Return(nil, nil)
	m.redis.EXPECT().DeleteIfValue(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	m.redis.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// The flip and the rollback the toggle attempts after it fails.
	m.balance.EXPECT().UpdateAllByAccountID(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID, gomock.Any()).
		Return(context.DeadlineExceeded).Times(2)

	err := m.uc.DeleteAllBalancesByAccountID(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID, uuid.NewString())

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestDeleteAllBalancesByAccountID_KnownWriteRefusalReleasesTheOwnership keeps the
// other half honest: a soft delete the server rejected is a resolved outcome.
func TestDeleteAllBalancesByAccountID_KnownWriteRefusalReleasesTheOwnership(t *testing.T) {
	m := newProtectionMocks(t)
	m.expectOwnershipRoundTrip()

	deletable := &mmodel.Balance{
		ID:             uuid.NewString(),
		Alias:          "wallet",
		Key:            constant.DefaultBalanceKey,
		OrganizationID: protectionOrgID.String(),
		LedgerID:       protectionLedgerID.String(),
		AccountID:      protectionAccountID.String(),
		Available:      decimal.Zero,
		OnHold:         decimal.Zero,
		OverdraftUsed:  decimal.Zero,
	}

	m.balance.EXPECT().ListByAccountID(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return([]*mmodel.Balance{deletable}, nil)

	m.redis.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(2)
	m.redis.EXPECT().ListBalanceByKey(gomock.Any(), protectionOrgID, protectionLedgerID, gomock.Any()).Return(nil, nil)
	m.redis.EXPECT().DeleteIfValue(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()

	m.balance.EXPECT().UpdateAllByAccountID(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID, gomock.Any()).
		Return(nil).Times(2)
	m.balance.EXPECT().DeleteAllByIDs(gomock.Any(), protectionOrgID, protectionLedgerID, gomock.Any()).
		Return(&pgconn.PgError{Code: "23503"})

	err := m.uc.DeleteAllBalancesByAccountID(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID, uuid.NewString())

	require.Error(t, err)
}

// TestCreateDefaultBalance_RefusesAClosedAccount covers AS-05 on the account
// creation path: a closing that won the race stops the default balance from being
// persisted into an account that is already closed.
func TestCreateDefaultBalance_RefusesAClosedAccount(t *testing.T) {
	m := newProtectionMocks(t)
	m.expectOwnershipRoundTrip()

	m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return(protectionClosedAt, true, nil)

	_, err := m.uc.CreateDefaultBalance(context.Background(), mmodel.CreateBalanceInput{
		OrganizationID: protectionOrgID,
		LedgerID:       protectionLedgerID,
		AccountID:      protectionAccountID,
		Alias:          "wallet",
		AssetCode:      "BRL",
		AccountType:    "deposit",
	})

	require.Error(t, err)

	var unprocessable midazpkg.UnprocessableOperationError
	require.True(t, errors.As(err, &unprocessable))
	assert.Equal(t, constant.ErrAccountClosed.Error(), unprocessable.Code)
}

// TestCreateDefaultBalance_UnresolvedWriteKeepsTheOwnership pins the same D5
// classification on the bootstrap creation.
func TestCreateDefaultBalance_UnresolvedWriteKeepsTheOwnership(t *testing.T) {
	m := newProtectionMocks(t)
	m.expectOwnershipHeld()
	m.expectAccountIsOpen()

	m.balance.EXPECT().ExistsByAccountIDAndKey(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID, constant.DefaultBalanceKey).
		Return(false, nil)
	m.balance.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil, context.DeadlineExceeded)

	_, err := m.uc.CreateDefaultBalance(context.Background(), mmodel.CreateBalanceInput{
		OrganizationID: protectionOrgID,
		LedgerID:       protectionLedgerID,
		AccountID:      protectionAccountID,
		Alias:          "wallet",
		AssetCode:      "BRL",
		AccountType:    "deposit",
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestCreateDefaultBalance_ExternalIsExempt proves the asset flow keeps its
// pre-existing shape: an external account cannot be closed at all (0074), so its
// default balance takes no ownership and reads no protection state.
func TestCreateDefaultBalance_ExternalIsExempt(t *testing.T) {
	m := newProtectionMocks(t)

	created := &mmodel.Balance{ID: uuid.NewString()}

	m.balance.EXPECT().ExistsByAccountIDAndKey(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID, constant.DefaultBalanceKey).
		Return(false, nil)
	m.balance.EXPECT().Create(gomock.Any(), gomock.Any()).Return(created, nil)

	balanceOut, err := m.uc.CreateDefaultBalance(context.Background(), mmodel.CreateBalanceInput{
		OrganizationID: protectionOrgID,
		LedgerID:       protectionLedgerID,
		AccountID:      protectionAccountID,
		Alias:          "@external/BRL",
		AssetCode:      "BRL",
		AccountType:    constant.ExternalAccountType,
	})

	require.NoError(t, err)
	assert.Equal(t, created, balanceOut)
}

// TestAccountProtection_WithoutRepositoriesIsInert proves a use case wired without
// the protection surface behaves exactly as before.
func TestAccountProtection_WithoutRepositoriesIsInert(t *testing.T) {
	uc := &UseCase{}

	admission, err := uc.acquireAccountOwnership(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID)
	require.NoError(t, err)

	admission.Release(context.Background())

	require.NoError(t, uc.ensureAccountsNotClosed(context.Background(), protectionOrgID, protectionLedgerID,
		constant.ErrAccountIneligibility, protectionAccountID))
}
