// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// newTestAdapter creates an adapter for testing with default config.
func newTestAdapter(t testing.TB) *Adapter {
	t.Helper()

	logger := testutil.NewMockLogger()
	cfg := AdapterConfig{
		CostLimit: DefaultCostLimit,
	}

	adapter, err := NewAdapter(cfg, logger)
	require.NoError(t, err)

	return adapter
}

// newTestAdapterWithCostLimit creates an adapter with a custom cost limit.
func newTestAdapterWithCostLimit(t testing.TB, costLimit uint64) *Adapter {
	t.Helper()

	logger := testutil.NewMockLogger()
	cfg := AdapterConfig{
		CostLimit: costLimit,
	}

	adapter, err := NewAdapter(cfg, logger)
	require.NoError(t, err)

	return adapter
}

// Test UUIDs for consistent testing
var (
	testAccountID   = uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	testMerchantID  = uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	testSegmentID   = uuid.MustParse("550e8400-e29b-41d4-a716-446655440020")
	testPortfolioID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440021")
)

// testTimestamp is a fixed, deterministic timestamp for testing (2025-01-15 10:30:00 UTC).
var testTimestamp = time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

// newTestRequest creates a full ValidationRequest for testing.
func newTestRequest() *model.ValidationRequest {
	subType := "instant"

	return &model.ValidationRequest{
		TransactionType:      "PIX",
		SubType:              &subType,
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "BRL",
		TransactionTimestamp: testTimestamp,
		Account: model.AccountContext{
			ID:     testAccountID,
			Type:   "checking",
			Status: "active",
		},
		Merchant: &model.MerchantContext{
			ID:       testMerchantID,
			Name:     "Test Store",
			Category: "5411",
			Country:  "BR",
		},
		Segment:   &model.SegmentContext{ID: testSegmentID, Name: "retail"},
		Portfolio: &model.PortfolioContext{ID: testPortfolioID, Name: "premium"},
		Metadata:  map[string]any{"channel": "mobile", "risk_score": 75},
	}
}
