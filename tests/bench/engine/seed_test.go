//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Seeds 10k twice and 1M once, each over a freshly rebuilt schema. The two 10k states must hash alike; the 1M
// state is checked through counts, sums, the constant columns of every row and the named rows only.
func TestBootstrap(t *testing.T) {
	env := requireDB(t)
	ctx := t.Context()

	var hashedRuns [][3]string

	for run, tc := range []struct {
		n     int
		limit time.Duration
	}{{10_000, time.Minute}, {10_000, time.Minute}, {1_000_000, 10 * time.Minute}} {
		schemaDur, err := timed(func() error { return bootstrapSchema(ctx, env) })
		require.NoError(t, err, "run %d schema", run)

		seedCtx, cancel := context.WithTimeout(ctx, tc.limit)
		seedDur, err := timed(func() error { return seedFixtures(seedCtx, env, tc.n) })

		cancel()
		require.NoError(t, err, "run %d seed within %s", run, tc.limit)

		conn := connectPrimary(ctx, t, env)
		balances, controls := assertSeeded(ctx, t, conn, tc.n)
		t.Logf("run=%d n=%d balances=%d controls=%d schema=%s seed=%s", run, tc.n, balances, controls, schemaDur, seedDur)

		if tc.n == 10_000 {
			// One server-side pass per table; any now(), random ID or leftover row changes the sum.
			var hashes [3]string

			require.NoError(t, conn.QueryRow(ctx, `SELECT
				(SELECT count(*) || ':' || coalesce(sum(hashtextextended(x::text, 0)), 0) FROM balance x),
				(SELECT count(*) || ':' || coalesce(sum(hashtextextended(x::text, 0)), 0) FROM account_control x),
				(SELECT count(*) || ':' || coalesce(sum(hashtextextended(x::text, 0)), 0) FROM engine_writer_fence x)`).
				Scan(&hashes[0], &hashes[1], &hashes[2]))
			t.Logf("run=%d balance|account_control|engine_writer_fence count:hashsum %v", run, hashes)

			hashedRuns = append(hashedRuns, hashes)
		}
	}

	assert.Equal(t, hashedRuns[0], hashedRuns[1], "two 10k bootstraps must leave identical rows")
}

// assertSeeded returns the balance and control row counts it read.
func assertSeeded(ctx context.Context, t *testing.T, conn *pgx.Conn, n int) (balances, controls int64) {
	t.Helper()

	var available, offBalances int64

	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*), coalesce(sum(available), 0)::bigint,
		count(*) FILTER (WHERE available <> $4 OR on_hold <> 0 OR overdraft_used <> 0 OR version IS DISTINCT FROM 0
			OR NOT (allow_sending AND allow_receiving) OR direction <> 'credit' OR asset_code <> 'BRL'
			OR account_type <> 'deposit' OR organization_id <> $1 OR ledger_id <> $2 OR created_at <> $3
			OR updated_at <> $3 OR deleted_at IS NOT NULL OR settings IS NOT NULL)
		FROM balance`, benchOrgID, benchLedgerID, benchSeedTime, benchFunds).Scan(&balances, &available, &offBalances))
	assert.Equal(t, int64(n+4), balances, "n ordinary + hot + fee + 2 multikey")
	assert.Equal(t, int64(n+4)*benchFunds, available)
	assert.Zero(t, offBalances, "balances off the seed constants")

	var limits, offControls, fences, written int64

	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*), coalesce(sum(limit_remaining), 0)::bigint,
		count(*) FILTER (WHERE blocked OR limit_remaining <> $3 OR version <> 0 OR organization_id <> $1 OR ledger_id <> $2),
		(SELECT count(*) FROM engine_writer_fence), (SELECT count(*) FROM engine_request) + (SELECT count(*) FROM outbox_events)
			+ (SELECT count(*) FROM "transaction") + (SELECT count(*) FROM operation)
		FROM account_control`, benchOrgID, benchLedgerID, benchFunds).Scan(&controls, &limits, &offControls, &fences, &written))
	assert.Equal(t, int64(n+3), controls, "multikey shares one control")
	assert.Equal(t, int64(n+3)*benchFunds, limits)
	assert.Zero(t, offControls, "controls off the seed constants")
	assert.Equal(t, int64(1), fences)
	assert.Zero(t, written, "no request, outbox, transaction or operation row")

	def := constant.DefaultBalanceKey
	named := []struct {
		class string
		i     int
		key   string
	}{
		{classOrdinary, 0, def},
		{classOrdinary, n - 1, def},
		{classHot, 0, def},
		{classFee, 0, def},
		{classMultikey, 0, def},
		{classMultikey, 0, keySecondary},
	}
	accounts := make([]uuid.UUID, 0, len(named))

	for _, r := range named {
		var (
			account    uuid.UUID
			alias, key string
		)

		require.NoError(t, conn.QueryRow(ctx, `SELECT account_id, alias, key FROM balance WHERE id = $1`,
			balanceID(r.class, r.i, r.key)).Scan(&account, &alias, &key), "%+v", r)
		assert.Equal(t, []any{accountID(r.class, r.i), benchAlias(r.class, r.i), r.key}, []any{account, alias, key}, "%+v", r)

		accounts = append(accounts, accountID(r.class, r.i))
	}

	var namedControls, multikeyBalances int

	// The multikey account appears twice in accounts; ANY still matches its one control once.
	require.NoError(t, conn.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM account_control WHERE account_id = ANY($1)),
		(SELECT count(*) FROM balance WHERE account_id = $2)`, accounts, accountID(classMultikey, 0)).
		Scan(&namedControls, &multikeyBalances))
	assert.Equal(t, 5, namedControls, "one control per named account")
	assert.Equal(t, 2, multikeyBalances, "multikey account holds both keys")

	var (
		owner              string
		epoch              int64
		seedLease, expired bool
	)

	require.NoError(t, conn.QueryRow(ctx, `SELECT owner_id, epoch, lease_until = $2, lease_until <= clock_timestamp()
		FROM engine_writer_fence WHERE tenant_id = $1`, benchTenantID, benchSeedTime).Scan(&owner, &epoch, &seedLease, &expired))
	assert.Equal(t, []any{"", int64(0), true, true}, []any{owner, epoch, seedLease, expired}, "ownerless fence, expired lease")

	return balances, controls
}
