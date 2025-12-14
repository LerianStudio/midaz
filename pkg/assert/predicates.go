package assert

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Positive returns true if n > 0.
//
// Example:
//
//	assert.That(assert.Positive(count), "count must be positive", "count", count)
func Positive(n int64) bool {
	return n > 0
}

// NonNegative returns true if n >= 0.
//
// Example:
//
//	assert.That(assert.NonNegative(balance), "balance must not be negative", "balance", balance)
func NonNegative(n int64) bool {
	return n >= 0
}

// NotZero returns true if n != 0.
//
// Example:
//
//	assert.That(assert.NotZero(divisor), "divisor must not be zero", "divisor", divisor)
func NotZero(n int64) bool {
	return n != 0
}

// InRange returns true if min <= n <= max.
//
// Note: If min > max (inverted range), always returns false. This is fail-safe
// behavior - callers should ensure min <= max for correct results.
//
// Example:
//
//	assert.That(assert.InRange(page, 1, 1000), "page out of range", "page", page)
func InRange(n, min, max int64) bool {
	return n >= min && n <= max
}

// ValidUUID returns true if s is a valid UUID string.
//
// Note: Accepts both canonical (with hyphens) and non-canonical (without hyphens)
// UUID formats per RFC 4122. Empty strings return false.
//
// Example:
//
//	assert.That(assert.ValidUUID(id), "invalid UUID format", "id", id)
func ValidUUID(s string) bool {
	if s == "" {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}

// ValidAmount returns true if the decimal's exponent is within reasonable bounds.
// The exponent must be in the range [-18, 18] to prevent overflow and maintain
// precision for financial calculations.
//
// Note: This validates exponent bounds only, not coefficient size. For user-facing
// validation, consider additional bounds checks on the coefficient.
//
// Example:
//
//	assert.That(assert.ValidAmount(amount), "amount has invalid precision", "amount", amount)
func ValidAmount(amount decimal.Decimal) bool {
	exp := amount.Exponent()
	return exp >= -18 && exp <= 18
}

// ValidScale returns true if scale is in the range [0, 18].
// Scale represents the number of decimal places for financial amounts.
//
// Example:
//
//	assert.That(assert.ValidScale(scale), "invalid scale", "scale", scale)
func ValidScale(scale int) bool {
	return scale >= 0 && scale <= 18
}

// PositiveDecimal returns true if amount > 0.
//
// Example:
//
//	assert.That(assert.PositiveDecimal(price), "price must be positive", "price", price)
func PositiveDecimal(amount decimal.Decimal) bool {
	return amount.IsPositive()
}

// NonNegativeDecimal returns true if amount >= 0.
//
// Example:
//
//	assert.That(assert.NonNegativeDecimal(balance), "balance must not be negative", "balance", balance)
func NonNegativeDecimal(amount decimal.Decimal) bool {
	return !amount.IsNegative()
}
