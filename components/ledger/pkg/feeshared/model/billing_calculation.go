// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// daysPerWeek is the number of days in a week, used for weekly period calculations.
const daysPerWeek = 7

// yearDigits is the expected length of the year component in period formats.
const yearDigits = 4

// weekDigits is the expected length of the week component per ISO 8601 (e.g. "01", "53").
const weekDigits = 2

// LooksLikeWeeklyPeriod reports whether the string structurally matches "YYYY-Www"
// without validating whether the week actually exists in that year.
// Use this to distinguish a wrong-format input from a valid-format-but-nonexistent-week input.
//
// WARNING: This function does NOT verify that the week exists in the given year.
// For full validation (including ISO week existence check), use ParseWeeklyPeriod.
func LooksLikeWeeklyPeriod(period string) bool {
	parts := strings.Split(period, "-")
	if len(parts) != 2 || len(parts[1]) < 2 || parts[1][0] != 'W' {
		return false
	}

	if len(parts[0]) != yearDigits {
		return false
	}

	for _, c := range parts[0] {
		if c < '0' || c > '9' {
			return false
		}
	}

	weekStr := parts[1][1:]

	if len(weekStr) != weekDigits {
		return false
	}

	week, err := strconv.Atoi(weekStr)

	return err == nil && week >= 1 && week <= 53
}

// ParseWeeklyPeriod parses a weekly period in ISO 8601 format "YYYY-Www" (e.g. "2026-W13").
// It returns the start (Monday 00:00 UTC) and end (following Monday 00:00 UTC) of the week.
// The third return value indicates whether the input was a valid weekly period.
// Both the structural format and the ISO week existence (e.g. W53 on years with 52 weeks) are validated.
func ParseWeeklyPeriod(period string) (time.Time, time.Time, bool) {
	parts := strings.Split(period, "-")
	if len(parts) != 2 || len(parts[1]) < 2 || parts[1][0] != 'W' {
		return time.Time{}, time.Time{}, false
	}

	year, err := strconv.Atoi(parts[0])
	if err != nil || len(parts[0]) != yearDigits {
		return time.Time{}, time.Time{}, false
	}

	weekStr := parts[1][1:]
	if len(weekStr) != weekDigits {
		return time.Time{}, time.Time{}, false
	}

	week, err := strconv.Atoi(weekStr)
	if err != nil || week < 1 || week > 53 {
		return time.Time{}, time.Time{}, false
	}

	// ISO 8601: Week 1 contains January 4th. Find Monday of week 1,
	// then advance to the requested week.
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	weekday := jan4.Weekday()

	// Subtract time.Monday (=1) from weekday to get the days-to-Monday offset; add 7 if negative.
	daysToMonday := int(weekday) - int(time.Monday)
	if daysToMonday < 0 {
		daysToMonday += 7
	}

	week1Monday := jan4.AddDate(0, 0, -daysToMonday)
	start := week1Monday.AddDate(0, 0, (week-1)*daysPerWeek)

	// Validate: the computed start must belong to the requested ISO year/week.
	isoYear, isoWeek := start.ISOWeek()
	if isoYear != year || isoWeek != week {
		return time.Time{}, time.Time{}, false
	}

	end := start.AddDate(0, 0, daysPerWeek)

	return start, end, true
}

// BillingCalculateRequest carries the parameters required to trigger a billing
// calculation for a given organisation, ledger, and period.
//
// Period must be in "YYYY-MM" (monthly), "YYYY-Www" (weekly), or "YYYY-MM-DD" (daily) format (e.g. "2026-01", "2026-W13", or "2026-01-15").
// Type is optional; when provided it restricts the calculation to "volume" or
// "maintenance" packages only. When omitted both types are calculated.
type BillingCalculateRequest struct {
	OrganizationID string `json:"-"`
	LedgerID       string `json:"ledgerId"       validate:"required" example:"00000000-0000-0000-0000-000000000000"`
	Period         string `json:"period"         validate:"required" example:"2026-01"` // YYYY-MM, YYYY-Www, or YYYY-MM-DD format
	Type           string `json:"type,omitempty" example:"volume"`                      // "volume", "maintenance", or empty for both
}

// BillingCalculationResult represents the billing outcome for a single billing
// package within the requested period.
//
// Volume metadata fields (stored in TransactionPayload.Metadata):
//   - "billingPackageId"   — ID of the billing package applied
//   - "billingPeriod"      — period in "YYYY-MM" format
//   - "totalAccounts"      — total number of accounts evaluated
//   - "totalCharged"       — number of accounts that were charged
//   - "totalSkipped"       — number of accounts skipped (e.g. free-quota exhausted)
//   - "unitPrice"          — unit price used for the calculation (decimal string)
//   - "discountPercentage" — discount percentage applied (decimal string)
//   - "discountAmount"     — total discount amount deducted (decimal string)
//
// Maintenance metadata fields (stored in TransactionPayload.Metadata):
//   - "billingPackageId"   — ID of the billing package applied
//   - "billingPeriod"      — period in "YYYY-MM" format
//   - "totalAccounts"      — total number of accounts charged in this batch
//   - "feeAmount"          — fixed maintenance fee per account (decimal string)
type BillingCalculationResult struct {
	BillingPackageID    string          `json:"billingPackageId" example:"00000000-0000-0000-0000-000000000000"`
	BillingPackageLabel string          `json:"billingPackageLabel" example:"Monthly Volume Billing"`
	BillingType         string          `json:"billingType" example:"volume" enums:"volume,maintenance"` // "volume" or "maintenance"
	Period              string          `json:"period" example:"2026-01"`
	TotalAccounts       int             `json:"totalAccounts" example:"500"`
	TotalCharged        int             `json:"totalCharged" example:"480"`
	TotalSkipped        int             `json:"totalSkipped" example:"20"`
	TotalNetAmount      decimal.Decimal `json:"totalNetAmount" swaggertype:"string" example:"123.45"`
	TransactionPayload  json.RawMessage `json:"transactionPayload" swaggertype:"object"`
}

// BillingCalculateSummary aggregates the totals across all BillingCalculationResult
// entries returned in a single BillingCalculateResponse.
type BillingCalculateSummary struct {
	TotalResults     int             `json:"totalResults" example:"10"`
	TotalVolume      int             `json:"totalVolume" example:"7"`
	TotalMaintenance int             `json:"totalMaintenance" example:"3"`
	TotalNetAmount   decimal.Decimal `json:"totalNetAmount" swaggertype:"string" example:"456.78"`
}

// BillingCalculateResponse is the top-level response returned by the billing
// calculation endpoint. It contains one BillingCalculationResult per billing
// package processed and a consolidated BillingCalculateSummary.
type BillingCalculateResponse struct {
	Results []BillingCalculationResult `json:"results"`
	Summary BillingCalculateSummary    `json:"summary"`
}

// DiscountDetail carries the discount information applied to a single pricing
// tier during a volume billing calculation. It is used internally when building
// the transaction payload metadata.
type DiscountDetail struct {
	DiscountPercentage decimal.Decimal `json:"discountPercentage" swaggertype:"string" example:"10.00"`
	DiscountAmount     decimal.Decimal `json:"discountAmount" swaggertype:"string" example:"12.34"`
	MinQuantity        int64           `json:"minQuantity"`
}
