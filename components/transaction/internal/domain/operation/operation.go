package operation

import (
	"database/sql"
	"github.com/google/uuid"
	"time"
)

// OperationPostgreSQLModel represents the entity OperationPostgreSQLModel into SQL context in Database
type OperationPostgreSQLModel struct {
	ID                    string
	TransactionID         string
	Description           string
	Type                  string
	InstrumentCode        string
	Amount                *float64
	AmountScale           *float64
	AvailableBalance      *float64
	BalanceScale          *float64
	OnHoldBalance         *float64
	AvailableBalanceAfter *float64
	OnHoldBalanceAfter    *float64
	BalanceScaleAfter     *float64
	Status                string
	StatusDescription     *string
	AccountID             string
	AccountAlias          string
	PortfolioID           string
	ChartOfAccounts       string
	OrganizationID        string
	LedgerID              string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             sql.NullTime
	Metadata              map[string]any
}

// Status structure for marshaling/unmarshalling JSON.
type Status struct {
	Code        string  `json:"code"`
	Description *string `json:"description"`
}

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}

// Amount structure for marshaling/unmarshalling JSON.
type Amount struct {
	Amount *float64 `json:"amount"`
	Scale  *float64 `json:"scale"`
}

// IsEmpty method that set empty or nil in fields
func (a Amount) IsEmpty() bool {
	return a.Amount == nil && a.Scale == nil
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

// Operation is a struct designed to encapsulate response payload data.
type Operation struct {
	ID              string         `json:"id"`
	TransactionID   string         `json:"transactionId"`
	Description     string         `json:"description"`
	Type            string         `json:"type"`
	InstrumentCode  string         `json:"instrumentCode"`
	ChartOfAccounts string         `json:"chartOfAccounts"`
	Amount          Amount         `json:"amount"`
	Balance         Balance        `json:"balance"`
	BalanceAfter    Balance        `json:"balanceAfter"`
	Status          Status         `json:"status"`
	AccountID       string         `json:"accountId"`
	AccountAlias    string         `json:"accountAlias"`
	PortfolioID     string         `json:"portfolioId"`
	OrganizationID  string         `json:"organizationId"`
	LedgerID        string         `json:"ledgerId"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       *time.Time     `json:"deletedAt"`
	Metadata        map[string]any `json:"metadata"`
}

// ToEntity converts an OperationPostgreSQLModel to entity Operation
func (t *OperationPostgreSQLModel) ToEntity() *Operation {
	status := Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	amount := Amount{
		Amount: t.Amount,
		Scale:  t.AmountScale,
	}

	balance := Balance{
		Available: t.AvailableBalance,
		OnHold:    t.OnHoldBalance,
		Scale:     t.BalanceScale,
	}

	balanceAfter := Balance{
		Available: t.AvailableBalanceAfter,
		OnHold:    t.OnHoldBalanceAfter,
		Scale:     t.BalanceScaleAfter,
	}

	Operation := &Operation{
		ID:              t.ID,
		TransactionID:   t.TransactionID,
		Description:     t.Description,
		Type:            t.Type,
		InstrumentCode:  t.InstrumentCode,
		ChartOfAccounts: t.ChartOfAccounts,
		Amount:          amount,
		Balance:         balance,
		BalanceAfter:    balanceAfter,
		Status:          status,
		AccountID:       t.AccountID,
		AccountAlias:    t.AccountAlias,
		LedgerID:        t.LedgerID,
		OrganizationID:  t.OrganizationID,
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
		DeletedAt:       nil,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		Operation.DeletedAt = &deletedAtCopy
	}

	return Operation
}

// FromEntity converts an entity Operation to OperationPostgreSQLModel
func (t *OperationPostgreSQLModel) FromEntity(Operation *Operation) {
	*t = OperationPostgreSQLModel{
		ID:                    uuid.New().String(),
		TransactionID:         Operation.TransactionID,
		Description:           Operation.Description,
		Type:                  Operation.Type,
		InstrumentCode:        Operation.InstrumentCode,
		ChartOfAccounts:       Operation.ChartOfAccounts,
		Amount:                Operation.Amount.Amount,
		AmountScale:           Operation.Amount.Scale,
		BalanceScale:          Operation.Balance.Scale,
		OnHoldBalance:         Operation.Balance.OnHold,
		AvailableBalance:      Operation.Balance.Available,
		BalanceScaleAfter:     Operation.BalanceAfter.Scale,
		AvailableBalanceAfter: Operation.BalanceAfter.Available,
		OnHoldBalanceAfter:    Operation.BalanceAfter.Scale,
		Status:                Operation.Status.Code,
		StatusDescription:     Operation.Status.Description,
		AccountID:             Operation.AccountID,
		AccountAlias:          Operation.AccountAlias,
		LedgerID:              Operation.LedgerID,
		OrganizationID:        Operation.OrganizationID,
		CreatedAt:             Operation.CreatedAt,
		UpdatedAt:             Operation.UpdatedAt,
	}

	if Operation.DeletedAt != nil {
		deletedAtCopy := *Operation.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
