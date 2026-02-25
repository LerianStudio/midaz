// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
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
		CreatedAt:      balance.CreatedAt,
		UpdatedAt:      balance.UpdatedAt,
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
	balance := &mmodel.Balance{
		ID:             b.ID,
		OrganizationID: b.OrganizationID,
		LedgerID:       b.LedgerID,
		AccountID:      b.AccountID,
		Alias:          b.Alias,
		Key:            b.Key,
		AssetCode:      b.AssetCode,
		Available:      b.Available,
		OnHold:         b.OnHold,
		Version:        b.Version,
		AccountType:    b.AccountType,
		AllowSending:   b.AllowSending,
		AllowReceiving: b.AllowReceiving,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
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
	return &mmodel.Balance{
		ID:             b.ID,
		OrganizationID: b.OrganizationID,
		LedgerID:       b.LedgerID,
		AccountID:      b.AccountID,
		Alias:          b.Alias,
		Key:            b.Key,
		AssetCode:      b.AssetCode,
		AccountType:    b.AccountType,
		Available:      b.Available,
		OnHold:         b.OnHold,
		Version:        b.Version,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
	}
}
