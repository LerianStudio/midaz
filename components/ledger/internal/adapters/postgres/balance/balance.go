// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
)

// BalancePostgreSQLModel represents the entity Balance into SQL context in Database
type BalancePostgreSQLModel struct {
	ID             string
	OrganizationID string
	LedgerID       string
	AccountID      string
	Alias          string
	Key            string
	AssetCode      string
	Available      decimal.Decimal
	OnHold         decimal.Decimal
	Version        int64
	AccountType    string
	AllowSending   bool
	AllowReceiving bool
	Direction      string          `db:"direction"`
	OverdraftUsed  decimal.Decimal `db:"overdraft_used"`
	Settings       []byte          `db:"settings"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      sql.NullTime
}

// FromEntity converts a request entity Balance to BalancePostgreSQLModel
func (b *BalancePostgreSQLModel) FromEntity(balance *mmodel.Balance) {
	*b = BalancePostgreSQLModel{
		ID:             balance.ID,
		OrganizationID: balance.OrganizationID,
		LedgerID:       balance.LedgerID,
		AccountID:      balance.AccountID,
		Alias:          balance.Alias,
		AssetCode:      balance.AssetCode,
		Available:      balance.Available,
		OnHold:         balance.OnHold,
		Version:        balance.Version,
		AccountType:    balance.AccountType,
		AllowSending:   balance.AllowSending,
		AllowReceiving: balance.AllowReceiving,
		Direction:      balance.Direction,
		OverdraftUsed:  balance.OverdraftUsed,
		CreatedAt:      balance.CreatedAt,
		UpdatedAt:      balance.UpdatedAt,
	}

	if balance.Settings != nil {
		// json.Marshal cannot fail for BalanceSettings: it contains only
		// JSON-safe primitives (string, bool, *string). We intentionally
		// discard the error and leave b.Settings as nil on the (impossible)
		// failure path, matching the existing behavior for a nil Settings.
		b.Settings, _ = json.Marshal(balance.Settings)
	}

	if libCommons.IsNilOrEmpty(&balance.Key) {
		b.Key = "default"
	} else {
		b.Key = balance.Key
	}

	if balance.DeletedAt != nil {
		deletedAtCopy := *balance.DeletedAt
		b.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}

// ToEntity converts an BalancePostgreSQLModel to a response entity Balance
func (b *BalancePostgreSQLModel) ToEntity() *mmodel.Balance {
	key := b.Key
	if libCommons.IsNilOrEmpty(&key) {
		key = constant.DefaultBalanceKey
	}

	balance := &mmodel.Balance{
		ID:             b.ID,
		OrganizationID: b.OrganizationID,
		LedgerID:       b.LedgerID,
		AccountID:      b.AccountID,
		Alias:          b.Alias,
		Key:            key,
		AssetCode:      b.AssetCode,
		Available:      b.Available,
		OnHold:         b.OnHold,
		Version:        b.Version,
		AccountType:    b.AccountType,
		AllowSending:   b.AllowSending,
		AllowReceiving: b.AllowReceiving,
		Direction:      b.Direction,
		OverdraftUsed:  b.OverdraftUsed,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
	}

	if len(b.Settings) > 0 {
		var settings mmodel.BalanceSettings
		if err := json.Unmarshal(b.Settings, &settings); err != nil {
			// Log corruption but don't fail reads — Settings stays nil
			// which makes the balance behave like a legacy row (no
			// overdraft, scope=transactional). The span/logger context is
			// not available at this layer, so we use the standard log
			// package as a last-resort signal.
			log.Printf("WARN: failed to unmarshal balance settings for balance %s: %v", b.ID, err)
		} else {
			balance.Settings = &settings
		}
	}

	return balance
}

// BalanceAtTimestampModel represents a balance snapshot at a specific point in time
type BalanceAtTimestampModel struct {
	ID             string
	OrganizationID string
	LedgerID       string
	AccountID      string
	Alias          string
	Key            string
	AssetCode      string
	AccountType    string
	Available      decimal.Decimal
	OnHold         decimal.Decimal
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ToEntity converts BalanceAtTimestampModel to mmodel.Balance
func (b *BalanceAtTimestampModel) ToEntity() *mmodel.Balance {
	key := b.Key
	if libCommons.IsNilOrEmpty(&key) {
		key = constant.DefaultBalanceKey
	}

	return &mmodel.Balance{
		ID:             b.ID,
		OrganizationID: b.OrganizationID,
		LedgerID:       b.LedgerID,
		AccountID:      b.AccountID,
		Alias:          b.Alias,
		Key:            key,
		AssetCode:      b.AssetCode,
		AccountType:    b.AccountType,
		Available:      b.Available,
		OnHold:         b.OnHold,
		Version:        b.Version,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
	}
}
