package accounttype

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// AccountTypePostgreSQLModel represents the database model for account types
type AccountTypePostgreSQLModel struct {
	ID             uuid.UUID    `db:"id"`
	OrganizationID uuid.UUID    `db:"organization_id"`
	LedgerID       uuid.UUID    `db:"ledger_id"`
	Name           string       `db:"name"`
	Description    string       `db:"description"`
	KeyValue       string       `db:"key_value"`
	CreatedAt      time.Time    `db:"created_at"`
	UpdatedAt      time.Time    `db:"updated_at"`
	DeletedAt      sql.NullTime `db:"deleted_at"`
}

// ToEntity converts the database model to a domain model
func (m *AccountTypePostgreSQLModel) ToEntity() *mmodel.AccountType {
	e := &mmodel.AccountType{
		ID:             m.ID,
		OrganizationID: m.OrganizationID,
		LedgerID:       m.LedgerID,
		Name:           m.Name,
		Description:    m.Description,
		KeyValue:       m.KeyValue,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}

	if m.DeletedAt.Valid {
		e.DeletedAt = &m.DeletedAt.Time
	}

	return e
}

// FromEntity converts a domain model to the database model
func (m *AccountTypePostgreSQLModel) FromEntity(accountType *mmodel.AccountType) {
	m.ID = accountType.ID
	m.OrganizationID = accountType.OrganizationID
	m.LedgerID = accountType.LedgerID
	m.Name = accountType.Name
	m.Description = accountType.Description
	m.KeyValue = accountType.KeyValue
	m.CreatedAt = accountType.CreatedAt
	m.UpdatedAt = accountType.UpdatedAt

	if accountType.DeletedAt != nil {
		m.DeletedAt = sql.NullTime{
			Time:  *accountType.DeletedAt,
			Valid: true,
		}
	} else {
		m.DeletedAt = sql.NullTime{}
	}
}
