// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetermineScale(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int32
	}{
		{"integer", "1500", 0},
		{"two_decimal_places", "1500.50", 2},
		{"eight_decimal_places", "0.00000001", 8},
		{"one_decimal_place", "42.5", 1},
		{"negative_value", "-1500.50", 2},
		{"zero", "0", 0},
		{"zero_with_decimals", "0.00", 2},
		{"large_integer", "999999999999999", 0},
		{"trailing_zeros_preserved", "10.10", 2}, // shopspring/decimal preserves trailing zeros
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := decimal.NewFromString(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, DetermineScale(d))
		})
	}
}

func TestMaxScale(t *testing.T) {
	t.Run("returns_highest_scale", func(t *testing.T) {
		a := decimal.NewFromFloat(1500.50) // 2dp (note: float may give more)
		b := decimal.RequireFromString("0.00000001")
		c := decimal.NewFromInt(1000)

		result := MaxScale(a, b, c)
		assert.GreaterOrEqual(t, result, int32(8))
	})

	t.Run("all_integers", func(t *testing.T) {
		a := decimal.NewFromInt(100)
		b := decimal.NewFromInt(200)

		assert.Equal(t, int32(0), MaxScale(a, b))
	})

	t.Run("empty_input", func(t *testing.T) {
		assert.Equal(t, int32(0), MaxScale())
	})

	t.Run("single_value", func(t *testing.T) {
		d := decimal.RequireFromString("1.23")
		assert.Equal(t, int32(2), MaxScale(d))
	})
}

func TestScaleToInt(t *testing.T) {
	t.Run("basic_conversion", func(t *testing.T) {
		d := decimal.RequireFromString("1500.50")
		val, err := ScaleToInt(d, 2)
		require.NoError(t, err)
		assert.Equal(t, int64(150050), val)
	})

	t.Run("higher_scale_pads_zeros", func(t *testing.T) {
		d := decimal.RequireFromString("1500.50")
		val, err := ScaleToInt(d, 4)
		require.NoError(t, err)
		assert.Equal(t, int64(15005000), val)
	})

	t.Run("satoshi_scale", func(t *testing.T) {
		d := decimal.RequireFromString("0.00000001")
		val, err := ScaleToInt(d, 8)
		require.NoError(t, err)
		assert.Equal(t, int64(1), val)
	})

	t.Run("integer_at_zero_scale", func(t *testing.T) {
		d := decimal.NewFromInt(1500)
		val, err := ScaleToInt(d, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(1500), val)
	})

	t.Run("negative_value", func(t *testing.T) {
		d := decimal.RequireFromString("-1500.50")
		val, err := ScaleToInt(d, 2)
		require.NoError(t, err)
		assert.Equal(t, int64(-150050), val)
	})

	t.Run("zero", func(t *testing.T) {
		d := decimal.NewFromInt(0)
		val, err := ScaleToInt(d, 2)
		require.NoError(t, err)
		assert.Equal(t, int64(0), val)
	})

	t.Run("precision_loss_error", func(t *testing.T) {
		d := decimal.RequireFromString("1500.123")
		_, err := ScaleToInt(d, 2) // 2dp can't hold 3dp value
		require.Error(t, err)
		assert.Contains(t, err.Error(), "more decimal places")
	})

	t.Run("overflow_error", func(t *testing.T) {
		// 10^16 at scale 0 exceeds 2^53
		d := decimal.RequireFromString("10000000000000000")
		_, err := ScaleToInt(d, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum safe integer")
	})

	t.Run("near_max_safe_integer", func(t *testing.T) {
		// Just under 2^53
		d := decimal.NewFromInt(MaxSafeInteger - 1)
		val, err := ScaleToInt(d, 0)
		require.NoError(t, err)
		assert.Equal(t, MaxSafeInteger-1, val)
	})

	t.Run("at_max_safe_integer", func(t *testing.T) {
		// Exactly 2^53
		d := decimal.NewFromInt(MaxSafeInteger)
		val, err := ScaleToInt(d, 0)
		require.NoError(t, err)
		assert.Equal(t, MaxSafeInteger, val)
	})

	t.Run("exceeds_max_safe_integer", func(t *testing.T) {
		d := decimal.NewFromInt(MaxSafeInteger + 1)
		_, err := ScaleToInt(d, 0)
		require.Error(t, err)
	})
}

func TestIntToDecimal(t *testing.T) {
	t.Run("basic_conversion", func(t *testing.T) {
		result := IntToDecimal(150050, 2)
		assert.True(t, decimal.RequireFromString("1500.50").Equal(result))
	})

	t.Run("satoshi", func(t *testing.T) {
		result := IntToDecimal(1, 8)
		assert.True(t, decimal.RequireFromString("0.00000001").Equal(result))
	})

	t.Run("zero_scale", func(t *testing.T) {
		result := IntToDecimal(1500, 0)
		assert.True(t, decimal.NewFromInt(1500).Equal(result))
	})

	t.Run("negative_value", func(t *testing.T) {
		result := IntToDecimal(-150050, 2)
		assert.True(t, decimal.RequireFromString("-1500.50").Equal(result))
	})
}

func TestScaleRoundTrip(t *testing.T) {
	values := []string{
		"0", "1", "100", "1500.50", "0.01", "0.00000001",
		"-1500.50", "-0.01", "99999999.99",
	}

	for _, s := range values {
		t.Run(s, func(t *testing.T) {
			original := decimal.RequireFromString(s)
			scale := DetermineScale(original)

			scaled, err := ScaleToInt(original, scale)
			require.NoError(t, err)

			restored := IntToDecimal(scaled, scale)
			assert.True(t, original.Equal(restored),
				"round-trip failed: %s → %d (scale %d) → %s",
				original, scaled, scale, restored,
			)
		})
	}
}

func TestConstants(t *testing.T) {
	t.Run("MaxAllowedScale_is_15", func(t *testing.T) {
		// MaxAllowedScale protects Lua 5.1's float64 arithmetic (2^53 safe integer limit).
		// At scale=15 the max representable amount is ~9007 (2^53 / 10^15), which is
		// sufficient for fiat (2-4dp), BTC satoshis (8dp), ETH gwei (9dp), and niche
		// high-precision tokens up to 15 decimal places.
		assert.Equal(t, int32(15), MaxAllowedScale)
	})

	t.Run("MaxAllowedScale_consistent_with_MaxScaledDigits", func(t *testing.T) {
		// The typed int32 constant and the untyped int should agree on the boundary value.
		assert.Equal(t, int32(MaxScaledDigits), MaxAllowedScale)
	})

	t.Run("MaxAllowedScale_below_overflow_threshold", func(t *testing.T) {
		// Demonstrate the math: at scale=MaxAllowedScale, a value of 1 unit becomes 10^15.
		// That's well within 2^53 ≈ 9.007×10^15, leaving room for amounts up to ~9007.
		scaleMultiplier := int64(1)
		for i := int32(0); i < MaxAllowedScale; i++ {
			scaleMultiplier *= 10
		}

		// 10^15 must be less than MaxSafeInteger (2^53)
		assert.Less(t, scaleMultiplier, MaxSafeInteger)
	})
}

func TestValidateAmountPrecision(t *testing.T) {
	t.Run("valid_amount", func(t *testing.T) {
		d := decimal.RequireFromString("1500.50")
		assert.NoError(t, ValidateAmountPrecision(d, 2))
	})

	t.Run("valid_with_auto_scale", func(t *testing.T) {
		d := decimal.RequireFromString("1500.50")
		assert.NoError(t, ValidateAmountPrecision(d, 0)) // auto-detect scale=2
	})

	t.Run("overflow_rejected", func(t *testing.T) {
		d := decimal.RequireFromString("100000000000000.00") // 10^14 at scale 2 = 10^16 > 2^53
		assert.Error(t, ValidateAmountPrecision(d, 2))
	})

	t.Run("precision_loss_rejected", func(t *testing.T) {
		d := decimal.RequireFromString("1.001") // 3dp can't fit in scale=2
		assert.Error(t, ValidateAmountPrecision(d, 2))
	})
}
