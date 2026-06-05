// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// int64Ptr is a test helper that returns a pointer to the given int64 value.
func int64Ptr(v int64) *int64 {
	return &v
}

func TestCalculateTiered(t *testing.T) {
	t.Parallel()

	// Reusable tier definitions for multiple test cases.
	multiTiers := []model.PricingTier{
		{MinQuantity: 0, MaxQuantity: int64Ptr(100), UnitPrice: decimal.NewFromFloat(1.50)},
		{MinQuantity: 101, MaxQuantity: int64Ptr(500), UnitPrice: decimal.NewFromFloat(1.00)},
		{MinQuantity: 501, MaxQuantity: nil, UnitPrice: decimal.NewFromFloat(0.85)},
	}

	tests := []struct {
		name            string
		billableEvents  int64
		tiers           []model.PricingTier
		wantUnitPrice   decimal.Decimal
		wantGrossAmount decimal.Decimal
		wantErr         bool
	}{
		{
			name:           "single unbounded tier with 100 events",
			billableEvents: 100,
			tiers: []model.PricingTier{
				{MinQuantity: 0, MaxQuantity: nil, UnitPrice: decimal.NewFromFloat(1.00)},
			},
			wantUnitPrice:   decimal.NewFromFloat(1.00),
			wantGrossAmount: decimal.NewFromFloat(100.00),
			wantErr:         false,
		},
		{
			name:            "multiple tiers hits first tier with 50 events",
			billableEvents:  50,
			tiers:           multiTiers,
			wantUnitPrice:   decimal.NewFromFloat(1.50),
			wantGrossAmount: decimal.NewFromFloat(75.00),
			wantErr:         false,
		},
		{
			name:            "multiple tiers hits middle tier with 300 events",
			billableEvents:  300,
			tiers:           multiTiers,
			wantUnitPrice:   decimal.NewFromFloat(1.00),
			wantGrossAmount: decimal.NewFromFloat(300.00),
			wantErr:         false,
		},
		{
			name:            "multiple tiers hits last unbounded tier with 763 events",
			billableEvents:  763,
			tiers:           multiTiers,
			wantUnitPrice:   decimal.NewFromFloat(0.85),
			wantGrossAmount: decimal.NewFromFloat(648.55),
			wantErr:         false,
		},
		{
			name:            "exact lower boundary of second tier with 101 events",
			billableEvents:  101,
			tiers:           multiTiers,
			wantUnitPrice:   decimal.NewFromFloat(1.00),
			wantGrossAmount: decimal.NewFromFloat(101.00),
			wantErr:         false,
		},
		{
			name:            "exact upper boundary of first tier with 100 events",
			billableEvents:  100,
			tiers:           multiTiers,
			wantUnitPrice:   decimal.NewFromFloat(1.50),
			wantGrossAmount: decimal.NewFromFloat(150.00),
			wantErr:         false,
		},
		{
			name:            "one below boundary stays in first tier with 99 events",
			billableEvents:  99,
			tiers:           multiTiers,
			wantUnitPrice:   decimal.NewFromFloat(1.50),
			wantGrossAmount: decimal.NewFromFloat(148.50),
			wantErr:         false,
		},
		{
			name:            "zero events returns zero amounts",
			billableEvents:  0,
			tiers:           multiTiers,
			wantUnitPrice:   decimal.Zero,
			wantGrossAmount: decimal.Zero,
			wantErr:         false,
		},
		{
			name:           "no matching tier returns error or zero",
			billableEvents: 500,
			tiers: []model.PricingTier{
				{MinQuantity: 0, MaxQuantity: int64Ptr(100), UnitPrice: decimal.NewFromFloat(1.50)},
				{MinQuantity: 200, MaxQuantity: int64Ptr(300), UnitPrice: decimal.NewFromFloat(1.00)},
			},
			wantUnitPrice:   decimal.Zero,
			wantGrossAmount: decimal.Zero,
			wantErr:         true,
		},
		{
			name:           "events in gap between tiers returns error",
			billableEvents: 150,
			tiers: []model.PricingTier{
				{MinQuantity: 0, MaxQuantity: int64Ptr(100), UnitPrice: decimal.NewFromFloat(1.50)},
				{MinQuantity: 200, MaxQuantity: int64Ptr(300), UnitPrice: decimal.NewFromFloat(1.00)},
			},
			wantUnitPrice:   decimal.Zero,
			wantGrossAmount: decimal.Zero,
			wantErr:         true,
		},
		{
			name:            "negative events returns zero without error",
			billableEvents:  -5,
			tiers:           multiTiers,
			wantUnitPrice:   decimal.Zero,
			wantGrossAmount: decimal.Zero,
			wantErr:         false,
		},
		{
			name:           "single event matches tier starting at min 1",
			billableEvents: 1,
			tiers: []model.PricingTier{
				{MinQuantity: 1, MaxQuantity: int64Ptr(10), UnitPrice: decimal.NewFromFloat(2.00)},
			},
			wantUnitPrice:   decimal.NewFromFloat(2.00),
			wantGrossAmount: decimal.NewFromFloat(2.00),
			wantErr:         false,
		},
		{
			name:            "empty tiers slice returns error or zero",
			billableEvents:  100,
			tiers:           []model.PricingTier{},
			wantUnitPrice:   decimal.Zero,
			wantGrossAmount: decimal.Zero,
			wantErr:         true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			unitPrice, grossAmount, err := CalculateTiered(tc.billableEvents, tc.tiers)

			if tc.wantErr {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
			assert.True(t, tc.wantUnitPrice.Equal(unitPrice),
				"unitPrice: want %s, got %s", tc.wantUnitPrice.String(), unitPrice.String())
			assert.True(t, tc.wantGrossAmount.Equal(grossAmount),
				"grossAmount: want %s, got %s", tc.wantGrossAmount.String(), grossAmount.String())
		})
	}
}

func TestApplyFreeQuota(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		totalEvents int64
		freeQuota   int
		want        int64
	}{
		{
			name:        "normal quota deduction with 750 events and quota 10",
			totalEvents: 750,
			freeQuota:   10,
			want:        740,
		},
		{
			name:        "zero quota returns totalEvents unchanged",
			totalEvents: 750,
			freeQuota:   0,
			want:        750,
		},
		{
			name:        "negative quota returns totalEvents unchanged",
			totalEvents: 750,
			freeQuota:   -5,
			want:        750,
		},
		{
			name:        "quota exceeds events returns zero",
			totalEvents: 5,
			freeQuota:   10,
			want:        0,
		},
		{
			name:        "zero events returns zero",
			totalEvents: 0,
			freeQuota:   10,
			want:        0,
		},
		{
			name:        "negative events returns zero",
			totalEvents: -5,
			freeQuota:   10,
			want:        0,
		},
		{
			name:        "quota equals events returns zero",
			totalEvents: 10,
			freeQuota:   10,
			want:        0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ApplyFreeQuota(tc.totalEvents, tc.freeQuota)

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestApplyDiscount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		grossAmount   decimal.Decimal
		totalEvents   int64
		discountTiers []model.DiscountTier
		wantNetAmount decimal.Decimal
		wantDetail    *model.DiscountDetail
	}{
		{
			name:        "single tier applies 5 percent discount",
			grossAmount: decimal.NewFromFloat(629.00),
			totalEvents: 740,
			discountTiers: []model.DiscountTier{
				{MinQuantity: 500, DiscountPercentage: decimal.NewFromInt(5)},
			},
			wantNetAmount: decimal.NewFromFloat(597.55),
			wantDetail: &model.DiscountDetail{
				MinQuantity:        500,
				DiscountPercentage: decimal.NewFromInt(5),
				DiscountAmount:     decimal.NewFromFloat(31.45),
			},
		},
		{
			name:        "multiple tiers uses highest applicable tier",
			grossAmount: decimal.NewFromInt(1000),
			totalEvents: 800,
			discountTiers: []model.DiscountTier{
				{MinQuantity: 100, DiscountPercentage: decimal.NewFromInt(2)},
				{MinQuantity: 500, DiscountPercentage: decimal.NewFromInt(5)},
				{MinQuantity: 1000, DiscountPercentage: decimal.NewFromInt(10)},
			},
			wantNetAmount: decimal.NewFromInt(950),
			wantDetail: &model.DiscountDetail{
				MinQuantity:        500,
				DiscountPercentage: decimal.NewFromInt(5),
				DiscountAmount:     decimal.NewFromInt(50),
			},
		},
		{
			name:        "no tier applies when events below all minQuantities",
			grossAmount: decimal.NewFromInt(100),
			totalEvents: 50,
			discountTiers: []model.DiscountTier{
				{MinQuantity: 100, DiscountPercentage: decimal.NewFromInt(5)},
			},
			wantNetAmount: decimal.NewFromInt(100),
			wantDetail:    nil,
		},
		{
			name:          "zero events with zero gross returns zero and nil detail",
			grossAmount:   decimal.Zero,
			totalEvents:   0,
			discountTiers: []model.DiscountTier{},
			wantNetAmount: decimal.Zero,
			wantDetail:    nil,
		},
		{
			name:        "full scenario from spec with 740 events and 5 percent discount",
			grossAmount: decimal.NewFromFloat(629.00),
			totalEvents: 740,
			discountTiers: []model.DiscountTier{
				{MinQuantity: 500, DiscountPercentage: decimal.NewFromInt(5)},
			},
			wantNetAmount: decimal.NewFromFloat(597.55),
			wantDetail: &model.DiscountDetail{
				MinQuantity:        500,
				DiscountPercentage: decimal.NewFromInt(5),
				DiscountAmount:     decimal.NewFromFloat(31.45),
			},
		},
		{
			name:          "empty discount tiers returns gross unchanged with nil detail",
			grossAmount:   decimal.NewFromInt(500),
			totalEvents:   100,
			discountTiers: []model.DiscountTier{},
			wantNetAmount: decimal.NewFromInt(500),
			wantDetail:    nil,
		},
		{
			name:        "events exactly at tier minQuantity applies discount",
			grossAmount: decimal.NewFromInt(1000),
			totalEvents: 500,
			discountTiers: []model.DiscountTier{
				{MinQuantity: 500, DiscountPercentage: decimal.NewFromInt(5)},
			},
			wantNetAmount: decimal.NewFromInt(950),
			wantDetail: &model.DiscountDetail{
				MinQuantity:        500,
				DiscountPercentage: decimal.NewFromInt(5),
				DiscountAmount:     decimal.NewFromInt(50),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			netAmount, detail := ApplyDiscount(tc.grossAmount, tc.totalEvents, tc.discountTiers)

			assert.True(t, tc.wantNetAmount.Equal(netAmount),
				"netAmount: want %s, got %s", tc.wantNetAmount.String(), netAmount.String())

			if tc.wantDetail == nil {
				assert.Nil(t, detail)
			} else {
				assert.NotNil(t, detail)
				assert.Equal(t, tc.wantDetail.MinQuantity, detail.MinQuantity)
				assert.True(t, tc.wantDetail.DiscountPercentage.Equal(detail.DiscountPercentage),
					"discountPercentage: want %s, got %s",
					tc.wantDetail.DiscountPercentage.String(), detail.DiscountPercentage.String())
				assert.True(t, tc.wantDetail.DiscountAmount.Equal(detail.DiscountAmount),
					"discountAmount: want %s, got %s",
					tc.wantDetail.DiscountAmount.String(), detail.DiscountAmount.String())
			}
		})
	}
}

func TestCalculateFixed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		billableEvents  int64
		unitPrice       decimal.Decimal
		wantGrossAmount decimal.Decimal
	}{
		{
			name:            "normal calculation with 763 events at 1.00",
			billableEvents:  763,
			unitPrice:       decimal.NewFromFloat(1.00),
			wantGrossAmount: decimal.NewFromFloat(763.00),
		},
		{
			name:            "zero events returns zero",
			billableEvents:  0,
			unitPrice:       decimal.NewFromFloat(1.00),
			wantGrossAmount: decimal.Zero,
		},
		{
			name:            "fractional price with 100 events at 0.15",
			billableEvents:  100,
			unitPrice:       decimal.NewFromFloat(0.15),
			wantGrossAmount: decimal.NewFromFloat(15.00),
		},
		{
			name:            "large volume with 1000000 events at 0.01",
			billableEvents:  1_000_000,
			unitPrice:       decimal.NewFromFloat(0.01),
			wantGrossAmount: decimal.NewFromFloat(10_000.00),
		},
		{
			name:            "negative events returns zero",
			billableEvents:  -10,
			unitPrice:       decimal.NewFromFloat(5.00),
			wantGrossAmount: decimal.Zero,
		},
		{
			name:            "single event at zero price returns zero",
			billableEvents:  1,
			unitPrice:       decimal.Zero,
			wantGrossAmount: decimal.Zero,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			grossAmount := CalculateFixed(tc.billableEvents, tc.unitPrice)

			assert.True(t, tc.wantGrossAmount.Equal(grossAmount),
				"grossAmount: want %s, got %s", tc.wantGrossAmount.String(), grossAmount.String())
		})
	}
}
