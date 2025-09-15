package mmodel

import "github.com/shopspring/decimal"

// BalanceDelta represents an incremental change to apply to a balance.
// Positive values increase Available/OnHold; negative values decrease.
type BalanceDelta struct {
    ID             string
    DeltaAvailable decimal.Decimal
    DeltaOnHold    decimal.Decimal
}

