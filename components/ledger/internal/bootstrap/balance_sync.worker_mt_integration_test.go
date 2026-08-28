//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/tenantcache"
	tmvalkey "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/valkey"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // register the "pgx" database/sql driver
	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	balancePG "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redisTx "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// syncRoundTimeout bounds each stage. The collector's default flush timeout is
// 500ms, so a stage that has not happened in this long is not going to.
const syncRoundTimeout = 30 * time.Second

// TestIntegration_BalanceSyncWorkerMT_RecoversFromClosedPool reproduces the
// production incident end to end and proves the cure.
//
// A tenant's pool is closed underneath a running collector — what the tenant
// manager does on a credentials rotation or a tenant.cache.invalidate event.
// With the handle frozen in the collector context the collector held that dead
// pool for the rest of the process's life: every flush failed with
// "sql: database is closed" and only a restart recovered. With per-flush
// resolution the flush after the pool is replaced persists, in the same
// collector and with no external action.
//
// Each stage syncs its own balance. A flush that fails leaves the claim lock in
// place for claimTTLSeconds, so re-enqueueing one balance across stages would
// stall on that lock rather than on the pool under test.
func TestIntegration_BalanceSyncWorkerMT_RecoversFromClosedPool(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	const tenantID = "acme"

	pg := pgtestutil.SetupLedgerContainer(t)
	redisContainer := redistestutil.SetupReusableContainer(t)
	redisConn := redistestutil.CreateConnectionWithDB(t, redisContainer.Addr, redisContainer.DB)

	redisRepo, err := redisTx.NewConsumerRedis(redisConn)
	require.NoError(t, err, "should create the Redis repository")

	// Two independent pools over the same database. dbA is the one the test kills;
	// dbB stands in for the pool the tenant manager rebuilds on the next GetDB.
	sqlA := openIntegrationPool(t, pg.DSN)
	sqlB := openIntegrationPool(t, pg.DSN)
	dbA := dbresolver.New(dbresolver.WithPrimaryDBs(sqlA))
	dbB := dbresolver.New(dbresolver.WithPrimaryDBs(sqlB))

	// Seed through the schema directly rather than HTTP, so the test owns exactly
	// the three balances it syncs.
	orgID := pgtestutil.CreateTestOrganization(t, pg.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, pg.DB, orgID)
	pgtestutil.CreateTestAsset(t, pg.DB, orgID, ledgerID, "USD")

	healthy := seedSyncTarget(t, pg.DB, orgID, ledgerID, "healthy")
	dead := seedSyncTarget(t, pg.DB, orgID, ledgerID, "dead")
	healed := seedSyncTarget(t, pg.DB, orgID, ledgerID, "healed")

	reader, factory := newBalanceSyncReaderFactory(t)

	resolver := &fakePGResolver{db: dbA}

	w := newTestWorkerWithResolver(t, tenantcache.NewTenantCache(), resolver).
		WithMetricsFactory(factory)

	// requireTenant makes the repository resolve its handle from the context only,
	// which is the multi-tenant contract this test exercises.
	w.useCase = &command.UseCase{
		TransactionRedisRepo: redisRepo,
		BalanceRepo:          balancePG.NewBalancePostgreSQLRepository(nil, false, true),
	}
	// The default batch size is deliberate: the claim script reads the schedule
	// with LIMIT 0 <batchSize>, so a batch of 1 would let the locked key left
	// behind by stage 2 sit at the head of the window and hide every later key
	// until its claim TTL expired. The 500ms TIMEOUT trigger flushes each stage.

	tenantCtx := tmcore.ContextWithTenantID(context.Background(), tenantID)

	scheduleKey, err := tmvalkey.GetKey(tenantID, utils.BalanceSyncScheduleKey)
	require.NoError(t, err, "should namespace the schedule key")

	// enqueue publishes a balance and schedules it as due. The ZSET member is the
	// tenant-namespaced key, matching what balance_atomic_operation.lua writes:
	// GetBalancesByKeys MGETs the members as-is.
	enqueue := func(target syncTarget, available int64) {
		t.Helper()

		unprefixed := utils.BalanceInternalKey(orgID, ledgerID, target.alias+"#default")

		member, keyErr := tmvalkey.GetKey(tenantID, unprefixed)
		require.NoError(t, keyErr, "should namespace the balance key")

		payload := marshalBalanceRedis(t, target, available)
		require.NoError(t, redisRepo.Set(tenantCtx, unprefixed, payload, 3600),
			"should store the balance payload")

		_, zErr := redisContainer.Client.ZAdd(context.Background(), scheduleKey, goredis.Z{
			Score:  float64(time.Now().Add(-time.Minute).Unix()),
			Member: member,
		}).Result()
		require.NoError(t, zErr, "should schedule the balance key as due")
	}

	// availableInPG reads through dbB, which stays healthy for the whole test so
	// the assertion never depends on the pool under test.
	availableInPG := func(target syncTarget) decimal.Decimal {
		t.Helper()

		var available decimal.Decimal
		require.NoError(t,
			sqlB.QueryRow(`SELECT available FROM balance WHERE id = $1`, target.balanceID).Scan(&available),
			"should read the balance back")

		return available
	}

	// processSyncBatch reads its metric factory off the context, not off the
	// worker, so the collector must be started from a context carrying it.
	ctx, cancel := context.WithCancel(
		libObservability.ContextWithMetricFactory(context.Background(), factory),
	)

	tc := w.startTenantCollector(ctx, tenantID)
	require.NotNil(t, tc, "the tenant's PG resolves, so the collector must start")

	t.Cleanup(func() {
		cancel()
		<-tc.done
	})

	// Stage 1: a healthy collector persists.
	t.Log("Stage 1: syncing on a healthy pool")
	enqueue(healthy, 1500)
	require.Eventually(t, func() bool { return availableInPG(healthy).Equal(decimal.NewFromInt(1500)) },
		syncRoundTimeout, 100*time.Millisecond,
		"the first sync must reach PostgreSQL")

	// Stage 2: close the pool underneath the running collector. The resolver keeps
	// handing back the same dead handle, exactly as a frozen context would.
	t.Log("Stage 2: closing the pool underneath the collector")
	require.NoError(t, sqlA.Close(), "should close the pool under test")

	failuresBefore := batchFailureCount(t, reader)

	enqueue(dead, 2500)
	require.Eventually(t, func() bool { return batchFailureCount(t, reader) > failuresBefore },
		syncRoundTimeout, 100*time.Millisecond,
		"a flush on the closed pool must fail and be counted")
	assert.True(t, availableInPG(dead).Equal(decimal.NewFromInt(1000)),
		"nothing must persist while the pool is closed")

	select {
	case <-tc.done:
		t.Fatal("the collector must survive the closed pool")
	default:
	}

	// Stage 3: the manager rebuilds the pool. Per-flush resolution is what lets the
	// next flush pick it up; a handle frozen at spawn would stay dead forever.
	t.Log("Stage 3: replacing the pool and expecting the next flush to heal")
	resolver.set(dbB, nil)

	enqueue(healed, 3500)
	require.Eventually(t, func() bool { return availableInPG(healed).Equal(decimal.NewFromInt(3500)) },
		syncRoundTimeout, 100*time.Millisecond,
		"the collector must persist again on the replaced pool, with no restart")

	select {
	case <-tc.done:
		t.Fatal("recovery must happen inside the original collector")
	default:
	}
}

// syncTarget is one seeded balance the test syncs, kept together with the alias
// its Redis key is built from.
type syncTarget struct {
	balanceID uuid.UUID
	accountID uuid.UUID
	alias     string
}

// seedSyncTarget inserts an account and its balance, seeded at version 0 and
// available 1000, so any sync the test drives is visible as a change.
func seedSyncTarget(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, name string) syncTarget {
	t.Helper()

	alias := "@sync-" + name

	accountID := pgtestutil.CreateTestAccount(t, db, orgID, ledgerID, nil, name, alias, "USD", nil)
	balanceID := pgtestutil.CreateTestBalanceSimple(t, db, orgID, ledgerID, accountID, alias, "USD")

	return syncTarget{balanceID: balanceID, accountID: accountID, alias: alias}
}

// openIntegrationPool opens an independent pool against dsn and closes it with
// the test unless the test closed it first.
func openIntegrationPool(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err, "should open a PostgreSQL pool")
	require.NoError(t, db.PingContext(context.Background()), "the pool must be reachable")

	t.Cleanup(func() { _ = db.Close() })

	return db
}

// marshalBalanceRedis builds the Redis payload the sync reads. Version 1 clears
// the seeded 0 so UpdateMany's version guard admits the write, and OverdraftUsed
// must be a parseable decimal string: UpdateMany SKIPS a row whose value is
// malformed, which would surface as a missing write rather than an error.
func marshalBalanceRedis(t *testing.T, target syncTarget, available int64) string {
	t.Helper()

	payload, err := json.Marshal(mmodel.BalanceRedis{
		ID:             target.balanceID.String(),
		Alias:          target.alias,
		Key:            "default",
		AccountID:      target.accountID.String(),
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(available),
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   1,
		AllowReceiving: 1,
		OverdraftUsed:  "0",
	})
	require.NoError(t, err, "should marshal the balance payload")

	return string(payload)
}

// batchFailureCount sums the batch-failure counter across all label sets.
func batchFailureCount(t *testing.T, reader *sdkmetric.ManualReader) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	var total int64

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != utils.BalanceSyncBatchFailures.Name {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "failure counter data type must be Sum[int64], got %T", m.Data)

			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}

	return total
}
