// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract

import (
	"context"
	"strings"

	"github.com/shopspring/decimal"
)

// Amount is decimal text on both JSON and protobuf transports. JSON numbers are
// deliberately rejected by encoding/json, preventing a caller from first losing
// monetary precision through a floating-point intermediary. Validate/Decimal
// must be called before use; the zero value and null JSON are invalid amounts.
type Amount string

// AmountFromDecimal bounds a producer's exact decimal before formatting it.
// Calling String first could expand a small coefficient with a huge exponent
// into an unbounded allocation. No float conversion or rounding is performed.
func AmountFromDecimal(ctx context.Context, value decimal.Decimal, limits Limits) (Amount, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if limits.MaxIntegerDigits <= 0 || limits.MaxFractionDigits < 0 {
		return "", invalid("decimal limits")
	}

	// Zero has no significant fractional digits, regardless of its stored
	// exponent. Avoid expanding an arbitrarily scaled zero when formatting.
	if value.IsZero() {
		return "0", nil
	}

	exponent := int64(value.Exponent())
	integerDigits := max(int64(value.NumDigits())+exponent, 1)
	fractionDigits := max(-exponent, 0)

	if fractionDigits > 0 {
		// Decimal arithmetic retains trailing coefficient zeros (for example
		// percentage division). Count the exact canonical scale without first
		// expanding the exponent into a potentially enormous decimal string.
		coefficient := value.Coefficient().String()
		trailingZeros := len(coefficient) - len(strings.TrimRight(coefficient, "0"))
		fractionDigits = max(fractionDigits-int64(trailingZeros), 0)
	}

	if integerDigits > int64(limits.MaxIntegerDigits) || fractionDigits > int64(limits.MaxFractionDigits) {
		return "", invalid("decimal size")
	}

	return Amount(value.String()), nil
}

// Decimal parses exact, plain decimal notation only after enforcing digit
// limits. Exponents, whitespace, leading plus signs and redundant leading zeros
// are rejected. Bounds are resource controls, never an instruction to round.
func (a Amount) Decimal(ctx context.Context, limits Limits) (decimal.Decimal, error) {
	if err := ctx.Err(); err != nil {
		return decimal.Decimal{}, err
	}

	if limits.MaxIntegerDigits <= 0 || limits.MaxFractionDigits < 0 {
		return decimal.Decimal{}, invalid("decimal limits")
	}

	raw := strings.TrimPrefix(string(a), "-")
	// Check the raw length in constant time before scanning for a decimal point.
	// Subtraction avoids overflow when caller-supplied bounds are large.
	if len(raw) > limits.MaxIntegerDigits && len(raw)-limits.MaxIntegerDigits-1 > limits.MaxFractionDigits {
		return decimal.Decimal{}, invalid("decimal size")
	}

	integer, fraction, hasFraction := strings.Cut(raw, ".")

	if len(integer) == 0 || len(integer) > limits.MaxIntegerDigits ||
		len(fraction) > limits.MaxFractionDigits || (hasFraction && len(fraction) == 0) ||
		(len(integer) > 1 && integer[0] == '0') {
		return decimal.Decimal{}, invalid("decimal shape or size")
	}

	if !digits(integer) || (hasFraction && !digits(fraction)) {
		return decimal.Decimal{}, invalid("decimal notation")
	}

	if err := ctx.Err(); err != nil {
		return decimal.Decimal{}, err
	}

	value, err := decimal.NewFromString(string(a))
	if err != nil {
		// Do not include the raw monetary value in an error crossing a boundary.
		return decimal.Decimal{}, invalid("decimal value")
	}

	return value, nil
}

func digits(s string) bool {
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}

	return true
}
