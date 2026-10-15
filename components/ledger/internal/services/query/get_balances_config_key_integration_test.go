//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// Before every engine execution the balances carry the configuration the engine
// decides with, and its versions. The configuration keys are the source while they
// live; when one is missing, the row is read from the PostgreSQL primary and written
// back under the version rule. These tests run that load against real PostgreSQL
// and Valkey.

// spyBalanceRepo counts the balance reads the configuration load can make, and
// can run an action right after the primary configuration read returns.
type spyBalanceRepo struct {
	balance.Repository

	primaryReads     atomic.Int32
	routedReads      atomic.Int32
	afterPrimaryRead func()
}

func (r *spyBalanceRepo) ListByAliasesWithKeysOnPrimary(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	r.primaryReads.Add(1)

	rows, err := r.Repository.ListByAliasesWithKeysOnPrimary(ctx, organizationID, ledgerID, aliases)

	if r.afterPrimaryRead != nil {
		r.afterPrimaryRead()
	}

	return rows, err
}

func (r *spyBalanceRepo) ListByAliasesWithKeys(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	r.routedReads.Add(1)

	return r.Repository.ListByAliasesWithKeys(ctx, organizationID, ledgerID, aliases)
}

// spyAccountRepo counts the account reads that resolve the blocked flag.
type spyAccountRepo struct {
	account.Repository

	listByIDs atomic.Int32
}

func (r *spyAccountRepo) ListAccountsByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error) {
	r.listByIDs.Add(1)

	return r.Repository.ListAccountsByIDs(ctx, organizationID, ledgerID, ids)
}

func (i *reseedTestInfra) spyOnRepositories() (*spyBalanceRepo, *spyAccountRepo) {
	balances := &spyBalanceRepo{Repository: i.uc.BalanceRepo}
	accounts := &spyAccountRepo{Repository: i.uc.AccountRepo}

	i.uc.BalanceRepo = balances
	i.uc.AccountRepo = accounts

	return balances, accounts
}

// addBalance creates one more balance of an existing account.
func (i *reseedTestInfra) addBalance(t *testing.T, accountID uuid.UUID, alias, key string) uuid.UUID {
	t.Helper()

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = alias
	params.Key = key
	params.Available = decimal.NewFromInt(100)

	return pgtestutil.CreateTestBalance(t, i.transactionDB.DB, i.orgID, i.ledgerID, accountID, params)
}

func (i *reseedTestInfra) execBalance(t *testing.T, statement string, args ...any) {
	t.Helper()

	_, err := i.transactionDB.DB.Exec(statement, args...)
	require.NoError(t, err)
}

// cacheBlob writes a live balance blob whose configuration copy is the defaults
// (sending, receiving, no overdraft), so any other configuration the load returns
// cannot have come from the blob.
func (i *reseedTestInfra) cacheBlob(t *testing.T, accountID, balanceID uuid.UUID, alias, key string, version int64) {
	t.Helper()
	i.cacheBlobWithCopyVersion(t, accountID, balanceID, alias, key, version, 0)
}

// cacheBlobWithCopyVersion is cacheBlob with the balance configuration copy at
// copyVersion.
func (i *reseedTestInfra) cacheBlobWithCopyVersion(t *testing.T, accountID, balanceID uuid.UUID, alias, key string, version, copyVersion int64) {
	t.Helper()

	snapshot := accounting.BalanceSnapshot{
		BalanceRef: alias + "#" + key, ID: balanceID, AccountID: accountID,
		AccountType: "deposit", AssetCode: "USD", Alias: alias, Key: key,
		Direction: "credit", BalanceScope: mmodel.BalanceScopeTransactional,
		Available: decimal.NewFromInt(100), Version: version,
		AllowSending: true, AllowReceiving: true, ConfigVersion: copyVersion,
	}

	encoded, err := balancecache.Encode(snapshot, balancecache.FormatDual)
	require.NoError(t, err)

	require.NoError(t, i.redis.Client.Set(context.Background(),
		utils.BalanceInternalKey(i.orgID, i.ledgerID, snapshot.BalanceRef), encoded, time.Hour).Err())
}

func (i *reseedTestInfra) readKeys(t *testing.T, ctx context.Context, refs []string, accountIDs []uuid.UUID) redis.ConfigKeyValues {
	t.Helper()

	values, err := i.uc.TransactionRedisRepo.ReadConfigKeys(ctx, i.orgID, i.ledgerID, refs, accountIDs)
	require.NoError(t, err)

	return values
}

func (i *reseedTestInfra) primaryConfig(t *testing.T, ref string) accounting.BalanceConfig {
	t.Helper()

	// Read past a spy so a hook that runs inside the spied read can call this too.
	repo := i.uc.BalanceRepo
	if spy, ok := repo.(*spyBalanceRepo); ok {
		repo = spy.Repository
	}

	rows, err := repo.ListByAliasesWithKeysOnPrimary(context.Background(), i.orgID, i.ledgerID, []string{ref})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	config, err := balancecache.BalanceConfigFromBalance(rows[0])
	require.NoError(t, err)

	return config
}

func engineBalanceByRef(t *testing.T, balances []*mmodel.Balance, ref string) *mmodel.Balance {
	t.Helper()

	for _, b := range balances {
		if got, err := balanceReference(b); err == nil && got == ref {
			return b
		}
	}

	t.Fatalf("balance %q is not in the engine pool", ref)

	return nil
}

func TestIntegration_EngineConfig_AbsentKeyIsSeededFromThePrimaryRow(t *testing.T) {
	infra := setupReseedTestInfra(t)
	ctx := context.Background()

	accountID, balanceID, alias := infra.seedBalance(t, "cfg-absent", decimal.NewFromInt(100), 1)
	ref := alias + "#" + constant.DefaultBalanceKey

	// Three real configuration changes: the row is at configuration version 3 and
	// differs from the blob's copy in every family field it touches.
	infra.execBalance(t, `UPDATE balance SET allow_receiving = false WHERE id = $1`, balanceID)
	infra.execBalance(t, `UPDATE balance SET settings = '{"balanceScope":"transactional","allowOverdraft":true,"overdraftLimitEnabled":true,"overdraftLimit":"50"}' WHERE id = $1`, balanceID)
	infra.execBalance(t, `UPDATE balance SET allow_sending = false WHERE id = $1`, balanceID)
	infra.cacheBlob(t, accountID, balanceID, alias, constant.DefaultBalanceKey, 1)

	balances, _ := infra.spyOnRepositories()

	explicit, execution, err := infra.uc.GetEngineBalances(ctx, infra.orgID, infra.ledgerID, []string{ref})
	require.NoError(t, err)
	require.Len(t, explicit, 1)

	got := engineBalanceByRef(t, execution, ref)
	assert.Same(t, explicit[0], got, "the explicit balance is the pooled one")

	assert.Equal(t, int64(3), got.ConfigVersion)
	assert.False(t, got.AllowSending)
	assert.False(t, got.AllowReceiving)
	require.NotNil(t, got.Settings)
	assert.True(t, got.Settings.AllowOverdraft)
	assert.True(t, got.Settings.OverdraftLimitEnabled)
	require.NotNil(t, got.Settings.OverdraftLimit)
	assert.True(t, decimal.NewFromInt(50).Equal(decimal.RequireFromString(*got.Settings.OverdraftLimit)))
	assert.True(t, decimal.NewFromInt(100).Equal(got.Available), "money still comes from the blob")
	assert.Equal(t, int64(1), got.Version)
	assert.False(t, got.Blocked)
	assert.Equal(t, int64(0), got.AccountConfigVersion)

	assert.Equal(t, int32(1), balances.primaryReads.Load(), "one batched primary read reloads the missing configuration")

	keys := infra.readKeys(t, ctx, []string{ref}, []uuid.UUID{accountID})
	stored, found := keys.Balances[ref]
	require.True(t, found, "the seed writes the balance key")
	assert.True(t, stored.Equal(infra.primaryConfig(t, ref)), "the key holds the primary row: %+v", stored)

	storedAccount, found := keys.Accounts[accountID]
	require.True(t, found, "the seed writes the account key")
	assert.Equal(t, accounting.AccountConfig{AccountID: accountID, Version: 0, Blocked: false}, storedAccount)

	ttl, err := infra.redis.Client.PTTL(ctx, utils.BalanceConfigKey(infra.orgID, infra.ledgerID, ref)).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, 59*time.Minute, "a seeded key lives one hour from the write")
}

func TestIntegration_EngineConfig_LiveKeysAreUsedWithoutReadingTheDatabase(t *testing.T) {
	infra := setupReseedTestInfra(t)
	ctx := context.Background()

	accountID, balanceID, alias := infra.seedBalance(t, "cfg-hot", decimal.NewFromInt(100), 4)
	companionID := infra.addBalance(t, accountID, alias, constant.OverdraftBalanceKey)
	ref := alias + "#" + constant.DefaultBalanceKey
	companionRef := alias + "#" + constant.OverdraftBalanceKey

	// Everything the load needs is cached: both blobs, both balance keys and the
	// account key. The keys disagree with the blobs and the rows on purpose.
	infra.cacheBlob(t, accountID, balanceID, alias, constant.DefaultBalanceKey, 4)
	infra.cacheBlob(t, accountID, companionID, alias, constant.OverdraftBalanceKey, 0)

	keyConfig := accounting.BalanceConfig{
		BalanceID: balanceID, Version: 5, AllowSending: false, AllowReceiving: true,
		AllowOverdraft: true, OverdraftLimitEnabled: true, OverdraftLimit: decimal.NewFromInt(10),
		BalanceScope: mmodel.BalanceScopeTransactional,
	}
	companionConfig := accounting.BalanceConfig{
		BalanceID: companionID, Version: 2, AllowSending: true, AllowReceiving: false,
		OverdraftLimit: decimal.Zero, BalanceScope: mmodel.BalanceScopeInternal,
	}

	for balanceRef, config := range map[string]accounting.BalanceConfig{ref: keyConfig, companionRef: companionConfig} {
		_, _, err := infra.uc.TransactionRedisRepo.WriteBalanceConfig(ctx, infra.orgID, infra.ledgerID, balanceRef, config)
		require.NoError(t, err)
	}

	_, _, err := infra.uc.TransactionRedisRepo.WriteAccountConfig(ctx, infra.orgID, infra.ledgerID,
		accounting.AccountConfig{AccountID: accountID, Version: 2, Blocked: true})
	require.NoError(t, err)

	balances, accounts := infra.spyOnRepositories()

	_, execution, err := infra.uc.GetEngineBalances(ctx, infra.orgID, infra.ledgerID, []string{ref})
	require.NoError(t, err)
	require.Len(t, execution, 2)

	assert.Zero(t, balances.primaryReads.Load(), "a live key needs no configuration read")
	assert.Zero(t, balances.routedReads.Load(), "a fully cached pool reads no balance row")
	assert.Zero(t, accounts.listByIDs.Load(), "a live account key needs no account read")

	got := engineBalanceByRef(t, execution, ref)
	assert.Equal(t, int64(5), got.ConfigVersion)
	assert.False(t, got.AllowSending)
	assert.True(t, got.AllowReceiving)
	require.NotNil(t, got.Settings)
	assert.True(t, got.Settings.AllowOverdraft)
	require.NotNil(t, got.Settings.OverdraftLimit)
	assert.True(t, decimal.NewFromInt(10).Equal(decimal.RequireFromString(*got.Settings.OverdraftLimit)))
	assert.True(t, got.Blocked)
	assert.Equal(t, int64(2), got.AccountConfigVersion)

	companion := engineBalanceByRef(t, execution, companionRef)
	assert.Equal(t, int64(2), companion.ConfigVersion)
	assert.False(t, companion.AllowReceiving)
	require.NotNil(t, companion.Settings)
	assert.Equal(t, mmodel.BalanceScopeInternal, companion.Settings.BalanceScope)
	assert.True(t, companion.Blocked, "the companion takes the blocked flag of its account")
	assert.Equal(t, int64(2), companion.AccountConfigVersion)
}

// A seed reads the primary at v4, then a configuration update commits v5 and
// writes its key before the seed writes. The seed must not move the key back, and
// the engine request must carry what the key holds afterwards.
func TestIntegration_EngineConfig_StaleSeedNeverOverwritesANewerKey(t *testing.T) {
	infra := setupReseedTestInfra(t)
	ctx := context.Background()

	accountID, balanceID, alias := infra.seedBalance(t, "cfg-stale-seed", decimal.NewFromInt(100), 1)
	ref := alias + "#" + constant.DefaultBalanceKey

	for _, allow := range []bool{false, true, false, true} {
		infra.execBalance(t, `UPDATE balance SET allow_receiving = $1 WHERE id = $2`, allow, balanceID)
	}

	infra.cacheBlob(t, accountID, balanceID, alias, constant.DefaultBalanceKey, 1)

	balances, _ := infra.spyOnRepositories()

	var once sync.Once

	balances.afterPrimaryRead = func() {
		once.Do(func() {
			infra.execBalance(t, `UPDATE balance SET allow_sending = false WHERE id = $1`, balanceID)

			newer := infra.primaryConfig(t, ref)
			require.Equal(t, int64(5), newer.Version)

			_, outcome, err := infra.uc.TransactionRedisRepo.WriteBalanceConfig(ctx, infra.orgID, infra.ledgerID, ref, newer)
			require.NoError(t, err)
			require.Equal(t, redis.ConfigWritten, outcome)
		})
	}

	_, execution, err := infra.uc.GetEngineBalances(ctx, infra.orgID, infra.ledgerID, []string{ref})
	require.NoError(t, err)

	got := engineBalanceByRef(t, execution, ref)
	assert.Equal(t, int64(5), got.ConfigVersion, "the request carries the newer configuration the key kept")
	assert.False(t, got.AllowSending)

	stored := infra.readKeys(t, ctx, []string{ref}, nil).Balances[ref]
	assert.Equal(t, int64(5), stored.Version, "the seed of v4 never overwrites v5")
	assert.False(t, stored.AllowSending)
}

func TestIntegration_EngineConfig_BlockedAccountIsSeededForEveryBalance(t *testing.T) {
	infra := setupReseedTestInfra(t)
	ctx := context.Background()

	accountID, balanceID, alias := infra.seedBalance(t, "cfg-blocked", decimal.NewFromInt(100), 1)
	savingsID := infra.addBalance(t, accountID, alias, "savings")
	ref := alias + "#" + constant.DefaultBalanceKey
	savingsRef := alias + "#savings"

	// One balance is cached, the other is not: both must end up blocked.
	infra.cacheBlob(t, accountID, balanceID, alias, constant.DefaultBalanceKey, 1)

	_, err := infra.onboardingDB.DB.Exec(`UPDATE account SET blocked = true WHERE id = $1`, accountID)
	require.NoError(t, err)

	_, execution, err := infra.uc.GetEngineBalances(ctx, infra.orgID, infra.ledgerID, []string{ref, savingsRef})
	require.NoError(t, err)
	require.Len(t, execution, 2)

	for _, b := range execution {
		assert.True(t, b.Blocked, "balance %s must be blocked", b.ID)
		assert.Equal(t, int64(1), b.AccountConfigVersion, "balance %s", b.ID)
	}

	keys := infra.readKeys(t, ctx, []string{ref, savingsRef}, []uuid.UUID{accountID})
	assert.Equal(t, accounting.AccountConfig{AccountID: accountID, Version: 1, Blocked: true}, keys.Accounts[accountID])
	assert.True(t, keys.Balances[ref].Describes(balanceID))
	assert.True(t, keys.Balances[savingsRef].Describes(savingsID), "the cold balance gets its key too")
}

// A balance deleted and recreated under the same alias#key leaves a key that
// describes the deleted balance. The seed must treat it as absent and replace it.
func TestIntegration_EngineConfig_KeyOfADeletedBalanceIsReplaced(t *testing.T) {
	infra := setupReseedTestInfra(t)
	ctx := context.Background()

	accountID, deletedID, alias := infra.seedBalance(t, "cfg-recreated", decimal.NewFromInt(100), 1)
	ref := alias + "#" + constant.DefaultBalanceKey

	for _, allow := range []bool{false, true, false} {
		infra.execBalance(t, `UPDATE balance SET allow_sending = $1 WHERE id = $2`, allow, deletedID)
	}

	deleted := infra.primaryConfig(t, ref)
	require.Equal(t, int64(3), deleted.Version)

	_, _, err := infra.uc.TransactionRedisRepo.WriteBalanceConfig(ctx, infra.orgID, infra.ledgerID, ref, deleted)
	require.NoError(t, err)

	infra.execBalance(t, `UPDATE balance SET deleted_at = now() WHERE id = $1`, deletedID)
	recreatedID := infra.addBalance(t, accountID, alias, constant.DefaultBalanceKey)

	_, execution, err := infra.uc.GetEngineBalances(ctx, infra.orgID, infra.ledgerID, []string{ref})
	require.NoError(t, err)

	got := engineBalanceByRef(t, execution, ref)
	require.Equal(t, recreatedID.String(), got.ID)
	assert.True(t, got.AllowSending, "the deleted balance's configuration never reaches the new one")
	assert.Equal(t, int64(0), got.ConfigVersion)

	stored := infra.readKeys(t, ctx, []string{ref}, nil).Balances[ref]
	assert.True(t, stored.Describes(recreatedID), "the key now describes the recreated balance")
	assert.Equal(t, int64(0), stored.Version)
	assert.True(t, stored.AllowSending)
}

func TestIntegration_EngineConfig_TenantsReadAndWriteOnlyTheirOwnKeys(t *testing.T) {
	infra := setupReseedTestInfra(t)

	_, balanceID, alias := infra.seedBalance(t, "cfg-tenants", decimal.NewFromInt(100), 1)
	ref := alias + "#" + constant.DefaultBalanceKey

	first := tmcore.ContextWithTenantID(t.Context(), "tenant-one")
	second := tmcore.ContextWithTenantID(t.Context(), "tenant-two")

	firstConfig := infra.primaryConfig(t, ref)
	firstConfig.Version = 7
	firstConfig.AllowSending = false

	_, _, err := infra.uc.TransactionRedisRepo.WriteBalanceConfig(first, infra.orgID, infra.ledgerID, ref, firstConfig)
	require.NoError(t, err)

	_, execution, err := infra.uc.GetEngineBalances(second, infra.orgID, infra.ledgerID, []string{ref})
	require.NoError(t, err)

	got := engineBalanceByRef(t, execution, ref)
	assert.True(t, got.AllowSending, "another tenant's key is not this tenant's configuration")
	assert.Equal(t, int64(0), got.ConfigVersion)

	secondStored := infra.readKeys(t, second, []string{ref}, nil).Balances[ref]
	assert.True(t, secondStored.Describes(balanceID))
	assert.Equal(t, int64(0), secondStored.Version, "the seed wrote the second tenant's own key")

	firstStored := infra.readKeys(t, first, []string{ref}, nil).Balances[ref]
	assert.True(t, firstStored.Equal(firstConfig), "the first tenant's key is untouched")

	exists, err := infra.redis.Client.Exists(t.Context(), utils.BalanceConfigKey(infra.orgID, infra.ledgerID, ref)).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "a tenant load never writes the unprefixed key")

	_, execution, err = infra.uc.GetEngineBalances(first, infra.orgID, infra.ledgerID, []string{ref})
	require.NoError(t, err)

	got = engineBalanceByRef(t, execution, ref)
	assert.False(t, got.AllowSending)
	assert.Equal(t, int64(7), got.ConfigVersion)
}

// The reload must read the primary even with read routing off and no primary
// intent on the request. Two independent databases stand in for primary and
// replica; the balance row exists only on the primary.
func TestIntegration_EngineConfig_ReloadReadsThePrimary(t *testing.T) {
	primary := pgtestutil.SetupMigratedContainer(t, "transaction")
	replica := pgtestutil.SetupMigratedContainer(t, "transaction")

	conn, err := libPostgres.New(libPostgres.Config{
		PrimaryDSN: pgtestutil.BuildConnectionString(primary.Host, primary.Port, primary.Config),
		ReplicaDSN: pgtestutil.BuildConnectionString(replica.Host, replica.Port, replica.Config),
	})
	require.NoError(t, err)
	require.NoError(t, conn.Connect(context.Background()))

	t.Cleanup(func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Logf("failed to close postgres client: %v", closeErr)
		}
	})

	onboarding := pgtestutil.SetupMigratedContainer(t, "onboarding")
	onboardingDSN := pgtestutil.BuildConnectionString(onboarding.Host, onboarding.Port, onboarding.Config)
	onboardingConn := pgtestutil.ConnectPostgresClient(t.Context(), t, onboardingDSN, onboardingDSN)

	redisContainer := redistestutil.SetupReusableContainer(t)
	redisRepo, err := redis.NewConsumerRedis(redistestutil.CreateConnectionWithDB(t, redisContainer.Addr, redisContainer.DB))
	require.NoError(t, err)

	orgID := pgtestutil.CreateTestOrganization(t, onboarding.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, onboarding.DB, orgID)
	alias := "@cfg-primary"
	accountID := pgtestutil.CreateTestAccount(t, onboarding.DB, orgID, ledgerID, nil, "cfg-primary", alias, "USD", nil)

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = alias
	balanceID := pgtestutil.CreateTestBalance(t, primary.DB, orgID, ledgerID, accountID, params)

	_, err = primary.DB.Exec(`UPDATE balance SET allow_receiving = false WHERE id = $1`, balanceID)
	require.NoError(t, err)

	infra := &reseedTestInfra{
		uc: &UseCase{
			BalanceRepo:          balance.NewBalancePostgreSQLRepository(conn, false),
			OperationRepo:        operation.NewOperationPostgreSQLRepository(conn),
			AccountRepo:          account.NewAccountPostgreSQLRepository(onboardingConn),
			TransactionRedisRepo: redisRepo,
		},
		redis:    redisContainer,
		orgID:    orgID,
		ledgerID: ledgerID,
	}

	ref := alias + "#" + constant.DefaultBalanceKey
	infra.cacheBlob(t, accountID, balanceID, alias, constant.DefaultBalanceKey, 0)

	_, execution, err := infra.uc.GetEngineBalances(context.Background(), orgID, ledgerID, []string{ref})
	require.NoError(t, err)

	got := engineBalanceByRef(t, execution, ref)
	assert.False(t, got.AllowReceiving, "the configuration comes from the primary row")
	assert.Equal(t, int64(1), got.ConfigVersion)

	stored, found := infra.readKeys(t, context.Background(), []string{ref}, nil).Balances[ref]
	require.True(t, found, "the key is seeded from the primary, which the replica could not provide")
	assert.Equal(t, int64(1), stored.Version)
	assert.False(t, stored.AllowReceiving)
}

// A configuration change committed without its key — a PATCH an earlier release
// served writes PostgreSQL and no configuration key — leaves a live key older than
// the row. A balance loaded from its row, because it was not cached, already holds
// that row and its account row, so the newer version decides and is written to the
// key, with no further read. An equal version keeps the key, and a cached balance
// keeps deciding with its key without reading its configuration rows.
func TestIntegration_EngineConfig_ColdBalanceDecidesWithARowNewerThanTheLiveKey(t *testing.T) {
	infra := setupReseedTestInfra(t)

	spy := func(t *testing.T) (*spyBalanceRepo, *spyAccountRepo) {
		t.Helper()

		balanceRepo, accountRepo := infra.uc.BalanceRepo, infra.uc.AccountRepo
		t.Cleanup(func() { infra.uc.BalanceRepo, infra.uc.AccountRepo = balanceRepo, accountRepo })

		return infra.spyOnRepositories()
	}

	// writeKeys stores the keys a load wrote before the configuration changed: the
	// balance's row at version 0 (sending allowed) and its account unblocked at 0,
	// with the given content changes applied.
	writeKeys := func(t *testing.T, ctx context.Context, ref string, accountID uuid.UUID, allowSending, blocked bool) {
		t.Helper()

		config := infra.primaryConfig(t, ref)
		require.Equal(t, int64(0), config.Version)
		require.True(t, config.AllowSending)

		config.AllowSending = allowSending

		_, outcome, err := infra.uc.TransactionRedisRepo.WriteBalanceConfig(ctx, infra.orgID, infra.ledgerID, ref, config)
		require.NoError(t, err)
		require.Equal(t, redis.ConfigWritten, outcome)

		_, outcome, err = infra.uc.TransactionRedisRepo.WriteAccountConfig(ctx, infra.orgID, infra.ledgerID,
			accounting.AccountConfig{AccountID: accountID, Version: 0, Blocked: blocked})
		require.NoError(t, err)
		require.Equal(t, redis.ConfigWritten, outcome)
	}

	changeRows := func(t *testing.T, accountID, balanceID uuid.UUID) {
		t.Helper()

		infra.execBalance(t, `UPDATE balance SET allow_sending = false WHERE id = $1`, balanceID)

		_, err := infra.onboardingDB.DB.Exec(`UPDATE account SET blocked = true WHERE id = $1`, accountID)
		require.NoError(t, err)
	}

	t.Run("a newer row of an uncached balance decides and moves the keys", func(t *testing.T) {
		ctx := context.Background()
		accountID, balanceID, alias := infra.seedBalance(t, "cfg-cold-newer", decimal.NewFromInt(100), 0)
		ref := alias + "#" + constant.DefaultBalanceKey

		writeKeys(t, ctx, ref, accountID, true, false)
		changeRows(t, accountID, balanceID)

		balances, accounts := spy(t)

		_, execution, err := infra.uc.GetEngineBalances(ctx, infra.orgID, infra.ledgerID, []string{ref})
		require.NoError(t, err)

		got := engineBalanceByRef(t, execution, ref)
		assert.Equal(t, int64(1), got.ConfigVersion, "the request carries the row's version")
		assert.False(t, got.AllowSending, "the request carries the row's configuration")
		assert.True(t, got.Blocked, "the request carries the account row's blocked flag")
		assert.Equal(t, int64(1), got.AccountConfigVersion)

		assert.Zero(t, balances.primaryReads.Load(), "the row the seed read decides; no reload")
		assert.Equal(t, int32(1), accounts.listByIDs.Load(), "only the seed's own account read")

		keys := infra.readKeys(t, ctx, []string{ref}, []uuid.UUID{accountID})
		assert.True(t, keys.Balances[ref].Equal(infra.primaryConfig(t, ref)), "the balance key takes the row: %+v", keys.Balances[ref])
		assert.Equal(t, accounting.AccountConfig{AccountID: accountID, Version: 1, Blocked: true}, keys.Accounts[accountID])
	})

	t.Run("an uncached balance without an account key reads its account once", func(t *testing.T) {
		ctx := context.Background()
		accountID, balanceID, alias := infra.seedBalance(t, "cfg-cold-no-account-key", decimal.NewFromInt(100), 0)
		ref := alias + "#" + constant.DefaultBalanceKey

		changeRows(t, accountID, balanceID)

		_, accounts := spy(t)

		_, execution, err := infra.uc.GetEngineBalances(ctx, infra.orgID, infra.ledgerID, []string{ref})
		require.NoError(t, err)

		got := engineBalanceByRef(t, execution, ref)
		assert.True(t, got.Blocked)
		assert.Equal(t, int64(1), got.AccountConfigVersion)
		assert.Equal(t, int32(1), accounts.listByIDs.Load(), "the seed's account read fills the absent account key")

		keys := infra.readKeys(t, ctx, []string{ref}, []uuid.UUID{accountID})
		assert.Equal(t, accounting.AccountConfig{AccountID: accountID, Version: 1, Blocked: true}, keys.Accounts[accountID])
	})

	t.Run("uncached balances of different accounts each take their own account row", func(t *testing.T) {
		ctx := context.Background()
		openAccountID, _, openAlias := infra.seedBalance(t, "cfg-cold-open-account", decimal.NewFromInt(100), 0)
		blockedAccountID, _, blockedAlias := infra.seedBalance(t, "cfg-cold-blocked-account", decimal.NewFromInt(100), 0)
		openRef := openAlias + "#" + constant.DefaultBalanceKey
		blockedRef := blockedAlias + "#" + constant.DefaultBalanceKey

		// One account stays open at version 0, the other is blocked at version 1, and
		// neither has an account key: each balance can only take its account's state
		// from the row the load read for that account.
		_, err := infra.onboardingDB.DB.Exec(`UPDATE account SET blocked = true WHERE id = $1`, blockedAccountID)
		require.NoError(t, err)

		_, accounts := spy(t)

		_, execution, err := infra.uc.GetEngineBalances(ctx, infra.orgID, infra.ledgerID, []string{openRef, blockedRef})
		require.NoError(t, err)

		open := engineBalanceByRef(t, execution, openRef)
		assert.False(t, open.Blocked, "the open account's balance stays open")
		assert.Equal(t, int64(0), open.AccountConfigVersion)

		blocked := engineBalanceByRef(t, execution, blockedRef)
		assert.True(t, blocked.Blocked, "the blocked account's balance is blocked")
		assert.Equal(t, int64(1), blocked.AccountConfigVersion)

		assert.Equal(t, int32(1), accounts.listByIDs.Load(), "one account read serves both balances")

		keys := infra.readKeys(t, ctx, nil, []uuid.UUID{openAccountID, blockedAccountID})
		assert.Equal(t, accounting.AccountConfig{AccountID: openAccountID, Version: 0, Blocked: false}, keys.Accounts[openAccountID])
		assert.Equal(t, accounting.AccountConfig{AccountID: blockedAccountID, Version: 1, Blocked: true}, keys.Accounts[blockedAccountID])
	})

	t.Run("an equal version keeps the key", func(t *testing.T) {
		ctx := context.Background()
		accountID, _, alias := infra.seedBalance(t, "cfg-cold-equal", decimal.NewFromInt(100), 0)
		ref := alias + "#" + constant.DefaultBalanceKey

		// Same version 0 as the rows, other content: the key wins the tie.
		writeKeys(t, ctx, ref, accountID, false, true)

		_, execution, err := infra.uc.GetEngineBalances(ctx, infra.orgID, infra.ledgerID, []string{ref})
		require.NoError(t, err)

		got := engineBalanceByRef(t, execution, ref)
		assert.Equal(t, int64(0), got.ConfigVersion)
		assert.False(t, got.AllowSending, "the key decides an equal version")
		assert.True(t, got.Blocked, "the account key decides an equal version")

		keys := infra.readKeys(t, ctx, []string{ref}, []uuid.UUID{accountID})
		assert.False(t, keys.Balances[ref].AllowSending, "the key is left as it was")
		assert.Equal(t, accounting.AccountConfig{AccountID: accountID, Version: 0, Blocked: true}, keys.Accounts[accountID])
	})

	t.Run("a cached balance decides with its key and reads no configuration row", func(t *testing.T) {
		ctx := context.Background()
		accountID, balanceID, alias := infra.seedBalance(t, "cfg-hot-older-key", decimal.NewFromInt(100), 1)
		ref := alias + "#" + constant.DefaultBalanceKey

		writeKeys(t, ctx, ref, accountID, true, false)
		changeRows(t, accountID, balanceID)
		// The blob's copy is newer than the key: it is left to the engine's decision
		// and never taken for a row the load read.
		infra.cacheBlobWithCopyVersion(t, accountID, balanceID, alias, constant.DefaultBalanceKey, 1, 1)
		before := infra.readKeys(t, ctx, []string{ref}, []uuid.UUID{accountID})

		balances, accounts := spy(t)

		_, execution, err := infra.uc.GetEngineBalances(ctx, infra.orgID, infra.ledgerID, []string{ref})
		require.NoError(t, err)

		got := engineBalanceByRef(t, execution, ref)
		assert.Equal(t, int64(0), got.ConfigVersion, "a live key decides a cached balance")
		assert.True(t, got.AllowSending)
		assert.False(t, got.Blocked)
		assert.Equal(t, before, infra.readKeys(t, ctx, []string{ref}, []uuid.UUID{accountID}), "a cached balance never rewrites its keys")

		// The uncached overdraft companion is still looked up by alias, as before; no
		// configuration row is read.
		assert.Zero(t, balances.primaryReads.Load(), "a cached balance with live keys reloads no configuration")
		assert.Zero(t, accounts.listByIDs.Load(), "a cached balance with a live account key reads no account row")
	})
}
