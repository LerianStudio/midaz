package balance

import "github.com/shopspring/decimal"

func (b *Balance) getAvailableDecimal() decimal.Decimal {
	if b.Available == "" {
		return decimal.Zero
	}

	decimalValue, err := decimal.NewFromString(b.Available)
	if err != nil {
		return decimal.Zero
	}

	return decimalValue
}

func (b *Balance) getOnHoldDecimal() decimal.Decimal {
	if b.OnHold == "" {
		return decimal.Zero
	}

	decimalValue, err := decimal.NewFromString(b.OnHold)
	if err != nil {
		return decimal.Zero
	}

	return decimalValue
}

func (b *Balance) HasZeroFunds() bool {
	return b.getAvailableDecimal().IsZero() && b.getOnHoldDecimal().IsZero()
}
