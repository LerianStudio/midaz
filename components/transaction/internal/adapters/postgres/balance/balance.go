package balance

import (
	"database/sql"
	"github.com/LerianStudio/midaz/pkg/constant"
	gold "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"time"
)

type BalancePostgreSQLModel struct {
	ID             string
	Alias          *string
	LedgerID       string
	OrganizationID string
	AssetCode      string
	Available      *float64
	OnHold         *float64
	Scale          *float64
	Version        int64
	AcceptNegative bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      sql.NullTime
}

type Balance struct {
	ID             string
	Alias          *string
	LedgerID       string
	OrganizationID string
	AssetCode      string
	Available      *float64
	OnHold         *float64
	Scale          *float64
	Version        int64
	AcceptNegative bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}

func (b *Balance) ToEntity() *BalancePostgreSQLModel {

	balance := &BalancePostgreSQLModel{
		ID:             b.ID,
		Alias:          b.Alias,
		LedgerID:       b.LedgerID,
		OrganizationID: b.OrganizationID,
		AssetCode:      b.AssetCode,
		Available:      b.Available,
		OnHold:         b.OnHold,
		Scale:          b.Scale,
		Version:        b.Version,
		AcceptNegative: b.AcceptNegative,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
	}

	if b.DeletedAt != nil {
		deletedAtCopy := *b.DeletedAt
		balance.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}

	return balance
}

func (b *BalancePostgreSQLModel) FromEntity() *Balance {
	balance := &Balance{
		ID:             b.ID,
		Alias:          b.Alias,
		LedgerID:       b.LedgerID,
		OrganizationID: b.OrganizationID,
		AssetCode:      b.AssetCode,
		Available:      b.Available,
		OnHold:         b.OnHold,
		Scale:          b.Scale,
		Version:        b.Version,
		AcceptNegative: b.AcceptNegative,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
	}

	return balance
}

// OperateAmounts Function to sum or sub two amounts and normalize the scale
func OperateAmounts(amount gold.Amount, balance *Balance, operation string) Balance {
	var (
		scale float64
		total float64
	)

	switch operation {
	case constant.DEBIT:
		if int(*balance.Scale) < amount.Scale {
			v0 := gold.Scale(int(*balance.Available), int(*balance.Scale), amount.Scale)
			total = v0 - float64(amount.Value)
			scale = float64(amount.Scale)
		} else {
			v0 := gold.Scale(amount.Value, amount.Scale, int(*balance.Scale))
			total = *balance.Available - v0
			scale = *balance.Scale
		}
	default:
		if int(*balance.Scale) < amount.Scale {
			v0 := gold.Scale(int(*balance.Available), int(*balance.Scale), amount.Scale)
			total = v0 + float64(amount.Value)
			scale = float64(amount.Scale)
		} else {
			v0 := gold.Scale(amount.Value, amount.Scale, int(*balance.Scale))
			total = *balance.Available + v0
			scale = *balance.Scale
		}
	}

	return Balance{
		ID:             balance.ID,
		Alias:          balance.Alias,
		LedgerID:       balance.LedgerID,
		OrganizationID: balance.OrganizationID,
		AssetCode:      balance.AssetCode,
		Available:      &total,
		OnHold:         balance.OnHold,
		Scale:          &scale,
		Version:        balance.Version,
		AcceptNegative: balance.AcceptNegative,
		CreatedAt:      balance.CreatedAt,
		UpdatedAt:      balance.UpdatedAt,
	}
}

func (b *Balance) IsEmpty() bool {
	return b.Available == nil && b.OnHold == nil && b.Scale == nil
}
