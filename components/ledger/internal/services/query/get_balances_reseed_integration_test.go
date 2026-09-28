//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// This is the G9 incident in miniature, with no live stack: the balance row lags
// behind the operation trail (the sync worker had not flushed when the cached
// balance was lost) and the cache misses, so the seed comes from PostgreSQL. Before
// the guard, that seed restarted the balance from the stale row and every later
// mutation forked off it.

// reseedTestInfra holds the real dependencies the seed path touches: the transaction
// database (balance + operation), the onboarding database (account) and Redis.
type reseedTestInfra struct {
	uc            *UseCase
	transactionDB *pgtestutil.ContainerResult
	onboardingDB  *pgtestutil.ContainerResult
	redis         *redistestutil.ContainerResult
	orgID         uuid.UUID
	ledgerID      uuid.UUID
	transactionID uuid.UUID
}

func setupReseedTestInfra(t *testing.T) *reseedTestInfra {
	t.Helper()

	transactionContainer := pgtestutil.SetupMigratedContainer(t, "transaction")
	transactionDSN := pgtestutil.BuildConnectionString(transactionContainer.Host, transactionContainer.Port, transactionContainer.Config)
	transactionConn := pgtestutil.ConnectPostgresClient(t.Context(), t, transactionDSN, transactionDSN)

	onboardingContainer := pgtestutil.SetupMigratedContainer(t, "onboarding")
	onboardingDSN := pgtestutil.BuildConnectionString(onboardingContainer.Host, onboardingContainer.Port, onboardingContainer.Config)
	onboardingConn := pgtestutil.ConnectPostgresClient(t.Context(), t, onboardingDSN, onboardingDSN)

	redisContainer := redistestutil.SetupReusableContainer(t)
	redisConn := redistestutil.CreateConnectionWithDB(t, redisContainer.Addr, redisContainer.DB)

	redisRepo, err := redis.NewConsumerRedis(redisConn)
	require.NoError(t, err, "failed to create Redis repository")

	orgID := pgtestutil.CreateTestOrganization(t, onboardingContainer.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, onboardingContainer.DB, orgID)

	return &reseedTestInfra{
		uc: &UseCase{
			BalanceRepo:          balance.NewBalancePostgreSQLRepository(transactionConn, false),
			OperationRepo:        operation.NewOperationPostgreSQLRepository(transactionConn),
			AccountRepo:          account.NewAccountPostgreSQLRepository(onboardingConn),
			TransactionRedisRepo: redisRepo,
		},
		transactionDB: transactionContainer,
		onboardingDB:  onboardingContainer,
		redis:         redisContainer,
		orgID:         orgID,
		ledgerID:      ledgerID,
		transactionID: pgtestutil.CreateTestTransactionWithStatus(t, transactionContainer.DB, orgID, ledgerID, "APPROVED", decimal.NewFromInt(50), "USD"),
	}
}

// seedBalance creates an account with one balance whose stored state is `available`
// at `version`, and returns both IDs plus the alias the seed looks up.
func (i *reseedTestInfra) seedBalance(t *testing.T, name string, available decimal.Decimal, version int64) (accountID, balanceID uuid.UUID, alias string) {
	t.Helper()

	alias = "@" + name

	accountID = pgtestutil.CreateTestAccount(t, i.onboardingDB.DB, i.orgID, i.ledgerID, nil, name, alias, "USD", nil)

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = alias
	params.Available = available
	params.AssetCode = "USD"

	balanceID = pgtestutil.CreateTestBalance(t, i.transactionDB.DB, i.orgID, i.ledgerID, accountID, params)

	// CreateTestBalance always writes version 0; the row's real version is what makes
	// it stale or current, so it is set explicitly.
	_, err := i.transactionDB.DB.Exec(`UPDATE balance SET version = $1 WHERE id = $2`, version, balanceID)
	require.NoError(t, err)

	return accountID, balanceID, alias
}

// recordOperation appends one balance-affecting operation to a balance's trail.
func (i *reseedTestInfra) recordOperation(t *testing.T, accountID, balanceID uuid.UUID, alias string, availableAfter, onHoldAfter decimal.Decimal, versionAfter int64, overdraftAfter string) {
	t.Helper()

	amount := decimal.NewFromInt(50)
	availableBefore := decimal.Zero
	onHoldBefore := decimal.Zero
	versionBefore := versionAfter - 1
	createdAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(versionAfter) * time.Minute)

	_, err := i.uc.OperationRepo.Create(context.Background(), &operation.Operation{
		ID:              uuid.Must(libCommons.GenerateUUIDv7()).String(),
		TransactionID:   i.transactionID.String(),
		Description:     "Re-seed guard fixture",
		Type:            "CREDIT",
		AssetCode:       "USD",
		ChartOfAccounts: "1000",
		Amount:          operation.Amount{Value: &amount},
		Balance: operation.Balance{
			Available: &availableBefore,
			OnHold:    &onHoldBefore,
			Version:   &versionBefore,
		},
		BalanceAfter: operation.Balance{
			Available: &availableAfter,
			OnHold:    &onHoldAfter,
			Version:   &versionAfter,
		},
		Status:          operation.Status{Code: "APPROVED"},
		AccountID:       accountID.String(),
		AccountAlias:    alias,
		BalanceKey:      constant.DefaultBalanceKey,
		BalanceID:       balanceID.String(),
		OrganizationID:  i.orgID.String(),
		LedgerID:        i.ledgerID.String(),
		BalanceAffected: true,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
		Snapshot: mmodel.OperationSnapshot{
			OverdraftUsedBefore: "0",
			OverdraftUsedAfter:  overdraftAfter,
		},
	})
	require.NoError(t, err)
}

func TestIntegration_ReseedGuard_StaleRowIsRebuiltFromTheTrail(t *testing.T) {
	infra := setupReseedTestInfra(t)

	// The row the sync worker last wrote: 100 at version 1.
	accountID, balanceID, alias := infra.seedBalance(t, "reseed-stale", decimal.NewFromInt(100), 1)

	// The trail has moved past it: a second credit landed and left 150 at version 2,
	// with 30 on hold, and the worker never flushed it.
	infra.recordOperation(t, accountID, balanceID, alias, decimal.NewFromInt(100), decimal.Zero, 1, "0")
	infra.recordOperation(t, accountID, balanceID, alias, decimal.NewFromInt(150), decimal.NewFromInt(30), 2, "20")

	balances, err := infra.uc.GetBalances(context.Background(), infra.orgID, infra.ledgerID,
		[]string{alias + "#" + constant.DefaultBalanceKey})

	require.NoError(t, err)
	require.Len(t, balances, 1)

	got := balances[0]
	assert.True(t, decimal.NewFromInt(150).Equal(got.Available),
		"the seed must carry the trail's state, not the stale row: got %s", got.Available)
	assert.True(t, decimal.NewFromInt(30).Equal(got.OnHold))
	assert.True(t, decimal.NewFromInt(20).Equal(got.OverdraftUsed))
	assert.Equal(t, int64(2), got.Version, "seeding at the row's version is what forks the balance")

	assert.Equal(t, balanceID.String(), got.ID, "identity comes from the row")
	assert.Equal(t, accountID.String(), got.AccountID)
	assert.Equal(t, alias, got.Alias)
	assert.Equal(t, constant.DefaultBalanceKey, got.Key)
	assert.Equal(t, "USD", got.AssetCode)
}

func TestIntegration_ReseedGuard_RowLevelWithTheTrailIsUntouched(t *testing.T) {
	infra := setupReseedTestInfra(t)

	accountID, balanceID, alias := infra.seedBalance(t, "reseed-current", decimal.NewFromInt(150), 2)
	infra.recordOperation(t, accountID, balanceID, alias, decimal.NewFromInt(150), decimal.Zero, 2, "0")

	balances, err := infra.uc.GetBalances(context.Background(), infra.orgID, infra.ledgerID,
		[]string{alias + "#" + constant.DefaultBalanceKey})

	require.NoError(t, err)
	require.Len(t, balances, 1)
	assert.True(t, decimal.NewFromInt(150).Equal(balances[0].Available))
	assert.Equal(t, int64(2), balances[0].Version)
}

func TestIntegration_ReseedGuard_NewAccountSeedsFromTheRow(t *testing.T) {
	infra := setupReseedTestInfra(t)

	_, _, alias := infra.seedBalance(t, "reseed-new", decimal.NewFromInt(100), 0)

	balances, err := infra.uc.GetBalances(context.Background(), infra.orgID, infra.ledgerID,
		[]string{alias + "#" + constant.DefaultBalanceKey})

	require.NoError(t, err)
	require.Len(t, balances, 1)
	assert.True(t, decimal.NewFromInt(100).Equal(balances[0].Available),
		"an account with no operations has nothing to rebuild from")
	assert.Equal(t, int64(0), balances[0].Version)
}
