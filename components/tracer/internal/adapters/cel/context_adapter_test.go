// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func testContextAdapter(t testing.TB) *ContextAdapter {
	t.Helper()
	adapter, err := NewContextAdapter(ContextAdapterConfig{Limits: decimalTestLimits(), CostLimit: 100000, MaxExpressionBytes: 5000})
	require.NoError(t, err)

	return adapter
}

func evaluationFacts() tracercontract.Context {
	blocked := false
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	asset := "BTC"
	return tracercontract.Context{
		Accounts: []tracercontract.Account{{ID: id, Type: "deposit", Status: "ACTIVE", Blocked: &blocked, Asset: asset}},
		Entries: []tracercontract.Entry{
			{AccountID: id, Direction: tracercontract.Debit, Amount: "9007199254740993", Asset: asset},
			{AccountID: id, Direction: tracercontract.Debit, Amount: "0.00000001", Asset: asset},
			{AccountID: id, Direction: tracercontract.Credit, Amount: "100", Asset: asset},
			{External: true, Direction: tracercontract.Credit, Amount: "1", Asset: asset},
		},
	}
}

func TestContextAdapterTypedFactsAndExactDebits(t *testing.T) {
	t.Parallel()
	adapter := testContextAdapter(t)
	activation, err := adapter.Prepare(context.Background(), evaluationFacts())
	require.NoError(t, err)
	for _, expression := range []string{
		`accounts.exists(a, a.type == "deposit" && a.status == "ACTIVE" && !a.blocked)`,
		`entries.exists(e, e.direction == "DEBIT" && e.amount.equal(decimal("0.00000001")))`,
		`debits.exists(d, d.asset == "BTC" && d.amount.equal(decimal("9007199254740993.00000001")))`,
		`accounts.exists(a, a.asset == "BTC") && entries.all(e, e.asset == "BTC")`,
		`entries.exists(e, e.external && !has(e.accountId))`,
		`accounts.all(a, has(a.blocked))`,
	} {
		t.Run(expression, func(t *testing.T) {
			program, err := adapter.Compile(context.Background(), expression)
			require.NoError(t, err)
			matched, cost, err := adapter.Evaluate(context.Background(), program, activation, 100000)
			require.NoError(t, err)
			require.True(t, matched)
			require.Positive(t, cost)
		})
	}
}

func TestContextAdapterAssetCodeMatchesExactly(t *testing.T) {
	t.Parallel()
	adapter := testContextAdapter(t)
	activation, err := adapter.Prepare(context.Background(), evaluationFacts())
	require.NoError(t, err)
	for _, expression := range []string{
		`debits.exists(d, d.asset == "XBT")`,
		`debits.exists(d, d.asset == "btc")`,
		`accounts.exists(a, a.asset == "XBT")`,
	} {
		t.Run(expression, func(t *testing.T) {
			t.Parallel()
			program, err := adapter.Compile(context.Background(), expression)
			require.NoError(t, err)
			matched, _, err := adapter.Evaluate(context.Background(), program, activation, 100000)
			require.NoError(t, err)
			require.False(t, matched)
		})
	}
}

func TestContextAdapterRejectsInvalidExpressions(t *testing.T) {
	t.Parallel()
	adapter := testContextAdapter(t)
	for _, expression := range []string{
		`entries.exists(e, e.amount > 0.1)`,
		`entries.exists(e, double(e.amount) > 0.1)`,
		`entries.exists(e, string(e.amount) == "1")`,
		`accounts.exists(a, a.blocked == "false")`,
		`debits.exists(d, d.asset.code == "BTC")`,
		`accounts.exists(a, a.asset.id == "BTC-A")`,
		`amount > 1`,
		`entries[0].amount`,
	} {
		t.Run(expression, func(t *testing.T) {
			_, err := adapter.Compile(context.Background(), expression)
			require.Error(t, err)
		})
	}
}

func TestContextAdapterSnapshotAndPresence(t *testing.T) {
	t.Parallel()
	adapter := testContextAdapter(t)
	facts := evaluationFacts()
	activation, err := adapter.Prepare(context.Background(), facts)
	require.NoError(t, err)
	program, err := adapter.Compile(context.Background(), `!accounts[0].blocked && entries[1].amount.equal(decimal("0.00000001"))`)
	require.NoError(t, err)
	*facts.Accounts[0].Blocked = true
	facts.Entries[1].Amount = "99"
	matched, _, err := adapter.Evaluate(context.Background(), program, activation, 100000)
	require.NoError(t, err)
	require.True(t, matched)
	facts.Accounts[0].Blocked = nil
	_, err = adapter.Prepare(context.Background(), facts)
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
}

func TestContextAdapterBudgetsAndCacheIsolation(t *testing.T) {
	t.Parallel()
	adapter := testContextAdapter(t)
	activation, err := adapter.Prepare(context.Background(), evaluationFacts())
	require.NoError(t, err)
	program, err := adapter.Compile(context.Background(), `entries.all(e, e.amount.greaterThan(decimal("0")))`)
	require.NoError(t, err)
	matched, _, err := adapter.Evaluate(context.Background(), program, activation, 1)
	require.ErrorIs(t, err, constant.ErrExpressionCostExceeded)
	require.False(t, matched)
	matched, _, err = adapter.Evaluate(context.Background(), program, activation, 100000)
	require.NoError(t, err)
	require.True(t, matched)
	other := testContextAdapter(t)
	_, _, err = other.Evaluate(context.Background(), program, activation, 100000)
	require.Error(t, err, "programs cannot bypass another adapter's limits")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = adapter.Evaluate(ctx, program, activation, 100000)
	require.ErrorIs(t, err, context.Canceled)
	_, _, err = adapter.Evaluate(context.Background(), program, activation, 0)
	require.ErrorIs(t, err, constant.ErrExpressionCostExceeded)

	// Cached programs and prepared activations are immutable between evaluations.
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			matched, _, evalErr := adapter.Evaluate(context.Background(), program, activation, 100000)
			if evalErr != nil || !matched {
				t.Errorf("concurrent evaluation: matched=%v err=%v", matched, evalErr)
			}
		})
	}
	workers.Wait()
}

func TestContextAdapterDoesNotExposeExpressionLiterals(t *testing.T) {
	t.Parallel()
	adapter := testContextAdapter(t)
	activation, err := adapter.Prepare(context.Background(), evaluationFacts())
	require.NoError(t, err)
	program, err := adapter.Compile(context.Background(), `{"present": 1}["sensitive-missing-key"] == 1`)
	require.NoError(t, err)
	_, _, err = adapter.Evaluate(context.Background(), program, activation, 100000)
	require.ErrorIs(t, err, constant.ErrExpressionEvaluation)
	require.NotContains(t, err.Error(), "sensitive-missing-key")
}
