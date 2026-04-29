// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// raceTestFixture returns a canonical (orgID, ledgerID, balanceID, current,
// update) bundle driving the false→true AllowOverdraft transition. Centralised
// so every race-handling test exercises the same initial state.
func raceTestFixture(t *testing.T) (uuid.UUID, uuid.UUID, uuid.UUID, *mmodel.Balance, mmodel.UpdateBalance) {
	t.Helper()

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	newSettings := &mmodel.BalanceSettings{
		BalanceScope:   mmodel.BalanceScopeTransactional,
		AllowOverdraft: true,
	}

	current := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          "@race",
		Key:            "default",
		AssetCode:      "USD",
		Direction:      constant.DirectionCredit,
		OverdraftUsed:  decimal.Zero,
		Settings:       mmodel.NewDefaultBalanceSettings(),
	}

	return orgID, ledgerID, balanceID, current, mmodel.UpdateBalance{Settings: newSettings}
}

// TestEnsureOverdraftBalance_ConcurrentCreate_ReturnsBenignSuccess verifies
// that when BalanceRepo.Create returns a PostgreSQL unique_violation
// (SQLSTATE 23505), ensureOverdraftBalance treats it as idempotent success
// as long as the second FindByAccountIDAndKey resolves the peer-created
// row. This is the core race-condition guarantee: two concurrent PATCH
// requests flipping AllowOverdraft MUST both succeed, with exactly one
// overdraft balance materialised in the end.
func TestEnsureOverdraftBalance_ConcurrentCreate_ReturnsBenignSuccess(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID, ledgerID, balanceID, current, update := raceTestFixture(t)

	expected := *current
	expected.Settings = update.Settings

	peerOverdraft := &mmodel.Balance{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		OrganizationID: current.OrganizationID,
		LedgerID:       current.LedgerID,
		AccountID:      current.AccountID,
		Alias:          current.Alias,
		Key:            constant.OverdraftBalanceKey,
		AssetCode:      current.AssetCode,
		Direction:      constant.DirectionDebit,
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(current, nil).
		Times(1)

	// First lookup: peer request has not yet committed → not found.
	// Second lookup (after the 23505 from Create): peer's row is visible.
	gomock.InOrder(
		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, gomock.Any(), constant.OverdraftBalanceKey).
			Return(nil, nil).
			Times(1),
		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, gomock.Any(), constant.OverdraftBalanceKey).
			Return(peerOverdraft, nil).
			Times(1),
	)

	// Simulate the partial UNIQUE index rejecting our insert.
	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil, &pgconn.PgError{Code: constant.UniqueViolationCode}).
		Times(1)

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), orgID, ledgerID, balanceID, update).
		Return(&expected, nil).
		Times(1)

	mockRedisRepo.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	// Cache settings are rewritten in-place after a settings update (live
	// transactional state is preserved — no Del).
	mockRedisRepo.EXPECT().
		UpdateBalanceCacheSettings(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), orgID, ledgerID, balanceID, update)

	require.NoError(t, err, "a benign 23505 race MUST NOT surface as an error to the caller")
	require.NotNil(t, result)
	require.NotNil(t, result.Settings)
	assert.True(t, result.Settings.AllowOverdraft)
}

// TestEnsureOverdraftBalance_UniqueViolation_WithMissingRow_PropagatesError
// covers the defensive branch: a 23505 was raised but the follow-up Find
// still returns nil. That almost certainly means the unique violation came
// from a DIFFERENT index than the one guarding our tuple, so we must NOT
// silently swallow the error — the caller needs to see it.
func TestEnsureOverdraftBalance_UniqueViolation_WithMissingRow_PropagatesError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID, ledgerID, balanceID, current, update := raceTestFixture(t)

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(current, nil).
		Times(1)

	// Both lookups return nil — peer did not actually create the row, so
	// the 23505 did not come from our target tuple.
	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, gomock.Any(), constant.OverdraftBalanceKey).
		Return(nil, nil).
		Times(2)

	pgErr := &pgconn.PgError{Code: constant.UniqueViolationCode}
	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil, pgErr).
		Times(1)

	// Update MUST NOT be called when ensureOverdraftBalance fails — the
	// parent balance's settings stay intact so the pair remains consistent.
	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), orgID, ledgerID, balanceID, update)

	require.Error(t, err, "unexplained 23505 MUST be surfaced to the caller")
	assert.Nil(t, result)
	assert.ErrorIs(t, err, pgErr, "the original Create error MUST be preserved")
}

// TestEnsureOverdraftBalance_NonUniqueViolation_PropagatesError verifies
// that non-23505 errors from Create (connectivity failures, FK violations,
// etc.) are propagated unchanged without touching the reload path.
func TestEnsureOverdraftBalance_NonUniqueViolation_PropagatesError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID, ledgerID, balanceID, current, update := raceTestFixture(t)

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(current, nil).
		Times(1)

	// Only the initial idempotency check runs. The reload path MUST NOT
	// fire for non-unique-violation errors.
	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, gomock.Any(), constant.OverdraftBalanceKey).
		Return(nil, nil).
		Times(1)

	createErr := errors.New("connection reset by peer")
	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil, createErr).
		Times(1)

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), orgID, ledgerID, balanceID, update)

	require.Error(t, err, "non-23505 Create errors MUST surface to the caller")
	assert.Nil(t, result)
	assert.ErrorIs(t, err, createErr, "the original Create error MUST be preserved")
}

// TestIsUniqueViolation_PgError_Code23505 asserts the helper that gates the
// race-recovery branch. It MUST return true only for pgconn.PgError values
// carrying SQLSTATE 23505 and false for every other error type (generic
// errors, wrapped errors with a different code, nil).
func TestIsUniqueViolation_PgError_Code23505(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "PgError with code 23505 is a unique violation",
			err:  &pgconn.PgError{Code: constant.UniqueViolationCode},
			want: true,
		},
		{
			name: "PgError with another code is not a unique violation",
			err:  &pgconn.PgError{Code: "23503"},
			want: false,
		},
		{
			name: "generic error is not a unique violation",
			err:  errors.New("boom"),
			want: false,
		},
		{
			name: "nil error is not a unique violation",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, isUniqueViolation(tc.err))
		})
	}
}
