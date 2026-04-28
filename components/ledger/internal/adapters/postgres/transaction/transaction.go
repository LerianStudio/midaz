// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"database/sql"
	"time"

	constant "github.com/LerianStudio/lib-commons/v4/commons/constants"
	"github.com/shopspring/decimal"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	pkgConstant "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/google/uuid"
)

// CountFilter holds optional filters for counting transactions.
type CountFilter struct {
	Route     string    // Empty means include all routes
	Status    string    // Empty means include all statuses
	StartDate time.Time // Mandatory lower bound on created_at
	EndDate   time.Time // Mandatory upper bound on created_at
}

// TransactionPostgreSQLModel represents the entity TransactionPostgreSQLModel into SQL context in Database
//
// @Description Database model for storing transaction information in PostgreSQL
type TransactionPostgreSQLModel struct {
	ID                       string                    // Unique identifier (UUID format)
	ParentTransactionID      *string                   // Parent transaction ID (for reversals or child transactions)
	Description              string                    // Human-readable description
	Status                   string                    // Status code (e.g., "ACTIVE", "PENDING")
	StatusDescription        *string                   // Status description
	Amount                   *decimal.Decimal          // Transaction amount value
	AssetCode                string                    // Asset code for the transaction
	ChartOfAccountsGroupName string                    // Chart of accounts group name for accounting
	LedgerID                 string                    // Ledger ID
	OrganizationID           string                    // Organization ID
	Body                     *mtransaction.Transaction // Transaction body containing detailed operation data
	CreatedAt                time.Time                 // Creation timestamp
	UpdatedAt                time.Time                 // Last update timestamp
	DeletedAt                sql.NullTime              // Deletion timestamp (if soft-deleted)
	Route                    *string                   // Deprecated: legacy route identifier. Use RouteID instead.
	RouteID                  *string                   // UUID of the transaction route (FK to transaction_route.id)
	Metadata                 map[string]any            // Additional custom attributes
}

// Status structure for marshaling/unmarshalling JSON.
//
// swagger:model Status
// @Description Status is the struct designed to represent the status of a transaction. Contains code and optional description for transaction states.
type Status struct {
	// Status code identifying the state of the transaction
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

// InputDSL is a struct design to encapsulate payload data.
//
// swagger:model InputDSL
// @Description Template-based transaction input for creating transactions from predefined templates with variable substitution.
type InputDSL struct {
	// Transaction type identifier
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	TransactionType uuid.UUID `json:"transactionType" format:"uuid"`

	// Transaction type code for reference
	// example: PAYMENT
	// maxLength: 50
	TransactionTypeCode string `json:"transactionTypeCode" maxLength:"50"`

	// Variables to substitute in the transaction template
	// example: {"amount": 1000, "recipient": "@person2"}
	Variables map[string]any `json:"variables,omitempty"`
}

// UpdateTransactionInput is a struct design to encapsulate payload data.
//
// swagger:model UpdateTransactionInput
// @Description UpdateTransactionInput is the input payload to update a transaction. Contains fields that can be modified after a transaction is created.
type UpdateTransactionInput struct {
	// Human-readable description of the transaction
	// example: Transaction description
	// maxLength: 256
	Description string `json:"description" validate:"max=256" example:"Transaction description" maxLength:"256"`

	// Additional custom attributes
	// example: {"purpose": "Monthly payment", "category": "Utility"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateTransactionInput

// Transaction is a struct designed to encapsulate response payload data.
//
// swagger:model Transaction
// @Description Transaction is a struct designed to store transaction data. Represents a financial transaction that consists of multiple operations affecting account balances, including details about the transaction's status, amounts, and related operations.
type Transaction struct {
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
	Status Status `json:"status"`

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

	// List of source account aliases or identifiers
	// example: ["@person1"]
	Source []string `json:"source" example:"@person1"`

	// List of destination account aliases or identifiers
	// example: ["@person2"]
	Destination []string `json:"destination" example:"@person2"`

	// Ledger identifier
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Organization identifier
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Transaction body containing detailed operation data (not exposed in JSON)
	Body mtransaction.Transaction `json:"-"`

	// Deprecated: legacy route identifier, use routeId instead. Contains the transaction route UUID as a free-form string for backwards compatibility.
	// example: 00000000-0000-0000-0000-000000000000
	// maxLength: 250
	// deprecated: true
	Route string `json:"route" example:"00000000-0000-0000-0000-000000000000" maxLength:"250"`

	// UUID of the transaction route. Primary field for route identification, validation, and accounting.
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	RouteID *string `json:"routeId,omitempty" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

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
} // @name Transaction

// IDtoUUID is a func that convert UUID string to uuid.UUID
func (t Transaction) IDtoUUID() uuid.UUID {
	return uuid.MustParse(t.ID)
}

// ToEntity converts an TransactionPostgreSQLModel to entity Transaction
func (t *TransactionPostgreSQLModel) ToEntity() *Transaction {
	status := Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	transaction := &Transaction{
		ID:                       t.ID,
		ParentTransactionID:      t.ParentTransactionID,
		Description:              t.Description,
		Status:                   status,
		Amount:                   t.Amount,
		AssetCode:                t.AssetCode,
		ChartOfAccountsGroupName: t.ChartOfAccountsGroupName,
		LedgerID:                 t.LedgerID,
		OrganizationID:           t.OrganizationID,
		CreatedAt:                t.CreatedAt,
		UpdatedAt:                t.UpdatedAt,
	}

	if t.Body != nil && !t.Body.IsEmpty() {
		transaction.Body = *t.Body
	}

	if t.Route != nil {
		transaction.Route = *t.Route
	}

	if t.RouteID != nil {
		transaction.RouteID = t.RouteID
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		transaction.DeletedAt = &deletedAtCopy
	}

	return transaction
}

// FromEntity converts an entity Transaction to TransactionPostgreSQLModel
func (t *TransactionPostgreSQLModel) FromEntity(transaction *Transaction) {
	ID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	if transaction.ID != "" {
		ID = transaction.ID
	}

	*t = TransactionPostgreSQLModel{
		ID:                       ID,
		ParentTransactionID:      transaction.ParentTransactionID,
		Description:              transaction.Description,
		Status:                   transaction.Status.Code,
		StatusDescription:        transaction.Status.Description,
		Amount:                   transaction.Amount,
		AssetCode:                transaction.AssetCode,
		ChartOfAccountsGroupName: transaction.ChartOfAccountsGroupName,
		LedgerID:                 transaction.LedgerID,
		OrganizationID:           transaction.OrganizationID,
		CreatedAt:                transaction.CreatedAt,
		UpdatedAt:                transaction.UpdatedAt,
	}

	if !transaction.Body.IsEmpty() {
		t.Body = &transaction.Body
	}

	if !libCommons.IsNilOrEmpty(&transaction.Route) {
		t.Route = &transaction.Route
	}

	if transaction.RouteID != nil {
		t.RouteID = transaction.RouteID
	}

	if transaction.DeletedAt != nil {
		deletedAtCopy := *transaction.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}

// TransactionRevert builds a reversed transaction by swapping from/to sides.
// Original CREDIT operations become sources (from) and original DEBIT operations
// become destinations (to). Direction is intentionally omitted because
// CalculateTotal re-derives it via DetermineOperation based on IsFrom.
func (t Transaction) TransactionRevert() mtransaction.Transaction {
	if t.Amount == nil {
		return mtransaction.Transaction{}
	}

	froms := make([]mtransaction.FromTo, 0)
	tos := make([]mtransaction.FromTo, 0)
	fromByAlias := make(map[string]int)
	toByAlias := make(map[string]int)

	addCompanionAmount := func(entries []mtransaction.FromTo, indexByAlias map[string]int, op *operation.Operation) []mtransaction.FromTo {
		idx, ok := indexByAlias[op.AccountAlias]
		if !ok {
			return entries
		}

		if entries[idx].Amount == nil {
			return entries
		}

		entries[idx].Amount.Value = entries[idx].Amount.Value.Add(*op.Amount.Value)

		return entries
	}

	for _, op := range t.Operations {
		if op.Amount.Value == nil {
			continue
		}

		switch op.Type {
		// Only CREDIT and DEBIT appear in APPROVED transactions, which is the
		// only status eligible for revert (guarded upstream).  ONHOLD and
		// RELEASE are excluded because they belong to PENDING flows.
		case pkgConstant.OVERDRAFT:
			switch op.Direction {
			case pkgConstant.DirectionCredit:
				froms = addCompanionAmount(froms, fromByAlias, op)
			case pkgConstant.DirectionDebit, "":
				tos = addCompanionAmount(tos, toByAlias, op)
			}

			continue

		case constant.CREDIT:
			if op.BalanceKey == pkgConstant.OverdraftBalanceKey {
				froms = addCompanionAmount(froms, fromByAlias, op)

				continue
			}

			balanceKey := op.BalanceKey
			if balanceKey == "" {
				balanceKey = pkgConstant.DefaultBalanceKey
			}

			froms = append(froms, mtransaction.FromTo{
				IsFrom:          true,
				AccountAlias:    op.AccountAlias,
				BalanceKey:      balanceKey,
				Amount:          &mtransaction.Amount{Asset: op.AssetCode, Value: *op.Amount.Value},
				Description:     op.Description,
				ChartOfAccounts: op.ChartOfAccounts,
				Metadata:        op.Metadata,
				Route:           op.Route,
				RouteID:         op.RouteID,
			})
			fromByAlias[op.AccountAlias] = len(froms) - 1
		case constant.DEBIT:
			if op.BalanceKey == pkgConstant.OverdraftBalanceKey {
				tos = addCompanionAmount(tos, toByAlias, op)

				continue
			}

			balanceKey := op.BalanceKey
			if balanceKey == "" {
				balanceKey = pkgConstant.DefaultBalanceKey
			}

			tos = append(tos, mtransaction.FromTo{
				IsFrom:          false,
				AccountAlias:    op.AccountAlias,
				BalanceKey:      balanceKey,
				Amount:          &mtransaction.Amount{Asset: op.AssetCode, Value: *op.Amount.Value},
				Description:     op.Description,
				ChartOfAccounts: op.ChartOfAccounts,
				Metadata:        op.Metadata,
				Route:           op.Route,
				RouteID:         op.RouteID,
			})
			toByAlias[op.AccountAlias] = len(tos) - 1
		}
	}

	send := mtransaction.Send{
		Asset: t.AssetCode,
		Value: *t.Amount,
		Source: mtransaction.Source{
			From: froms,
		},
		Distribute: mtransaction.Distribute{
			To: tos,
		},
	}

	transaction := mtransaction.Transaction{
		ChartOfAccountsGroupName: t.ChartOfAccountsGroupName,
		Description:              t.Description,
		Pending:                  false,
		Metadata:                 t.Metadata,
		Route:                    t.Route,
		RouteID:                  t.RouteID,
		Send:                     send,
	}

	return transaction
}

// TransactionProcessingPayload contains all data needed to process a transaction
// via message queue (create balances, transaction record, and operations).
//
// This struct is serialized via msgpack to RabbitMQ. The msgpack tags preserve
// backward compatibility with messages serialized before the rename.
//
// @Description Container for transaction data exchanged via message queues.
type TransactionProcessingPayload struct {
	// Validation responses from the transaction processing
	Validate *mtransaction.Responses `json:"validate" msgpack:"Validate"`

	// Account balances affected by the transaction (BEFORE state)
	Balances []*mmodel.Balance `json:"balances" msgpack:"Balances"`

	// Account balances post-mutation from the Lua atomic script (AFTER state).
	// When present, UpdateBalances persists these directly without recalculating.
	// When nil (legacy payloads from rolling update), UpdateBalances falls back
	// to OperateBalances for backward compatibility.
	BalancesAfter []*mmodel.Balance `json:"balancesAfter,omitempty" msgpack:"BalancesAfter,omitempty"`

	// The transaction being processed
	Transaction *Transaction `json:"transaction" msgpack:"Transaction"`

	// Input transaction data (renamed from ParseDSL for clarity)
	Input *mtransaction.Transaction `json:"input" msgpack:"ParseDSL"`

	// Version discriminates the payload format for rolling-update compatibility.
	// "v2": produced by v3.6.2+ — balance persistence is handled by BalanceSyncWorker.
	// ""  : produced by v3.5.x  — consumer must call UpdateBalances() directly,
	//       because the sync worker may not have ZSET entries for these transactions.
	Version string `json:"version,omitempty" msgpack:"Version,omitempty"`
}

// TransactionResponse represents a success response containing a single transaction.
//
// swagger:response TransactionResponse
// @Description Successful response containing a single transaction entity.
type TransactionResponse struct {
	// in: body
	Body Transaction
}

// TransactionsResponse represents a success response containing a paginated list of transactions.
//
// swagger:response TransactionsResponse
// @Description Successful response containing a paginated list of transactions.
type TransactionsResponse struct {
	// in: body
	Body struct {
		Items      []Transaction `json:"items"`
		Pagination struct {
			Limit      int     `json:"limit"`
			NextCursor *string `json:"next_cursor,omitempty"`
			PrevCursor *string `json:"prev_cursor,omitempty"`
		} `json:"pagination"`
	}
}
