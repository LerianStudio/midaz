//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// accountClosingInstant is the fixed closing instant every test that needs one
// writes into the negative cache. A closing is compared, never measured, so the
// tests carry no clock of their own.
const accountClosingInstant = "2026-09-17T12:00:00.000000001Z"

// sourceAccountProtection returns the closing controls of the account behind the
// fixture's @source balance.
func sourceAccountProtection(t *testing.T, f *integrationFixture) resolvedAccountKeys {
	t.Helper()

	f.syncAccountProtection(t)

	protection, exists := f.resolved.Accounts[f.input.Execution.Balances[0].AccountID]
	require.True(t, exists)

	return protection
}

func TestIntegrationAccountClosingRefusesAClosedAccountBeforeAnyWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	protection := sourceAccountProtection(t, f)
	require.NoError(t, container.Client.Set(context.Background(), protection.Closed, accountClosingInstant, time.Hour).Err())

	before := f.capture(t)
	_, err := f.run(t)
	require.ErrorContains(t, err, `"code":"account_closed"`)
	require.Equal(t, before, f.capture(t), "a closed account refuses before any accounting write")
	require.Equal(t, int64(0), container.Client.Exists(context.Background(), f.resolved.Balances["@source#default"].Balance).Val(),
		"a refused admission seeds no balance")
}

func TestIntegrationAccountClosingRefusesAnAccountBeingClosed(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	protection := sourceAccountProtection(t, f)
	require.NoError(t, container.Client.Set(context.Background(), protection.Closing, "closing-attempt-token", 0).Err())

	before := f.capture(t)
	_, err := f.run(t)
	require.ErrorContains(t, err, `"code":"account_closing_in_progress"`)
	require.Equal(t, before, f.capture(t), "a closing in progress refuses before any accounting write")
	require.Equal(t, int64(0), container.Client.Exists(context.Background(), f.resolved.Balances["@source#default"].Balance).Val(),
		"a refused admission seeds no balance, so the list the closing is validating cannot grow")
}

func TestIntegrationAccountClosingAdmitsAProtectedSeedOnAColdCache(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	protection := sourceAccountProtection(t, f)

	require.Equal(t, int64(0), container.Client.Exists(context.Background(), protection.Closing, protection.Closed).Val(),
		"an open account owns no marker")

	raw, err := f.run(t)
	require.NoError(t, err)

	result := decodeIntegrationResult(t, raw)
	require.Equal(t, "70", result.Final[0].Available)
}

func TestIntegrationAccountClosingRefusesAnUnprotectedSeed(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	for _, test := range []struct {
		name  string
		shape func(t *testing.T, client redis.UniversalClient, ownership string)
	}{
		{name: "no ownership at all", shape: func(*testing.T, redis.UniversalClient, string) {}},
		{name: "only admissions of other loads", shape: func(t *testing.T, client redis.UniversalClient, ownership string) {
			admitSharedSeeds(t, client, ownership, "another-admission-token", "a-third-admission-token")
		}},
		{name: "the caller's admission was released while another stays live", shape: func(t *testing.T, client redis.UniversalClient, ownership string) {
			admitSharedSeeds(t, client, ownership, testAdmissionToken, "another-admission-token")
			require.NoError(t, client.ZRem(context.Background(), ownership, testAdmissionToken).Err())
		}},
		{name: "an exclusive owner of another operation", shape: func(t *testing.T, client redis.UniversalClient, ownership string) {
			require.NoError(t, client.Set(context.Background(), ownership, "another-admission-token", 0).Err())
		}},
		// An exclusive owner excludes every seed, even one naming the caller's own
		// token, so an exclusive admission an older release took can never stand in
		// for a shared one.
		{name: "an exclusive owner carrying the caller's token", shape: func(t *testing.T, client redis.UniversalClient, ownership string) {
			require.NoError(t, client.Set(context.Background(), ownership, testAdmissionToken, 0).Err())
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			protection := sourceAccountProtection(t, f)

			require.NoError(t, container.Client.Del(context.Background(), protection.Ownership).Err())
			test.shape(t, container.Client, protection.Ownership)

			before := f.capture(t)
			_, err := f.run(t)
			require.ErrorContains(t, err, `"code":"admission_not_confirmed"`)
			require.Equal(t, before, f.capture(t), "an unconfirmed admission refuses before any accounting write")
			require.Zero(t, container.Client.Exists(context.Background(), f.resolved.Balances["@source#default"].Balance).Val(),
				"an unconfirmed admission seeds no balance")
		})
	}
}

func TestIntegrationAccountClosingKeepsAWarmBalanceIndependentOfTheAdmission(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	protection := sourceAccountProtection(t, f)

	f.seed(t, 0, f.input.Execution.Balances[0])
	require.NoError(t, container.Client.Del(context.Background(), protection.Ownership).Err())

	raw, err := f.run(t)
	require.NoError(t, err)

	result := decodeIntegrationResult(t, raw)
	require.Equal(t, "70", result.Final[0].Available)
}

func TestIntegrationAccountClosingRepeatsTheCheckOnACompanionItMoves(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	f.addCompanion("0")

	// The primary is warm and the companion is not: only the debt this posting
	// creates reaches the companion, and its seed is admitted at that mutation site.
	overdrawn := f.input.Execution.Balances[0]
	overdrawn.Available = decimal.NewFromInt(10)
	overdrawn.OverdraftLimit = decimal.NewFromInt(1000)
	overdrawn.OverdraftLimitEnabled = true
	f.input.Execution.Balances[0] = overdrawn
	f.seed(t, 0, overdrawn)

	protection := sourceAccountProtection(t, f)
	require.NoError(t, container.Client.Del(context.Background(), protection.Ownership).Err())

	before := f.capture(t)
	_, err := f.run(t)
	require.ErrorContains(t, err, `"code":"admission_not_confirmed"`)
	require.Equal(t, before, f.capture(t), "a companion seed is admitted under the same protection as its primary")
}

func TestIntegrationAccountClosingLeavesAnUnusedPoolBalanceAlone(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)

	// The balance belongs to the pool but to no requirement and no posting. Its
	// account is closed, and that must not refuse an execution that never uses it.
	unused := f.input.Execution.Balances[0]
	unused.ID = uuid.MustParse("6a44f6c6-8f83-4a6f-9c4f-1a0c2df5f5a1")
	unused.AccountID = uuid.MustParse("7c2ec6a8-0f1a-4a63-9a83-9e2b6fd3c1a4")
	unused.Alias, unused.BalanceRef = "@unused", "@unused#default"
	f.input.Execution.Balances = append(f.input.Execution.Balances, unused)
	f.resolved.Balances[unused.BalanceRef] = testResolvedBalanceKeys(
		strings.Replace(f.resolved.Balances["@source#default"].Balance, "@source#default", unused.BalanceRef, 1),
	)
	f.seed(t, 1, unused)

	f.syncAccountProtection(t)
	closedAccount := f.resolved.Accounts[unused.AccountID]
	require.NoError(t, container.Client.Set(context.Background(), closedAccount.Closed, accountClosingInstant, time.Hour).Err())

	raw, err := f.run(t)
	require.NoError(t, err)

	result := decodeIntegrationResult(t, raw)
	require.Len(t, result.Final, 1)
	require.Equal(t, "70", result.Final[0].Available)
}

func TestIntegrationAccountClosingPreservesAProvenReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	protection := sourceAccountProtection(t, f)

	applied, err := f.run(t)
	require.NoError(t, err)

	// The account is closed afterwards: its balances are evicted, the negative
	// cache expires and the admission ends. None of that turns the recorded
	// outcome into a new movement.
	require.NoError(t, container.Client.Del(context.Background(),
		f.resolved.Balances["@source#default"].Balance, protection.Ownership, protection.Closed).Err())

	before := f.capture(t)
	replayed, err := f.run(t)
	require.NoError(t, err)
	require.Equal(t, applied, replayed)
	require.Equal(t, before, f.capture(t), "a proven replay applies no accounting and refreshes no expiry")
}

func TestIntegrationAccountClosingRefusesAnUnreadableMarker(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	blank := func(key func(resolvedAccountKeys) string) func(*testing.T, redis.UniversalClient, resolvedAccountKeys) {
		return func(t *testing.T, client redis.UniversalClient, protection resolvedAccountKeys) {
			require.NoError(t, client.Del(context.Background(), key(protection)).Err())
			require.NoError(t, client.Set(context.Background(), key(protection), "", time.Hour).Err())
		}
	}

	for _, test := range []struct {
		name  string
		shape func(*testing.T, redis.UniversalClient, resolvedAccountKeys)
	}{
		{name: "closing", shape: blank(func(p resolvedAccountKeys) string { return p.Closing })},
		{name: "closed", shape: blank(func(p resolvedAccountKeys) string { return p.Closed })},
		{name: "blank exclusive owner", shape: blank(func(p resolvedAccountKeys) string { return p.Ownership })},
		// The ownership key is either an exclusive owner or the shared admissions;
		// any other shape is a failure of the protection surface, not an absence.
		{name: "ownership of an unexpected type", shape: func(t *testing.T, client redis.UniversalClient, protection resolvedAccountKeys) {
			require.NoError(t, client.Del(context.Background(), protection.Ownership).Err())
			require.NoError(t, client.HSet(context.Background(), protection.Ownership, "member", testAdmissionToken).Err())
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			protection := sourceAccountProtection(t, f)
			test.shape(t, container.Client, protection)

			before := f.capture(t)
			_, err := f.run(t)
			require.ErrorContains(t, err, `"code":"account_protection_unreadable"`)
			require.Equal(t, before, f.capture(t), "an unreadable marker is refused, never read as absence")
		})
	}
}

func TestIntegrationAccountClosingIsolatesTheScopeOfItsControls(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	protection := sourceAccountProtection(t, f)

	// The same account identifier under another organization and another ledger
	// carries its own controls, and a closing recorded there reaches nothing here.
	accountID := f.input.Execution.Balances[0].AccountID
	for _, foreign := range []string{
		"account-closed:{transactions}:11111111-1111-4111-8111-111111111111:" + f.input.Execution.LedgerID.String() + ":" + accountID.String(),
		"account-closed:{transactions}:" + f.input.Execution.OrganizationID.String() + ":22222222-2222-4222-8222-222222222222:" + accountID.String(),
	} {
		key := f.prefix + foreign
		require.NoError(t, container.Client.Set(context.Background(), key, accountClosingInstant, time.Hour).Err())
		t.Cleanup(func() { container.Client.Del(context.Background(), key) })
	}

	require.Equal(t, int64(0), container.Client.Exists(context.Background(), protection.Closed).Val())

	raw, err := f.run(t)
	require.NoError(t, err)
	require.Equal(t, "70", decodeIntegrationResult(t, raw).Final[0].Available)
}

func TestIntegrationAccountClosingIsNotBypassedByABlockException(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	f.input.Execution.Transactions[0].RejectBlockedBalances = true

	blocked := f.input.Execution.Balances[0]
	blocked.Blocked = true
	f.seed(t, 0, blocked)

	grantKey := f.addGrant(t, "@source", "30")
	protection := sourceAccountProtection(t, f)
	require.NoError(t, container.Client.Set(context.Background(), protection.Closed, accountClosingInstant, time.Hour).Err())

	before := f.capture(t)
	_, err := f.run(t)
	require.ErrorContains(t, err, `"code":"account_closed"`)
	require.Equal(t, before, f.capture(t))
	require.Equal(t, int64(1), container.Client.Exists(context.Background(), grantKey).Val(),
		"a refused execution consumes no single-use grant")
}

// TestIntegrationAccountClosingAnswersBeforeALimitRepairIsRequested proves the
// closing controls decide before the balance pool is read: a malformed cached
// limit would otherwise request a precommit repair, and a closed account must not
// see even that write attempted on its behalf.
func TestIntegrationAccountClosingAnswersBeforeALimitRepairIsRequested(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	protection := sourceAccountProtection(t, f)

	encoded, err := balancecache.Encode(f.input.Execution.Balances[0], balancecache.FormatDual)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))

	fields["OverdraftLimit"], err = json.Marshal("1E+3")
	require.NoError(t, err)

	encoded, err = json.Marshal(fields)
	require.NoError(t, err)

	key := f.resolved.Balances["@source#default"].Balance
	require.NoError(t, container.Client.Set(context.Background(), key, encoded, time.Hour).Err())
	require.NoError(t, container.Client.Set(context.Background(), protection.Closed, accountClosingInstant, time.Hour).Err())

	before := f.capture(t)
	_, err = f.run(t)
	require.ErrorContains(t, err, `"code":"account_closed"`)
	require.NotContains(t, err.Error(), "BALANCE_LIMIT_NORMALIZATION_REQUIRED:")
	require.Equal(t, before, f.capture(t))
}
