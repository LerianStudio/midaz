package transaction

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/common"
	goldModel "github.com/LerianStudio/midaz/common/gold/transaction/model"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/google/uuid"
)

// TransactionPostgreSQLModel represents the entity TransactionPostgreSQLModel into SQL context in Database
type TransactionPostgreSQLModel struct {
	ID                       string
	ParentTransactionID      *string
	Description              string
	Template                 string
	Status                   string
	StatusDescription        *string
	Amount                   *float64
	AmountScale              *float64
	AssetCode                string
	ChartOfAccountsGroupName string
	LedgerID                 string
	OrganizationID           string
	CreatedAt                time.Time
	UpdatedAt                time.Time
	DeletedAt                sql.NullTime
	Metadata                 map[string]any
}

// Status structure for marshaling/unmarshalling JSON.
//
// swagger:model Status
// @Description Status is the struct designed to represent the status of a transaction.
type Status struct {
	Code        string  `json:"code" validate:"max=100" example:"ACTIVE"`
	Description *string `json:"description" validate:"omitempty,max=256" example:"Active status"`
} // @name Status

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}

// CreateTransactionInput is  a struct design to encapsulate payload data.
//
// swagger:model CreateTransactionInput
// @Description CreateTransactionInput is the input payload to create a transaction.
type CreateTransactionInput struct {
	ChartOfAccountsGroupName string                `json:"chartOfAccountsGroupName,omitempty" validate:"max=256"`
	Description              string                `json:"description,omitempty" validate:"max=256"`
	Code                     string                `json:"code,omitempty" validate:"max=100"`
	Pending                  bool                  `json:"pending,omitempty"`
	Metadata                 map[string]any        `json:"metadata,omitempty"`
	Send                     *goldModel.Send       `json:"send,omitempty" validate:"required,dive"`
	Distribute               *goldModel.Distribute `json:"distribute,omitempty" validate:"required,dive"`
} // @name CreateTransactionInput

// InputDSL is a struct design to encapsulate payload data.
type InputDSL struct {
	TransactionType     uuid.UUID      `json:"transactionType"`
	TransactionTypeCode string         `json:"transactionTypeCode"`
	Variables           map[string]any `json:"variables,omitempty"`
}

// UpdateTransactionInput is a struct design to encapsulate payload data.
//
// swagger:model UpdateTransactionInput
// @Description UpdateTransactionInput is the input payload to update a transaction.
type UpdateTransactionInput struct {
	Description string         `json:"description" validate:"max=256" example:"Transaction description"`
	Metadata    map[string]any `json:"metadata,omitempty"`
} // @name UpdateTransactionInput

// Transaction is a struct designed to encapsulate response payload data.
//
// swagger:model Transaction
// @Description Transaction is a struct designed to store transaction data.
type Transaction struct {
	ID                       string                 `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	ParentTransactionID      *string                `json:"parentTransactionId,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	Description              string                 `json:"description" example:"Transaction description"`
	Template                 string                 `json:"template" example:"Transaction template"`
	Status                   Status                 `json:"status"`
	Amount                   *float64               `json:"amount" example:"1500"`
	AmountScale              *float64               `json:"amountScale" example:"2"`
	AssetCode                string                 `json:"assetCode" example:"BRL"`
	ChartOfAccountsGroupName string                 `json:"chartOfAccountsGroupName" example:"Chart of accounts group name"`
	Source                   []string               `json:"source" example:"@person1"`
	Destination              []string               `json:"destination" example:"@person2"`
	LedgerID                 string                 `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID           string                 `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	CreatedAt                time.Time              `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt                time.Time              `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt                *time.Time             `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata                 map[string]any         `json:"metadata,omitempty"`
	Operations               []*operation.Operation `json:"operations"`
} // @name Transaction

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
	*t = TransactionPostgreSQLModel{
		ID:                       common.GenerateUUIDv7().String(),
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
		CreatedAt:                transaction.CreatedAt,
		UpdatedAt:                transaction.UpdatedAt,
	}

	if transaction.DeletedAt != nil {
		deletedAtCopy := *transaction.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}

// FromDSl converts an entity FromDSl to goldModel.Transaction
func (cti *CreateTransactionInput) FromDSl() *goldModel.Transaction {
	dsl := &goldModel.Transaction{
		ChartOfAccountsGroupName: cti.ChartOfAccountsGroupName,
		Description:              cti.Description,
		Code:                     cti.Code,
		Pending:                  cti.Pending,
		Metadata:                 cti.Metadata,
	}

	if cti.Send != nil {
		dsl.Send = *cti.Send
	}

	if cti.Distribute != nil {
		dsl.Distribute = *cti.Distribute
	}

	return dsl
}
