package mmodel

import (
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/shopspring/decimal"
)

// OperationAmount structure for marshaling/unmarshalling JSON.
//
// swagger:model OperationAmount
// @Description OperationAmount is the struct designed to represent the amount of an operation. Contains the value and scale (decimal places) of an operation amount.
type OperationAmount struct {
	// The amount value in the smallest unit of the asset (e.g., cents)
	// example: 1500
	// minimum: 0
	Value *decimal.Decimal `json:"value" example:"1500" minimum:"0"`
} // @name OperationAmount

// IsEmpty method that set empty or nil in fields
func (a OperationAmount) IsEmpty() bool {
	return a.Value == nil
}

// OperationBalance structure for marshaling/unmarshalling JSON.
//
// swagger:model OperationBalance
// @Description OperationBalance is the struct designed to represent the account balance. Contains available and on-hold amounts along with the scale (decimal places).
type OperationBalance struct {
	// Amount available for transactions (in the smallest unit of asset)
	// example: 1500
	// minimum: 0
	Available *decimal.Decimal `json:"available" example:"1500" minimum:"0"`

	// Amount on hold and unavailable for transactions (in the smallest unit of asset)
	// example: 500
	// minimum: 0
	OnHold *decimal.Decimal `json:"onHold" example:"500" minimum:"0"`

	// Balance version after the operation
	// example: 2
	// minimum: 0
	Version *int64 `json:"version" example:"2" minimum:"0"`
} // @name OperationBalance

// IsEmpty method that set empty or nil in fields
func (b OperationBalance) IsEmpty() bool {
	return b.Available == nil && b.OnHold == nil
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
	Amount OperationAmount `json:"amount"`

	// Balance before the operation
	Balance OperationBalance `json:"balance"`

	// Balance after the operation
	BalanceAfter OperationBalance `json:"balanceAfter"`

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

	// Unique key for the balance
	// example: asset-freeze
	// maxLength: 100
	BalanceKey string `json:"balanceKey" example:"asset-freeze" maxLength:"100"`

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

	// Route
	// example: 00000000-0000-0000-0000-000000000000
	// format: string
	Route string `json:"route" example:"00000000-0000-0000-0000-000000000000" format:"string"`

	// BalanceAffected default true
	// format: boolean
	BalanceAffected bool `json:"balanceAffected" example:"true" format:"boolean"`

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
	Amount OperationAmount `json:"amount"`

	// Balance before the operation
	Balance OperationBalance `json:"balance"`

	// Balance after the operation
	BalanceAfter OperationBalance `json:"balanceAfter"`

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

	// Unique key for the balance (required, max length 256 characters)
	// example: asset-freeze
	BalanceKey string `json:"balanceKey" example:"asset-freeze"`

	// Balance identifier affected by this operation
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	BalanceID string `json:"balanceId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Timestamp when the operation log was created
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Route for the operation
	// example: 00000000-0000-0000-0000-000000000000
	// format: string
	Route string `json:"route" example:"00000000-0000-0000-0000-000000000000" format:"string"`

	// BalanceAffected default true
	// format: boolean
	BalanceAffected bool `json:"balanceAffected" example:"true" format:"boolean"`
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
		BalanceKey:      o.BalanceKey,
		BalanceID:       o.BalanceID,
		Route:           o.Route,
		CreatedAt:       o.CreatedAt,
		BalanceAffected: o.BalanceAffected,
	}
}

// NewOperation creates a new Operation with validated invariants.
// Panics if any invariant is violated (programming errors).
func NewOperation(id, transactionID, opType, assetCode string, amountValue decimal.Decimal) *Operation {
	assert.That(assert.ValidUUID(id),
		"operation ID must be valid UUID",
		"id", id)
	assert.That(assert.ValidUUID(transactionID),
		"transaction ID must be valid UUID",
		"transactionID", transactionID)
	assert.That(opType == constant.DEBIT || opType == constant.CREDIT,
		"operation type must be DEBIT or CREDIT",
		"type", opType)
	assert.NotEmpty(assetCode,
		"assetCode must not be empty")
	assert.That(assert.NonNegativeDecimal(amountValue),
		"amount must be non-negative",
		"value", amountValue.String())

	now := time.Now()

	return &Operation{
		ID:            id,
		TransactionID: transactionID,
		Type:          opType,
		AssetCode:     assetCode,
		Amount:        OperationAmount{Value: &amountValue},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}
