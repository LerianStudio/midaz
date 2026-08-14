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

// This file is the /v2 transaction RESPONSE contract seam. Every one of the seven v2 ops
// (direct, hold, block, unblock create; commit, cancel, revert lifecycle) answers with
// TransactionV2 instead of the canonical transaction.Transaction the v1 ops carry: the only
// difference is that `source`/`destination` are spelled `debit`/`credit`. TransactionV2 mirrors
// the canonical shape field-by-field rather than embedding transaction.Transaction, so a future
// canonical field addition does not leak onto the v2 wire contract without a deliberate edit
// here. newTransactionV2 is the single conversion point every v2 output builds through.

// TransactionV2 is the /v2 wire shape of a transaction. It carries every field
// transaction.Transaction does, spelling the two leg-alias lists `debit`/`credit` instead of
// `source`/`destination`; everything else keeps its v1 name, type, and tags.
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

	// Chart of accounts group name for accounting purposes
	// example: Chart of accounts group name
	// maxLength: 256
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName" example:"Chart of accounts group name" maxLength:"256"`

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

	// Deprecated: legacy route identifier, use routeId instead. Contains the transaction route UUID as a free-form string for backwards compatibility.
	// example: 00000000-0000-0000-0000-000000000000
	// maxLength: 250
	// deprecated: true
	Route string `json:"route" example:"00000000-0000-0000-0000-000000000000" maxLength:"250"`

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
	Operations []*operation.Operation `json:"operations"`
}

// newTransactionV2 converts the canonical transaction.Transaction into its /v2 wire shape,
// renaming Source->Debit and Destination->Credit and copying every other field unchanged. It is
// the single conversion point every v2 output (create, commit, cancel, revert) builds through.
// Returns nil for a nil input so callers can convert the core's result without an extra guard.
func newTransactionV2(t *transaction.Transaction) *TransactionV2 {
	if t == nil {
		return nil
	}

	return &TransactionV2{
		ID:                       t.ID,
		ParentTransactionID:      t.ParentTransactionID,
		Description:              t.Description,
		Status:                   t.Status,
		Amount:                   t.Amount,
		AssetCode:                t.AssetCode,
		ChartOfAccountsGroupName: t.ChartOfAccountsGroupName,
		Debit:                    t.Source,
		Credit:                   t.Destination,
		LedgerID:                 t.LedgerID,
		OrganizationID:           t.OrganizationID,
		Route:                    t.Route, //nolint:staticcheck // legacy field kept for backward compatibility; RouteID is canonical
		RouteID:                  t.RouteID,
		FeesSkipped:              t.FeesSkipped,
		TracerSkipped:            t.TracerSkipped,
		CreatedAt:                t.CreatedAt,
		UpdatedAt:                t.UpdatedAt,
		DeletedAt:                t.DeletedAt,
		Metadata:                 t.Metadata,
		Operations:               t.Operations,
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
