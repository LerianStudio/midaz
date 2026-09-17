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
		owner string
	}{
		{name: "no ownership at all"},
		{name: "ownership of another operation", owner: "another-admission-token"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			protection := sourceAccountProtection(t, f)

			require.NoError(t, container.Client.Del(context.Background(), protection.Ownership).Err())

			if test.owner != "" {
				require.NoError(t, container.Client.Set(context.Background(), protection.Ownership, test.owner, 0).Err())
			}

			before := f.capture(t)
			_, err := f.run(t)
			require.ErrorContains(t, err, `"code":"admission_not_confirmed"`)
			require.Equal(t, before, f.capture(t), "an unconfirmed admission refuses before any accounting write")
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
		strings.Replace(f.resolved.Balances["@source#default"].Balance, "@source#default", unused.BalanceRef, 1))
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

	for _, test := range []struct {
		name string
		key  func(resolvedAccountKeys) string
	}{
		{name: "closing", key: func(p resolvedAccountKeys) string { return p.Closing }},
		{name: "closed", key: func(p resolvedAccountKeys) string { return p.Closed }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			protection := sourceAccountProtection(t, f)
			require.NoError(t, container.Client.Set(context.Background(), test.key(protection), "", time.Hour).Err())

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
