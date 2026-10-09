// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testhelper"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// newResolverForTest wires a LimitCheckerService whose limit repository returns the
// supplied limits, so ResolveReservations can be driven without a real database.
func newResolverForTest(t *testing.T, limits []model.Limit) *LimitCheckerService {
	t.Helper()

	return newResolverForTestAt(t, limits, testutil.NewDefaultMockClock())
}

// newResolverForTestAt is newResolverForTest with the server clock supplied.
func newResolverForTestAt(t *testing.T, limits []model.Limit, serverClock clock.Clock) *LimitCheckerService {
	t.Helper()

	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)
	limitRepo := NewMockLimitRepository(ctrl)
	usageRepo := NewMockUsageCounterRepository(ctrl)

	limitRepo.EXPECT().
		List(gomock.Any(), gomock.Any()).
		Return(&model.ListLimitsResult{Limits: limits, HasMore: false}, nil).
		AnyTimes()

	checker, err := NewLimitChecker(limitRepo, usageRepo, serverClock)
	require.NoError(t, err)

	return checker
}

// TestResolveReservations_PreservesFractionalAmount is the money-path regression
// guard for the reservation seam: a fractional transaction amount must reach the
// reservation spec intact. Under the pre-fix int64 IntPart() hop, 10.50 collapsed to
// 10 and any sub-unitary amount (0 < x < 1) collapsed to 0 — silently under-holding,
// or wholly dropping, held capacity. Amounts are compared with decimal.Equal so the
// check is exact and insensitive to scale / trailing zeros.
//
// Tests here stay sequential (no t.Parallel): newResolverForTest wires
// SetupTestTracing, which installs a process-global tracer provider under a mutex —
// a parallel-gate exception, not an omission.
func TestResolveReservations_PreservesFractionalAmount(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(9100)
	limitID := testutil.MustDeterministicUUID(9101)

	tests := []struct {
		name   string
		amount string
	}{
		{name: "fractional above one preserves the cents", amount: "10.5"},
		{name: "sub-unitary preserves the whole fraction", amount: "0.99"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			checker := newResolverForTest(t, []model.Limit{
				{
					ID:        limitID,
					Name:      "Daily fractional cap",
					LimitType: model.LimitTypeDaily,
					MaxAmount: decimal.RequireFromString("20.75"),
					Asset:     "USD",
					Scopes:    []model.Scope{{AccountID: &accountID}},
					Status:    model.LimitStatusActive,
				},
			})

			want := decimal.RequireFromString(tt.amount)
			wantMax := decimal.RequireFromString("20.75")

			input, err := model.NewCheckLimitsInput(
				want,
				"USD",
				accountID,
				nil, nil, nil, nil, nil,
				testutil.DefaultTestTime,
			)
			require.NoError(t, err)

			specs, denied, err := checker.ResolveReservations(context.Background(), input)
			require.NoError(t, err)
			require.False(t, denied, "%s against a 20.75 cap must not be denied", tt.amount)
			require.Len(t, specs, 1)

			require.True(t, want.Equal(specs[0].Amount),
				"reservation amount must preserve the fraction, not truncate %s -> integer; got %s",
				tt.amount, specs[0].Amount)

			require.True(t, wantMax.Equal(specs[0].MaxAmount),
				"reservation cap must preserve the fraction, not truncate 20.75 -> integer; got %s",
				specs[0].MaxAmount)
		})
	}
}

// TestResolveReservations_PeriodKeyFollowsResetTime checks that a reservation is
// keyed by the period the limit's resetTime defines, so the reserve path and the
// validation path share one counter per overnight window.
func TestResolveReservations_PeriodKeyFollowsResetTime(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(15510)
	limitID := testutil.MustDeterministicUUID(15511)
	nineAM := testhelper.MustNewTimeOfDay("09:00")
	serverNow := time.Date(2026, 10, 2, 0, 30, 0, 0, time.UTC)

	tests := []struct {
		name      string
		resetTime *model.TimeOfDay
		wantKey   string
	}{
		{name: "reset at 09:00 keeps 00:30 in the previous day's period", resetTime: &nineAM, wantKey: "2026-10-01"},
		{name: "absent reset time keys 00:30 by its own UTC date", resetTime: nil, wantKey: "2026-10-02"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			windowStart := testhelper.MustNewTimeOfDay("23:00")
			windowEnd := testhelper.MustNewTimeOfDay("09:00")

			checker := newResolverForTestAt(t, []model.Limit{
				{
					ID:              limitID,
					Name:            "Overnight Pix cap",
					LimitType:       model.LimitTypeDaily,
					MaxAmount:       decimal.RequireFromString("1000"),
					Asset:           "BRL",
					Scopes:          []model.Scope{{AccountID: &accountID}},
					Status:          model.LimitStatusActive,
					ActiveTimeStart: &windowStart,
					ActiveTimeEnd:   &windowEnd,
					ResetTime:       tt.resetTime,
				},
			}, testutil.NewMockClock(serverNow))

			input, err := model.NewCheckLimitsInput(
				decimal.RequireFromString("1000"),
				"BRL",
				accountID,
				nil, nil, nil, nil, nil,
				serverNow,
			)
			require.NoError(t, err)

			specs, denied, err := checker.ResolveReservations(context.Background(), input)
			require.NoError(t, err)
			require.False(t, denied)
			require.Len(t, specs, 1)
			require.Equal(t, tt.wantKey, specs[0].PeriodKey)
		})
	}
}

// TestResolveReservations_CounterExpiresAtIsPeriodRetention checks that a
// reservation spec carries the counter's period-retention expiry — the same value
// the synchronous validate path writes to usage_counters.expires_at — which lies
// well past any reservation lifetime, so a cleanup sweep after a reservation
// expires cannot delete a counter whose period is still running.
func TestResolveReservations_CounterExpiresAtIsPeriodRetention(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(15520)
	limitID := testutil.MustDeterministicUUID(15521)
	serverNow := time.Date(2026, 10, 2, 0, 30, 0, 0, time.UTC)
	customStart := serverNow.Add(-24 * time.Hour)
	customEnd := serverNow.Add(48 * time.Hour)
	longestReservationTTL := 720 * time.Hour

	tests := []struct {
		name  string
		limit model.Limit
	}{
		{
			name: "daily limit expires its counter at the next reset plus retention",
			limit: model.Limit{
				ID:        limitID,
				Name:      "Daily cap",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Asset:     "BRL",
				Scopes:    []model.Scope{{AccountID: &accountID}},
				Status:    model.LimitStatusActive,
			},
		},
		{
			name: "custom limit expires its counter at the custom end plus retention",
			limit: model.Limit{
				ID:              limitID,
				Name:            "Custom cap",
				LimitType:       model.LimitTypeCustom,
				MaxAmount:       decimal.RequireFromString("1000"),
				Asset:           "BRL",
				Scopes:          []model.Scope{{AccountID: &accountID}},
				Status:          model.LimitStatusActive,
				CustomStartDate: &customStart,
				CustomEndDate:   &customEnd,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := newResolverForTestAt(t, []model.Limit{tt.limit}, testutil.NewMockClock(serverNow))

			input, err := model.NewCheckLimitsInput(
				decimal.RequireFromString("100"),
				"BRL",
				accountID,
				nil, nil, nil, nil, nil,
				serverNow,
			)
			require.NoError(t, err)

			specs, denied, err := checker.ResolveReservations(context.Background(), input)
			require.NoError(t, err)
			require.False(t, denied)
			require.Len(t, specs, 1)

			resetAt := tt.limit.NextResetAt(serverNow)
			require.NotNil(t, resetAt)

			var want time.Time
			if tt.limit.LimitType == model.LimitTypeCustom {
				want = tt.limit.CustomEndDate.AddDate(0, 0, trcConstant.CounterRetentionDays)
			} else {
				want = resetAt.AddDate(0, 0, trcConstant.CounterRetentionDays)
			}

			require.NotNil(t, specs[0].CounterExpiresAt)
			require.True(t, want.Equal(*specs[0].CounterExpiresAt),
				"counter expiry must be the period-retention expiry; want %s, got %s", want, specs[0].CounterExpiresAt)
			require.True(t, specs[0].CounterExpiresAt.After(serverNow.Add(longestReservationTTL)),
				"counter expiry must outlive every reservation lifetime")
		})
	}
}
