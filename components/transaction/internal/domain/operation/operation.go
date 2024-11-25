package operation

import (
	"database/sql"
	"github.com/LerianStudio/midaz/common"
	"time"
)

// OperationPostgreSQLModel represents the entity OperationPostgreSQLModel into SQL context in Database
type OperationPostgreSQLModel struct {
	ID                    string
	TransactionID         string
	Description           string
	Type                  string
	AssetCode             string
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
	PortfolioID           *string
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
	Code        string  `json:"code" validate:"max=100"`
	Description *string `json:"description" validate:"omitempty,max=256"`
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
	AssetCode       string         `json:"assetCode"`
	ChartOfAccounts string         `json:"chartOfAccounts"`
	Amount          Amount         `json:"amount"`
	Balance         Balance        `json:"balance"`
	BalanceAfter    Balance        `json:"balanceAfter"`
	Status          Status         `json:"status"`
	AccountID       string         `json:"accountId"`
	AccountAlias    string         `json:"accountAlias"`
	PortfolioID     *string        `json:"portfolioId"`
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
		AssetCode:       t.AssetCode,
		ChartOfAccounts: t.ChartOfAccounts,
		Amount:          amount,
		Balance:         balance,
		BalanceAfter:    balanceAfter,
		Status:          status,
		AccountID:       t.AccountID,
		AccountAlias:    t.AccountAlias,
		LedgerID:        t.LedgerID,
		OrganizationID:  t.OrganizationID,
		PortfolioID:     t.PortfolioID,
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
func (t *OperationPostgreSQLModel) FromEntity(operation *Operation) {
	*t = OperationPostgreSQLModel{
		ID:                    common.GenerateUUIDv7().String(),
		TransactionID:         operation.TransactionID,
		Description:           operation.Description,
		Type:                  operation.Type,
		AssetCode:             operation.AssetCode,
		ChartOfAccounts:       operation.ChartOfAccounts,
		Amount:                operation.Amount.Amount,
		AmountScale:           operation.Amount.Scale,
		BalanceScale:          operation.Balance.Scale,
		OnHoldBalance:         operation.Balance.OnHold,
		AvailableBalance:      operation.Balance.Available,
		BalanceScaleAfter:     operation.BalanceAfter.Scale,
		AvailableBalanceAfter: operation.BalanceAfter.Available,
		OnHoldBalanceAfter:    operation.BalanceAfter.OnHold,
		Status:                operation.Status.Code,
		StatusDescription:     operation.Status.Description,
		AccountID:             operation.AccountID,
		AccountAlias:          operation.AccountAlias,
		PortfolioID:           operation.PortfolioID,
		LedgerID:              operation.LedgerID,
		OrganizationID:        operation.OrganizationID,
		CreatedAt:             operation.CreatedAt,
		UpdatedAt:             operation.UpdatedAt,
	}

	if operation.DeletedAt != nil {
		deletedAtCopy := *operation.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}

	if common.IsNilOrEmpty(operation.PortfolioID) {
		t.PortfolioID = nil
	}
}

// UpdateOperationInput is a struct design to encapsulate payload data.
type UpdateOperationInput struct {
	Description string         `json:"description" validate:"max=256"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}
