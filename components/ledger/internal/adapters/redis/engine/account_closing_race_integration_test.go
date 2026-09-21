//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// addCounterparty appends a second account to the pool and credits it, so an
// execution carries two legs over two different accounts. It is what makes "no leg
// of a refused execution is applied" observable: the refusal is decided over one
// account and the other account's money must be untouched all the same.
func (f *integrationFixture) addCounterparty(t *testing.T, alias string, balanceID, accountID uuid.UUID, amount string) accounting.BalanceSnapshot {
	t.Helper()

	balance := f.input.Execution.Balances[0]
	balance.ID, balance.AccountID = balanceID, accountID
	balance.Alias, balance.Key, balance.BalanceRef = alias, "default", alias+"#default"
	balance.Direction, balance.BalanceScope = "credit", "transactional"
	balance.Available, balance.OnHold, balance.OverdraftUsed = decimal.Zero, decimal.Zero, decimal.Zero
	balance.Version, balance.AllowOverdraft = 0, false

	f.input.Execution.Balances = append(f.input.Execution.Balances, balance)
	f.resolved.Balances[balance.BalanceRef] = testResolvedBalanceKeys(
		strings.Replace(f.resolved.Balances["@source#default"].Balance, "@source#default", balance.BalanceRef, 1))

	f.input.Execution.Transactions[0].Postings = append(f.input.Execution.Transactions[0].Postings, accounting.Posting{
		Ref: "credit-0", BalanceRef: balance.BalanceRef, Type: accounting.PostingCredit,
		Amount: decimal.RequireFromString(amount), DrawPolicy: accounting.DrawForbidden, OverdraftAmount: decimal.Zero,
	})

	f.syncAccountProtection(t)

	return balance
}

// balanceOf reads one cached balance blob, reporting whether the key exists.
func (f *integrationFixture) balanceOf(t *testing.T, ref string) (string, bool) {
	t.Helper()

	value, err := f.client.Get(context.Background(), f.resolved.Balances[ref].Balance).Result()
	if errors.Is(err, redis.Nil) {
		return "", false
	}

	require.NoError(t, err)

	return value, true
}

// TestIntegrationAccountClosingRefusesAnExecutionPreparedBeforeTheProtection is
// AS-02 with an explicit barrier instead of a delay: the payload is prepared while
// the account is open, the closing marker is installed, and only then is the
// prepared request executed. It must be refused before any write, and no leg of the
// batch — not even the counterparty the closing says nothing about — may move.
func TestIntegrationAccountClosingRefusesAnExecutionPreparedBeforeTheProtection(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	counterparty := f.addCounterparty(t,
		"@counterparty",
		uuid.MustParse("3f1d3ba6-3f8b-4a0b-9d63-45f1a0b0c001"),
		uuid.MustParse("3f1d3ba6-3f8b-4a0b-9d63-45f1a0b0c002"),
		"30")

	protection := sourceAccountProtection(t, f)

	// Barrier: the request is fully prepared while the account is still open.
	prepared := f.prepared(t)

	// Barrier: the protection lands between the preparation and the execution.
	require.NoError(t, container.Client.Set(context.Background(), protection.Closing, "closing-attempt-token", 0).Err())

	before := f.capture(t)
	_, err := f.runRaw(t, string(prepared.Payload))
	require.ErrorContains(t, err, `"code":"account_closing_in_progress"`)
	require.Equal(t, before, f.capture(t), "a closing in progress refuses every leg before any accounting write")

	_, exists := f.balanceOf(t, counterparty.BalanceRef)
	require.False(t, exists, "a refused batch admits no balance of its counterparty either")
}

// TestIntegrationAccountClosingRefusesARetainedSnapshotBeyondTheNegativeCache is
// AS-17: a request read its snapshot while the account was open and was then held.
// Meanwhile the closing recorded the instant, evicted the cached balances, released
// its protection and the negative cache expired. The held request must still be
// refused before any mutation, and it must not recreate the balance it carries.
//
// The five minutes of the negative cache are never waited for: expiry is what a
// missing key IS, so the key is simply removed.
func TestIntegrationAccountClosingRefusesARetainedSnapshotBeyondTheNegativeCache(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	protection := sourceAccountProtection(t, f)

	// Barrier: the request is prepared from the snapshot of an open account.
	prepared := f.prepared(t)

	// The closing runs to the end: the balances are evicted, the negative cache is
	// installed and the closing marker and the ownership are given back.
	require.NoError(t, container.Client.Del(context.Background(),
		f.resolved.Balances["@source#default"].Balance, protection.Closing, protection.Ownership).Err())
	require.NoError(t, container.Client.Set(context.Background(), protection.Closed, accountClosingInstant, time.Hour).Err())

	// The negative cache expires. An expired denial is an absent key, not an open
	// account, and the held request must not profit from it.
	require.NoError(t, container.Client.Del(context.Background(), protection.Closed).Err())

	before := f.capture(t)
	_, err := f.runRaw(t, string(prepared.Payload))
	require.ErrorContains(t, err, `"code":"admission_not_confirmed"`)
	require.Equal(t, before, f.capture(t), "a retained snapshot applies no accounting")

	_, exists := f.balanceOf(t, "@source#default")
	require.False(t, exists, "an expired denial never readmits the evicted balance")
}

// TestIntegrationAccountClosingRefusesASnapshotWhoseCacheVanishedBeforeExecution is
// AS-18: the request prepared over a warm cache, so it took no administrative
// ownership, and the balance it uses disappeared before the execution reached the
// engine. The engine must refuse with the admission code before any write and must
// not turn the prepared snapshot into a new seed; a retry belongs to a fresh
// authoritative load, never to this request.
func TestIntegrationAccountClosingRefusesASnapshotWhoseCacheVanishedBeforeExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	protection := sourceAccountProtection(t, f)

	// A warm cache is what a preparation reads without owning anything.
	f.seed(t, 0, f.input.Execution.Balances[0])
	require.NoError(t, container.Client.Del(context.Background(), protection.Ownership).Err())

	prepared := f.prepared(t)

	// Barrier: the blob vanishes between the preparation and the execution.
	require.NoError(t, container.Client.Del(context.Background(), f.resolved.Balances["@source#default"].Balance).Err())

	before := f.capture(t)
	_, err := f.runRaw(t, string(prepared.Payload))
	require.ErrorContains(t, err, `"code":"admission_not_confirmed"`)
	require.Equal(t, before, f.capture(t), "an unconfirmed admission refuses before any accounting write")

	_, exists := f.balanceOf(t, "@source#default")
	require.False(t, exists, "the vanished balance is not recreated from the prepared snapshot")
}

// AS-05 and AS-06 at this seam — a held admission still refused by the marker the
// closing installed — are covered by
// TestIntegrationAccountClosingRefusesAClosedAccountBeforeAnyWrite and
// TestIntegrationAccountClosingRefusesAnAccountBeingClosed in
// account_closing_integration_test.go, which assert the same refusals and carry the
// "no balance is seeded" claim explicitly.
