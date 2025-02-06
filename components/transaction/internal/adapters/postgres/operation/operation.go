package operation

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/pkg"
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
	BalanceID             string
	ChartOfAccounts       string
	OrganizationID        string
	LedgerID              string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             sql.NullTime
	Metadata              map[string]any
}

// Status structure for marshaling/unmarshalling JSON.
//
// swagger:model Status
// @Description Status is the struct designed to represent the status of an operation.
type Status struct {
	Code        string  `json:"code" validate:"max=100" example:"ACTIVE"`
	Description *string `json:"description" validate:"omitempty,max=256" example:"Active status"`
} // @name Status

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}

// Amount structure for marshaling/unmarshalling JSON.
//
// swagger:model Amount
// @Description Amount is the struct designed to represent the amount of an operation.
type Amount struct {
	Amount *float64 `json:"amount" example:"1500"`
	Scale  *float64 `json:"scale" example:"2"`
} // @name Amount

// IsEmpty method that set empty or nil in fields
func (a Amount) IsEmpty() bool {
	return a.Amount == nil && a.Scale == nil
}

// Balance structure for marshaling/unmarshalling JSON.
//
// swagger:model Balance
// @Description Balance is the struct designed to represent the account balance.
type Balance struct {
	Available *float64 `json:"available" example:"1500"`
	OnHold    *float64 `json:"onHold" example:"500"`
	Scale     *float64 `json:"scale" example:"2"`
} // @name Balance

// IsEmpty method that set empty or nil in fields
func (b Balance) IsEmpty() bool {
	return b.Available == nil && b.OnHold == nil && b.Scale == nil
}

// Operation is a struct designed to encapsulate response payload data.
//
// swagger:model Operation
// @Description Operation is a struct designed to store operation data.
type Operation struct {
	ID              string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	TransactionID   string         `json:"transactionId" example:"00000000-0000-0000-0000-000000000000"`
	Description     string         `json:"description" example:"Credit card operation"`
	Type            string         `json:"type" example:"creditCard"`
	AssetCode       string         `json:"assetCode" example:"BRL"`
	ChartOfAccounts string         `json:"chartOfAccounts" example:"1000"`
	Amount          Amount         `json:"amount"`
	Balance         Balance        `json:"balance"`
	BalanceAfter    Balance        `json:"balanceAfter"`
	Status          Status         `json:"status"`
	AccountID       string         `json:"accountId" example:"00000000-0000-0000-0000-000000000000"`
	AccountAlias    string         `json:"accountAlias" example:"@person1"`
	BalanceID       string         `json:"balanceId" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID  string         `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID        string         `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	CreatedAt       time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt       time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt       *time.Time     `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata        map[string]any `json:"metadata"`
} // @name Operation

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
		BalanceID:       t.BalanceID,
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
		ID:                    pkg.GenerateUUIDv7().String(),
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
		BalanceID:             operation.BalanceID,
		LedgerID:              operation.LedgerID,
		OrganizationID:        operation.OrganizationID,
		CreatedAt:             operation.CreatedAt,
		UpdatedAt:             operation.UpdatedAt,
	}

	if operation.DeletedAt != nil {
		deletedAtCopy := *operation.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}

// UpdateOperationInput is a struct design to encapsulate payload data.
//
// swagger:model UpdateOperationInput
// @Description UpdateOperationInput is the input payload to update an operation.
type UpdateOperationInput struct {
	Description string         `json:"description" validate:"max=256" example:"Credit card operation"`
	Metadata    map[string]any `json:"metadata,omitempty"`
} // @name UpdateOperationInput

// OperationLog is a struct designed to represent the operation data that should be stored in the audit log
type OperationLog struct {
	ID              string    `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	TransactionID   string    `json:"transactionId" example:"00000000-0000-0000-0000-000000000000"`
	Type            string    `json:"type" example:"creditCard"`
	AssetCode       string    `json:"assetCode" example:"BRL"`
	ChartOfAccounts string    `json:"chartOfAccounts" example:"1000"`
	Amount          Amount    `json:"amount"`
	Balance         Balance   `json:"balance"`
	BalanceAfter    Balance   `json:"balanceAfter"`
	Status          Status    `json:"status"`
	AccountID       string    `json:"accountId" example:"00000000-0000-0000-0000-000000000000"`
	AccountAlias    string    `json:"accountAlias" example:"@person1"`
	BalanceID       string    `json:"balanceId" example:"00000000-0000-0000-0000-000000000000"`
	CreatedAt       time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z"`
}

// ToLog converts an Operation excluding the fields that are not immutable
func (o *Operation) ToLog() *OperationLog {
	return &OperationLog{
		ID:              o.ID,
		TransactionID:   o.TransactionID,
		Type:            o.Type,
		AssetCode:       o.AssetCode,
		ChartOfAccounts: o.ChartOfAccounts,
		Amount:          o.Amount,
		Balance:         o.Balance,
		BalanceAfter:    o.BalanceAfter,
		Status:          o.Status,
		AccountID:       o.AccountID,
		AccountAlias:    o.AccountAlias,
		BalanceID:       o.BalanceID,
		CreatedAt:       o.CreatedAt,
	}
}
