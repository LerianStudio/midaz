//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// accountClosingClosureSamples is how many closings each series below performs.
// Every sample needs an account of its own — a closing happens once — so the count
// is kept low; the report quotes the median of each series.
const accountClosingClosureSamples = 8

// accountClosingClosureSeries is one measured closing series.
type accountClosingClosureSeries struct {
	name    string
	samples []time.Duration
}

func (s accountClosingClosureSeries) quantile(fraction float64) time.Duration {
	if len(s.samples) == 0 {
		return 0
	}

	return s.samples[int(float64(len(s.samples)-1)*fraction)]
}

func (s accountClosingClosureSeries) mean() time.Duration {
	if len(s.samples) == 0 {
		return 0
	}

	var total time.Duration

	for _, sample := range s.samples {
		total += sample
	}

	return total / time.Duration(len(s.samples))
}

// measureAccountClosings closes one freshly prepared account per sample and
// records how long the whole verification, write and finalization took.
func (h *accountClosingHarness) measureAccountClosings(t *testing.T, name string, balances int, warm bool) accountClosingClosureSeries {
	t.Helper()

	ctx := context.Background()
	series := accountClosingClosureSeries{name: name, samples: make([]time.Duration, 0, accountClosingClosureSamples)}

	for sample := range accountClosingClosureSamples {
		alias := fmt.Sprintf("@closing-cost-%s-%02d", strings.ReplaceAll(name, " ", "-"), sample)
		accountID := h.seedAccount(t, alias, "deposit")

		for index := range balances {
			seed := accountClosingBalanceSeed{alias: alias, key: fmt.Sprintf("key%02d", index)}
			balanceID := h.seedBalance(t, accountID, seed)

			if warm {
				h.cacheBalance(t, accountID, balanceID, seed)
			}
		}

		start := time.Now()

		_, err := h.close(ctx, accountID)

		series.samples = append(series.samples, time.Since(start))

		require.NoError(t, err)
	}

	sort.Slice(series.samples, func(i, j int) bool { return series.samples[i] < series.samples[j] })

	return series
}

// seedRecoveryBacklog fills the legacy recovery hash with records of OTHER
// accounts, which is the backlog every closing has to walk before it can claim
// that none of the pending completions touches its own account.
func (h *accountClosingHarness) seedRecoveryBacklog(t *testing.T, records int) {
	t.Helper()

	ctx := context.Background()
	pipeline := h.client.Pipeline()

	for index := range records {
		record := mmodel.TransactionRedisQueue{
			TransactionID:  uuid.Must(libCommons.GenerateUUIDv7()),
			OrganizationID: h.organizationID,
			LedgerID:       h.ledgerID,
			Action:         constant.ActionDirect,
			Balances:       []mmodel.BalanceRedis{{ID: uuid.NewString(), AccountID: uuid.NewString(), Key: constant.DefaultBalanceKey}},
		}

		raw, err := json.Marshal(record)
		require.NoError(t, err)

		pipeline.HSet(ctx, txRedis.TransactionBackupQueue, record.TransactionID.String(), string(raw))

		if index%1000 == 999 {
			_, err = pipeline.Exec(ctx)
			require.NoError(t, err)
		}
	}

	if _, err := pipeline.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		require.NoError(t, err)
	}

	length, err := h.client.HLen(ctx, txRedis.TransactionBackupQueue).Result()
	require.NoError(t, err)
	require.EqualValues(t, records, length)
}

// reportAccountClosingClosureCost prints the series as one table.
func reportAccountClosingClosureCost(t *testing.T, series ...accountClosingClosureSeries) {
	t.Helper()

	var table strings.Builder

	table.WriteString(fmt.Sprintf("\n%-56s %12s %12s %12s\n", "series", "mean", "p50", "p95"))

	for _, entry := range series {
		table.WriteString(fmt.Sprintf("%-56s %12s %12s %12s\n",
			entry.name, entry.mean(), entry.quantile(0.5), entry.quantile(0.95)))
	}

	t.Log(table.String())
}

// TestIntegrationAccountClosingClosureCostProfile measures where the cost of one
// closing actually goes, which is the evidence behind leaving the recovery walk on
// the closing rather than indexing it per account.
//
// The series vary the two things a closing is linear in: how many balances the
// account owns, and how large the recovery backlog of the tenant is. The absolute
// values belong to the machine that ran them; the shape of the growth is what the
// report quotes.
func TestIntegrationAccountClosingClosureCostProfile(t *testing.T) {
	h := newAccountClosingHarness(t)

	oneBalanceCold := h.measureAccountClosings(t, "1 balance cold cache empty backlog", 1, false)
	manyBalancesCold := h.measureAccountClosings(t, "20 balances cold cache empty backlog", 20, false)
	manyBalancesWarm := h.measureAccountClosings(t, "20 balances warm cache empty backlog", 20, true)

	h.seedRecoveryBacklog(t, 2000)
	backlogSmall := h.measureAccountClosings(t, "20 balances warm cache 2k backlog", 20, true)

	require.NoError(t, h.client.Del(context.Background(), txRedis.TransactionBackupQueue).Err())
	h.seedRecoveryBacklog(t, 8000)
	backlogLarge := h.measureAccountClosings(t, "20 balances warm cache 8k backlog", 20, true)

	reportAccountClosingClosureCost(t, oneBalanceCold, manyBalancesCold, manyBalancesWarm, backlogSmall, backlogLarge)

	perRecord := (backlogLarge.quantile(0.5) - backlogSmall.quantile(0.5)) / time.Duration(6000)
	t.Logf("median cost of one additional recovery record on the closing walk: %s", perRecord)

	require.Greater(t, backlogLarge.quantile(0.5), backlogSmall.quantile(0.5),
		"the recovery walk is the term that grows with the tenant's backlog")
}
