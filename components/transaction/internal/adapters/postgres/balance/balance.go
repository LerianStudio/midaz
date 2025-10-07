// Package balance provides the repository implementation for balance entity persistence.
//
// This package implements the Repository pattern for the Balance entity, providing
// PostgreSQL-based data access. Balances track available and on-hold amounts for
// accounts, supporting double-entry accounting and optimistic locking.
package balance

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
)

// BalancePostgreSQLModel represents the PostgreSQL database model for balances.
//
// This model stores account balance information with:
//   - Available and on-hold amounts (decimal precision)
//   - Version number for optimistic locking
//   - Balance key for multiple balances per account
//   - Allow flags for transaction control
//   - Soft delete support
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

// FromEntity converts a domain Balance entity to a PostgreSQL model.
//
// Transforms business logic representation to database representation,
// handling key defaulting and DeletedAt conversion.
//
// Parameters:
//   - balance: Domain model to convert
//
// Side Effects:
//   - Modifies the receiver (*b) in place
//   - Defaults key to "default" if empty
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

// ToEntity converts a PostgreSQL model to a domain Balance entity.
//
// Transforms database representation to business logic representation.
//
// Returns:
//   - *mmodel.Balance: Domain model with all fields populated
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
