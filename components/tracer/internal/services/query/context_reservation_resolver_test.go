// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func contextReservationFixture() (tracercontract.Context, []model.ContextAccountLimit, ContextReservationConfig) {
	a, b := testutil.MustDeterministicUUID(80101), testutil.MustDeterministicUUID(80102)
	asset := "TOKEN"
	blocked := false
	facts := tracercontract.Context{
		Accounts: []tracercontract.Account{
			{ID: a, Type: "checking", Status: "ACTIVE", Blocked: &blocked, Asset: asset},
			{ID: b, Type: "fee", Status: "ACTIVE", Blocked: &blocked, Asset: asset},
		},
		Entries: []tracercontract.Entry{
			{AccountID: a, Direction: tracercontract.Debit, Amount: "10.125", Asset: asset},
			{AccountID: a, Direction: tracercontract.Debit, Amount: "0.005", Asset: asset},
			{AccountID: a, Direction: tracercontract.Credit, Amount: "10.13", Asset: asset},
			{AccountID: b, Direction: tracercontract.Debit, Amount: "0.01", Asset: asset},
			{External: true, Direction: tracercontract.Debit, Amount: "900", Asset: asset},
		},
	}
	limits := []model.ContextAccountLimit{{Definition: model.Limit{
		ID: testutil.MustDeterministicUUID(80201), Asset: asset, Status: model.LimitStatusActive,
		LimitType: model.LimitTypeDaily, MaxAmount: decimal.RequireFromString("20"),
		Scopes: []model.Scope{{AccountID: &b}, {AccountID: &a}},
	}}}
	return facts, limits, ContextReservationConfig{
		Facts:     tracercontract.Limits{MaxAccounts: 10, MaxEntries: 30, MaxTextBytes: 100, MaxIntegerDigits: 20, MaxFractionDigits: 10},
		MaxLimits: 10, MaxScopesPerLimit: 10, MaxReservations: 20,
	}
}

func TestContextReservationsGrossDebitsAndStableCoordinates(t *testing.T) {
	facts, limits, config := contextReservationFixture()
	at := time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC)
	stale := at.AddDate(-1, 0, 0)
	limits[0].Definition.ResetAt = &stale
	resolver, err := NewContextReservationResolver(clock.NewFixedClock(at), config)
	require.NoError(t, err)
	plan, err := resolver.Execute(t.Context(), facts, limits)
	require.NoError(t, err)
	require.False(t, plan.Denied)
	require.Len(t, plan.Reservations, 2)
	require.Len(t, plan.AccountIDs, 2)
	amounts := map[string]string{}
	for _, spec := range plan.Reservations {
		amounts[spec.ScopeKey] = spec.Amount.String()
		require.Equal(t, limits[0].Definition.ID, spec.LimitID)
		require.Equal(t, "2026-09-24", spec.PeriodKey)
		require.True(t, spec.CounterExpiresAt.After(at))
	}
	require.Equal(t, map[string]string{"acct:" + facts.Accounts[0].ID.String(): "10.13", "acct:" + facts.Accounts[1].ID.String(): "0.01"}, amounts)
	slices.Reverse(facts.Accounts)
	slices.Reverse(facts.Entries)
	slices.Reverse(limits[0].Definition.Scopes)
	again, err := resolver.Execute(t.Context(), facts, limits)
	require.NoError(t, err)
	require.Equal(t, plan, again)
}

func TestContextReservationsAssetCode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*model.ContextAccountLimit)
		bad    bool
		count  int
	}{
		{name: "exact code", change: func(*model.ContextAccountLimit) {}, count: 2},
		{name: "scope accounts debited in another code", change: func(l *model.ContextAccountLimit) { l.Definition.Asset = "OTHER" }, count: 0},
		{name: "missing code", change: func(l *model.ContextAccountLimit) { l.Definition.Asset = "" }, bad: true},
		{name: "lowercase code", change: func(l *model.ContextAccountLimit) { l.Definition.Asset = "token" }, bad: true},
		{name: "code longer than the asset code rule", change: func(l *model.ContextAccountLimit) { l.Definition.Asset = strings.Repeat("A", 101) }, bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			facts, limits, config := contextReservationFixture()
			tc.change(&limits[0])
			resolver, err := NewContextReservationResolver(clock.NewFixedClock(testutil.FixedTime()), config)
			require.NoError(t, err)
			plan, err := resolver.Execute(t.Context(), facts, limits)
			if tc.bad {
				require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
				require.Nil(t, plan)
				return
			}
			require.NoError(t, err)
			require.False(t, plan.Denied)
			require.Len(t, plan.Reservations, tc.count)
		})
	}
}

func TestContextReservationsCompleteConfigurationBeforeDenial(t *testing.T) {
	cases := []struct {
		name   string
		change func(*model.ContextAccountLimit)
	}{
		{"global", func(l *model.ContextAccountLimit) { l.Definition.Scopes = nil }},
		{"empty scope", func(l *model.ContextAccountLimit) { l.Definition.Scopes = []model.Scope{{}} }},
		{"segment", func(l *model.ContextAccountLimit) {
			id := testutil.MustDeterministicUUID(80999)
			l.Definition.Scopes[0].SegmentID = &id
		}},
		{"nil account", func(l *model.ContextAccountLimit) { l.Definition.Scopes[0].AccountID = nil }},
		{"zero account", func(l *model.ContextAccountLimit) { id := uuid.Nil; l.Definition.Scopes[0].AccountID = &id }},
		{"duplicate account", func(l *model.ContextAccountLimit) {
			l.Definition.Scopes = append(l.Definition.Scopes, l.Definition.Scopes[0])
		}},
		{"invalid type", func(l *model.ContextAccountLimit) { l.Definition.LimitType = "UNKNOWN" }},
		{"inactive", func(l *model.ContextAccountLimit) { l.Definition.Status = model.LimitStatusInactive }},
		{"zero amount", func(l *model.ContextAccountLimit) { l.Definition.MaxAmount = decimal.Zero }},
		{"half window", func(l *model.ContextAccountLimit) {
			v, _ := model.NewTimeOfDay("09:00")
			l.Definition.ActiveTimeStart = &v
		}},
		{"invalid window", func(l *model.ContextAccountLimit) {
			v := model.TimeOfDay{}
			w, _ := model.NewTimeOfDay("09:00")
			l.Definition.ActiveTimeStart = &v
			l.Definition.ActiveTimeEnd = &w
		}},
		{"missing custom dates", func(l *model.ContextAccountLimit) { l.Definition.LimitType = model.LimitTypeCustom }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts, limits, config := contextReservationFixture()
			broken := limits[0]
			broken.Definition.ID = testutil.MustDeterministicUUID(80202)
			broken.Definition.Scopes = slices.Clone(broken.Definition.Scopes)
			tc.change(&broken)
			limits[0].Definition.MaxAmount = decimal.RequireFromString("0.001")
			limits = append(limits, broken)
			resolver, err := NewContextReservationResolver(clock.NewFixedClock(testutil.FixedTime()), config)
			require.NoError(t, err)
			plan, err := resolver.Execute(t.Context(), facts, limits)
			require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
			require.Nil(t, plan)
		})
	}
}

func TestContextReservationsCapAndResourceBounds(t *testing.T) {
	for _, kind := range []model.LimitType{model.LimitTypeDaily, model.LimitTypePerTransaction} {
		t.Run(string(kind), func(t *testing.T) {
			facts, limits, config := contextReservationFixture()
			limits[0].Definition.LimitType = kind
			limits[0].Definition.MaxAmount = decimal.RequireFromString("10.13")
			resolver, err := NewContextReservationResolver(clock.NewFixedClock(testutil.FixedTime()), config)
			require.NoError(t, err)
			plan, err := resolver.Execute(t.Context(), facts, limits)
			require.NoError(t, err)
			require.False(t, plan.Denied)
			if kind == model.LimitTypePerTransaction {
				require.Empty(t, plan.Reservations)
			}
			limits[0].Definition.MaxAmount = decimal.RequireFromString("10.129")
			plan, err = resolver.Execute(t.Context(), facts, limits)
			require.NoError(t, err)
			require.True(t, plan.Denied)
			require.Empty(t, plan.Reservations)
			require.Equal(t, []uuid.UUID{limits[0].Definition.ID}, plan.ExceededLimitIDs)
		})
	}
	for _, tc := range []struct {
		name   string
		change func(*ContextReservationConfig)
	}{
		{"limits", func(c *ContextReservationConfig) { c.MaxLimits = 1 }},
		{"scopes", func(c *ContextReservationConfig) { c.MaxScopesPerLimit = 1 }},
		{"reservations", func(c *ContextReservationConfig) { c.MaxReservations = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts, limits, config := contextReservationFixture()
			tc.change(&config)
			if tc.name == "limits" {
				other := limits[0]
				other.Definition.ID = testutil.MustDeterministicUUID(80202)
				limits = append(limits, other)
			}
			resolver, err := NewContextReservationResolver(clock.NewFixedClock(testutil.FixedTime()), config)
			require.NoError(t, err)
			plan, err := resolver.Execute(t.Context(), facts, limits)
			require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
			require.Nil(t, plan)
		})
	}
}

func TestContextReservationsCancellationAndNoLimits(t *testing.T) {
	facts, _, config := contextReservationFixture()
	resolver, err := NewContextReservationResolver(clock.NewFixedClock(testutil.FixedTime()), config)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	plan, err := resolver.Execute(ctx, facts, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, plan)
	plan, err = resolver.Execute(t.Context(), facts, nil)
	require.NoError(t, err)
	require.False(t, plan.Denied)
	require.Empty(t, plan.Reservations)
	require.Len(t, plan.AccountIDs, 2)
	facts.Entries[0].Amount = "-1"
	plan, err = resolver.Execute(t.Context(), facts, nil)
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	require.Nil(t, plan)
}

func TestContextReservationsPeriodsAndServerTime(t *testing.T) {
	at := time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		kind   model.LimitType
		period string
		reset  time.Time
	}{
		{model.LimitTypeDaily, "2026-09-24", time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)},
		{model.LimitTypeWeekly, "2026-W39", time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)},
		{model.LimitTypeMonthly, "2026-09", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		{model.LimitTypeCustom, "custom", at.Add(time.Hour)},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			facts, limits, config := contextReservationFixture()
			limits[0].Definition.LimitType = tc.kind
			if tc.kind == model.LimitTypeCustom {
				start := at.Add(-time.Hour)
				end := tc.reset
				limits[0].Definition.CustomStartDate = &start
				limits[0].Definition.CustomEndDate = &end
			}
			resolver, err := NewContextReservationResolver(clock.NewFixedClock(at), config)
			require.NoError(t, err)
			plan, err := resolver.Execute(t.Context(), facts, limits)
			require.NoError(t, err)
			require.Len(t, plan.Reservations, 2)
			for _, spec := range plan.Reservations {
				require.Equal(t, tc.period, spec.PeriodKey)
				// The existing counter retention policy is independent of hold TTL.
				require.Equal(t, tc.reset.AddDate(0, 0, trcConstant.CounterRetentionDays), spec.CounterExpiresAt)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		now    time.Time
		custom bool
		count  int
	}{
		{"window start", at, false, 2},
		{"window end", at.Add(time.Hour), false, 0},
		{"custom start", at, true, 2},
		{"custom end", at.Add(time.Hour), true, 0},
		{"before custom", at.Add(-time.Minute), true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts, limits, config := contextReservationFixture()
			if tc.custom {
				limits[0].Definition.LimitType = model.LimitTypeCustom
				start, end := at, at.Add(time.Hour)
				limits[0].Definition.CustomStartDate = &start
				limits[0].Definition.CustomEndDate = &end
			} else {
				start, err := model.NewTimeOfDay("15:00")
				require.NoError(t, err)
				end, err := model.NewTimeOfDay("16:00")
				require.NoError(t, err)
				limits[0].Definition.ActiveTimeStart = &start
				limits[0].Definition.ActiveTimeEnd = &end
			}
			resolver, err := NewContextReservationResolver(clock.NewFixedClock(tc.now), config)
			require.NoError(t, err)
			plan, err := resolver.Execute(t.Context(), facts, limits)
			require.NoError(t, err)
			require.Len(t, plan.Reservations, tc.count)
		})
	}
}

func TestContextReservationsOrderingAndDistinctAssets(t *testing.T) {
	facts, limits, config := contextReservationFixture()
	a, b := facts.Accounts[0].ID, facts.Accounts[1].ID
	facts.Accounts[1].Asset = "FEE"
	facts.Entries[3].Asset = facts.Accounts[1].Asset
	limits[0].Definition.Scopes = []model.Scope{{AccountID: &a}}
	other := limits[0]
	other.Definition.ID = testutil.MustDeterministicUUID(80202)
	other.Definition.Asset = facts.Accounts[1].Asset
	other.Definition.Scopes = []model.Scope{{AccountID: &b}}
	limits = append(limits, other)
	resolver, err := NewContextReservationResolver(clock.NewFixedClock(testutil.FixedTime()), config)
	require.NoError(t, err)
	plan, err := resolver.Execute(t.Context(), facts, limits)
	require.NoError(t, err)
	require.Len(t, plan.Reservations, 2)
	require.True(t, bytes.Compare(plan.Reservations[0].LimitID[:], plan.Reservations[1].LimitID[:]) < 0)
	require.True(t, bytes.Compare(plan.AccountIDs[0][:], plan.AccountIDs[1][:]) < 0)
	slices.Reverse(limits)
	again, err := resolver.Execute(t.Context(), facts, limits)
	require.NoError(t, err)
	require.Equal(t, plan, again)
	// Returned values must not alias the policy/facts or subsequent plans.
	plan.AccountIDs[0] = uuid.Nil
	plan.Reservations[0].Amount = decimal.Zero
	after, err := resolver.Execute(t.Context(), facts, limits)
	require.NoError(t, err)
	require.Equal(t, again, after)
	limits = append(limits, limits[0])
	plan, err = resolver.Execute(t.Context(), facts, limits)
	require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
	require.Nil(t, plan)
}

func TestContextReservationsNoDebitAndInvalidConfig(t *testing.T) {
	facts, limits, config := contextReservationFixture()
	resolver, err := NewContextReservationResolver(clock.NewFixedClock(testutil.FixedTime()), config)
	require.NoError(t, err)
	for i := range facts.Entries {
		facts.Entries[i].Direction = tracercontract.Credit
	}
	plan, err := resolver.Execute(t.Context(), facts, limits)
	require.NoError(t, err)
	require.Empty(t, plan.AccountIDs)
	require.Empty(t, plan.Reservations)
	facts.Accounts = nil
	facts.Entries = facts.Entries[len(facts.Entries)-1:]
	facts.Entries[0].Direction = tracercontract.Debit
	plan, err = resolver.Execute(t.Context(), facts, limits)
	require.NoError(t, err)
	require.Empty(t, plan.AccountIDs)
	require.Empty(t, plan.Reservations)
	_, err = NewContextReservationResolver(nil, config)
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	for _, change := range []func(*ContextReservationConfig){
		func(c *ContextReservationConfig) { c.MaxLimits = 0 }, func(c *ContextReservationConfig) { c.MaxScopesPerLimit = 0 },
		func(c *ContextReservationConfig) { c.MaxReservations = 0 }, func(c *ContextReservationConfig) { c.Facts.MaxEntries = 0 },
	} {
		bad := config
		change(&bad)
		_, err := NewContextReservationResolver(clock.NewFixedClock(testutil.FixedTime()), bad)
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	}
}

func TestContextReservationsScopeAccountOfAnotherCodeIsOutsideLimit(t *testing.T) {
	t.Parallel()
	facts, limits, config := contextReservationFixture()
	a := facts.Accounts[0].ID
	facts.Accounts[1].Asset = "FEE"
	facts.Entries[3].Asset = facts.Accounts[1].Asset
	resolver, err := NewContextReservationResolver(clock.NewFixedClock(testutil.FixedTime()), config)
	require.NoError(t, err)
	plan, err := resolver.Execute(t.Context(), facts, limits)
	require.NoError(t, err)
	require.False(t, plan.Denied)
	require.Len(t, plan.Reservations, 1)
	require.Equal(t, "acct:"+a.String(), plan.Reservations[0].ScopeKey)
}

func TestContextReservationsDebitOfAnotherCodeIsNotLimited(t *testing.T) {
	t.Parallel()
	facts, limits, config := contextReservationFixture()
	a, b := facts.Accounts[0].ID, facts.Accounts[1].ID
	facts.Accounts[1].Asset = "FEE"
	facts.Entries[3].Asset = "FEE"
	facts.Entries[3].Amount = "900"
	// b stays in the TOKEN limit's scope; its FEE debit exceeds the TOKEN cap
	// and must neither be reserved against nor deny through that limit.
	require.ElementsMatch(t, []uuid.UUID{a, b}, []uuid.UUID{*limits[0].Definition.Scopes[0].AccountID, *limits[0].Definition.Scopes[1].AccountID})
	resolver, err := NewContextReservationResolver(clock.NewFixedClock(testutil.FixedTime()), config)
	require.NoError(t, err)
	plan, err := resolver.Execute(t.Context(), facts, limits)
	require.NoError(t, err)
	require.False(t, plan.Denied)
	require.Empty(t, plan.ExceededLimitIDs)
	require.Len(t, plan.Reservations, 1)
	require.Equal(t, "acct:"+a.String(), plan.Reservations[0].ScopeKey)
	require.Contains(t, plan.AccountIDs, b)
}

func TestContextReservationsMultiAssetScopeMatchesByCode(t *testing.T) {
	t.Parallel()
	x, y := testutil.MustDeterministicUUID(80301), testutil.MustDeterministicUUID(80302)
	blocked := false
	limit := model.ContextAccountLimit{Definition: model.Limit{
		ID: testutil.MustDeterministicUUID(80303), Asset: "BTC", Status: model.LimitStatusActive,
		LimitType: model.LimitTypeDaily, MaxAmount: decimal.RequireFromString("100"),
		Scopes: []model.Scope{{AccountID: &x}, {AccountID: &y}},
	}}
	facts := func(debits map[uuid.UUID]string) tracercontract.Context {
		c := tracercontract.Context{}
		for _, id := range []uuid.UUID{x, y} {
			amount, ok := debits[id]
			if !ok {
				continue
			}
			asset := "BTC"
			if id == x {
				asset = "XBT"
			}
			c.Accounts = append(c.Accounts, tracercontract.Account{ID: id, Type: "checking", Status: "ACTIVE", Blocked: &blocked, Asset: asset})
			c.Entries = append(c.Entries, tracercontract.Entry{AccountID: id, Direction: tracercontract.Debit, Amount: tracercontract.Amount(amount), Asset: asset})
		}
		return c
	}
	_, _, config := contextReservationFixture()
	resolver, err := NewContextReservationResolver(clock.NewFixedClock(testutil.FixedTime()), config)
	require.NoError(t, err)
	yScope := "acct:" + y.String()
	for _, tc := range []struct {
		name     string
		debits   map[uuid.UUID]string
		denied   bool
		reserved []string
	}{
		{"XBT on X and BTC over cap on Y", map[uuid.UUID]string{x: "500", y: "150"}, true, nil},
		{"BTC over cap on Y alone", map[uuid.UUID]string{y: "150"}, true, nil},
		{"XBT on X and BTC within cap on Y", map[uuid.UUID]string{x: "500", y: "50"}, false, []string{yScope}},
		{"BTC within cap on Y alone", map[uuid.UUID]string{y: "50"}, false, []string{yScope}},
		{"XBT on X alone", map[uuid.UUID]string{x: "500"}, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plan, err := resolver.Execute(t.Context(), facts(tc.debits), []model.ContextAccountLimit{limit})
			require.NoError(t, err)
			require.Equal(t, tc.denied, plan.Denied)
			if tc.denied {
				require.Equal(t, []uuid.UUID{limit.Definition.ID}, plan.ExceededLimitIDs)
			} else {
				require.Empty(t, plan.ExceededLimitIDs)
			}
			keys := make([]string, 0, len(plan.Reservations))
			for _, spec := range plan.Reservations {
				keys = append(keys, spec.ScopeKey)
			}
			require.ElementsMatch(t, tc.reserved, keys)
		})
	}
}

func TestContextReservationsExactLargeAmountAndNoTruncation(t *testing.T) {
	facts, limits, config := contextReservationFixture()
	facts.Entries[0].Amount = "9007199254740993.125"
	limits[0].Definition.MaxAmount = decimal.RequireFromString("9007199254740993.13")
	resolver, err := NewContextReservationResolver(clock.NewFixedClock(testutil.FixedTime()), config)
	require.NoError(t, err)
	plan, err := resolver.Execute(t.Context(), facts, limits)
	require.NoError(t, err)
	require.False(t, plan.Denied)
	require.Len(t, plan.Reservations, 2)
	for _, spec := range plan.Reservations {
		if spec.ScopeKey == "acct:"+facts.Accounts[0].ID.String() {
			require.Equal(t, "9007199254740993.13", spec.Amount.String())
		}
	}
	limits[0].Definition.MaxAmount = decimal.RequireFromString("9007199254740993.129")
	plan, err = resolver.Execute(t.Context(), facts, limits)
	require.NoError(t, err)
	require.True(t, plan.Denied)
	require.Empty(t, plan.Reservations)
	limits[0].Definition.MaxAmount = decimal.RequireFromString("9007199254740993.12345678901")
	plan, err = resolver.Execute(t.Context(), facts, limits)
	require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
	require.Nil(t, plan)
}
