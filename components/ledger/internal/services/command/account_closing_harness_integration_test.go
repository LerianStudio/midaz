//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"database/sql"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	onboardingMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	postgresTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// The closing is compared against fixed instants, never measured against a clock,
// so every row a harness writes carries one of these.
var (
	accountClosingSeedInstant  = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	accountClosingFixedInstant = time.Date(2026, 3, 4, 5, 6, 7, 123456000, time.UTC)
)

// accountClosingHarness drives the real CloseAccount use case against the real
// dependencies it coordinates: the onboarding database that owns account.closed_at,
// the transaction database that owns the balance, transaction and operation rows,
// and the cache that holds the protection markers and the live balances.
//
// The two databases are separate containers because the onboarding and transaction
// migration sets share one golang-migrate bookkeeping table and cannot both be
// applied to one database. No foreign key crosses the boundary, so the same
// organization and ledger identifiers address both sides.
type accountClosingHarness struct {
	uc *UseCase

	onboardingDB  *sql.DB
	transactionDB *sql.DB
	client        redis.UniversalClient

	accountRepo *account.AccountPostgreSQLRepository

	organizationID uuid.UUID
	ledgerID       uuid.UUID
}

func newAccountClosingHarness(t *testing.T) *accountClosingHarness {
	t.Helper()

	onboarding := pgtestutil.SetupMigratedContainer(t, "onboarding")
	transaction := pgtestutil.SetupMigratedContainer(t, "transaction")
	cache := redistestutil.SetupReusableContainer(t)

	onboardingConn := pgtestutil.ConnectPostgresClient(t.Context(), t,
		pgtestutil.BuildConnectionString(onboarding.Host, onboarding.Port, onboarding.Config),
		pgtestutil.BuildConnectionString(onboarding.Host, onboarding.Port, onboarding.Config))
	transactionConn := pgtestutil.ConnectPostgresClient(t.Context(), t,
		pgtestutil.BuildConnectionString(transaction.Host, transaction.Port, transaction.Config),
		pgtestutil.BuildConnectionString(transaction.Host, transaction.Port, transaction.Config))

	redisRepo, err := txRedis.NewConsumerRedis(redistestutil.CreateConnectionWithDB(t, cache.Addr, cache.DB))
	require.NoError(t, err)

	accountRepo := account.NewAccountPostgreSQLRepository(onboardingConn)

	organizationID := pgtestutil.CreateTestOrganization(t, onboarding.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, onboarding.DB, organizationID)

	return &accountClosingHarness{
		uc: &UseCase{
			AccountRepo:          accountRepo,
			BalanceRepo:          balance.NewBalancePostgreSQLRepository(transactionConn, false),
			TransactionRepo:      postgresTransaction.NewTransactionPostgreSQLRepository(transactionConn, false),
			OperationRepo:        operation.NewOperationPostgreSQLRepository(transactionConn),
			TransactionRedisRepo: redisRepo,
		},
		onboardingDB:   onboarding.DB,
		transactionDB:  transaction.DB,
		client:         cache.Client,
		accountRepo:    accountRepo,
		organizationID: organizationID,
		ledgerID:       ledgerID,
	}
}

// close runs the closing of one account of this harness's scope.
func (h *accountClosingHarness) close(ctx context.Context, accountID uuid.UUID) (time.Time, error) {
	return h.uc.CloseAccount(ctx, h.organizationID, h.ledgerID, accountID)
}

// withOnboardingMetadata gives the use case the real onboarding metadata
// repository and returns the database behind it, so a test can observe that the
// closing leaves the account's metadata exactly as it found it.
func (h *accountClosingHarness) withOnboardingMetadata(t *testing.T) *mongo.Database {
	t.Helper()

	container := mongotestutil.SetupReusableContainer(t)
	database := mongotestutil.CreateOwnedDatabase(t, container)
	connection := mongotestutil.CreateConnection(t, container.URI, database.Name())

	h.uc.OnboardingMetadataRepo = onboardingMongo.NewMetadataMongoDBRepository(connection)

	return database
}

// seedAccount writes one account row with fixed timestamps. accountType decides
// whether the account is eligible at all: "external" never closes.
func (h *accountClosingHarness) seedAccount(t *testing.T, alias, accountType string) uuid.UUID {
	t.Helper()

	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	_, err := h.onboardingDB.Exec(`
		INSERT INTO account (id, name, asset_code, organization_id, ledger_id, status, alias, type, blocked, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
	`, accountID, "Account Closing Harness", "USD", h.organizationID, h.ledgerID, "ACTIVE", alias, accountType, false, accountClosingSeedInstant)
	require.NoError(t, err, "failed to seed the account to close")

	return accountID
}

// accountClosingBalanceSeed describes one persisted balance row of the account.
type accountClosingBalanceSeed struct {
	alias         string
	key           string
	available     string
	onHold        string
	overdraftUsed string
	version       int64
	scope         string
}

// seedBalance writes one balance row with fixed timestamps and returns its id.
func (h *accountClosingHarness) seedBalance(t *testing.T, accountID uuid.UUID, seed accountClosingBalanceSeed) uuid.UUID {
	t.Helper()

	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	_, err := h.transactionDB.Exec(`
		INSERT INTO balance (
			id, organization_id, ledger_id, account_id, alias, key, asset_code,
			available, on_hold, overdraft_used, version, account_type,
			allow_sending, allow_receiving, direction, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, true, true, 'credit', $13, $13)
	`, balanceID, h.organizationID, h.ledgerID, accountID, seed.alias, seed.key, "USD",
		accountClosingDecimal(seed.available), accountClosingDecimal(seed.onHold), accountClosingDecimal(seed.overdraftUsed),
		seed.version, "deposit", accountClosingSeedInstant)
	require.NoError(t, err, "failed to seed the balance of the account to close")

	return balanceID
}

func accountClosingDecimal(value string) decimal.Decimal {
	if value == "" {
		return decimal.Zero
	}

	return decimal.RequireFromString(value)
}

// cacheBalance publishes one live balance blob, which is the state a movement
// leaves behind in the transaction cache. The closing reads it and, when it is
// ahead of the row, refuses instead of closing over unsynchronized money.
func (h *accountClosingHarness) cacheBalance(t *testing.T, accountID, balanceID uuid.UUID, seed accountClosingBalanceSeed) string {
	t.Helper()

	scope := seed.scope
	if scope == "" {
		scope = "transactional"
	}

	snapshot := accounting.BalanceSnapshot{
		BalanceRef: seed.alias + "#" + seed.key, ID: balanceID, AccountID: accountID,
		AccountType: "deposit", AssetCode: "USD", Alias: seed.alias, Key: seed.key,
		Direction: "credit", BalanceScope: scope,
		Available: accountClosingDecimal(seed.available), OnHold: accountClosingDecimal(seed.onHold),
		OverdraftUsed: accountClosingDecimal(seed.overdraftUsed), Version: seed.version,
		AllowSending: true, AllowReceiving: true,
	}

	encoded, err := balancecache.Encode(snapshot, balancecache.FormatDual)
	require.NoError(t, err)

	key := utils.BalanceInternalKey(h.organizationID, h.ledgerID, snapshot.BalanceRef)
	require.NoError(t, h.client.Set(context.Background(), key, encoded, time.Hour).Err())

	return key
}

// syncBalanceRow advances the persisted row to the state the cache already holds,
// which is what the existing balance-sync worker does.
func (h *accountClosingHarness) syncBalanceRow(t *testing.T, balanceID uuid.UUID, seed accountClosingBalanceSeed) {
	t.Helper()

	_, err := h.transactionDB.Exec(`
		UPDATE balance SET available = $1, on_hold = $2, overdraft_used = $3, version = $4, updated_at = $5 WHERE id = $6
	`, accountClosingDecimal(seed.available), accountClosingDecimal(seed.onHold), accountClosingDecimal(seed.overdraftUsed),
		seed.version, accountClosingFixedInstant, balanceID)
	require.NoError(t, err, "failed to synchronize the balance row")
}

// protectionKeys are the three controls one account may carry.
type accountClosingProtectionKeys struct {
	closing   string
	closed    string
	ownership string
}

func (h *accountClosingHarness) protection(accountID uuid.UUID) accountClosingProtectionKeys {
	return accountClosingProtectionKeys{
		closing:   utils.AccountClosingMarkerKey(h.organizationID, h.ledgerID, accountID),
		closed:    utils.AccountClosedMarkerKey(h.organizationID, h.ledgerID, accountID),
		ownership: utils.AccountAdminOwnershipKey(h.organizationID, h.ledgerID, accountID),
	}
}

// exists reports how many of the given keys are present.
func (h *accountClosingHarness) exists(t *testing.T, keys ...string) int64 {
	t.Helper()

	count, err := h.client.Exists(context.Background(), keys...).Result()
	require.NoError(t, err)

	return count
}

// closedAt reads the authoritative closing instant of one account.
func (h *accountClosingHarness) closedAt(t *testing.T, accountID uuid.UUID) sql.NullTime {
	t.Helper()

	var closedAt sql.NullTime

	require.NoError(t, h.onboardingDB.QueryRow(`SELECT closed_at FROM account WHERE id = $1`, accountID).Scan(&closedAt))

	return closedAt
}

// balanceRow reads back the monetary components and the version of one row, which
// is how a refusal proves it moved no money.
func (h *accountClosingHarness) balanceRow(t *testing.T, balanceID uuid.UUID) accountClosingBalanceSeed {
	t.Helper()

	var (
		available, onHold, overdraftUsed decimal.Decimal
		version                          int64
	)

	require.NoError(t, h.transactionDB.QueryRow(`
		SELECT available, on_hold, overdraft_used, version FROM balance WHERE id = $1
	`, balanceID).Scan(&available, &onHold, &overdraftUsed, &version))

	return accountClosingBalanceSeed{
		available: available.String(), onHold: onHold.String(),
		overdraftUsed: overdraftUsed.String(), version: version,
	}
}

// countRows counts rows of one transaction-side table for the harness scope, so a
// refusal or a closing can be shown to have created no history of its own.
func (h *accountClosingHarness) countRows(t *testing.T, table string) int {
	t.Helper()

	var count int

	query := `SELECT count(*) FROM "` + table + `" WHERE organization_id = $1 AND ledger_id = $2`

	require.NoError(t, h.transactionDB.QueryRow(query, h.organizationID, h.ledgerID).Scan(&count))

	return count
}
