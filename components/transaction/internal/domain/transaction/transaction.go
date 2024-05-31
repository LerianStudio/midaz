package transaction

import (
	"time"

	"database/sql"
	"github.com/google/uuid"
)

// TransactionPostgreSQLModel represents the entity TransactionPostgreSQLModel into SQL context in Database
type TransactionPostgreSQLModel struct {
	ID                       string
	ParentTransactionID      *string
	Description              string
	Template                 string
	Status                   string
	Amount                   *float64
	AmountScale              *float64
	InstrumentCode           string
	ChartOfAccountsGroupName string
	LedgerID                 string
	OrganizationID           string
	CreatedAt                time.Time
	UpdatedAt                time.Time
	DeletedAt                sql.NullTime
	Metadata                 map[string]any
}

// Transaction is a struct designed to encapsulate response payload data.
type Transaction struct {
	ID                       string         `json:"id"`
	ParentTransactionID      *string        `json:"parentTransactionId,omitempty"`
	Description              string         `json:"description"`
	Template                 string         `json:"template"`
	Status                   string         `json:"status"`
	Amount                   *float64       `json:"amount"`
	AmountScale              *float64       `json:"amountScale"`
	InstrumentCode           string         `json:"InstrumentCode"`
	ChartOfAccountsGroupName string         `json:"chartOfAccountsGroupName"`
	LedgerID                 string         `json:"ledgerId"`
	OrganizationID           string         `json:"organizationId"`
	CreatedAt                time.Time      `json:"createdAt"`
	UpdatedAt                time.Time      `json:"updatedAt"`
	DeletedAt                *time.Time     `json:"deletedAt"`
	Metadata                 map[string]any `json:"metadata,omitempty"`
}

// ToEntity converts an TransactionPostgreSQLModel to entity Transaction
func (t *TransactionPostgreSQLModel) ToEntity() *Transaction {
	transaction := &Transaction{
		ID:                       t.ID,
		ParentTransactionID:      t.ParentTransactionID,
		Description:              t.Description,
		Template:                 t.Template,
		Status:                   t.Status,
		Amount:                   t.Amount,
		AmountScale:              t.AmountScale,
		InstrumentCode:           t.InstrumentCode,
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
		ID:                       uuid.New().String(),
		ParentTransactionID:      transaction.ParentTransactionID,
		Description:              transaction.Description,
		Template:                 transaction.Template,
		Status:                   transaction.Status,
		Amount:                   transaction.Amount,
		AmountScale:              transaction.AmountScale,
		InstrumentCode:           transaction.InstrumentCode,
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
