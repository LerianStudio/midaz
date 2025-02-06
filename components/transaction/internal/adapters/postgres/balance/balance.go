package balance

import (
	"database/sql"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"time"
)

// BalancePostgreSQLModel represents the entity Balance into SQL context in Database
type BalancePostgreSQLModel struct {
	ID             string
	OrganizationID string
	LedgerID       string
	AccountID      string
	Alias          string
	AssetCode      string
	Available      int64
	OnHold         int64
	Scale          int64
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
		Scale:          balance.Scale,
		Version:        balance.Version,
		AccountType:    balance.AccountType,
		AllowSending:   balance.AllowSending,
		AllowReceiving: balance.AllowReceiving,
		CreatedAt:      balance.CreatedAt,
		UpdatedAt:      balance.UpdatedAt,
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
		AssetCode:      b.AssetCode,
		Available:      b.Available,
		OnHold:         b.OnHold,
		Scale:          b.Scale,
		Version:        b.Version,
		AccountType:    b.AccountType,
		AllowSending:   b.AllowSending,
		AllowReceiving: b.AllowReceiving,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
	}

	return balance
}
