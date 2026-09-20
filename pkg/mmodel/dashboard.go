// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"time"

	"github.com/shopspring/decimal"
)

// Dashboard money is ALWAYS reported per asset, and these types have no field
// that could hold a cross-asset total. A ledger carries as many assets as its
// operator opened; adding BRL to USD produces a number that is not money, and
// the only reliable way to keep a dashboard from publishing one is to leave it
// nowhere to put it.
//
// Every amount is exact end to end — decimal from the scan to the wire, never
// through a float — but the SCALE is not normative, because decimal normalises
// it: a stored 41000.00 renders "41000" and 10352080.80 renders "10352080.8".
// A consumer formats to the asset's exponent rather than echoing the string,
// and never passes the value through a binary float.

// DashboardAssetVolume is one asset's settled volume inside a window.
//
// Amount is the sum of transaction.amount over SETTLED transactions only
// (constant.SettledTransactionStatuses — APPROVED), and Transactions counts
// exactly the rows that sum contains. The pair therefore always describes the
// same set of rows: a reader can divide one by the other and get an average
// that means something.
type DashboardAssetVolume struct {
	// Asset is the asset code the volume was carried in.
	Asset string `json:"asset" example:"BRL"`
	// Amount is the settled volume in that asset, as an exact decimal string.
	Amount decimal.Decimal `json:"amount" example:"41000.00"`
	// Transactions is how many settled transactions produced Amount.
	Transactions int64 `json:"transactions" example:"128"`
}

// DashboardMetrics is the headline panel: how much traffic the window carried,
// how it was decided, and how much money actually moved in each asset.
type DashboardMetrics struct {
	// Total is every transaction created in the window, whatever its status and
	// whatever asset it carried. It is the one figure in this type that is NOT
	// per asset, and it is a count rather than money.
	Total int64 `json:"total" example:"1420"`

	// ByStatus carries one entry per status the ledger knows, present with 0
	// when the window contains none — a status that simply vanished from the
	// response would read as "this ledger has no such state" rather than "none
	// happened". A status the data carries but the ledger's list does not also
	// appears, so an unknown status is visible instead of silently folded into
	// Total.
	ByStatus map[string]int64 `json:"byStatus"`

	// VolumeByAsset holds one entry per asset that carried at least one settled
	// transaction in the window. Empty when the window settled nothing.
	VolumeByAsset []DashboardAssetVolume `json:"volumeByAsset"`

	WindowStart time.Time `json:"windowStart"`
	WindowEnd   time.Time `json:"windowEnd"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// DashboardVolumePoint is one UTC calendar day of the volume series.
//
// Transactions counts EVERY transaction created that day, matching
// DashboardMetrics.Total's rule, so the points sum to it. ByAsset applies
// DashboardMetrics.VolumeByAsset's settled rule to that day alone, so summing a
// given asset's Amount across the points reproduces that asset's entry in
// /metrics. Those two identities are what let an operator reconcile the chart
// against the headline instead of wondering which one is lying.
type DashboardVolumePoint struct {
	// Date is the UTC calendar day, rendered YYYY-MM-DD.
	Date string `json:"date" example:"2026-09-19"`
	// Transactions is every transaction created that day, any status, any asset.
	Transactions int64 `json:"transactions" example:"212"`
	// ByAsset is that day's settled volume per asset. Empty on a day that
	// settled nothing, including a day with PENDING traffic only.
	ByAsset []DashboardAssetVolume `json:"byAsset"`
}

// DashboardVolume is the per-day series over the window.
//
// Every UTC calendar day the window TOUCHES is present, including days with no
// transactions, which carry Transactions 0 and an empty ByAsset. A window is
// half-open and its bounds snap to the minute rather than to midnight, so a 7d
// window opened mid-afternoon touches EIGHT days: the partial day it started
// in and the partial day it ends in. That is the contract, not an off-by-one.
type DashboardVolume struct {
	Points      []DashboardVolumePoint `json:"points"`
	WindowStart time.Time              `json:"windowStart"`
	WindowEnd   time.Time              `json:"windowEnd"`
	UpdatedAt   time.Time              `json:"updatedAt"`
}

// DashboardAssetPosition is the ledger's CURRENT position in one asset.
//
// Available and OnHold are summed from the balance table, whose values are
// DECIMAL with each row's scale already folded in (migration 000005), so the
// sum is exact whatever precision the individual accounts hold the asset at.
// The sum is per asset and there is nowhere here to put a cross-asset total.
type DashboardAssetPosition struct {
	// Asset is the asset code.
	Asset string `json:"asset" example:"BRL"`
	// Accounts is how many DISTINCT accounts hold a balance in this asset. An
	// account may carry several balance rows for one asset (one per balance
	// key), and counting rows would report more accounts than exist.
	Accounts int64 `json:"accounts" example:"42"`
	// Available is the summed available position, as an exact decimal string.
	Available decimal.Decimal `json:"available" example:"128400.55"`
	// OnHold is the summed held position, as an exact decimal string.
	OnHold decimal.Decimal `json:"onHold" example:"1200.00"`
}

// DashboardAssets is the current position per asset.
//
// It carries NO window and ignores any window parameter: a balance is a
// running total the ledger maintains, not something aggregated over history,
// so "the position over the last 7 days" is not a question the balance table
// can answer. Reading it is proportional to the ledger's account count, never
// to its transaction history.
type DashboardAssets struct {
	Assets    []DashboardAssetPosition `json:"assets"`
	UpdatedAt time.Time                `json:"updatedAt"`
}
