package operation

import (
	"database/sql"
	"fmt"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/shopspring/decimal"
	"math"
	"time"
)

// OperationPostgreSQLModel represents the entity OperationPostgreSQLModel into SQL context in Database
//
// @Description Database model for storing operation information in PostgreSQL
type OperationPostgreSQLModel struct {
	ID                    string         // Unique identifier (UUID format)
	TransactionID         string         // Parent transaction ID
	Description           string         // Operation description
	Type                  string         // Operation type (e.g., "DEBIT", "CREDIT")
	AssetCode             string         // Asset code for the operation
	Amount                *int64         // Operation amount value
	AmountScale           *int64         // Decimal places for amount
	AvailableBalance      *int64         // Available balance before operation
	BalanceScale          *int64         // Decimal places for balance
	OnHoldBalance         *int64         // On-hold balance before operation
	AvailableBalanceAfter *int64         // Available balance after operation
	OnHoldBalanceAfter    *int64         // On-hold balance after operation
	BalanceScaleAfter     *int64         // Decimal places for balance after operation
	Status                string         // Status code (e.g., "ACTIVE", "PENDING")
	StatusDescription     *string        // Status description
	AccountID             string         // Account ID associated with operation
	AccountAlias          string         // Account alias
	BalanceID             string         // Balance ID affected by operation
	ChartOfAccounts       string         // Chart of accounts code
	OrganizationID        string         // Organization ID
	LedgerID              string         // Ledger ID
	CreatedAt             time.Time      // Creation timestamp
	UpdatedAt             time.Time      // Last update timestamp
	DeletedAt             sql.NullTime   // Deletion timestamp (if soft-deleted)
	Metadata              map[string]any // Additional custom attributes
}

// OperationPostgreSQLModelPoC represents the entity OperationPostgreSQLModel into SQL context in Database
//
// @Description Database model for storing operation information for a PoCin PostgreSQL
type OperationPostgreSQLModelPoC struct {
	ID                    string           // Unique identifier (UUID format)
	TransactionID         string           // Parent transaction ID
	Description           string           // Operation description
	Type                  string           // Operation type (e.g., "DEBIT", "CREDIT")
	AssetCode             string           // Asset code for the operation
	Amount                *decimal.Decimal // Operation amount value
	AmountScale           *int64           // Decimal places for amount
	AvailableBalance      *decimal.Decimal // Available balance before operation
	BalanceScale          *int64           // Decimal places for balance
	OnHoldBalance         *decimal.Decimal // On-hold balance before operation
	AvailableBalanceAfter *decimal.Decimal // Available balance after operation
	OnHoldBalanceAfter    *decimal.Decimal // On-hold balance after operation
	BalanceScaleAfter     *int64           // Decimal places for balance after operation
	Status                string           // Status code (e.g., "ACTIVE", "PENDING")
	StatusDescription     *string          // Status description
	AccountID             string           // Account ID associated with operation
	AccountAlias          string           // Account alias
	BalanceID             string           // Balance ID affected by operation
	ChartOfAccounts       string           // Chart of accounts code
	OrganizationID        string           // Organization ID
	LedgerID              string           // Ledger ID
	CreatedAt             time.Time        // Creation timestamp
	UpdatedAt             time.Time        // Last update timestamp
	DeletedAt             sql.NullTime     // Deletion timestamp (if soft-deleted)
	Metadata              map[string]any   // Additional custom attributes
}

// Status structure for marshaling/unmarshalling JSON.
//
// swagger:model Status
// @Description Status is the struct designed to represent the status of an operation. Contains code and optional description for operation states.
type Status struct {
	// Status code identifying the state of the operation
	// example: ACTIVE
	// maxLength: 100
	Code string `json:"code" validate:"max=100" example:"ACTIVE" maxLength:"100"`

	// Optional descriptive text explaining the status
	// example: Active status
	// maxLength: 256
	Description *string `json:"description" validate:"omitempty,max=256" example:"Active status" maxLength:"256"`
} // @name Status

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}

// Amount structure for marshaling/unmarshalling JSON.
//
// swagger:model Amount
// @Description Amount is the struct designed to represent the amount of an operation. Contains the value and scale (decimal places) of an operation amount.
type Amount struct {
	// The amount value in the smallest unit of the asset (e.g., cents)
	// example: 1500
	// minimum: 0
	Amount *int64 `json:"amount" example:"1500" minimum:"0"`

	// Decimal places for the amount (e.g., 2 for dollars/euros, 8 for BTC)
	// example: 2
	// minimum: 0
	Scale *int64 `json:"scale" example:"2" minimum:"0"`
} // @name Amount

// IsEmpty method that set empty or nil in fields
func (a Amount) IsEmpty() bool {
	return a.Amount == nil && a.Scale == nil
}

// Balance structure for marshaling/unmarshalling JSON.
//
// swagger:model Balance
// @Description Balance is the struct designed to represent the account balance. Contains available and on-hold amounts along with the scale (decimal places).
type Balance struct {
	// Amount available for transactions (in smallest unit of asset)
	// example: 1500
	// minimum: 0
	Available *int64 `json:"available" example:"1500" minimum:"0"`

	// Amount on hold and unavailable for transactions (in smallest unit of asset)
	// example: 500
	// minimum: 0
	OnHold *int64 `json:"onHold" example:"500" minimum:"0"`

	// Decimal places for the balance (e.g., 2 for dollars/euros)
	// example: 2
	// minimum: 0
	Scale *int64 `json:"scale" example:"2" minimum:"0"`
} // @name Balance

// IsEmpty method that set empty or nil in fields
func (b Balance) IsEmpty() bool {
	return b.Available == nil && b.OnHold == nil && b.Scale == nil
}

// Operation is a struct designed to encapsulate response payload data.
//
// swagger:model Operation
// @Description Operation is a struct designed to store operation data. Represents a financial operation that affects account balances, including details such as amount, balance before and after, transaction association, and metadata.
type Operation struct {
	// Unique identifier for the operation
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Parent transaction identifier
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	TransactionID string `json:"transactionId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Human-readable description of the operation
	// example: Credit card operation
	// maxLength: 256
	Description string `json:"description" example:"Credit card operation" maxLength:"256"`

	// Type of operation (e.g., DEBIT, CREDIT)
	// example: DEBIT
	// maxLength: 50
	Type string `json:"type" example:"DEBIT" maxLength:"50"`

	// Asset code for the operation
	// example: BRL
	// minLength: 2
	// maxLength: 10
	AssetCode string `json:"assetCode" example:"BRL" minLength:"2" maxLength:"10"`

	// Chart of accounts code for accounting purposes
	// example: 1000
	// maxLength: 20
	ChartOfAccounts string `json:"chartOfAccounts" example:"1000" maxLength:"20"`

	// Operation amount information
	Amount Amount `json:"amount"`

	// Balance before the operation
	Balance Balance `json:"balance"`

	// Balance after the operation
	BalanceAfter Balance `json:"balanceAfter"`

	// Operation status information
	Status Status `json:"status"`

	// Account identifier associated with this operation
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	AccountID string `json:"accountId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Human-readable alias for the account
	// example: @person1
	// maxLength: 256
	AccountAlias string `json:"accountAlias" example:"@person1" maxLength:"256"`

	// Balance identifier affected by this operation
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	BalanceID string `json:"balanceId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Organization identifier
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Ledger identifier
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Timestamp when the operation was created
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the operation was last updated
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the operation was deleted (if soft-deleted)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Additional custom attributes
	// example: {"reason": "Purchase refund", "reference": "INV-12345"}
	Metadata map[string]any `json:"metadata"`
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
	ID := libCommons.GenerateUUIDv7().String()
	if operation.ID != "" {
		ID = operation.ID
	}

	*t = OperationPostgreSQLModel{
		ID:                    ID,
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
// @Description UpdateOperationInput is the input payload to update an operation. Contains fields that can be modified after an operation is created.
type UpdateOperationInput struct {
	// Human-readable description of the operation
	// example: Credit card operation
	// maxLength: 256
	Description string `json:"description" validate:"max=256" example:"Credit card operation" maxLength:"256"`

	// Additional custom attributes
	// example: {"reason": "Purchase refund", "reference": "INV-12345"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateOperationInput

// OperationResponse represents a success response containing a single operation.
//
// swagger:response OperationResponse
// @Description Successful response containing a single operation entity.
type OperationResponse struct {
	// in: body
	Body Operation
}

// OperationsResponse represents a success response containing a paginated list of operations.
//
// swagger:response OperationsResponse
// @Description Successful response containing a paginated list of operations.
type OperationsResponse struct {
	// in: body
	Body struct {
		Items      []Operation `json:"items"`
		Pagination struct {
			Limit      int     `json:"limit"`
			NextCursor *string `json:"next_cursor,omitempty"`
			PrevCursor *string `json:"prev_cursor,omitempty"`
		} `json:"pagination"`
	}
}

// OperationLog is a struct designed to represent the operation data that should be stored in the audit log
//
// @Description Immutable log entry for audit purposes representing a snapshot of operation state at a specific point in time.
type OperationLog struct {
	// Unique identifier for the operation
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Parent transaction identifier
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	TransactionID string `json:"transactionId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Type of operation (e.g., creditCard, transfer, payment)
	// example: creditCard
	// maxLength: 50
	Type string `json:"type" example:"creditCard" maxLength:"50"`

	// Asset code for the operation
	// example: BRL
	// minLength: 2
	// maxLength: 10
	AssetCode string `json:"assetCode" example:"BRL" minLength:"2" maxLength:"10"`

	// Chart of accounts code for accounting purposes
	// example: 1000
	// maxLength: 20
	ChartOfAccounts string `json:"chartOfAccounts" example:"1000" maxLength:"20"`

	// Operation amount information
	Amount Amount `json:"amount"`

	// Balance before the operation
	Balance Balance `json:"balance"`

	// Balance after the operation
	BalanceAfter Balance `json:"balanceAfter"`

	// Operation status information
	Status Status `json:"status"`

	// Account identifier associated with this operation
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	AccountID string `json:"accountId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Human-readable alias for the account
	// example: @person1
	// maxLength: 256
	AccountAlias string `json:"accountAlias" example:"@person1" maxLength:"256"`

	// Balance identifier affected by this operation
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	BalanceID string `json:"balanceId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Timestamp when the operation log was created
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
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

// ToDecimal is a helper func create to use on POC
func (t *OperationPostgreSQLModelPoC) ToDecimal(value int64, scale int64) decimal.Decimal {
	d := decimal.NewFromInt(value)
	return d.Shift(-int32(scale))
}

// FromEntityPoC converts an entity Operation to OperationPostgreSQLModel
func (t *OperationPostgreSQLModelPoC) FromEntityPoC(operation *Operation) {
	ID := libCommons.GenerateUUIDv7().String()
	if operation.ID != "" {
		ID = operation.ID
	}

	amount := t.ToDecimal(*operation.Amount.Amount, *operation.Amount.Scale)
	onHoldBalance := t.ToDecimal(*operation.Balance.OnHold, *operation.Balance.Scale)
	availableBalance := t.ToDecimal(*operation.Balance.Available, *operation.Balance.Scale)
	availableBalanceAfter := t.ToDecimal(*operation.BalanceAfter.Available, *operation.BalanceAfter.Scale)
	onHoldBalanceAfter := t.ToDecimal(*operation.BalanceAfter.OnHold, *operation.BalanceAfter.Scale)

	*t = OperationPostgreSQLModelPoC{
		ID:                    ID,
		TransactionID:         operation.TransactionID,
		Description:           operation.Description,
		Type:                  operation.Type,
		AssetCode:             operation.AssetCode,
		ChartOfAccounts:       operation.ChartOfAccounts,
		Amount:                &amount,
		AmountScale:           operation.Amount.Scale,
		BalanceScale:          operation.Balance.Scale,
		OnHoldBalance:         &onHoldBalance,
		AvailableBalance:      &availableBalance,
		BalanceScaleAfter:     operation.BalanceAfter.Scale,
		AvailableBalanceAfter: &availableBalanceAfter,
		OnHoldBalanceAfter:    &onHoldBalanceAfter,
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

// FromDecimal is a helper func create to use on POC
func (t *OperationPostgreSQLModelPoC) FromDecimal(d decimal.Decimal, scale int64) (int64, error) {
	scaled := d.Shift(int32(scale))
	if !scaled.IsInteger() {
		return 0, fmt.Errorf("value has more decimal digits than scale allows: %s", d.String())
	}

	if !scaled.IsZero() && (scaled.Cmp(decimal.NewFromInt(math.MinInt64)) < 0 || scaled.Cmp(decimal.NewFromInt(math.MaxInt64)) > 0) {
		return 0, fmt.Errorf("value overflows int64: %s", scaled.String())
	}

	return scaled.IntPart(), nil
}

// ToEntityPoC converts an OperationPostgreSQLModelPoC to entity poc Operation
func (t *OperationPostgreSQLModelPoC) ToEntityPoC() *Operation {
	status := Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}
	amt, _ := t.FromDecimal(*t.Amount, *t.AmountScale)

	amount := Amount{
		Amount: &amt,
		Scale:  t.AmountScale,
	}

	blca, _ := t.FromDecimal(*t.AvailableBalance, *t.BalanceScale)
	blcoh, _ := t.FromDecimal(*t.OnHoldBalance, *t.BalanceScale)

	balance := Balance{
		Available: &blca,
		OnHold:    &blcoh,
		Scale:     t.BalanceScale,
	}

	blcaf, _ := t.FromDecimal(*t.AvailableBalanceAfter, *t.BalanceScaleAfter)
	blcaoh, _ := t.FromDecimal(*t.OnHoldBalanceAfter, *t.BalanceScaleAfter)

	balanceAfter := Balance{
		Available: &blcaf,
		OnHold:    &blcaoh,
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
