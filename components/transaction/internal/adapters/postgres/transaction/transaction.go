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
type TransactionPostgreSQLModel struct {
	ID                       string
	ParentTransactionID      *string
	Description              string
	Template                 string
	Status                   string
	StatusDescription        *string
	Amount                   *int64
	AmountScale              *int64
	AssetCode                string
	ChartOfAccountsGroupName string
	LedgerID                 string
	OrganizationID           string
	Body                     libTransaction.Transaction
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
	ChartOfAccountsGroupName string               `json:"chartOfAccountsGroupName,omitempty" validate:"max=256"`
	Description              string               `json:"description,omitempty" validate:"max=256"`
	Code                     string               `json:"code,omitempty" validate:"max=100"`
	Pending                  bool                 `json:"pending,omitempty"`
	Metadata                 map[string]any       `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
	Send                     *libTransaction.Send `json:"send,omitempty" validate:"required,dive"`
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
	Metadata    map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateTransactionInput

// Transaction is a struct designed to encapsulate response payload data.
//
// swagger:model Transaction
// @Description Transaction is a struct designed to store transaction data.
type Transaction struct {
	ID                       string                     `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	ParentTransactionID      *string                    `json:"parentTransactionId,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	Description              string                     `json:"description" example:"Transaction description"`
	Template                 string                     `json:"template" example:"Transaction template"`
	Status                   Status                     `json:"status"`
	Amount                   *int64                     `json:"amount" example:"1500"`
	AmountScale              *int64                     `json:"amountScale" example:"2"`
	AssetCode                string                     `json:"assetCode" example:"BRL"`
	ChartOfAccountsGroupName string                     `json:"chartOfAccountsGroupName" example:"Chart of accounts group name"`
	Source                   []string                   `json:"source" example:"@person1"`
	Destination              []string                   `json:"destination" example:"@person2"`
	LedgerID                 string                     `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID           string                     `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	Body                     libTransaction.Transaction `json:"-"`
	CreatedAt                time.Time                  `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt                time.Time                  `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt                *time.Time                 `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata                 map[string]any             `json:"metadata,omitempty"`
	Operations               []*operation.Operation     `json:"operations"`
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
type TransactionQueue struct {
	Validate    *libTransaction.Responses
	Balances    []*mmodel.Balance
	Transaction *Transaction
	ParseDSL    *libTransaction.Transaction
}
