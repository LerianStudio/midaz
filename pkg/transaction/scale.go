// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"fmt"

	"github.com/shopspring/decimal"
)

const (
	// MaxSafeInteger is the maximum integer value that can be represented exactly in a float64 (used by Lua 5.1)
	MaxSafeInteger int64 = 9007199254740992 // 2^53

	// MaxScaledDigits is the maximum number of digits in a scaled integer value
	MaxScaledDigits = 15

	// DefaultScale is used when no scale is specified (2 for fiat currencies)
	DefaultScale int32 = 2

	// MaxAllowedScale is the maximum supported scale for integer arithmetic.
	// At scale=15, the maximum representable value is ~9007 (2^53 / 10^15).
	// Higher scales risk integer overflow in Lua's float64 arithmetic.
	// This covers fiat currencies (2-4dp), most crypto (8-10dp for BTC/ETH),
	// and edge cases up to 15 decimal places.
	MaxAllowedScale int32 = 15
)

// DetermineScale returns the number of decimal places needed to represent a decimal.Decimal value
// as an integer without loss of precision. For example:
// - "1500.50" → 2
// - "0.00000001" → 8
// - "1500" → 0
func DetermineScale(d decimal.Decimal) int32 {
	exp := d.Exponent()
	if exp >= 0 {
		return 0
	}

	return -exp
}

// MaxScale returns the maximum scale across multiple decimal values.
// This is used to find a common scale for all values in a transaction.
func MaxScale(values ...decimal.Decimal) int32 {
	var maxScale int32

	for _, v := range values {
		s := DetermineScale(v)
		if s > maxScale {
			maxScale = s
		}
	}

	return maxScale
}

// ScaleToInt converts a decimal.Decimal to a scaled int64 at the given scale.
// For example: ScaleToInt("1500.50", 2) → 150050
// Returns the scaled value and an error if the result exceeds MaxSafeInteger.
func ScaleToInt(d decimal.Decimal, scale int32) (int64, error) {
	scaled := d.Shift(scale) // multiply by 10^scale

	if scaled.Exponent() < 0 {
		// Still has fractional part after scaling — this means precision loss
		return 0, fmt.Errorf("amount %s has more decimal places than scale %d allows", d.String(), scale)
	}

	val := scaled.IntPart()

	if val > MaxSafeInteger || val < -MaxSafeInteger {
		return 0, fmt.Errorf("scaled amount %d exceeds maximum safe integer %d (original: %s, scale: %d)", val, MaxSafeInteger, d.String(), scale)
	}

	return val, nil
}

// IntToDecimal converts a scaled int64 back to a decimal.Decimal at the given scale.
// For example: IntToDecimal(150050, 2) → "1500.50"
func IntToDecimal(val int64, scale int32) decimal.Decimal {
	return decimal.New(val, -scale)
}

// ValidateAmountPrecision checks that an amount can be safely represented as a
// scaled integer for Lua float64 arithmetic. This is the 15-digit guard that
// prevents silent precision loss in the Redis atomic balance operations.
//
// If scale is 0, the amount's own decimal places are used as the scale.
func ValidateAmountPrecision(amount decimal.Decimal, scale int32) error {
	if scale == 0 {
		scale = DetermineScale(amount)
	}

	_, err := ScaleToInt(amount, scale)

	return err
}
