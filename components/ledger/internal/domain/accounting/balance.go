// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accounting

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// BalanceSnapshot supplies a cache-miss seed. When the physical cache key
// exists, implementations use its live state and settings. A snapshot in the
// pool need not be touched; overdraft companions are required only when needed.
type BalanceSnapshot struct {
	BalanceRef            string          `json:"balanceRef"`
	ID                    uuid.UUID       `json:"id"`
	AccountID             uuid.UUID       `json:"accountId"`
	AccountType           string          `json:"accountType"`
	AssetCode             string          `json:"assetCode"`
	Alias                 string          `json:"alias"`
	Key                   string          `json:"key"`
	Direction             string          `json:"direction"`
	BalanceScope          string          `json:"balanceScope"`
	Available             decimal.Decimal `json:"available"`
	OnHold                decimal.Decimal `json:"onHold"`
	OverdraftUsed         decimal.Decimal `json:"overdraftUsed"`
	OverdraftLimit        decimal.Decimal `json:"overdraftLimit"`
	Version               int64           `json:"version"`
	AllowSending          bool            `json:"allowSending"`
	AllowReceiving        bool            `json:"allowReceiving"`
	Blocked               bool            `json:"blocked"`
	AllowOverdraft        bool            `json:"allowOverdraft"`
	OverdraftLimitEnabled bool            `json:"overdraftLimitEnabled"`
}

// BalanceState contains the real monetary state at a movement boundary.
type BalanceState struct {
	Available     decimal.Decimal `json:"available"`
	OnHold        decimal.Decimal `json:"onHold"`
	OverdraftUsed decimal.Decimal `json:"overdraftUsed"`
	Version       int64           `json:"version"`
}
