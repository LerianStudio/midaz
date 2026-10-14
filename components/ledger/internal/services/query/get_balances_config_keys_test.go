// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
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

			require.NoError(t, f.uc.resolveEngineConfiguration(t.Context(), f.orgID, f.ledgerID, []*mmodel.Balance{f.loaded}, nil))
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

	require.NoError(t, f.uc.resolveEngineConfiguration(t.Context(), f.orgID, f.ledgerID, []*mmodel.Balance{f.loaded}, nil))
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

			err := f.uc.resolveEngineConfiguration(t.Context(), f.orgID, f.ledgerID, []*mmodel.Balance{f.loaded}, nil)
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

	require.NoError(t, f.uc.resolveEngineConfiguration(t.Context(), f.orgID, f.ledgerID, []*mmodel.Balance{f.loaded}, nil))

	assert.True(t, f.loaded.AllowSending, "another balance's row never configures this one")
	assert.Zero(t, f.loaded.ConfigVersion)
	assert.False(t, f.loaded.Blocked, "an account without a row is not blocked")
	assert.Zero(t, f.loaded.AccountConfigVersion)
}

// A balance loaded with a copy that carries the version the engine decides with but
// other content is reported once per balance and once per account, with a Warn and
// the divergence counter by family; a copy at another version, or with the same
// content, is not.
func TestResolveEngineConfiguration_ReportsACopyDivergingUnderTheDecidedVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		seeded          bool
		shape           func(f configResolutionFixture, sibling *mmodel.Balance)
		balanceDiverges int64
		accountDiverges int64
	}{
		{
			name: "same versions, other content",
			shape: func(f configResolutionFixture, sibling *mmodel.Balance) {
				// An earlier release turned sending on and unblocked the account in
				// the blobs without changing the version fields it does not know.
				f.loaded.ConfigVersion, f.loaded.AccountConfigVersion = 4, 2
				sibling.AccountConfigVersion = 2
			},
			balanceDiverges: 1,
			accountDiverges: 1,
		},
		{
			// Rows read on a cache miss that tie with the keys: the keys decide and
			// the divergence is reported as for a blob copy.
			name:   "seeded rows, same versions, other content",
			seeded: true,
			shape: func(f configResolutionFixture, sibling *mmodel.Balance) {
				f.loaded.ConfigVersion, f.loaded.AccountConfigVersion = 4, 2
				sibling.AccountConfigVersion = 2
			},
			balanceDiverges: 1,
			accountDiverges: 1,
		},
		{
			name:   "seeded rows, older versions",
			seeded: true,
			shape: func(f configResolutionFixture, sibling *mmodel.Balance) {
				f.loaded.ConfigVersion, f.loaded.AccountConfigVersion = 3, 1
				sibling.AccountConfigVersion = 1
			},
		},
		{
			name: "older versions",
			shape: func(f configResolutionFixture, sibling *mmodel.Balance) {
				f.loaded.ConfigVersion, f.loaded.AccountConfigVersion = 3, 1
				sibling.AccountConfigVersion = 1
			},
		},
		{
			name: "same versions, same content",
			shape: func(f configResolutionFixture, sibling *mmodel.Balance) {
				*f.loaded = *f.primary
				f.loaded.Blocked, f.loaded.AccountConfigVersion = true, 2
				sibling.Blocked, sibling.AccountConfigVersion = true, 2
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newConfigResolutionFixture(t)
			reader := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

			factory, err := metrics.NewMetricsFactory(provider.Meter("config-copy-divergence-test"), nil)
			require.NoError(t, err)

			f.uc.MetricsFactory = factory

			// A second balance of the same account, with its own key, so the
			// account is reported once whatever the number of its balances.
			siblingID := uuid.New()
			sibling := &mmodel.Balance{
				ID: siblingID.String(), AccountID: f.accountID.String(), Alias: "@cfg", Key: "savings",
				AllowSending: true, AllowReceiving: true,
			}
			siblingConfig := accounting.BalanceConfig{
				BalanceID: siblingID, AllowSending: true, AllowReceiving: true,
				OverdraftLimit: decimal.Zero, BalanceScope: mmodel.BalanceScopeTransactional,
			}
			tt.shape(f, sibling)

			balanceConfig, accountConfig := f.primaryConfigs(t)
			f.redis.EXPECT().ReadConfigKeys(gomock.Any(), f.orgID, f.ledgerID, []string{f.ref, "@cfg#savings"}, []uuid.UUID{f.accountID}).
				Return(redis.ConfigKeyValues{
					Balances: map[string]accounting.BalanceConfig{f.ref: balanceConfig, "@cfg#savings": siblingConfig},
					Accounts: map[uuid.UUID]accounting.AccountConfig{f.accountID: accountConfig},
				}, nil)

			logs := &recordingLogger{}
			ctx := libObservability.ContextWithLogger(t.Context(), logs)

			var seeded map[string]struct{}
			if tt.seeded {
				seeded = map[string]struct{}{f.loaded.ID: {}, sibling.ID: {}}
			}

			// No key write is expected: a row that does not outrank its key never
			// replaces it.
			require.NoError(t, f.uc.resolveEngineConfiguration(ctx, f.orgID, f.ledgerID, []*mmodel.Balance{f.loaded, sibling}, seeded))
			assertCarriesPrimaryConfiguration(t, f.loaded)

			divergent := counterByLabel(t, reader, utils.ConfigCopyDivergent.Name, "family")
			assert.Equal(t, tt.balanceDiverges, divergent["balance"])
			assert.Equal(t, tt.accountDiverges, divergent["account"])

			const message = "Configuration copy holds other content under the version the engine decides with"
			if tt.balanceDiverges == 0 && tt.accountDiverges == 0 {
				assert.Empty(t, logs.entriesWith(message))

				return
			}

			entry := logs.entryWith(t, message)
			assert.Equal(t, []string{f.ref}, entry.fields["balance_refs"])
			assert.Equal(t, []string{f.accountID.String()}, entry.fields["account_ids"])
		})
	}
}

// A balance seeded from rows newer than its live keys decides with the rows, writes
// them to the keys, and reports the older keys; the rows are not a divergent copy.
func TestResolveEngineConfiguration_SeededRowsNewerThanTheKeysDecide(t *testing.T) {
	t.Parallel()

	f := newConfigResolutionFixture(t)
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	factory, err := metrics.NewMetricsFactory(provider.Meter("config-seeded-rows-test"), nil)
	require.NoError(t, err)

	f.uc.MetricsFactory = factory

	// The keys hold the primary fixture (balance v4 sending off, account v2
	// blocked); the seeded rows moved past them: sending on at v5, unblocked at v3.
	keyBalance, keyAccount := f.primaryConfigs(t)
	f.loaded.ConfigVersion, f.loaded.AccountConfigVersion, f.loaded.Blocked = 5, 3, false

	rowBalance, err := balancecache.BalanceConfigFromBalance(f.loaded)
	require.NoError(t, err)

	rowAccount := accounting.AccountConfig{AccountID: f.accountID, Version: 3, Blocked: false}

	f.redis.EXPECT().ReadConfigKeys(gomock.Any(), f.orgID, f.ledgerID, []string{f.ref}, []uuid.UUID{f.accountID}).
		Return(redis.ConfigKeyValues{
			Balances: map[string]accounting.BalanceConfig{f.ref: keyBalance},
			Accounts: map[uuid.UUID]accounting.AccountConfig{f.accountID: keyAccount},
		}, nil)
	f.redis.EXPECT().WriteBalanceConfig(gomock.Any(), f.orgID, f.ledgerID, f.ref, rowBalance).
		Return(rowBalance, redis.ConfigWritten, nil)
	f.redis.EXPECT().WriteAccountConfig(gomock.Any(), f.orgID, f.ledgerID, rowAccount).
		Return(rowAccount, redis.ConfigWritten, nil)

	logs := &recordingLogger{}
	ctx := libObservability.ContextWithLogger(t.Context(), logs)

	require.NoError(t, f.uc.resolveEngineConfiguration(ctx, f.orgID, f.ledgerID, []*mmodel.Balance{f.loaded}, map[string]struct{}{f.loaded.ID: {}}))

	assert.Equal(t, int64(5), f.loaded.ConfigVersion)
	assert.True(t, f.loaded.AllowSending)
	assert.Nil(t, f.loaded.Settings, "the row's default settings, not the key's overdraft")
	assert.False(t, f.loaded.Blocked)
	assert.Equal(t, int64(3), f.loaded.AccountConfigVersion)

	assert.Empty(t, counterByLabel(t, reader, utils.ConfigCopyDivergent.Name, "family"), "a newer row is not a divergent copy")

	older := logs.entriesWith("Configuration key is older than the row the balance was seeded from, deciding with the row")
	require.Len(t, older, 2, "one report per family")
	assert.ElementsMatch(t, []any{configFamilyBalance, configFamilyAccount}, []any{older[0].fields["family"], older[1].fields["family"]})
}

// counterByLabel sums one counter's data points by the value of one label.
func counterByLabel(t *testing.T, reader *sdkmetric.ManualReader, name, label string) map[string]int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	totals := map[string]int64{}

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "%s must be a Sum[int64], got %T", name, m.Data)

			for _, dp := range sum.DataPoints {
				value, _ := dp.Attributes.Value(attribute.Key(label))
				totals[value.AsString()] += dp.Value
			}
		}
	}

	return totals
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
