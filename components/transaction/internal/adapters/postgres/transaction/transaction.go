package transaction

import (
	"database/sql"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"time"
)

// TransactionPostgreSQLModel represents the entity TransactionPostgreSQLModel into SQL context in Database
//
// @Description Database model for storing transaction information in PostgreSQL
type TransactionPostgreSQLModel struct {
	ID                       string                     // Unique identifier (UUID format)
	ParentTransactionID      *string                    // Parent transaction ID (for reversals or child transactions)
	Description              string                     // Human-readable description
	Template                 string                     // Template used to create this transaction
	Status                   string                     // Status code (e.g., "ACTIVE", "PENDING")
	StatusDescription        *string                    // Status description
	Amount                   *int64                     // Transaction amount value
	AmountScale              *int64                     // Decimal places for amount
	AssetCode                string                     // Asset code for the transaction
	ChartOfAccountsGroupName string                     // Chart of accounts group name for accounting
	LedgerID                 string                     // Ledger ID
	OrganizationID           string                     // Organization ID
	Body                     libTransaction.Transaction // Transaction body containing detailed operation data
	CreatedAt                time.Time                  // Creation timestamp
	UpdatedAt                time.Time                  // Last update timestamp
	DeletedAt                sql.NullTime               // Deletion timestamp (if soft-deleted)
	Metadata                 map[string]any             // Additional custom attributes
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

// CreateTransactionInput is a struct design to encapsulate payload data.
//
// swagger:model CreateTransactionInput
// @Description CreateTransactionInput is the input payload to create a transaction. Contains all necessary fields to create a financial transaction, including source and destination information.
type CreateTransactionInput struct {
	// Chart of accounts group name for accounting purposes
	// example: Chart of accounts group name
	// maxLength: 256
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName,omitempty" validate:"max=256" maxLength:"256"`
	
	// Human-readable description of the transaction
	// example: Transaction description
	// maxLength: 256
	Description string `json:"description,omitempty" validate:"max=256" example:"Transaction description" maxLength:"256"`
	
	// Transaction code for reference
	// example: TR12345
	// maxLength: 100
	Code string `json:"code,omitempty" validate:"max=100" example:"code" maxLength:"100"`
	
	// Whether the transaction should be created in pending state
	// example: true
	Pending bool `json:"pending,omitempty" example:"true"`
	
	// Additional custom attributes
	// example: {"purpose": "Monthly payment", "category": "Utility"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	
	// Send operation details including source and distribution
	// required: true
	Send *libTransaction.Send `json:"send,omitempty" validate:"required,dive"`
} // @name CreateTransactionInput

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

	// Template used to create this transaction
	// example: Transaction template
	// maxLength: 100
	Template string `json:"template" example:"Transaction template" maxLength:"100"`

	// Transaction status information
	Status Status `json:"status"`

	// Transaction amount value in smallest unit of the asset
	// example: 1500
	// minimum: 0
	Amount *int64 `json:"amount" example:"1500" minimum:"0"`

	// Decimal places for the transaction amount
	// example: 2
	// minimum: 0
	AmountScale *int64 `json:"amountScale" example:"2" minimum:"0"`

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
	Body libTransaction.Transaction `json:"-"`

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
		Amount:                   t.Amount,
		AmountScale:              t.AmountScale,
		AssetCode:                t.AssetCode,
		ChartOfAccountsGroupName: t.ChartOfAccountsGroupName,
		LedgerID:                 t.LedgerID,
		OrganizationID:           t.OrganizationID,
		Body:                     t.Body,
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

	*t = TransactionPostgreSQLModel{
		ID:                       ID,
		ParentTransactionID:      transaction.ParentTransactionID,
		Description:              transaction.Description,
		Template:                 transaction.Template,
		Status:                   transaction.Status.Code,
		StatusDescription:        transaction.Status.Description,
		Amount:                   transaction.Amount,
		AmountScale:              transaction.AmountScale,
		AssetCode:                transaction.AssetCode,
		ChartOfAccountsGroupName: transaction.ChartOfAccountsGroupName,
		LedgerID:                 transaction.LedgerID,
		OrganizationID:           transaction.OrganizationID,
		Body:                     transaction.Body,
		CreatedAt:                transaction.CreatedAt,
		UpdatedAt:                transaction.UpdatedAt,
	}

	if transaction.DeletedAt != nil {
		deletedAtCopy := *transaction.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}

// FromDSl converts an entity FromDSl to goldModel.Transaction
func (cti *CreateTransactionInput) FromDSl() *libTransaction.Transaction {
	dsl := &libTransaction.Transaction{
		ChartOfAccountsGroupName: cti.ChartOfAccountsGroupName,
		Description:              cti.Description,
		Code:                     cti.Code,
		Pending:                  cti.Pending,
		Metadata:                 cti.Metadata,
	}

	if cti.Send != nil {
		for i := range cti.Send.Source.From {
			cti.Send.Source.From[i].IsFrom = true
		}

		dsl.Send = *cti.Send
	}

	return dsl
}

// TransactionRevert is a func that revert transaction
func (t Transaction) TransactionRevert() libTransaction.Transaction {
	froms := make([]libTransaction.FromTo, 0)

	for _, to := range t.Body.Send.Distribute.To {
		to.IsFrom = true
		froms = append(froms, to)
	}

	newSource := libTransaction.Source{
		From:      froms,
		Remaining: t.Body.Send.Distribute.Remaining,
	}

	tos := make([]libTransaction.FromTo, 0)

	for _, from := range t.Body.Send.Source.From {
		from.IsFrom = false
		tos = append(tos, from)
	}

	newDistribute := libTransaction.Distribute{
		To:        tos,
		Remaining: t.Body.Send.Source.Remaining,
	}

	send := libTransaction.Send{
		Asset:      t.Body.Send.Asset,
		Value:      t.Body.Send.Value,
		Scale:      t.Body.Send.Scale,
		Source:     newSource,
		Distribute: newDistribute,
	}

	transaction := libTransaction.Transaction{
		ChartOfAccountsGroupName: t.Body.ChartOfAccountsGroupName,
		Description:              t.Body.Description,
		Code:                     t.Body.Code,
		Pending:                  t.Body.Pending,
		Metadata:                 t.Body.Metadata,
		Send:                     send,
	}

	return transaction
}

// TransactionQueue this is a struct that is responsible to send and receive from queue.
//
// @Description Container for transaction data exchanged via message queues, including validation responses, balances, and transaction details.
type TransactionQueue struct {
	// Validation responses from the transaction processing
	Validate *libTransaction.Responses `json:"validate"`
	
	// Account balances affected by the transaction
	Balances []*mmodel.Balance `json:"balances"`
	
	// The transaction being processed
	Transaction *Transaction `json:"transaction"`
	
	// Parsed transaction DSL
	ParseDSL *libTransaction.Transaction `json:"parseDSL"`
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
