// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"

	"github.com/shopspring/decimal"
)

// CalculateTiered finds the pricing tier that matches billableEvents and returns
// the tier's unit price and the gross amount (billableEvents * unitPrice).
// All events are charged at the single matching tier's rate (volume pricing).
// Returns an error when tiers is empty or no tier covers billableEvents.
func CalculateTiered(billableEvents int64, tiers []model.PricingTier) (decimal.Decimal, decimal.Decimal, error) {
	if billableEvents <= 0 {
		return decimal.Zero, decimal.Zero, nil
	}

	if len(tiers) == 0 {
		return decimal.Zero, decimal.Zero, fmt.Errorf("no pricing tiers provided")
	}

	for _, tier := range tiers {
		if !eventFallsInTier(billableEvents, tier) {
			continue
		}

		gross := decimal.NewFromInt(billableEvents).Mul(tier.UnitPrice)

		return tier.UnitPrice, gross, nil
	}

	return decimal.Zero, decimal.Zero, fmt.Errorf("no matching tier for %d events", billableEvents)
}

// eventFallsInTier returns true when billableEvents is within the tier's
// [MinQuantity, MaxQuantity] range. A nil MaxQuantity means unbounded (no upper limit).
func eventFallsInTier(billableEvents int64, tier model.PricingTier) bool {
	if billableEvents < tier.MinQuantity {
		return false
	}

	if tier.MaxQuantity != nil && billableEvents > *tier.MaxQuantity {
		return false
	}

	return true
}

// ApplyFreeQuota subtracts freeQuota from totalEvents and returns the billable
// event count. The result is clamped to zero (never negative).
// If freeQuota is zero or negative the totalEvents value is returned unchanged.
// If totalEvents is zero or negative the result is always zero.
func ApplyFreeQuota(totalEvents int64, freeQuota int) int64 {
	if totalEvents <= 0 {
		return 0
	}

	if freeQuota <= 0 {
		return totalEvents
	}

	billable := totalEvents - int64(freeQuota)
	if billable < 0 {
		return 0
	}

	return billable
}

// ApplyDiscount finds the highest applicable discount tier for the given
// totalEvents count and applies its percentage to grossAmount.
// Only one tier is used (the one with the largest MinQuantity that totalEvents
// meets or exceeds). The function returns the net amount after discount and a
// DiscountDetail describing the tier used. When no tier applies the gross
// amount is returned unchanged with a nil detail.
func ApplyDiscount(grossAmount decimal.Decimal, totalEvents int64, discountTiers []model.DiscountTier) (decimal.Decimal, *model.DiscountDetail) {
	if len(discountTiers) == 0 {
		return grossAmount, nil
	}

	oneHundred := decimal.NewFromInt(100)

	// Find the highest applicable tier (largest MinQuantity <= totalEvents).
	bestIdx := -1

	for i, tier := range discountTiers {
		if totalEvents >= tier.MinQuantity {
			if bestIdx == -1 || tier.MinQuantity > discountTiers[bestIdx].MinQuantity {
				bestIdx = i
			}
		}
	}

	if bestIdx == -1 {
		return grossAmount, nil
	}

	tier := discountTiers[bestIdx]
	discountAmount := grossAmount.Mul(tier.DiscountPercentage).Div(oneHundred)
	netAmount := grossAmount.Sub(discountAmount)

	detail := &model.DiscountDetail{
		MinQuantity:        tier.MinQuantity,
		DiscountPercentage: tier.DiscountPercentage,
		DiscountAmount:     discountAmount,
	}

	return netAmount, detail
}

// CalculateFixed returns grossAmount = billableEvents * unitPrice.
// If billableEvents is zero or negative the result is decimal.Zero.
func CalculateFixed(billableEvents int64, unitPrice decimal.Decimal) decimal.Decimal {
	if billableEvents <= 0 {
		return decimal.Zero
	}

	return decimal.NewFromInt(billableEvents).Mul(unitPrice)
}
