// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import "github.com/shopspring/decimal"

// scopeInput represents scope criteria for validation requests.
// Shared across multiple test files in this package.
type scopeInput struct {
	AccountID       *string `json:"accountId,omitempty"`
	SegmentID       *string `json:"segmentId,omitempty"`
	PortfolioID     *string `json:"portfolioId,omitempty"`
	MerchantID      *string `json:"merchantId,omitempty"`
	TransactionType *string `json:"transactionType,omitempty"`
	SubType         *string `json:"subType,omitempty"`
}

// usageCounterResponse represents a single usage counter for limit tracking.
type usageCounterResponse struct {
	ID            string          `json:"usageCounterId"`
	LimitID       string          `json:"limitId"`
	ScopeKey      string          `json:"scopeKey"`
	PeriodKey     string          `json:"periodKey"`
	CurrentUsage  decimal.Decimal `json:"currentUsage"`
	LastUpdatedAt string          `json:"lastUpdatedAt"`
}
