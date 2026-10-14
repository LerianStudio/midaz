// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

type configResolutionFixture struct {
	uc        *UseCase
	redis     *redis.MockRedisRepository
	balances  *balance.MockRepository
	accounts  *account.MockRepository
	orgID     uuid.UUID
	ledgerID  uuid.UUID
	loaded    *mmodel.Balance
	primary   *mmodel.Balance
	account   *mmodel.Account
	ref       string
	accountID uuid.UUID
}

// newConfigResolutionFixture loads one balance whose configuration keys are both
// absent. The loaded copy carries the defaults; the primary row is at version 4
// with sending off and an overdraft, and the account row is blocked at version 2.
func newConfigResolutionFixture(t *testing.T) configResolutionFixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	orgID, ledgerID, accountID, balanceID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	limit := "30"
	blocked := true

	loaded := &mmodel.Balance{
		ID: balanceID.String(), AccountID: accountID.String(), Alias: "@cfg", Key: "default",
		AllowSending: true, AllowReceiving: true,
	}
	primary := &mmodel.Balance{
		ID: balanceID.String(), AccountID: accountID.String(), Alias: "@cfg", Key: "default",
		AllowSending: false, AllowReceiving: true, ConfigVersion: 4,
		Settings: &mmodel.BalanceSettings{
			BalanceScope: mmodel.BalanceScopeTransactional, AllowOverdraft: true,
			OverdraftLimitEnabled: true, OverdraftLimit: &limit,
		},
	}

	f := configResolutionFixture{
		redis:     redis.NewMockRedisRepository(ctrl),
		balances:  balance.NewMockRepository(ctrl),
		accounts:  account.NewMockRepository(ctrl),
		orgID:     orgID,
		ledgerID:  ledgerID,
		loaded:    loaded,
		primary:   primary,
		account:   &mmodel.Account{ID: accountID.String(), Blocked: &blocked, ConfigVersion: 2},
		ref:       "@cfg#default",
		accountID: accountID,
	}
	f.uc = &UseCase{TransactionRedisRepo: f.redis, BalanceRepo: f.balances, AccountRepo: f.accounts}

	return f
}

func (f configResolutionFixture) primaryConfigs(t *testing.T) (accounting.BalanceConfig, accounting.AccountConfig) {
	t.Helper()

	balanceConfig, err := balancecache.BalanceConfigFromBalance(f.primary)
	require.NoError(t, err)

	accountConfig, err := balancecache.AccountConfigFromAccount(f.account)
	require.NoError(t, err)

	return balanceConfig, accountConfig
}

func (f configResolutionFixture) expectPrimaryReads() {
	f.balances.EXPECT().ListByAliasesWithKeysOnPrimary(gomock.Any(), f.orgID, f.ledgerID, []string{f.ref}).
		Return([]*mmodel.Balance{f.primary}, nil)
	f.accounts.EXPECT().ListAccountsByIDs(gomock.Any(), f.orgID, f.ledgerID, []uuid.UUID{f.accountID}).
		Return([]*mmodel.Account{f.account}, nil)
}

func assertCarriesPrimaryConfiguration(t *testing.T, b *mmodel.Balance) {
	t.Helper()

	assert.False(t, b.AllowSending)
	assert.Equal(t, int64(4), b.ConfigVersion)
	require.NotNil(t, b.Settings)
	assert.True(t, b.Settings.AllowOverdraft)
	assert.True(t, b.Blocked)
	assert.Equal(t, int64(2), b.AccountConfigVersion)
}

// A key write that fails is not a failed load: the primary row is the truth and
// the engine takes the request's configuration while the key is missing.
func TestResolveEngineConfiguration_FailedKeyWritesKeepThePrimaryRow(t *testing.T) {
	t.Parallel()

	for name, writeErr := range map[string]error{
		"cache unavailable":                    errors.New("connection refused"),
		"other content under the same version": redis.ErrConfigVersionConflict,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newConfigResolutionFixture(t)
			f.redis.EXPECT().ReadConfigKeys(gomock.Any(), f.orgID, f.ledgerID, []string{f.ref}, []uuid.UUID{f.accountID}).
				Return(redis.ConfigKeyValues{}, nil)
			f.expectPrimaryReads()
			f.redis.EXPECT().WriteBalanceConfig(gomock.Any(), f.orgID, f.ledgerID, f.ref, gomock.Any()).
				Return(accounting.BalanceConfig{}, redis.ConfigWriteOutcome(0), writeErr)
			f.redis.EXPECT().WriteAccountConfig(gomock.Any(), f.orgID, f.ledgerID, gomock.Any()).
				Return(accounting.AccountConfig{}, redis.ConfigWriteOutcome(0), writeErr)

			require.NoError(t, f.uc.resolveEngineConfiguration(t.Context(), f.orgID, f.ledgerID, []*mmodel.Balance{f.loaded}))
			assertCarriesPrimaryConfiguration(t, f.loaded)
		})
	}
}

// A key read that fails is absence: every configuration is reloaded and written.
func TestResolveEngineConfiguration_FailedKeyReadReloadsFromThePrimary(t *testing.T) {
	t.Parallel()

	f := newConfigResolutionFixture(t)
	balanceConfig, accountConfig := f.primaryConfigs(t)

	f.redis.EXPECT().ReadConfigKeys(gomock.Any(), f.orgID, f.ledgerID, []string{f.ref}, []uuid.UUID{f.accountID}).
		Return(redis.ConfigKeyValues{}, errors.New("connection refused"))
	f.expectPrimaryReads()
	f.redis.EXPECT().WriteBalanceConfig(gomock.Any(), f.orgID, f.ledgerID, f.ref, balanceConfig).
		Return(balanceConfig, redis.ConfigWritten, nil)
	f.redis.EXPECT().WriteAccountConfig(gomock.Any(), f.orgID, f.ledgerID, accountConfig).
		Return(accountConfig, redis.ConfigWritten, nil)

	require.NoError(t, f.uc.resolveEngineConfiguration(t.Context(), f.orgID, f.ledgerID, []*mmodel.Balance{f.loaded}))
	assertCarriesPrimaryConfiguration(t, f.loaded)
}

// A configuration that cannot be read from the primary cannot be assumed.
func TestResolveEngineConfiguration_FailedPrimaryReadFailsTheLoad(t *testing.T) {
	t.Parallel()

	for name, expect := range map[string]func(configResolutionFixture, error){
		"balance": func(f configResolutionFixture, err error) {
			f.balances.EXPECT().ListByAliasesWithKeysOnPrimary(gomock.Any(), f.orgID, f.ledgerID, []string{f.ref}).
				Return(nil, err)
		},
		"account": func(f configResolutionFixture, err error) {
			f.balances.EXPECT().ListByAliasesWithKeysOnPrimary(gomock.Any(), f.orgID, f.ledgerID, []string{f.ref}).
				Return([]*mmodel.Balance{f.primary}, nil)
			f.redis.EXPECT().WriteBalanceConfig(gomock.Any(), f.orgID, f.ledgerID, f.ref, gomock.Any()).
				Return(accounting.BalanceConfig{}, redis.ConfigWritten, nil)
			f.accounts.EXPECT().ListAccountsByIDs(gomock.Any(), f.orgID, f.ledgerID, []uuid.UUID{f.accountID}).
				Return(nil, err)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newConfigResolutionFixture(t)
			primaryErr := errors.New("primary unavailable")

			f.redis.EXPECT().ReadConfigKeys(gomock.Any(), f.orgID, f.ledgerID, []string{f.ref}, []uuid.UUID{f.accountID}).
				Return(redis.ConfigKeyValues{}, nil)
			expect(f, primaryErr)

			err := f.uc.resolveEngineConfiguration(t.Context(), f.orgID, f.ledgerID, []*mmodel.Balance{f.loaded})
			require.ErrorIs(t, err, primaryErr)
		})
	}
}

// An account without a row is not blocked and has no key to write; a balance the
// primary no longer has under its identity keeps the configuration it was loaded
// with, and nothing is written for it.
func TestResolveEngineConfiguration_RowsMissingOnThePrimaryWriteNothing(t *testing.T) {
	t.Parallel()

	f := newConfigResolutionFixture(t)
	f.loaded.Blocked = true
	recreated := *f.primary
	recreated.ID = uuid.New().String()

	f.redis.EXPECT().ReadConfigKeys(gomock.Any(), f.orgID, f.ledgerID, []string{f.ref}, []uuid.UUID{f.accountID}).
		Return(redis.ConfigKeyValues{}, nil)
	f.balances.EXPECT().ListByAliasesWithKeysOnPrimary(gomock.Any(), f.orgID, f.ledgerID, []string{f.ref}).
		Return([]*mmodel.Balance{&recreated}, nil)
	f.accounts.EXPECT().ListAccountsByIDs(gomock.Any(), f.orgID, f.ledgerID, []uuid.UUID{f.accountID}).
		Return(nil, nil)

	require.NoError(t, f.uc.resolveEngineConfiguration(t.Context(), f.orgID, f.ledgerID, []*mmodel.Balance{f.loaded}))

	assert.True(t, f.loaded.AllowSending, "another balance's row never configures this one")
	assert.Zero(t, f.loaded.ConfigVersion)
	assert.False(t, f.loaded.Blocked, "an account without a row is not blocked")
	assert.Zero(t, f.loaded.AccountConfigVersion)
}

// The configuration a balance carries must read back as the same configuration,
// limit included, so the engine request holds exactly what the key holds.
func TestApplyBalanceConfig_RoundTripsThroughTheBalance(t *testing.T) {
	t.Parallel()

	balanceID := uuid.New()

	for name, config := range map[string]accounting.BalanceConfig{
		"defaults": {
			BalanceID: balanceID, Version: 1, AllowSending: true, AllowReceiving: true,
			OverdraftLimit: decimal.Zero, BalanceScope: mmodel.BalanceScopeTransactional,
		},
		"enabled limit": {
			BalanceID: balanceID, Version: 2, AllowOverdraft: true, OverdraftLimitEnabled: true,
			OverdraftLimit: decimal.RequireFromString("12.5"), BalanceScope: mmodel.BalanceScopeTransactional,
		},
		"limit kept while disabled": {
			BalanceID: balanceID, Version: 3, AllowSending: true,
			OverdraftLimit: decimal.NewFromInt(7), BalanceScope: mmodel.BalanceScopeTransactional,
		},
		"internal scope": {
			BalanceID: balanceID, Version: 4,
			OverdraftLimit: decimal.Zero, BalanceScope: mmodel.BalanceScopeInternal,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			limit := "999"
			b := &mmodel.Balance{ID: balanceID.String(), Settings: &mmodel.BalanceSettings{OverdraftLimit: &limit}}

			applyBalanceConfig(b, config)

			got, err := balancecache.BalanceConfigFromBalance(b)
			require.NoError(t, err)
			assert.True(t, got.Equal(config), "got %+v, want %+v", got, config)
		})
	}
}
