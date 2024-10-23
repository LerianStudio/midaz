package transaction

import (
	"database/sql"
	"github.com/LerianStudio/midaz/common"
	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	"time"

	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
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
type Status struct {
	Code        string  `json:"code" validate:"max=100"`
	Description *string `json:"description" validate:"max=256"`
}

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}

// CreateTransactionInput is a struct design to encapsulate payload data.
type CreateTransactionInput struct {
	ChartOfAccountsGroupName string          `json:"chartOfAccountsGroupName" validate:"max=256"`
	Description              string          `json:"description,omitempty" validate:"max=256"`
	Code                     string          `json:"code,omitempty" validate:"max=100"`
	Pending                  bool            `json:"pending,omitempty"`
	Metadata                 map[string]any  `json:"metadata,omitempty"`
	Send                     gold.Send       `json:"send,omitempty"`
	Distribute               gold.Distribute `json:"distribute,omitempty"`
}

// InputDSL is a struct design to encapsulate payload data.
type InputDSL struct {
	TransactionType     uuid.UUID      `json:"transactionType"`
	TransactionTypeCode string         `json:"transactionTypeCode"`
	Variables           map[string]any `json:"variables,omitempty"`
}

// UpdateTransactionInput is a struct design to encapsulate payload data.
type UpdateTransactionInput struct {
	Description string         `json:"description" validate:"max=256"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Transaction is a struct designed to encapsulate response payload data.
type Transaction struct {
	ID                       string         `json:"id"`
	ParentTransactionID      *string        `json:"parentTransactionId,omitempty"`
	Description              string         `json:"description"`
	Template                 string         `json:"template"`
	Status                   Status         `json:"status"`
	Amount                   *float64       `json:"amount"`
	AmountScale              *float64       `json:"amountScale"`
	AssetCode                string         `json:"assetCode"`
	ChartOfAccountsGroupName string         `json:"chartOfAccountsGroupName"`
	Source                   []string       `json:"source"`
	Destination              []string       `json:"destination"`
	LedgerID                 string         `json:"ledgerId"`
	OrganizationID           string         `json:"organizationId"`
	CreatedAt                time.Time      `json:"createdAt"`
	UpdatedAt                time.Time      `json:"updatedAt"`
	DeletedAt                *time.Time     `json:"deletedAt"`
	Metadata                 map[string]any `json:"metadata,omitempty"`
	Operations               []*o.Operation `json:"operations"`
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

// FromDSl converts an entity FromDSl to gold.Transaction
func (cti *CreateTransactionInput) FromDSl() *gold.Transaction {
	dsl := &gold.Transaction{
		ChartOfAccountsGroupName: cti.ChartOfAccountsGroupName,
		Description:              cti.Description,
		Code:                     cti.Code,
		Pending:                  cti.Pending,
		Metadata:                 cti.Metadata,
		Send:                     cti.Send,
		Distribute:               cti.Distribute,
	}

	return dsl
}
