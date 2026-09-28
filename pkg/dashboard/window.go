// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package dashboard holds the time window every dashboard read aggregates
// over. It lives in the root pkg/ because both deploy surfaces answer a
// /dashboard family — the tracer over its validation trail, the ledger over
// its transaction table — and a window whose bounds are computed two
// different ways is a window whose cache entries and figures disagree between
// them for reasons no reader could ever guess.
//
// It knows nothing about either component: no repository, no entity, no SQL.
package dashboard

import (
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Window bounds. The window is capped so a single request can never ask the
// database to aggregate an unbounded slice of history.
const (
	// MaxWindow is the longest window any dashboard read may cover.
	MaxWindow = 90 * 24 * time.Hour

	// DefaultPeriod is the period applied when the caller names none.
	DefaultPeriod = "30d"

	// Granularity is the resolution both bounds are snapped to. It equals the
	// dashboard cache TTL on purpose: snapping here is what makes a relative
	// period ("last 30 days") name the SAME window for every caller inside the
	// same minute, which is the only reason a cache entry can ever be hit.
	// Without it every request would carry a microsecond-unique "now" and
	// compute its own aggregation.
	//
	// The start rounds DOWN and the end rounds UP, so the window is always a
	// superset of what the caller named. Rounding the end DOWN instead hides up
	// to 60 seconds of the most recent activity, which reads as "nothing
	// happened" — the one failure a live dashboard must not have, because it is
	// indistinguishable from a healthy quiet period.
	Granularity = time.Minute
)

// periods maps the three supported relative periods to their length. A period
// the map does not carry is rejected; the set is deliberately closed so no
// caller can widen the window through this parameter.
var periods = map[string]time.Duration{
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
	"90d": 90 * 24 * time.Hour,
}

// Window is the half-open time range [From, To) a dashboard read aggregates
// over, already normalized to UTC and truncated to Granularity.
//
// Both bounds are truncated rather than only the cache key, so the cached
// value is the value OF the named window: two callers asking for "30d" in the
// same minute get one number computed once, and a caller who asks twice gets
// the same number rather than two that differ by the seconds between the
// reads. The cost is staleness bounded at one truncation (up to 60s) plus one
// cache TTL (60s).
type Window struct {
	From time.Time
	To   time.Time
	// Period is the relative period the caller named ("7d"/"30d"/"90d"), or the
	// empty string when the window came from explicit dates.
	Period string
}

// NewWindow normalizes the query parameters of a dashboard read into a Window.
// period and the startDate/endDate pair are mutually exclusive: naming both is
// a caller error rather than a silent precedence rule nobody can guess.
//
// now is injected (never time.Now()) so the caller's clock is the service
// clock and tests are deterministic.
func NewWindow(period, startDate, endDate string, now time.Time) (Window, error) {
	hasPeriod := period != ""
	hasDates := startDate != "" || endDate != ""

	if hasPeriod && hasDates {
		return Window{}, fmt.Errorf("%w: period and start_date/end_date are mutually exclusive", constant.ErrInvalidDashboardWindow)
	}

	if hasDates {
		return explicitWindow(startDate, endDate)
	}

	if period == "" {
		period = DefaultPeriod
	}

	length, ok := periods[period]
	if !ok {
		return Window{}, fmt.Errorf("%w: unsupported period %q (want 7d, 30d or 90d)", constant.ErrInvalidDashboardWindow, period)
	}

	to := ceilBound(now)

	return Window{From: to.Add(-length), To: to, Period: period}, nil
}

// explicitWindow builds the window from an RFC3339 date pair. Both bounds are
// required: a half-open request would silently borrow "now" or the beginning
// of history, and neither is a window the caller asked for.
func explicitWindow(startDate, endDate string) (Window, error) {
	if startDate == "" || endDate == "" {
		return Window{}, fmt.Errorf("%w: startDate and endDate must be supplied together", constant.ErrInvalidDashboardWindow)
	}

	from, err := time.Parse(time.RFC3339, startDate)
	if err != nil {
		return Window{}, fmt.Errorf("%w: startDate is not RFC3339", constant.ErrInvalidDashboardWindow)
	}

	to, err := time.Parse(time.RFC3339, endDate)
	if err != nil {
		return Window{}, fmt.Errorf("%w: endDate is not RFC3339", constant.ErrInvalidDashboardWindow)
	}

	// Both checks run on what the caller WROTE, before snapping. Snapping widens
	// the window by under a minute at each end, so validating after it would
	// accept an empty range (startDate == endDate becomes a one-minute window)
	// and reject an exactly-90-day range whose end carries seconds. The window
	// that reaches the database is therefore at most the cap plus TWO
	// granularity steps — the start rounds down and the end rounds up, so each
	// bound can move by one — which is the price of a cacheable window.
	if !to.After(from) {
		return Window{}, fmt.Errorf("%w: endDate must be after startDate", constant.ErrInvalidDashboardWindow)
	}

	if to.Sub(from) > MaxWindow {
		return Window{}, fmt.Errorf("%w: window may not exceed 90 days", constant.ErrInvalidDashboardWindow)
	}

	return Window{From: from.UTC().Truncate(Granularity), To: ceilBound(to)}, nil
}

// ceilBound snaps a window end UP to the next granularity boundary. An instant
// already on a boundary is left alone, so a caller naming an exact minute gets
// exactly that minute rather than a spurious extra one.
func ceilBound(t time.Time) time.Time {
	utc := t.UTC()

	floor := utc.Truncate(Granularity)
	if floor.Equal(utc) {
		return floor
	}

	return floor.Add(Granularity)
}

// CacheKey renders the window as the stable fragment of a cache key. It names
// the period when the caller used one, so "30d" and an explicit 30-day range
// stay distinct entries: they are the same length but not the same question,
// and conflating them would serve a pinned historical range from a rolling
// one's entry.
func (w Window) CacheKey() string {
	if w.Period != "" {
		return "p=" + w.Period + ":" + w.To.Format(time.RFC3339)
	}

	return w.From.Format(time.RFC3339) + ":" + w.To.Format(time.RFC3339)
}
