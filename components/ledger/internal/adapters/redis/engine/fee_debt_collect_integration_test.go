//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// newStandaloneCollectFixture is a transaction holding one collect of @source's debts,
// as a standalone collection composes it: no other posting and no balance requirement.
func newStandaloneCollectFixture(t *testing.T, client redis.UniversalClient, available int64, amount string, items ...string) *integrationFixture {
	t.Helper()

	f := newIntegrationFixture(t, client)
	f.input.Execution.Balances[0].Available = decimal.NewFromInt(available)
	f.addPoolBalance("@fees", 0, nil)
	f.addPoolBalance("@fees2", 0, nil)
	f.declareFeeDebts(t, "@source#default")
	collect := feePosting("collect", "@source#default", accounting.PostingCollect, amount)
	collect.Items = items
	f.input.Execution.Transactions[0].Postings = []accounting.Posting{collect}
	f.input.Execution.Transactions[0].BalanceRequirements = []accounting.BalanceRequirement{}

	return f
}

func TestIntegrationFeeDebtStandaloneCollect(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	o1, o2, o3 := uuid.MustParse("0a4b6d8e-3333-4a1a-8a1a-000000000001"), uuid.MustParse("0a4b6d8e-3333-4a1a-8a1a-000000000002"), uuid.MustParse("0a4b6d8e-3333-4a1a-8a1a-000000000003")
	d1, d2, d3 := feeDebt(o1, "fee-debit", "@fees#default", "50", "50", 1), feeDebt(o2, "fee-debit", "@fees2#default", "80", "80", 2), feeDebt(o3, "fee-debit", "@fees#default", "40", "40", 3)

	t.Run("available funds settle oldest first across two fee accounts, the last partially", func(t *testing.T) {
		f := newStandaloneCollectFixture(t, container.Client, 100, "170", d1.ID, d2.ID, d3.ID)
		f.seedFeeDebts(t, "@source#default", 4, d1, d2, d3)

		raw, result := f.execute(t)
		require.Equal(t, []string{
			"collect fee_debt_debit:0 @source#default debit 50",
			"collect fee_debt_credit:0 @fees#default credit 50",
			"collect fee_debt_debit:1 @source#default debit 50",
			"collect fee_debt_credit:1 @fees2#default credit 50",
		}, movementLines(result))
		require.Equal(t, map[string]string{"@source#default": "0", "@fees#default": "50", "@fees2#default": "50"}, finalAvailable(result))
		require.Equal(t, []string{
			`settled "collect" ` + d1.ID + ` @source#default->@fees#default 50/50 seq 1 USD`,
			`settled "collect" ` + d2.ID + ` @source#default->@fees2#default 50/80 seq 2 USD`,
		}, changeLines(result))
		partial := d2
		partial.Remaining = "30"
		require.Equal(t, []integrationFeeDebt{partial, d3}, f.storedFeeDebts(t, "@source#default").Items)

		f.requireReplay(t, raw)
	})

	t.Run("the collect amount caps what funds would settle", func(t *testing.T) {
		f := newStandaloneCollectFixture(t, container.Client, 100, "60", d1.ID, d2.ID, d3.ID)
		f.seedFeeDebts(t, "@source#default", 4, d1, d2, d3)

		_, result := f.execute(t)
		require.Equal(t, map[string]string{"@source#default": "40", "@fees#default": "50", "@fees2#default": "10"}, finalAvailable(result))
		partial := d2
		partial.Remaining = "70"
		require.Equal(t, []integrationFeeDebt{partial, d3}, f.storedFeeDebts(t, "@source#default").Items)
	})

	t.Run("no available funds settle nothing and write nothing", func(t *testing.T) {
		f := newStandaloneCollectFixture(t, container.Client, 0, "170", d1.ID, d2.ID, d3.ID)
		f.seedFeeDebts(t, "@source#default", 4, d1, d2, d3)
		before := f.capture(t)

		raw, result := f.execute(t)
		require.Empty(t, result.Movements)
		require.Empty(t, result.FeeDebt)
		require.NotContains(t, raw, `"feeDebt"`)
		require.Equal(t, before, f.capture(t), "every key stays byte-identical")
	})
}
