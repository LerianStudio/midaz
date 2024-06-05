package account

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// AccountPostgreSQLModel represents the entity Account into SQL context in Database
type AccountPostgreSQLModel struct {
	ID                string
	Name              string
	ParentAccountID   *string
	EntityID          string
	InstrumentCode    string
	OrganizationID    string
	LedgerID          string
	PortfolioID       string
	ProductID         *string
	AvailableBalance  *float64
	OnHoldBalance     *float64
	BalanceScale      *float64
	Status            string
	StatusDescription *string
	AllowSending      bool
	AllowReceiving    bool
	Alias             *string
	Type              string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         sql.NullTime
	Metadata          map[string]any
}

// CreateAccountInput is a struct design to encapsulate request create payload data.
type CreateAccountInput struct {
	InstrumentCode  string         `json:"instrumentCode"`
	Name            string         `json:"name"`
	Alias           *string        `json:"alias"`
	Type            string         `json:"type"`
	ParentAccountID *string        `json:"parentAccountId" validate:"omitempty,uuid"`
	ProductID       *string        `json:"productId" validate:"uuid"`
	EntityID        *string        `json:"entityId"`
	Status          Status         `json:"status"`
	Metadata        map[string]any `json:"metadata"`
}

// UpdateAccountInput is a struct design to encapsulate request update payload data.
type UpdateAccountInput struct {
	Name      string         `json:"name"`
	Status    Status         `json:"status"`
	Alias     *string        `json:"alias"`
	ProductID *string        `json:"productId" validate:"uuid"`
	Metadata  map[string]any `json:"metadata"`
}

// Account is a struct designed to encapsulate response payload data.
type Account struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	ParentAccountID *string        `json:"parentAccountId"`
	EntityID        string         `json:"entityId"`
	InstrumentCode  string         `json:"instrumentCode"`
	OrganizationID  string         `json:"organizationId"`
	LedgerID        string         `json:"ledgerId"`
	PortfolioID     string         `json:"portfolioId"`
	ProductID       *string        `json:"productId"`
	Balance         Balance        `json:"balance"`
	Status          Status         `json:"status"`
	Alias           *string        `json:"alias"`
	Type            string         `json:"type"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       *time.Time     `json:"deletedAt"`
	Metadata        map[string]any `json:"metadata"`
}

// Status structure for marshaling/unmarshalling JSON.
type Status struct {
	Code           string  `json:"code"`
	Description    *string `json:"description"`
	AllowSending   bool    `json:"allowSending"`
	AllowReceiving bool    `json:"allowReceiving"`
}

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}

// Balance structure for marshaling/unmarshalling JSON.
type Balance struct {
	Available *float64 `json:"available"`
	OnHold    *float64 `json:"onHold"`
	Scale     *float64 `json:"scale"`
}

// IsEmpty method that set empty or nil in fields
func (b Balance) IsEmpty() bool {
	return b.Available == nil && b.OnHold == nil && b.Scale == nil
}

// ToEntity converts an AccountPostgreSQLModel to a response entity Account
func (t *AccountPostgreSQLModel) ToEntity() *Account {
	status := Status{
		Code:           t.Status,
		Description:    t.StatusDescription,
		AllowSending:   t.AllowSending,
		AllowReceiving: t.AllowReceiving,
	}

	balance := Balance{
		Available: t.AvailableBalance,
		OnHold:    t.OnHoldBalance,
		Scale:     t.BalanceScale,
	}

	acc := &Account{
		ID:              t.ID,
		Name:            t.Name,
		ParentAccountID: t.ParentAccountID,
		EntityID:        t.EntityID,
		InstrumentCode:  t.InstrumentCode,
		OrganizationID:  t.OrganizationID,
		LedgerID:        t.LedgerID,
		PortfolioID:     t.PortfolioID,
		ProductID:       t.ProductID,
		Balance:         balance,
		Status:          status,
		Alias:           t.Alias,
		Type:            t.Type,
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
func (t *AccountPostgreSQLModel) FromEntity(account *Account) {
	*t = AccountPostgreSQLModel{
		ID:                uuid.New().String(),
		Name:              account.Name,
		ParentAccountID:   account.ParentAccountID,
		EntityID:          account.EntityID,
		InstrumentCode:    account.InstrumentCode,
		OrganizationID:    account.OrganizationID,
		LedgerID:          account.LedgerID,
		PortfolioID:       account.PortfolioID,
		ProductID:         account.ProductID,
		AvailableBalance:  account.Balance.Available,
		OnHoldBalance:     account.Balance.OnHold,
		BalanceScale:      account.Balance.Scale,
		Status:            account.Status.Code,
		StatusDescription: account.Status.Description,
		AllowSending:      account.Status.AllowSending,
		AllowReceiving:    account.Status.AllowReceiving,
		Alias:             account.Alias,
		Type:              account.Type,
		CreatedAt:         account.CreatedAt,
		UpdatedAt:         account.UpdatedAt,
	}

	if account.DeletedAt != nil {
		deletedAtCopy := *account.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
