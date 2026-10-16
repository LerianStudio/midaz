//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	transactionRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// TestIntegrationEngineColdSeedDecidesWithARowNewerThanTheLiveKey commits a
// configuration change to PostgreSQL the way a PATCH served by an earlier release
// does, without writing the configuration key, while a key written before the change
// still lives. The source balance is not cached, so the load reads its row and its
// account row: the newer rows must decide. The transaction is refused, the key moves
// to the row's version, and no blob takes the older configuration, which a refusal
// would never heal and an earlier release would obey after a rollback.
func TestIntegrationEngineColdSeedDecidesWithARowNewerThanTheLiveKey(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	infra := setupEngineWriteBehindHTTPIntegration(t)
	infra.command.TransactionWriteBehindAsync = false
	app := infra.newHTTPApp("")

	t.Run("account blocked after its key was written", func(t *testing.T) {
		ctx := context.Background()
		seeded := infra.seedColdTransfer(t, "blocked")

		_, outcome, err := infra.redisRepo.WriteAccountConfig(ctx, infra.organization, infra.ledger,
			accounting.AccountConfig{AccountID: seeded.sourceAccount, Version: 0, Blocked: false})
		require.NoError(t, err)
		require.Equal(t, transactionRedis.ConfigWritten, outcome)

		_, err = infra.db.ExecContext(ctx, `UPDATE account SET blocked = true WHERE id = $1`, seeded.sourceAccount)
		require.NoError(t, err)
		require.Equal(t, int64(1), infra.configVersion(t, `SELECT config_version FROM account WHERE id = $1`, seeded.sourceAccount))
		require.Zero(t, infra.redis.Exists(ctx, seeded.sourceBlobKey).Val(), "the source balance is not cached")

		created := infra.postCreate(t, app, "v1", seeded.aliases, "cold-blocked-"+uuid.NewString())
		require.Equalf(t, http.StatusUnprocessableEntity, created.status, "a blocked account must not transact: %s", created.body)
		assert.Equal(t, constant.ErrAccountBlocked.Error(), created.decoded["code"])

		if blob, cached := infra.cachedBlob(t, ctx, seeded.sourceBlobKey); cached {
			assert.True(t, blob.Blocked, "no blob may hold the unblocked copy the older key carried")
		}

		keys, err := infra.redisRepo.ReadConfigKeys(ctx, infra.organization, infra.ledger, nil, []uuid.UUID{seeded.sourceAccount})
		require.NoError(t, err)
		assert.Equal(t, accounting.AccountConfig{AccountID: seeded.sourceAccount, Version: 1, Blocked: true},
			keys.Accounts[seeded.sourceAccount], "the key takes the newer row")
	})

	t.Run("sending disallowed after the balance key was written", func(t *testing.T) {
		ctx := context.Background()
		seeded := infra.seedColdTransfer(t, "sending")

		_, outcome, err := infra.redisRepo.WriteBalanceConfig(ctx, infra.organization, infra.ledger, seeded.sourceRef,
			accounting.BalanceConfig{
				BalanceID: seeded.sourceBalance, Version: 0, AllowSending: true, AllowReceiving: true,
				OverdraftLimit: decimal.Zero, BalanceScope: mmodel.BalanceScopeTransactional,
			})
		require.NoError(t, err)
		require.Equal(t, transactionRedis.ConfigWritten, outcome)

		_, err = infra.db.ExecContext(ctx, `UPDATE balance SET allow_sending = false WHERE id = $1`, seeded.sourceBalance)
		require.NoError(t, err)
		require.Equal(t, int64(1), infra.configVersion(t, `SELECT config_version FROM balance WHERE id = $1`, seeded.sourceBalance))
		require.Zero(t, infra.redis.Exists(ctx, seeded.sourceBlobKey).Val(), "the source balance is not cached")

		created := infra.postCreate(t, app, "v1", seeded.aliases, "cold-sending-"+uuid.NewString())
		require.Equalf(t, http.StatusUnprocessableEntity, created.status, "a balance that may not send must not send: %s", created.body)
		assert.Equal(t, constant.ErrAccountStatusTransactionRestriction.Error(), created.decoded["code"])

		if blob, cached := infra.cachedBlob(t, ctx, seeded.sourceBlobKey); cached {
			assert.False(t, blob.AllowSending, "no blob may hold the sending copy the older key carried")
		}

		keys, err := infra.redisRepo.ReadConfigKeys(ctx, infra.organization, infra.ledger, []string{seeded.sourceRef}, nil)
		require.NoError(t, err)

		stored := keys.Balances[seeded.sourceRef]
		assert.Equal(t, int64(1), stored.Version, "the key takes the newer row")
		assert.False(t, stored.AllowSending)
	})
}

type coldTransfer struct {
	aliases       engineWriteBehindAliases
	sourceAccount uuid.UUID
	sourceBalance uuid.UUID
	sourceRef     string
	sourceBlobKey string
}

// seedColdTransfer creates a funded source and an empty destination in PostgreSQL
// only, so the first transaction seeds both balances from their rows.
func (infra *engineWriteBehindHTTPIntegration) seedColdTransfer(t *testing.T, name string) coldTransfer {
	t.Helper()

	suffix := name + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	seeded := coldTransfer{aliases: engineWriteBehindAliases{source: "@cold-source-" + suffix, destination: "@cold-destination-" + suffix}}

	for alias, available := range map[string]decimal.Decimal{
		seeded.aliases.source: decimal.NewFromInt(1000), seeded.aliases.destination: decimal.Zero,
	} {
		accountID := postgrestestutil.CreateTestAccount(t, infra.db, infra.organization, infra.ledger, nil, alias, alias, "USD", nil)
		params := postgrestestutil.DefaultBalanceParams()
		params.Alias, params.AssetCode, params.Available = alias, "USD", available
		balanceID := postgrestestutil.CreateTestBalance(t, infra.db, infra.organization, infra.ledger, accountID, params)

		if alias == seeded.aliases.source {
			seeded.sourceAccount, seeded.sourceBalance = accountID, balanceID
		}
	}

	seeded.sourceRef = seeded.aliases.source + "#" + constant.DefaultBalanceKey
	seeded.sourceBlobKey = utils.BalanceInternalKey(infra.organization, infra.ledger, seeded.sourceRef)

	return seeded
}

func (infra *engineWriteBehindHTTPIntegration) configVersion(t *testing.T, query string, id uuid.UUID) int64 {
	t.Helper()

	var version int64
	require.NoError(t, infra.db.QueryRow(query, id).Scan(&version))

	return version
}

func (infra *engineWriteBehindHTTPIntegration) cachedBlob(t *testing.T, ctx context.Context, key string) (accounting.BalanceSnapshot, bool) {
	t.Helper()

	raw, err := infra.redis.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return accounting.BalanceSnapshot{}, false
	}

	require.NoError(t, err)

	blob, err := balancecache.Decode(raw)
	require.NoError(t, err)

	return blob, true
}
