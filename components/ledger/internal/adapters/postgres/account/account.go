package account

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
)

// AccountPostgreSQLModel represents the entity Account into SQL context in Database
type AccountPostgreSQLModel struct {
	ID                string
	Name              string
	ParentAccountID   *string
	EntityID          *string
	AssetCode         string
	OrganizationID    string
	LedgerID          string
	PortfolioID       *string
	SegmentID         *string
	AvailableBalance  *float64
	OnHoldBalance     *float64
	BalanceScale      *float64
	Status            string
	StatusDescription *string
	AllowSending      bool
	AllowReceiving    bool
	Alias             *string
	Type              string
	Version           int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         sql.NullTime
	Metadata          map[string]any
}

// ToEntity converts an AccountPostgreSQLModel to a response entity Account
func (t *AccountPostgreSQLModel) ToEntity() *mmodel.Account {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	balance := mmodel.Balance{
		Available: t.AvailableBalance,
		OnHold:    t.OnHoldBalance,
		Scale:     t.BalanceScale,
	}

	acc := &mmodel.Account{
		ID:              t.ID,
		Name:            t.Name,
		ParentAccountID: t.ParentAccountID,
		EntityID:        t.EntityID,
		AssetCode:       t.AssetCode,
		OrganizationID:  t.OrganizationID,
		LedgerID:        t.LedgerID,
		PortfolioID:     t.PortfolioID,
		SegmentID:       t.SegmentID,
		Balance:         balance,
		Status:          status,
		AllowSending:    &t.AllowSending,
		AllowReceiving:  &t.AllowReceiving,
		Alias:           t.Alias,
		Type:            t.Type,
		Version:         t.Version,
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
		DeletedAt:       nil,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		acc.DeletedAt = &deletedAtCopy
	}

	return acc
}

// FromEntity converts a request entity Account to AccountPostgreSQLModel
func (t *AccountPostgreSQLModel) FromEntity(account *mmodel.Account) {
	*t = AccountPostgreSQLModel{
		ID:                pkg.GenerateUUIDv7().String(),
		Name:              account.Name,
		ParentAccountID:   account.ParentAccountID,
		EntityID:          account.EntityID,
		AssetCode:         account.AssetCode,
		OrganizationID:    account.OrganizationID,
		LedgerID:          account.LedgerID,
		SegmentID:         account.SegmentID,
		AvailableBalance:  account.Balance.Available,
		OnHoldBalance:     account.Balance.OnHold,
		BalanceScale:      account.Balance.Scale,
		Status:            account.Status.Code,
		StatusDescription: account.Status.Description,
		Alias:             account.Alias,
		Type:              account.Type,
		Version:           account.Version,
		CreatedAt:         account.CreatedAt,
		UpdatedAt:         account.UpdatedAt,
	}

	if account.AllowSending != nil {
		t.AllowSending = *account.AllowSending
	}

	if account.AllowReceiving != nil {
		t.AllowReceiving = *account.AllowReceiving
	}

	if !pkg.IsNilOrEmpty(account.PortfolioID) {
		t.PortfolioID = account.PortfolioID
	}

	if account.DeletedAt != nil {
		deletedAtCopy := *account.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
