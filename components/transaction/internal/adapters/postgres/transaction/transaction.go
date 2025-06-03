package transaction

import (
	"database/sql"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"time"
)

// TransactionPostgreSQLModel represents the entity TransactionPostgreSQLModel into SQL context in Database
//
// @Description Database model for storing transaction information in PostgreSQL
type TransactionPostgreSQLModel struct {
	ID                       string             // Unique identifier (UUID format)
	ParentTransactionID      *string            // Parent transaction ID (for reversals or child transactions)
	Description              string             // Human-readable description
	Template                 string             // Template used to create this transaction
	Status                   string             // Status code (e.g., "ACTIVE", "PENDING")
	StatusDescription        *string            // Status description
	Amount                   *decimal.Decimal   // Transaction amount value
	AssetCode                string             // Asset code for the transaction
	ChartOfAccountsGroupName string             // Chart of accounts group name for accounting
	LedgerID                 string             // Ledger ID
	OrganizationID           string             // Organization ID
	Body                     mmodel.Transaction // Transaction body containing detailed operation data
	Route                    string             // Route
	CreatedAt                time.Time          // Creation timestamp
	UpdatedAt                time.Time          // Last update timestamp
	DeletedAt                sql.NullTime       // Deletion timestamp (if soft-deleted)
	Metadata                 map[string]any     // Additional custom attributes
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

	// Template used to create this transaction
	// example: Transaction template
	// maxLength: 100
	Template string `json:"template" example:"Transaction template" maxLength:"100"`

	// Transaction status information
	Status Status `json:"status"`

	// Transaction amount value in the smallest unit of the asset
	// example: 1500
	// minimum: 0
	Amount string `json:"amount" example:"1500" minimum:"0"`

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
	Body mmodel.Transaction `json:"-"`

	// Route
	// example: 00000000-0000-0000-0000-000000000000
	// format: string
	Route string `json:"route" example:"00000000-0000-0000-0000-000000000000" format:"string"`

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
		Template:                 t.Template,
		Status:                   status,
		Amount:                   t.Amount.String(),
		AssetCode:                t.AssetCode,
		ChartOfAccountsGroupName: t.ChartOfAccountsGroupName,
		LedgerID:                 t.LedgerID,
		OrganizationID:           t.OrganizationID,
		Body:                     t.Body,
		Route:                    t.Route,
		CreatedAt:                t.CreatedAt,
		UpdatedAt:                t.UpdatedAt,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		transaction.DeletedAt = &deletedAtCopy
	}

	return transaction
}

// FromEntity converts an entity Transaction to TransactionPostgreSQLModel
func (t *TransactionPostgreSQLModel) FromEntity(transaction *Transaction) {
	ID := libCommons.GenerateUUIDv7().String()
	if transaction.ID != "" {
		ID = transaction.ID
	}

	amount := decimal.RequireFromString(transaction.Amount)

	*t = TransactionPostgreSQLModel{
		ID:                       ID,
		ParentTransactionID:      transaction.ParentTransactionID,
		Description:              transaction.Description,
		Template:                 transaction.Template,
		Status:                   transaction.Status.Code,
		StatusDescription:        transaction.Status.Description,
		Amount:                   &amount,
		AssetCode:                transaction.AssetCode,
		ChartOfAccountsGroupName: transaction.ChartOfAccountsGroupName,
		LedgerID:                 transaction.LedgerID,
		OrganizationID:           transaction.OrganizationID,
		Body:                     transaction.Body,
		Route:                    transaction.Route,
		CreatedAt:                transaction.CreatedAt,
		UpdatedAt:                transaction.UpdatedAt,
	}

	if transaction.DeletedAt != nil {
		deletedAtCopy := *transaction.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}

// TransactionRevert is a func that revert transaction
func (t Transaction) TransactionRevert() mmodel.Transaction {
	froms := make([]mmodel.FromTo, 0)

	for _, to := range t.Body.Send.Distribute.To {
		to.IsFrom = true
		froms = append(froms, to)
	}

	newSource := mmodel.Source{
		From:      froms,
		Remaining: t.Body.Send.Distribute.Remaining,
	}

	tos := make([]mmodel.FromTo, 0)

	for _, from := range t.Body.Send.Source.From {
		from.IsFrom = false
		tos = append(tos, from)
	}

	newDistribute := mmodel.Distribute{
		To:        tos,
		Remaining: t.Body.Send.Source.Remaining,
	}

	send := mmodel.Send{
		Asset:      t.Body.Send.Asset,
		Value:      t.Body.Send.Value,
		Source:     newSource,
		Distribute: newDistribute,
	}

	transaction := mmodel.Transaction{
		ChartOfAccountsGroupName: t.Body.ChartOfAccountsGroupName,
		Description:              t.Body.Description,
		Code:                     t.Body.Code,
		Pending:                  t.Body.Pending,
		Metadata:                 t.Body.Metadata,
		Send:                     send,
	}

	return transaction
}
