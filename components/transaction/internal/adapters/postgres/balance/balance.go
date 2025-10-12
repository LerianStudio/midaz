package balance

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
)

// BalancePostgreSQLModel represents the balance entity in PostgreSQL context.
//
// This model maps the domain balance entity to the database schema, handling
// SQL-specific types like sql.NullTime for optional timestamps.
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

// FromEntity converts a domain balance entity to the PostgreSQL model.
//
// This method maps domain types to SQL types and applies defaults:
// - Sets "default" as the Key if not specified
// - Converts DeletedAt pointer to sql.NullTime
//
// Parameters:
//   - balance: The domain balance entity to convert
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

// ToEntity converts the PostgreSQL model back to a domain balance entity.
//
// Returns:
//   - *mmodel.Balance: The domain balance entity
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
