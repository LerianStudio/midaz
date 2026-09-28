// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package dashboard_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/dashboard"
)

// fixedNow is the injected clock for every case here. It deliberately carries
// seconds and nanoseconds so the floor/ceil behaviour is observable: a clock on
// an exact minute would make both roundings no-ops and every snapping
// assertion below vacuous.
var fixedNow = time.Date(2026, 9, 20, 14, 30, 45, 123456789, time.UTC)

func TestNewWindow_DefaultsTo30d(t *testing.T) {
	window, err := dashboard.NewWindow("", "", "", fixedNow)
	require.NoError(t, err)

	assert.Equal(t, dashboard.DefaultPeriod, window.Period)
	assert.Equal(t, 30*24*time.Hour, window.To.Sub(window.From))
}

func TestNewWindow_PeriodEndCeilsToTheMinute(t *testing.T) {
	window, err := dashboard.NewWindow("7d", "", "", fixedNow)
	require.NoError(t, err)

	// 14:30:45.123456789 ceils UP to 14:31:00, never down: rounding down hides
	// up to 60s of the most recent transactions from a live dashboard.
	assert.Equal(t, time.Date(2026, 9, 20, 14, 31, 0, 0, time.UTC), window.To)
	assert.Equal(t, time.Date(2026, 9, 13, 14, 31, 0, 0, time.UTC), window.From)
	assert.Equal(t, 7*24*time.Hour, window.To.Sub(window.From))
}

func TestNewWindow_PeriodEndOnAMinuteBoundaryIsLeftAlone(t *testing.T) {
	onBoundary := time.Date(2026, 9, 20, 14, 31, 0, 0, time.UTC)

	window, err := dashboard.NewWindow("7d", "", "", onBoundary)
	require.NoError(t, err)

	assert.Equal(t, onBoundary, window.To)
}

func TestNewWindow_RejectsUnsupportedPeriod(t *testing.T) {
	for _, period := range []string{"1d", "14d", "180d", "7D", "7", "d7", "7days"} {
		_, err := dashboard.NewWindow(period, "", "", fixedNow)

		require.Error(t, err, "period %q must be refused", period)
		assert.True(t, errors.Is(err, constant.ErrInvalidDashboardWindow), "period %q must carry 0498", period)
	}
}

func TestNewWindow_RejectsBothForms(t *testing.T) {
	cases := map[string][3]string{
		"period and both dates": {"7d", "2026-09-01T00:00:00Z", "2026-09-10T00:00:00Z"},
		"period and start only": {"7d", "2026-09-01T00:00:00Z", ""},
		"period and end only":   {"7d", "", "2026-09-10T00:00:00Z"},
	}

	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := dashboard.NewWindow(args[0], args[1], args[2], fixedNow)

			require.Error(t, err)
			assert.True(t, errors.Is(err, constant.ErrInvalidDashboardWindow))
		})
	}
}

func TestNewWindow_RejectsHalfSuppliedDatePair(t *testing.T) {
	for name, args := range map[string][2]string{
		"start only": {"2026-09-01T00:00:00Z", ""},
		"end only":   {"", "2026-09-10T00:00:00Z"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := dashboard.NewWindow("", args[0], args[1], fixedNow)

			require.Error(t, err)
			assert.True(t, errors.Is(err, constant.ErrInvalidDashboardWindow))
		})
	}
}

func TestNewWindow_ExplicitDatesFloorStartAndCeilEnd(t *testing.T) {
	window, err := dashboard.NewWindow("", "2026-09-01T10:15:30Z", "2026-09-10T18:45:30Z", fixedNow)
	require.NoError(t, err)

	assert.Equal(t, time.Date(2026, 9, 1, 10, 15, 0, 0, time.UTC), window.From)
	assert.Equal(t, time.Date(2026, 9, 10, 18, 46, 0, 0, time.UTC), window.To)
	assert.Empty(t, window.Period, "an explicit range names no period")
}

func TestNewWindow_ExplicitDatesNormalizeToUTC(t *testing.T) {
	window, err := dashboard.NewWindow("", "2026-09-01T07:15:00-03:00", "2026-09-02T07:15:00-03:00", fixedNow)
	require.NoError(t, err)

	assert.Equal(t, time.Date(2026, 9, 1, 10, 15, 0, 0, time.UTC), window.From)
	assert.Equal(t, time.Date(2026, 9, 2, 10, 15, 0, 0, time.UTC), window.To)
}

func TestNewWindow_RejectsRangeLongerThan90Days(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(dashboard.MaxWindow + time.Second)

	_, err := dashboard.NewWindow("", start.Format(time.RFC3339), end.Format(time.RFC3339), fixedNow)

	require.Error(t, err)
	assert.True(t, errors.Is(err, constant.ErrInvalidDashboardWindow))
}

func TestNewWindow_AcceptsExactly90Days(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(dashboard.MaxWindow)

	window, err := dashboard.NewWindow("", start.Format(time.RFC3339), end.Format(time.RFC3339), fixedNow)

	require.NoError(t, err)
	assert.Equal(t, dashboard.MaxWindow, window.To.Sub(window.From))
}

func TestNewWindow_RejectsEndNotAfterStart(t *testing.T) {
	for name, pair := range map[string][2]string{
		"end before start": {"2026-09-10T00:00:00Z", "2026-09-01T00:00:00Z"},
		"end equals start": {"2026-09-10T00:00:00Z", "2026-09-10T00:00:00Z"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := dashboard.NewWindow("", pair[0], pair[1], fixedNow)

			require.Error(t, err)
			assert.True(t, errors.Is(err, constant.ErrInvalidDashboardWindow))
		})
	}
}

func TestNewWindow_RejectsNonRFC3339Dates(t *testing.T) {
	for name, pair := range map[string][2]string{
		"start not rfc3339": {"2026-09-01", "2026-09-10T00:00:00Z"},
		"end not rfc3339":   {"2026-09-01T00:00:00Z", "10/09/2026"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := dashboard.NewWindow("", pair[0], pair[1], fixedNow)

			require.Error(t, err)
			assert.True(t, errors.Is(err, constant.ErrInvalidDashboardWindow))
		})
	}
}

func TestWindow_CacheKeySeparatesPeriodFromEquivalentRange(t *testing.T) {
	relative, err := dashboard.NewWindow("30d", "", "", fixedNow)
	require.NoError(t, err)

	explicit, err := dashboard.NewWindow("", relative.From.Format(time.RFC3339), relative.To.Format(time.RFC3339), fixedNow)
	require.NoError(t, err)

	assert.Equal(t, relative.From, explicit.From)
	assert.Equal(t, relative.To, explicit.To)
	assert.NotEqual(t, relative.CacheKey(), explicit.CacheKey(),
		"a rolling period and a pinned range cover the same instants but are not the same question")
}

func TestWindow_CacheKeyIsStableWithinTheMinute(t *testing.T) {
	early, err := dashboard.NewWindow("30d", "", "", time.Date(2026, 9, 20, 14, 30, 1, 0, time.UTC))
	require.NoError(t, err)

	late, err := dashboard.NewWindow("30d", "", "", time.Date(2026, 9, 20, 14, 30, 59, 0, time.UTC))
	require.NoError(t, err)

	assert.Equal(t, early.CacheKey(), late.CacheKey())
}
