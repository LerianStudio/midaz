// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// fixedNow is a deterministic "now". It is a hardcoded instant rather than
// testutil.FixedTime(), which is time.Now() minus a minute and therefore
// carries arbitrary seconds — the truncation assertions below would straddle a
// minute boundary at random. Its seconds and nanoseconds are non-zero on
// purpose, so every truncation assertion proves the truncation happened.
func fixedNow(t *testing.T) time.Time {
	t.Helper()

	return time.Date(2026, 9, 20, 14, 30, 12, 345_000_000, time.UTC)
}

func TestNewDashboardWindow_Periods(t *testing.T) {
	t.Parallel()

	now := fixedNow(t)

	tests := []struct {
		name   string
		period string
		want   time.Duration
	}{
		{name: "seven days", period: "7d", want: 7 * 24 * time.Hour},
		{name: "thirty days", period: "30d", want: 30 * 24 * time.Hour},
		{name: "ninety days", period: "90d", want: 90 * 24 * time.Hour},
		{name: "empty defaults to thirty days", period: "", want: 30 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			window, err := model.NewDashboardWindow(tt.period, "", "", now)
			require.NoError(t, err)

			assert.Equal(t, tt.want, window.To.Sub(window.From), "window length")
			assert.Equal(t, now.Truncate(time.Minute).Add(time.Minute), window.To,
				"To rounds UP to the minute, so traffic in the current minute is not hidden")
			assert.Equal(t, time.UTC, window.To.Location(), "window is UTC")
		})
	}
}

// The default is not merely "some period": a caller that names nothing must get
// the documented 30d, because the cache key, the response and the console all
// assume it.
func TestNewDashboardWindow_DefaultPeriodIsNamed(t *testing.T) {
	t.Parallel()

	window, err := model.NewDashboardWindow("", "", "", fixedNow(t))
	require.NoError(t, err)

	assert.Equal(t, model.DashboardDefaultPeriod, window.Period)
}

func TestNewDashboardWindow_Rejects(t *testing.T) {
	t.Parallel()

	now := fixedNow(t)
	start := now.Add(-48 * time.Hour).Format(time.RFC3339)
	end := now.Format(time.RFC3339)

	tests := []struct {
		name                       string
		period, startDate, endDate string
	}{
		{name: "unsupported period", period: "45d"},
		{name: "period with dates is ambiguous", period: "30d", startDate: start, endDate: end},
		{name: "period with only a start date", period: "30d", startDate: start},
		{name: "start date without end date", startDate: start},
		{name: "end date without start date", endDate: end},
		{name: "start date is not RFC3339", startDate: "2026-09-01", endDate: end},
		{name: "end date is not RFC3339", startDate: start, endDate: "tomorrow"},
		{
			name:      "window longer than ninety days",
			startDate: now.Add(-91 * 24 * time.Hour).Format(time.RFC3339),
			endDate:   end,
		},
		{
			name:      "end date equals start date",
			startDate: start,
			endDate:   start,
		},
		{
			name:      "end date before start date",
			startDate: end,
			endDate:   start,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := model.NewDashboardWindow(tt.period, tt.startDate, tt.endDate, now)

			require.Error(t, err)
			assert.ErrorIs(t, err, constant.ErrInvalidDashboardWindow,
				"every rejection must carry the canonical sentinel so the handler can render a 400")
		})
	}
}

// Exactly 90 days is the boundary the cap admits, not the one it rejects.
func TestNewDashboardWindow_NinetyDaysExactlyIsAccepted(t *testing.T) {
	t.Parallel()

	now := fixedNow(t)
	end := now.Truncate(time.Minute)
	start := end.Add(-model.DashboardMaxWindow)

	window, err := model.NewDashboardWindow("", start.Format(time.RFC3339), end.Format(time.RFC3339), now)
	require.NoError(t, err)

	// Both bounds already sit on a minute boundary, so snapping is a no-op and
	// the accepted window is exactly the cap.
	assert.Equal(t, model.DashboardMaxWindow, window.To.Sub(window.From))
	assert.Empty(t, window.Period, "an explicit range names no period")
}

func TestNewDashboardWindow_ExplicitDatesAreTruncatedAndUTC(t *testing.T) {
	t.Parallel()

	// A non-UTC zone with sub-minute precision on both bounds.
	zone := time.FixedZone("UTC-3", -3*60*60)
	start := time.Date(2026, 9, 1, 10, 15, 42, 500, zone)
	end := time.Date(2026, 9, 8, 10, 15, 42, 500, zone)

	window, err := model.NewDashboardWindow("", start.Format(time.RFC3339), end.Format(time.RFC3339), fixedNow(t))
	require.NoError(t, err)

	assert.Equal(t, start.UTC().Truncate(time.Minute), window.From, "start rounds down")
	assert.Equal(t, end.UTC().Truncate(time.Minute).Add(time.Minute), window.To, "end rounds up")
}

// The cache key is the whole reason the window is truncated. Two reads a few
// seconds apart must land on ONE key, or the cache never hits and every request
// reaches the database — which is the load the cache exists to prevent.
func TestDashboardWindow_CacheKey_StableWithinTheMinute(t *testing.T) {
	t.Parallel()

	base := fixedNow(t)

	first, err := model.NewDashboardWindow("30d", "", "", base)
	require.NoError(t, err)

	second, err := model.NewDashboardWindow("30d", "", "", base.Add(20*time.Second))
	require.NoError(t, err)

	assert.Equal(t, first.CacheKey(), second.CacheKey())
}

// And it must roll forward at the minute, or a cache entry would outlive the
// window it describes.
func TestDashboardWindow_CacheKey_RollsAtTheMinute(t *testing.T) {
	t.Parallel()

	base := fixedNow(t).Truncate(time.Minute)

	first, err := model.NewDashboardWindow("30d", "", "", base)
	require.NoError(t, err)

	second, err := model.NewDashboardWindow("30d", "", "", base.Add(time.Minute))
	require.NoError(t, err)

	assert.NotEqual(t, first.CacheKey(), second.CacheKey())
}

// A pinned 30-day range and a rolling "30d" have the same length and are not
// the same question. Sharing a key would serve one from the other's entry.
func TestDashboardWindow_CacheKey_PeriodAndExplicitRangeDiffer(t *testing.T) {
	t.Parallel()

	now := fixedNow(t)

	rolling, err := model.NewDashboardWindow("30d", "", "", now)
	require.NoError(t, err)

	pinned, err := model.NewDashboardWindow("",
		rolling.From.Format(time.RFC3339), rolling.To.Format(time.RFC3339), now)
	require.NoError(t, err)

	assert.Equal(t, rolling.From, pinned.From, "same window by construction")
	assert.Equal(t, rolling.To, pinned.To)
	assert.NotEqual(t, rolling.CacheKey(), pinned.CacheKey(), "but not the same cache entry")
}

func TestDashboardWindow_CacheKey_PeriodsDoNotCollide(t *testing.T) {
	t.Parallel()

	now := fixedNow(t)

	seen := map[string]string{}

	for _, period := range []string{"7d", "30d", "90d"} {
		window, err := model.NewDashboardWindow(period, "", "", now)
		require.NoError(t, err)

		key := window.CacheKey()
		prev, clash := seen[key]
		require.False(t, clash, "period %s collides with %s on key %s", period, prev, key)
		seen[key] = period
	}
}

func TestDashboardMetrics_ApplyRates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                                string
		metrics                             model.DashboardMetrics
		wantApproval, wantFraud, wantReview float64
		wantAmountSaved, wantAsset          string
	}{
		{
			name: "rates are fractions of the total",
			metrics: model.DashboardMetrics{
				TransactionsProcessed: 1000,
				Allowed:               950,
				FraudsBlocked:         30,
				ManualReviews:         20,
			},
			wantApproval: 0.95, wantFraud: 0.03, wantReview: 0.02,
		},
		{
			name:         "an empty window divides by nothing",
			metrics:      model.DashboardMetrics{},
			wantApproval: 0, wantFraud: 0, wantReview: 0,
		},
		{
			name: "a single asset fills the convenience pair",
			metrics: model.DashboardMetrics{
				TransactionsProcessed: 10,
				AmountSavedByAsset: []model.AssetAmount{
					{Asset: "BRL", Amount: decimal.RequireFromString("1250.75")},
				},
			},
			wantApproval:    0,
			wantAmountSaved: "1250.75",
			wantAsset:       "BRL",
		},
		{
			name: "two assets fill nothing, because there is no single figure",
			metrics: model.DashboardMetrics{
				TransactionsProcessed: 10,
				AmountSavedByAsset: []model.AssetAmount{
					{Asset: "USD", Amount: decimal.RequireFromString("900.00")},
					{Asset: "BRL", Amount: decimal.RequireFromString("100.00")},
				},
			},
			wantAmountSaved: "",
			wantAsset:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			metrics := tt.metrics
			metrics.ApplyRates()

			assert.InDelta(t, tt.wantApproval, metrics.ApprovalRate, 1e-9)
			assert.InDelta(t, tt.wantFraud, metrics.FraudDetectionRate, 1e-9)
			assert.InDelta(t, tt.wantReview, metrics.ManualReviewRate, 1e-9)
			assert.Equal(t, tt.wantAmountSaved, metrics.AmountSaved)
			assert.Equal(t, tt.wantAsset, metrics.Asset)
		})
	}
}

// Money never crosses assets. This is the assertion that would fail if someone
// "helpfully" summed the breakdown into the scalar.
func TestDashboardMetrics_ApplyRates_NeverSumsAcrossAssets(t *testing.T) {
	t.Parallel()

	metrics := model.DashboardMetrics{
		TransactionsProcessed: 3,
		AmountSavedByAsset: []model.AssetAmount{
			{Asset: "USD", Amount: decimal.RequireFromString("100.00")},
			{Asset: "BRL", Amount: decimal.RequireFromString("500.00")},
			{Asset: "EUR", Amount: decimal.RequireFromString("50.00")},
		},
	}

	metrics.ApplyRates()

	assert.Empty(t, metrics.AmountSaved)
	assert.NotEqual(t, "650", metrics.AmountSaved, "650 is not money in any asset")
	assert.Len(t, metrics.AmountSavedByAsset, 3, "the breakdown is what the caller renders")
}

// REGRESSION (found by the live smoke, 2026-09-20): the window end must never
// exclude traffic that has already been recorded. Truncating `to` DOWN to the
// minute hid up to 60 seconds of the most recent decisions, so a tenant that
// blocked two transactions half a minute ago saw a dashboard reporting zero
// fraud — the worst failure a fraud console has, because it is indistinguishable
// from "nothing is wrong".
func TestNewDashboardWindow_EndIncludesTheCurrentMinute(t *testing.T) {
	t.Parallel()

	now := fixedNow(t) // :30:12.345 — mid-minute on purpose

	window, err := model.NewDashboardWindow("30d", "", "", now)
	require.NoError(t, err)

	assert.True(t, window.To.After(now),
		"a validation recorded at %s must fall inside a window ending at %s", now, window.To)
	assert.Equal(t, now.Truncate(time.Minute).Add(time.Minute), window.To,
		"the end rounds UP to the minute, so nothing already recorded is hidden")
}

// An explicit range is treated the same way and for the same reason: the window
// is a superset of what the caller named, never a subset.
func TestNewDashboardWindow_ExplicitEndRoundsUp(t *testing.T) {
	t.Parallel()

	start := "2026-09-01T10:00:00Z"
	end := "2026-09-08T10:15:42Z"

	window, err := model.NewDashboardWindow("", start, end, fixedNow(t))
	require.NoError(t, err)

	assert.Equal(t, time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC), window.From,
		"the start rounds DOWN, so nothing at the beginning is hidden either")
	assert.Equal(t, time.Date(2026, 9, 8, 10, 16, 0, 0, time.UTC), window.To)
}

// Rounding up must not break the quantisation the cache depends on.
func TestNewDashboardWindow_RoundingUpKeepsTheKeyStable(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 9, 20, 14, 30, 1, 0, time.UTC)

	first, err := model.NewDashboardWindow("30d", "", "", base)
	require.NoError(t, err)

	second, err := model.NewDashboardWindow("30d", "", "", base.Add(55*time.Second))
	require.NoError(t, err)

	assert.Equal(t, first.CacheKey(), second.CacheKey(), "same minute, same entry")
}
