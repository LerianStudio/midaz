//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// accountClosingCostSamples is how many executions each measurement below runs.
// It is small on purpose: these are cost observations recorded in
// docs/performance/account-closing-report.md, not a load test, and the suite
// has to stay fast enough to run on every change.
const accountClosingCostSamples = 150

// accountClosingCostWarmups are the untimed iterations that precede every series.
// They pay for the script cache, the connection pool and the first allocations, so
// the recorded samples describe steady state rather than start-up.
const accountClosingCostWarmups = 25

// accountClosingCostScript submits the engine through EVALSHA, as the adapter
// does. Sending the script body on every call would measure the transfer of the
// script instead of the execution.
var accountClosingCostScript = redis.NewScript(accountingScriptSource)

// accountClosingCostProfile is one measured series, kept sorted.
type accountClosingCostProfile struct {
	name    string
	samples []time.Duration
}

func (p accountClosingCostProfile) quantile(fraction float64) time.Duration {
	if len(p.samples) == 0 {
		return 0
	}

	return p.samples[int(float64(len(p.samples)-1)*fraction)]
}

func (p accountClosingCostProfile) mean() time.Duration {
	if len(p.samples) == 0 {
		return 0
	}

	var total time.Duration

	for _, sample := range p.samples {
		total += sample
	}

	return total / time.Duration(len(p.samples))
}

// measureAccountClosingCost runs one series and returns it sorted, so the report
// can quote a median and a tail rather than a single number.
func measureAccountClosingCost(name string, samples int, run func()) accountClosingCostProfile {
	profile := accountClosingCostProfile{name: name, samples: make([]time.Duration, 0, samples)}

	for range accountClosingCostWarmups {
		run()
	}

	for range samples {
		start := time.Now()

		run()

		profile.samples = append(profile.samples, time.Since(start))
	}

	sort.Slice(profile.samples, func(i, j int) bool { return profile.samples[i] < profile.samples[j] })

	return profile
}

// reportAccountClosingCost prints the series as one table.
func reportAccountClosingCost(t *testing.T, profiles ...accountClosingCostProfile) {
	t.Helper()

	var table strings.Builder

	table.WriteString(fmt.Sprintf("\n%-56s %12s %12s %12s\n", "series", "mean", "p50", "p95"))

	for _, profile := range profiles {
		table.WriteString(fmt.Sprintf("%-56s %12s %12s %12s\n",
			profile.name, profile.mean(), profile.quantile(0.5), profile.quantile(0.95)))
	}

	t.Log(table.String())
}

// evalOnce prepares the declared execution and submits it exactly once, which is
// what one movement costs end to end on the adapter side.
func (f *integrationFixture) evalOnce(t *testing.T) (string, error) {
	t.Helper()

	prepared, err := prepareExecution(context.Background(), f.input, f.limits, f.resolved)
	require.NoError(t, err)

	return accountClosingCostScript.Run(context.Background(), f.client, prepared.Keys, string(prepared.Payload),
		strconv.Itoa(f.limits.MaxRequestBytes), strconv.Itoa(f.limits.MaxPreparedBytes)).Text()
}

// rotateExecution gives the next sample a fresh execution and transaction
// identity, so every iteration measures a first submission rather than a replay or
// a guard conflict.
func (f *integrationFixture) rotateExecution() {
	transactionID := uuid.New()

	f.input.Execution.ExecutionID = uuid.New()
	f.input.Execution.Transactions[0].ID = transactionID
	f.input.Guards[0].TransactionID = transactionID
	f.input.CompletionPlans[0].TransactionID = transactionID
}

// addUnusedPoolAccount appends one balance of its own account to the declared pool
// without posting to it. Each one costs the execution exactly what one more
// protected account costs: its keys, its control reads and its pool entry.
func (f *integrationFixture) addUnusedPoolAccount(t *testing.T, index int) {
	t.Helper()

	balance := f.input.Execution.Balances[0]
	balance.ID, balance.AccountID = uuid.New(), uuid.New()
	balance.Alias = fmt.Sprintf("@pool-%02d", index)
	balance.Key, balance.BalanceRef = "default", balance.Alias+"#default"
	balance.Available, balance.OnHold, balance.OverdraftUsed = decimal.NewFromInt(100), decimal.Zero, decimal.Zero
	balance.Version = 0

	f.input.Execution.Balances = append(f.input.Execution.Balances, balance)
	f.resolved.Balances[balance.BalanceRef] = testResolvedBalanceKeys(
		strings.Replace(f.resolved.Balances["@source#default"].Balance, "@source#default", balance.BalanceRef, 1))

	f.seed(t, len(f.input.Execution.Balances)-1, balance)
	f.syncAccountProtection(t)
}

// newAccountClosingCostFixture builds a fixture whose posting is negligible, so a
// long series of executions never drains the balance it moves and every sample
// measures the same work.
func newAccountClosingCostFixture(t *testing.T, container *redistestutil.ContainerResult) *integrationFixture {
	t.Helper()

	f := newIntegrationFixture(t, container.Client)
	f.input.Execution.Transactions[0].Postings[0].Amount = decimal.RequireFromString("0.0000000000000000001")

	// The default request budget is sized for the single-account fixture; the
	// pooled series declares twenty, so the byte bound is lifted to the production
	// ceiling rather than the fixture's.
	f.limits.MaxRequestBytes = 4 << 20

	return f
}

// TestIntegrationAccountClosingCostProfile measures what the closing controls cost
// the engine, so the decision to add neither an index nor a global lock rests on
// numbers rather than on assumption.
//
// Four series are recorded on the same Valkey, over the same declared execution:
// the warm hot path, the same path with twenty protected accounts in the pool, the
// cold-cache path that admits a seed under an administrative ownership, and the
// refusal a closed account answers with. The absolute values belong to the machine
// that ran them; the ratios between the series are what the report quotes.
func TestIntegrationAccountClosingCostProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	warm := newAccountClosingCostFixture(t, container)
	warm.seed(t, 0, warm.input.Execution.Balances[0])
	require.Len(t, protectedAccounts(warm.input.Execution), 1)

	// A container that has just started, a cold connection pool and an unloaded
	// script would all be charged to whichever series runs first. This untimed pass
	// pays for them before any sample is recorded.
	for range 200 {
		warm.rotateExecution()

		_, err := warm.evalOnce(t)
		require.NoError(t, err)
	}

	warmProfile := measureAccountClosingCost("1 protected account, warm cache", accountClosingCostSamples, func() {
		warm.rotateExecution()

		_, err := warm.evalOnce(t)
		require.NoError(t, err)
	})

	const pooledAccounts = 20

	pooled := newAccountClosingCostFixture(t, container)
	pooled.seed(t, 0, pooled.input.Execution.Balances[0])

	for index := 1; index < pooledAccounts; index++ {
		pooled.addUnusedPoolAccount(t, index)
	}

	require.Len(t, protectedAccounts(pooled.input.Execution), pooledAccounts)

	pooledProfile := measureAccountClosingCost(
		fmt.Sprintf("%d protected accounts, warm cache", pooledAccounts), accountClosingCostSamples, func() {
			pooled.rotateExecution()

			_, err := pooled.evalOnce(t)
			require.NoError(t, err)
		})

	cold := newAccountClosingCostFixture(t, container)
	coldBalanceKey := cold.resolved.Balances["@source#default"].Balance

	coldProfile := measureAccountClosingCost("1 protected account, cold cache (seed admitted)", accountClosingCostSamples, func() {
		cold.rotateExecution()
		require.NoError(t, container.Client.Del(context.Background(), coldBalanceKey).Err())

		_, err := cold.evalOnce(t)
		require.NoError(t, err)
	})

	refused := newAccountClosingCostFixture(t, container)
	refused.seed(t, 0, refused.input.Execution.Balances[0])

	refusedProtection := sourceAccountProtection(t, refused)
	require.NoError(t, container.Client.Set(context.Background(), refusedProtection.Closed, accountClosingInstant, time.Hour).Err())

	refusedProfile := measureAccountClosingCost("1 protected account, refused as closed", accountClosingCostSamples, func() {
		refused.rotateExecution()

		_, err := refused.evalOnce(t)
		require.ErrorContains(t, err, `"code":"account_closed"`)
	})

	reportAccountClosingCost(t, warmProfile, pooledProfile, coldProfile, refusedProfile)

	perAccount := (pooledProfile.quantile(0.5) - warmProfile.quantile(0.5)) / time.Duration(pooledAccounts-1)
	t.Logf("median cost of one additional protected account (keys, controls and pool entry): %s", perAccount)

	require.Less(t, refusedProfile.quantile(0.5), 3*warmProfile.quantile(0.5),
		"a refusal costs the order of an execution, never a multiple of it")
}

// TestIntegrationAccountClosingMarkerReadCost isolates the cache cost of the
// controls themselves: the three keys the preflight resolves per account, over the
// same Valkey the engine uses. It is the floor under the end-to-end numbers.
func TestIntegrationAccountClosingMarkerReadCost(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	ctx := context.Background()

	organizationID, ledgerID := uuid.New(), uuid.New()
	accountIDs := make([]uuid.UUID, 20)

	for index := range accountIDs {
		accountIDs[index] = uuid.New()
	}

	read := func(count int) func() {
		return func() {
			keys := make([]string, 0, count*3)

			for _, accountID := range accountIDs[:count] {
				keys = append(keys,
					utils.AccountClosingMarkerKey(organizationID, ledgerID, accountID),
					utils.AccountClosedMarkerKey(organizationID, ledgerID, accountID),
					utils.AccountAdminOwnershipKey(organizationID, ledgerID, accountID))
			}

			require.NoError(t, container.Client.MGet(ctx, keys...).Err())
		}
	}

	reportAccountClosingCost(t,
		measureAccountClosingCost("3 control reads (1 account, all absent)", accountClosingCostSamples, read(1)),
		measureAccountClosingCost("60 control reads (20 accounts, all absent)", accountClosingCostSamples, read(20)))
}
