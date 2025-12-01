// Package operation provides PostgreSQL data models for financial operation persistence.
//
// This package implements the infrastructure layer for operation storage in PostgreSQL,
// following the hexagonal architecture pattern. Operations represent individual
// debit/credit entries within a transaction, forming the atomic units of double-entry
// accounting.
//
// Domain Concept:
//
// An Operation in the ledger system:
//   - Represents a single debit or credit affecting an account balance
//   - Belongs to exactly one transaction (parent relationship)
//   - Records balance state before and after the operation
//   - Supports optimistic locking via balance versioning
//   - Enables audit trail with full balance snapshots
//
// Double-Entry Accounting:
//
// Each transaction contains multiple operations that must balance:
//   - DEBIT operations decrease source account balances
//   - CREDIT operations increase destination account balances
//   - Sum of debits must equal sum of credits per transaction
//
// Data Flow:
//
//	Domain Entity (Operation) -> OperationPostgreSQLModel -> PostgreSQL
//	PostgreSQL -> OperationPostgreSQLModel -> Domain Entity (Operation)
//
// Related Packages:
//   - transaction: Parent transaction containing operations
//   - balance: Account balances affected by operations
//   - mmodel: Domain model definitions
package operation

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/shopspring/decimal"
)

// OperationPostgreSQLModel represents the operation entity in PostgreSQL.
//
// This model maps directly to the 'operation' table with SQL-specific types.
// It captures the complete state of a financial operation including balance
// snapshots before and after execution for audit purposes.
//
// Table Schema:
//
//	CREATE TABLE operation (
//	    id UUID PRIMARY KEY,
//	    transaction_id UUID NOT NULL REFERENCES transaction(id),
//	    description TEXT,
//	    type VARCHAR(10) NOT NULL,  -- 'DEBIT' or 'CREDIT'
//	    asset_code VARCHAR(10) NOT NULL,
//	    amount DECIMAL(38,18) NOT NULL,
//	    available_balance DECIMAL(38,18),
//	    on_hold_balance DECIMAL(38,18),
//	    version_balance BIGINT,
//	    available_balance_after DECIMAL(38,18),
//	    on_hold_balance_after DECIMAL(38,18),
//	    version_balance_after BIGINT,
//	    status VARCHAR(50) NOT NULL,
//	    status_description TEXT,
//	    account_id UUID NOT NULL,
//	    account_alias VARCHAR(256),
//	    balance_key VARCHAR(100),
//	    balance_id UUID NOT NULL,
//	    chart_of_accounts VARCHAR(100),
//	    organization_id UUID NOT NULL,
//	    ledger_id UUID NOT NULL,
//	    route VARCHAR(256),
//	    balance_affected BOOLEAN DEFAULT true,
//	    created_at TIMESTAMP WITH TIME ZONE,
//	    updated_at TIMESTAMP WITH TIME ZONE,
//	    deleted_at TIMESTAMP WITH TIME ZONE
//	);
//
// Balance Snapshots:
//
// The model captures balance state at two points:
//   - Before: AvailableBalance, OnHoldBalance, VersionBalance
//   - After: AvailableBalanceAfter, OnHoldBalanceAfter, VersionBalanceAfter
//
// This enables:
//   - Audit reconstruction of balance history
//   - Idempotency verification (version checking)
//   - Transaction reversal calculations
//
// Thread Safety:
//
// OperationPostgreSQLModel is not thread-safe. Each goroutine should work
// with its own instance.
//
// @Description Database model for storing operation information in PostgreSQL
type OperationPostgreSQLModel struct {
	ID                    string           // Unique identifier (UUID format)
	TransactionID         string           // Parent transaction ID
	Description           string           // Operation description
	Type                  string           // Operation type (e.g., "DEBIT", "CREDIT")
	AssetCode             string           // Asset code for the operation
	Amount                *decimal.Decimal // Operation amount value
	AvailableBalance      *decimal.Decimal // Available balance before operation
	OnHoldBalance         *decimal.Decimal // On-hold balance before operation
	VersionBalance        *int64           // Balance version before operation
	AvailableBalanceAfter *decimal.Decimal // Available balance after operation
	OnHoldBalanceAfter    *decimal.Decimal // On-hold balance after operation
	VersionBalanceAfter   *int64           // Balance version after operation
	Status                string           // Status code (e.g., "ACTIVE", "PENDING")
	StatusDescription     *string          // Status description
	AccountID             string           // Account ID associated with operation
	AccountAlias          string           // Account alias
	BalanceKey            string           // Balance key for additional balances
	BalanceID             string           // Balance ID affected by operation
	ChartOfAccounts       string           // Chart of accounts code
	OrganizationID        string           // Organization ID
	LedgerID              string           // Ledger ID
	CreatedAt             time.Time        // Creation timestamp
	UpdatedAt             time.Time        // Last update timestamp
	DeletedAt             sql.NullTime     // Deletion timestamp (if soft-deleted)
	Route                 *string          // Route
	BalanceAffected       bool             // BalanceAffected default true
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
	Value *decimal.Decimal `json:"value" swaggertype:"string" example:"1500.00" minimum:"0"`
} // @name Amount

// IsEmpty method that set empty or nil in fields
func (a Amount) IsEmpty() bool {
	return a.Value == nil
}

// Balance structure for marshaling/unmarshalling JSON.
//
// swagger:model Balance
// @Description Balance is the struct designed to represent the account balance. Contains available and on-hold amounts along with the scale (decimal places).
type Balance struct {
	// Amount available for transactions (in the smallest unit of asset)
	// example: 1500
	// minimum: 0
	Available *decimal.Decimal `json:"available" swaggertype:"string" example:"1500.00" minimum:"0"`

	// Amount on hold and unavailable for transactions (in the smallest unit of asset)
	// example: 500
	// minimum: 0
	OnHold *decimal.Decimal `json:"onHold" swaggertype:"string" example:"500.00" minimum:"0"`

	// Balance version after the operation
	// example: 2
	// minimum: 0
	Version *int64 `json:"version" example:"2" minimum:"0"`
} // @name Balance

// IsEmpty method that set empty or nil in fields
func (b Balance) IsEmpty() bool {
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

// ToEntity converts an OperationPostgreSQLModel to the domain Operation model.
//
// This method implements the outbound mapping in hexagonal architecture,
// transforming the persistence model back to the domain representation.
//
// Mapping Process:
//  1. Create Status value object from code and description
//  2. Create Amount value object from decimal value
//  3. Create Balance value objects for before/after states
//  4. Map all direct fields including timestamps
//  5. Handle nullable Route and DeletedAt fields
//
// Returns:
//   - *Operation: Domain model with all fields mapped
func (t *OperationPostgreSQLModel) ToEntity() *Operation {
	status := Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	amount := Amount{
		Value: t.Amount,
	}

	balance := Balance{
		Available: t.AvailableBalance,
		OnHold:    t.OnHoldBalance,
		Version:   t.VersionBalance,
	}

	balanceAfter := Balance{
		Available: t.AvailableBalanceAfter,
		OnHold:    t.OnHoldBalanceAfter,
		Version:   t.VersionBalanceAfter,
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
		BalanceKey:      t.BalanceKey,
		LedgerID:        t.LedgerID,
		OrganizationID:  t.OrganizationID,
		BalanceAffected: t.BalanceAffected,
		BalanceID:       t.BalanceID,
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
		DeletedAt:       nil,
	}

	if t.Route != nil {
		Operation.Route = *t.Route
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		Operation.DeletedAt = &deletedAtCopy
	}

	return Operation
}

// FromEntity converts a domain Operation model to OperationPostgreSQLModel.
//
// This method implements the inbound mapping in hexagonal architecture,
// transforming the domain representation to the persistence model.
//
// Mapping Process:
//  1. Generate UUIDv7 if ID not provided (new operation)
//  2. Extract values from embedded value objects (Status, Amount, Balance)
//  3. Map all direct fields with type conversions
//  4. Handle optional Route field
//  5. Convert nullable DeletedAt to sql.NullTime
//
// Parameters:
//   - operation: Domain Operation model to convert
//
// ID Generation:
//
// Uses UUID v7 which provides:
//   - Time-ordered IDs for index efficiency
//   - Globally unique identifiers
//   - Sortable by creation time
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
		Amount:                operation.Amount.Value,
		OnHoldBalance:         operation.Balance.OnHold,
		AvailableBalance:      operation.Balance.Available,
		VersionBalance:        operation.Balance.Version,
		AvailableBalanceAfter: operation.BalanceAfter.Available,
		OnHoldBalanceAfter:    operation.BalanceAfter.OnHold,
		VersionBalanceAfter:   operation.BalanceAfter.Version,
		Status:                operation.Status.Code,
		StatusDescription:     operation.Status.Description,
		AccountID:             operation.AccountID,
		AccountAlias:          operation.AccountAlias,
		BalanceKey:            operation.BalanceKey,
		BalanceID:             operation.BalanceID,
		LedgerID:              operation.LedgerID,
		OrganizationID:        operation.OrganizationID,
		CreatedAt:             operation.CreatedAt,
		UpdatedAt:             operation.UpdatedAt,
		BalanceAffected:       operation.BalanceAffected,
	}

	if !libCommons.IsNilOrEmpty(&operation.Route) {
		t.Route = &operation.Route
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

// ToLog converts an Operation to an immutable audit log entry.
//
// This method creates a snapshot of the operation for audit purposes,
// excluding mutable fields like Description and Metadata that may change
// after creation.
//
// Audit Log Purpose:
//
// The OperationLog captures the financial state at execution time:
//   - Transaction reference (immutable)
//   - Amount and balance changes (immutable)
//   - Account associations (immutable)
//   - Creation timestamp (immutable)
//
// Excluded Fields:
//   - Description: May be updated for clarification
//   - Metadata: May be extended with additional context
//   - UpdatedAt: Changes on any modification
//   - DeletedAt: Represents soft delete state
//
// Returns:
//   - *OperationLog: Immutable snapshot for audit storage
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
