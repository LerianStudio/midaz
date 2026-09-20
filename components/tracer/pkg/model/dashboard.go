// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Dashboard window bounds. The window is capped so a single request can never
// ask the database to aggregate an unbounded slice of the validation trail.
const (
	// DashboardMaxWindow is the longest window any dashboard read may cover.
	DashboardMaxWindow = 90 * 24 * time.Hour

	// DashboardDefaultPeriod is the period applied when the caller names none.
	DashboardDefaultPeriod = "30d"

	// DashboardWindowGranularity is the resolution both bounds are snapped to.
	// It equals the cache TTL on purpose: snapping here is what makes a
	// relative period ("last 30 days") name the SAME window for every caller
	// inside the same minute, which is the only reason a cache entry can ever
	// be hit. Without it every request would carry a microsecond-unique "now"
	// and compute its own aggregation.
	//
	// The start rounds DOWN and the end rounds UP, so the window is always a
	// superset of what the caller named. Rounding the end DOWN instead hid up
	// to 60 seconds of the most recent decisions, which a live run caught as a
	// dashboard reporting zero fraud half a minute after blocking two
	// transactions — the one failure a fraud console must not have, because it
	// is indistinguishable from "nothing is wrong".
	DashboardWindowGranularity = time.Minute

	// DashboardTopRulesLimit caps the top-rules panel. Ten is what the console
	// renders; it is not a caller-supplied page size, because the query's cost is
	// in the aggregation over the window and not in the rows it returns, so a
	// larger limit would buy a caller nothing and a paging contract even less.
	DashboardTopRulesLimit = 10
)

// dashboardPeriods maps the three supported relative periods to their length.
// A period the map does not carry is rejected; the set is deliberately closed
// so no caller can widen the window through this parameter.
var dashboardPeriods = map[string]time.Duration{
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
	"90d": 90 * 24 * time.Hour,
}

// DashboardWindow is the half-open time range [From, To) every dashboard read
// aggregates over, already normalized to UTC and truncated to the minute.
//
// Both bounds are truncated rather than only the key, so the cached value is
// the value OF the named window: two callers asking for "30d" in the same
// minute get one number computed once, and a caller who asks twice gets the
// same number rather than two that differ by the seconds between the reads.
// The cost is staleness bounded at one truncation (up to 60s) plus one cache
// TTL (60s); UpdatedAt on every response reports when the figures were
// actually computed.
type DashboardWindow struct {
	From time.Time
	To   time.Time
	// Period is the relative period the caller named ("7d"/"30d"/"90d"), or
	// the empty string when the window came from explicit dates.
	Period string
}

// NewDashboardWindow normalizes the query parameters of a dashboard read into a
// window. period and the start_date/end_date pair are mutually exclusive: naming
// both is a caller error rather than a silent precedence rule nobody can guess.
//
// now is injected (never time.Now()) so the caller's clock is the service
// clock and tests are deterministic.
func NewDashboardWindow(period, startDate, endDate string, now time.Time) (DashboardWindow, error) {
	hasPeriod := period != ""
	hasDates := startDate != "" || endDate != ""

	if hasPeriod && hasDates {
		return DashboardWindow{}, fmt.Errorf("%w: period and start_date/end_date are mutually exclusive", constant.ErrInvalidDashboardWindow)
	}

	if hasDates {
		return explicitDashboardWindow(startDate, endDate)
	}

	if period == "" {
		period = DashboardDefaultPeriod
	}

	length, ok := dashboardPeriods[period]
	if !ok {
		return DashboardWindow{}, fmt.Errorf("%w: unsupported period %q (want 7d, 30d or 90d)", constant.ErrInvalidDashboardWindow, period)
	}

	to := ceilWindowBound(now)

	return DashboardWindow{From: to.Add(-length), To: to, Period: period}, nil
}

// explicitDashboardWindow builds the window from an RFC3339 date pair. Both
// bounds are required: a half-open request would silently borrow "now" or the
// beginning of the trail, and neither is a window the caller asked for.
func explicitDashboardWindow(startDate, endDate string) (DashboardWindow, error) {
	if startDate == "" || endDate == "" {
		return DashboardWindow{}, fmt.Errorf("%w: startDate and endDate must be supplied together", constant.ErrInvalidDashboardWindow)
	}

	from, err := time.Parse(time.RFC3339, startDate)
	if err != nil {
		return DashboardWindow{}, fmt.Errorf("%w: startDate is not RFC3339", constant.ErrInvalidDashboardWindow)
	}

	to, err := time.Parse(time.RFC3339, endDate)
	if err != nil {
		return DashboardWindow{}, fmt.Errorf("%w: endDate is not RFC3339", constant.ErrInvalidDashboardWindow)
	}

	// Both checks run on what the caller WROTE, before snapping. Snapping
	// widens the window by under a minute at each end, so validating after it
	// would accept an empty range (startDate == endDate becomes a one-minute
	// window) and reject an exactly-90-day range whose end carries seconds.
	// The window that reaches the database is therefore at most the cap plus
	// TWO granularity steps — the start rounds down and the end rounds up, so
	// each bound can move by one — which is the price of a cacheable window.
	if !to.After(from) {
		return DashboardWindow{}, fmt.Errorf("%w: endDate must be after startDate", constant.ErrInvalidDashboardWindow)
	}

	if to.Sub(from) > DashboardMaxWindow {
		return DashboardWindow{}, fmt.Errorf("%w: window may not exceed 90 days", constant.ErrInvalidDashboardWindow)
	}

	return DashboardWindow{From: from.UTC().Truncate(DashboardWindowGranularity), To: ceilWindowBound(to)}, nil
}

// ceilWindowBound snaps a window end UP to the next granularity boundary. An
// instant already on a boundary is left alone, so a caller naming an exact
// minute gets exactly that minute rather than a spurious extra one.
func ceilWindowBound(t time.Time) time.Time {
	utc := t.UTC()

	floor := utc.Truncate(DashboardWindowGranularity)
	if floor.Equal(utc) {
		return floor
	}

	return floor.Add(DashboardWindowGranularity)
}

// CacheKey renders the window as the stable fragment of a cache key. It names
// the period when the caller used one so "30d" and an explicit 30-day range
// stay distinct entries: they are the same length but not the same question,
// and conflating them would serve a pinned historical range from a rolling
// one's entry.
func (w DashboardWindow) CacheKey() string {
	if w.Period != "" {
		return "p=" + w.Period + ":" + w.To.Format(time.RFC3339)
	}

	return w.From.Format(time.RFC3339) + ":" + w.To.Format(time.RFC3339)
}

// AssetAmount is one asset's share of a monetary figure. Dashboard money is
// always reported per asset: Tracer records amounts in the transaction's own
// asset, and adding BRL to USD produces a number that is not money. Callers
// render the breakdown, never a cross-asset total.
type AssetAmount struct {
	Asset  string          `json:"asset" example:"USD"`
	Amount decimal.Decimal `json:"amount" swaggertype:"string" example:"12500.00"`
}

// DashboardMetrics is the headline panel: how much traffic the window carried,
// how it was decided, what was blocked and how fast the decisions were.
//
// The three rates are decimal fractions in [0,1] (0.023 means 2.3%), matching
// what the console renders; they are computed in Go from the counts rather
// than in SQL so a zero-traffic window yields 0 instead of a division error.
type DashboardMetrics struct {
	TransactionsProcessed int64 `json:"transactionsProcessed" example:"82144"`
	Allowed               int64 `json:"allowed" example:"78037"`
	FraudsBlocked         int64 `json:"fraudsBlocked" example:"4107"`
	ManualReviews         int64 `json:"manualReviews" example:"1026"`

	ApprovalRate       float64 `json:"approvalRate" example:"0.95"`
	FraudDetectionRate float64 `json:"fraudDetectionRate" example:"0.05"`
	ManualReviewRate   float64 `json:"manualReviewRate" example:"0.0125"`

	// AmountSavedByAsset is the sum of the amounts of DENY decisions in the
	// window, split by asset. Always present; empty when nothing was blocked.
	//
	// The VALUE is exact end to end — decimal from the scan to the wire, never
	// through a float — but the SCALE is not preserved, because decimal
	// normalises it: a stored 41000.00 renders "41000" and 10352080.80 renders
	// "10352080.8". A consumer formats to the asset's exponent rather than
	// echoing the string, or it will show a money field with varying decimal
	// places.
	AmountSavedByAsset []AssetAmount `json:"amountSavedByAsset"`

	// AmountSaved and Asset are the single-asset convenience pair: populated
	// only when exactly one asset carried blocked volume in the window, absent
	// otherwise. A multi-asset window has no single figure to name, and naming
	// one anyway is how a dashboard starts reporting money that does not exist.
	AmountSaved string `json:"amountSaved,omitempty" example:"12500.00"`
	Asset       string `json:"asset,omitempty" example:"USD"`

	// AvgProcessingTimeMs is the mean decision latency over the window.
	AvgProcessingTimeMs float64 `json:"avgProcessingTimeMs" example:"12.4"`

	ActiveRules  int64 `json:"activeRules" example:"20"`
	ActiveLimits int64 `json:"activeLimits" example:"12"`

	WindowStart time.Time `json:"windowStart"`
	WindowEnd   time.Time `json:"windowEnd"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// ApplyRates fills the three derived rates and the single-asset convenience
// pair from the counts already scanned. Keeping it on the entity means the
// postgres adapter scans and this decides, so the cache stores exactly what
// the wire carries and a cache hit cannot disagree with a miss.
func (m *DashboardMetrics) ApplyRates() {
	total := m.TransactionsProcessed
	if total > 0 {
		denominator := float64(total)
		m.ApprovalRate = float64(m.Allowed) / denominator
		m.FraudDetectionRate = float64(m.FraudsBlocked) / denominator
		m.ManualReviewRate = float64(m.ManualReviews) / denominator
	}

	if len(m.AmountSavedByAsset) == 1 {
		m.AmountSaved = m.AmountSavedByAsset[0].Amount.String()
		m.Asset = m.AmountSavedByAsset[0].Asset
	}
}

// VolumePoint is one day of the volume series. Date is a calendar day in UTC
// rendered YYYY-MM-DD, and Volume is a COUNT of validations, never an amount.
type VolumePoint struct {
	Date   string `json:"date" example:"2026-09-19"`
	Volume int64  `json:"volume" example:"2740"`
}

// DashboardVolume is the volume series over the window.
//
// Every period buckets by DAY, including 7d. The bucket is the unit the
// console's data point carries (a YYYY-MM-DD date), so an hourly bucket would
// have nowhere to go on the wire; and a day bucket keeps the series bounded at
// 90 points for the longest window this API accepts.
type DashboardVolume struct {
	Points      []VolumePoint `json:"points"`
	WindowStart time.Time     `json:"windowStart"`
	WindowEnd   time.Time     `json:"windowEnd"`
	UpdatedAt   time.Time     `json:"updatedAt"`
}

// FraudTypeSlice is one transaction type's share of the flagged traffic.
type FraudTypeSlice struct {
	// Type is the transaction type (CARD, WIRE, PIX, CRYPTO).
	Type string `json:"type" example:"CARD"`
	// Count is the number of DENY + REVIEW decisions of this type.
	Count int64 `json:"count" example:"1287"`
	// Percentage is this type's share of all flagged decisions in the window,
	// as a fraction in [0,1]. Shares sum to 1 across the slices.
	Percentage float64 `json:"percentage" example:"0.31"`
	// Total is every decision of this type in the window, flagged or not, so a
	// reader can tell a type that is rare from one that is merely clean.
	Total int64 `json:"total" example:"20536"`
}

// DashboardFraudTypes is the flagged-traffic breakdown by transaction type.
//
// The breakdown is by transaction_type only. sub_type is a free VARCHAR with
// no enum behind it, so grouping on it would produce a label set that grows
// with whatever callers happen to send — an unbounded axis on a fixed chart.
type DashboardFraudTypes struct {
	Types        []FraudTypeSlice `json:"types"`
	TotalFlagged int64            `json:"totalFlagged" example:"4133"`
	WindowStart  time.Time        `json:"windowStart"`
	WindowEnd    time.Time        `json:"windowEnd"`
	UpdatedAt    time.Time        `json:"updatedAt"`
}

// TopRule is one rule's contribution to the window's traffic.
//
// Executions counts the validations that EVALUATED the rule; Matches counts
// those where it fired. DetectionRate is Matches/Executions, so a rule that
// never fires reads 0 rather than being absent — "this rule guards nothing"
// is the answer an operator most needs and the one a MATCHED-only query
// silently withholds.
//
// AvgProcessingMs averages the whole validation's latency over the MATCHED
// validations, not the rule's own evaluation cost, which the trail does not
// record per rule. It answers "are the validations this rule fires on slow",
// never "is this rule slow"; the field name and this sentence are the only
// things standing between a reader and the wrong conclusion.
type TopRule struct {
	// Name is the rule's name, the label the console renders.
	Name string `json:"name" example:"high-value-wire"`
	// ProductType is the transaction type the rule is SCOPED to (CARD, WIRE,
	// PIX, CRYPTO), read from the rule's own scopes — never inferred from the
	// traffic it happened to see. Empty when the rule scopes no transaction
	// type, which means it applies to all of them.
	ProductType string `json:"productType,omitempty" example:"WIRE"`
	// Matches is the number of validations in the window where the rule fired.
	Matches int64 `json:"matches" example:"1287"`
	// Executions is the number of validations in the window that evaluated the
	// rule, whether or not it fired.
	Executions int64 `json:"executions" example:"20536"`
	// DetectionRate is Matches/Executions as a fraction in [0,1]. Zero when the
	// rule was never evaluated.
	DetectionRate float64 `json:"detectionRate" example:"0.063"`
	// AvgProcessingMs is the mean end-to-end validation latency over the
	// validations this rule matched. Zero when it matched none. See the type
	// comment: this is the validation's latency, not the rule's.
	AvgProcessingMs float64 `json:"avgProcessingMs" example:"41.2"`
}

// DashboardTopRules is the busiest rules in the window.
//
// Capped at DashboardTopRulesLimit rows and ordered by Matches descending,
// then by Name ascending so a tie renders the same way on every refresh — an
// unstable order makes a dashboard flicker between two identical readings and
// reads as data changing when nothing has.
//
// Rules that were evaluated but have since been deleted do not appear: the
// rows are joined to the rule table to read the name and scope, so a rule with
// no row has no name to render.
type DashboardTopRules struct {
	Rules       []TopRule `json:"rules"`
	WindowStart time.Time `json:"windowStart"`
	WindowEnd   time.Time `json:"windowEnd"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
