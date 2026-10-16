// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accounting

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// BalanceConfig is the configuration of one balance row as PostgreSQL holds it,
// with the row's configuration version. Version orders configurations of the SAME
// balance only: it is never compared across balances, nor with an account version.
// BalanceID ties the configuration to that row, so a configuration of a deleted
// balance can never be taken for one of a balance recreated under its alias#key.
type BalanceConfig struct {
	BalanceID             uuid.UUID
	Version               int64
	AllowSending          bool
	AllowReceiving        bool
	AllowOverdraft        bool
	OverdraftLimitEnabled bool
	OverdraftLimit        decimal.Decimal
	BalanceScope          string
}

// Describes reports whether the configuration belongs to the balance with the given
// ID. A configuration of another balance is absent for this one.
func (c BalanceConfig) Describes(balanceID uuid.UUID) bool {
	return c.BalanceID == balanceID
}

// Equal reports whether two configurations carry the same identity, version and
// content, comparing the limit by value.
func (c BalanceConfig) Equal(other BalanceConfig) bool {
	return c.BalanceID == other.BalanceID &&
		c.Version == other.Version &&
		c.AllowSending == other.AllowSending &&
		c.AllowReceiving == other.AllowReceiving &&
		c.AllowOverdraft == other.AllowOverdraft &&
		c.OverdraftLimitEnabled == other.OverdraftLimitEnabled &&
		c.OverdraftLimit.Equal(other.OverdraftLimit) &&
		c.BalanceScope == other.BalanceScope
}

// AccountConfig is the configuration of one account row (its blocked flag) with the
// row's configuration version. Version orders configurations of the SAME account
// only and is independent of every balance version.
type AccountConfig struct {
	AccountID uuid.UUID
	Version   int64
	Blocked   bool
}
