// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg/dashboard"
)

// Dashboard window bounds and parsing live in the root pkg/dashboard package,
// shared with the ledger's own /dashboard family. The aliases below keep this
// package's published names — every call site and every SDK-facing sentence
// still reads model.NewDashboardWindow — while there is exactly ONE definition
// of what "last 30 days" means across the two deploy surfaces.
const (
	// DashboardMaxWindow is the longest window any dashboard read may cover.
	DashboardMaxWindow = dashboard.MaxWindow

	// DashboardDefaultPeriod is the period applied when the caller names none.
	DashboardDefaultPeriod = dashboard.DefaultPeriod

	// DashboardWindowGranularity is the resolution both bounds are snapped to;
	// it equals the cache TTL. See pkg/dashboard for why start floors and end
	// ceils.
	DashboardWindowGranularity = dashboard.Granularity

	// DashboardTopRulesLimit caps the top-rules panel. Ten is what the console
	// renders; it is not a caller-supplied page size, because the query's cost is
	// in the aggregation over the window and not in the rows it returns, so a
	// larger limit would buy a caller nothing and a paging contract even less.
	DashboardTopRulesLimit = 10
)

// DashboardWindow is the half-open time range [From, To) every dashboard read
// aggregates over.
type DashboardWindow = dashboard.Window

// NewDashboardWindow normalizes the query parameters of a dashboard read into
// a window. now is injected (never time.Now()) so the caller's clock is the
// service clock and tests are deterministic.
func NewDashboardWindow(period, startDate, endDate string, now time.Time) (DashboardWindow, error) {
	return dashboard.NewWindow(period, startDate, endDate, now)
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
