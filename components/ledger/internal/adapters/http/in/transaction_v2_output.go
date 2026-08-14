// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
)

// This file is the /v2 transaction RESPONSE contract seam. Every v2 transaction op answers with
// TransactionV2 (and its operations with OperationV2) instead of the canonical
// transaction.Transaction / operation.Operation the v1 ops carry. The v2 shape differs from v1 in
// two ways: the two leg-alias lists are spelled `debit`/`credit` instead of `source`/`destination`,
// and the deprecated fields are dropped — transaction-level `chartOfAccountsGroupName` and `route`,
// and operation-level `chartOfAccounts` and `route`. The canonical `routeId` (and the operation's
// `routeCode`/`routeDescription`) stay. TransactionV2 and OperationV2 mirror the canonical shapes
// field-by-field rather than embedding them, so a future canonical field addition does not leak
// onto the v2 wire contract without a deliberate edit here. newTransactionV2 is the single
// conversion point every v2 output builds through.

// TransactionV2 is the /v2 wire shape of a transaction. It carries the canonical transaction
// fields spelling the two leg-alias lists `debit`/`credit` instead of `source`/`destination`, and
// omitting the deprecated `chartOfAccountsGroupName` and `route`; everything else keeps its v1
// name, type, and tags.
type TransactionV2 struct {
	// Unique identifier for the transaction
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Parent transaction identifier (for reversals or child transactions)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ParentTransactionID *string `json:"parentTransactionId,omitempty" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Human-readable description of the transaction
	// example: Transaction description
	// maxLength: 256
	Description string `json:"description" example:"Transaction description" maxLength:"256"`

	// Transaction status information
	Status transaction.Status `json:"status"`

	// Transaction amount value in the smallest unit of the asset
	// example: 1500
	// minimum: 0
	Amount *decimal.Decimal `json:"amount" example:"1500" minimum:"0"`

	// Asset code for the transaction
	// example: BRL
	// minLength: 2
	// maxLength: 10
	AssetCode string `json:"assetCode" example:"BRL" minLength:"2" maxLength:"10"`

	// List of debit account aliases or identifiers
	// example: ["@person1"]
	Debit []string `json:"debit" example:"@person1"`

	// List of credit account aliases or identifiers
	// example: ["@person2"]
	Credit []string `json:"credit" example:"@person2"`

	// Ledger identifier
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Organization identifier
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// UUID of the transaction route. Primary field for route identification, validation, and accounting.
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	RouteID *string `json:"routeId,omitempty" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Whether an honored per-call fee skip bypassed the fee engine for this transaction
	// example: false
	FeesSkipped bool `json:"feesSkipped" example:"false"`

	// Whether an honored per-call tracer skip bypassed the tracer reserve for this transaction
	// example: false
	TracerSkipped bool `json:"tracerSkipped" example:"false"`

	// Timestamp when the transaction was created
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the transaction was last updated
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the transaction was deleted (if soft-deleted)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Additional custom attributes
	// example: {"purpose": "Monthly payment", "category": "Utility"}
	Metadata map[string]any `json:"metadata,omitempty"`

	// List of operations associated with this transaction
	Operations []*OperationV2 `json:"operations"`
}

// OperationV2 is the /v2 wire shape of an operation. It mirrors operation.Operation field-by-field
// except that the deprecated `chartOfAccounts` and `route` are dropped; the canonical
// `routeId`/`routeCode`/`routeDescription` and every other field keep their v1 name, type, and
// tags.
type OperationV2 struct {
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

	// Type of operation. One of: DEBIT, CREDIT, ON_HOLD, RELEASE, OVERDRAFT, BLOCK, UNBLOCK.
	// example: DEBIT
	// maxLength: 50
	Type string `json:"type" example:"DEBIT" maxLength:"50"`

	// Asset code for the operation
	// example: BRL
	// minLength: 2
	// maxLength: 10
	AssetCode string `json:"assetCode" example:"BRL" minLength:"2" maxLength:"10"`

	// Operation amount information
	Amount operation.Amount `json:"amount"`

	// Balance before the operation
	Balance operation.Balance `json:"balance"`

	// Balance after the operation
	BalanceAfter operation.Balance `json:"balanceAfter"`

	// Operation status information
	Status operation.Status `json:"status"`

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

	// BalanceAffected default true
	// format: boolean
	BalanceAffected bool `json:"balanceAffected" example:"true" format:"boolean"`

	// Direction of the operation (debit, credit)
	// example: debit
	// maxLength: 50
	Direction string `json:"direction,omitempty" example:"debit" maxLength:"50" enums:"debit,credit"`

	// UUID of the operation route that generated this operation. Primary field for route identification, validation, and accounting.
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	RouteID *string `json:"routeId,omitempty" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Human-readable code of the operation route for accounting traceability
	// example: ROUTE-001
	// maxLength: 100
	RouteCode *string `json:"routeCode,omitempty" example:"ROUTE-001" maxLength:"100"`

	// Human-readable description of the operation route for accounting traceability
	// example: Settlement route for service charges
	// maxLength: 250
	RouteDescription *string `json:"routeDescription,omitempty" example:"Settlement route for service charges" maxLength:"250"`

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
}

// newTransactionV2 converts the canonical transaction.Transaction into its /v2 wire shape,
// renaming Source->Debit and Destination->Credit, dropping the deprecated chartOfAccountsGroupName
// and route, mapping each operation to its OperationV2 shape, and copying every other field
// unchanged. It is the single conversion point every v2 output (create, commit, cancel, revert,
// read, update) builds through. Returns nil for a nil input so callers can convert the core's
// result without an extra guard.
func newTransactionV2(t *transaction.Transaction) *TransactionV2 {
	if t == nil {
		return nil
	}

	return &TransactionV2{
		ID:                  t.ID,
		ParentTransactionID: t.ParentTransactionID,
		Description:         t.Description,
		Status:              t.Status,
		Amount:              t.Amount,
		AssetCode:           t.AssetCode,
		Debit:               t.Source,
		Credit:              t.Destination,
		LedgerID:            t.LedgerID,
		OrganizationID:      t.OrganizationID,
		RouteID:             t.RouteID,
		FeesSkipped:         t.FeesSkipped,
		TracerSkipped:       t.TracerSkipped,
		CreatedAt:           t.CreatedAt,
		UpdatedAt:           t.UpdatedAt,
		DeletedAt:           t.DeletedAt,
		Metadata:            t.Metadata,
		Operations:          newOperationsV2(t.Operations),
	}
}

// newOperationsV2 maps a slice of canonical operations to their /v2 wire shape, preserving a nil
// input as nil so an operation-less transaction keeps the canonical operations encoding.
func newOperationsV2(ops []*operation.Operation) []*OperationV2 {
	if ops == nil {
		return nil
	}

	out := make([]*OperationV2, 0, len(ops))
	for _, op := range ops {
		out = append(out, newOperationV2(op))
	}

	return out
}

// newOperationV2 converts a canonical operation.Operation into its /v2 wire shape, dropping the
// deprecated chartOfAccounts and route and copying every other field unchanged. Returns nil for a
// nil input.
func newOperationV2(op *operation.Operation) *OperationV2 {
	if op == nil {
		return nil
	}

	return &OperationV2{
		ID:               op.ID,
		TransactionID:    op.TransactionID,
		Description:      op.Description,
		Type:             op.Type,
		AssetCode:        op.AssetCode,
		Amount:           op.Amount,
		Balance:          op.Balance,
		BalanceAfter:     op.BalanceAfter,
		Status:           op.Status,
		AccountID:        op.AccountID,
		AccountAlias:     op.AccountAlias,
		BalanceKey:       op.BalanceKey,
		BalanceID:        op.BalanceID,
		OrganizationID:   op.OrganizationID,
		LedgerID:         op.LedgerID,
		BalanceAffected:  op.BalanceAffected,
		Direction:        op.Direction,
		RouteID:          op.RouteID,
		RouteCode:        op.RouteCode,
		RouteDescription: op.RouteDescription,
		CreatedAt:        op.CreatedAt,
		UpdatedAt:        op.UpdatedAt,
		DeletedAt:        op.DeletedAt,
		Metadata:         op.Metadata,
	}
}

// CreateTransactionOutputV2Huma pins 201 (matching http.Created) and carries the
// X-Idempotency-Replayed response header, mirroring CreateTransactionOutputHuma but wrapping
// the /v2 wire shape (TransactionV2) instead of the canonical transaction.Transaction.
type CreateTransactionOutputV2Huma struct {
	Status              int
	IdempotencyReplayed string `header:"X-Idempotency-Replayed"`
	Body                *TransactionV2
}

// StateTransactionOutputV2Huma pins 201 (matching http.Created) and carries the resulting
// transaction in the /v2 wire shape, mirroring StateTransactionOutputHuma for the v2
// commit/cancel ops.
type StateTransactionOutputV2Huma struct {
	Status int
	Body   *TransactionV2
}
