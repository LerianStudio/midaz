// Package accounttype provides the repository implementation for account type entity persistence.
//
// This package implements the Repository pattern for the AccountType entity, providing
// PostgreSQL-based data access. Account types classify accounts for accounting validation
// and reporting purposes (e.g., asset, liability, equity, revenue, expense).
package accounttype

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// AccountTypePostgreSQLModel represents the PostgreSQL database model for account types.
//
// Account types provide classification for accounts, enabling accounting validation
// rules and structured financial reporting.
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

// ToEntity converts a PostgreSQL model to a domain AccountType entity.
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

// FromEntity converts a domain AccountType entity to a PostgreSQL model.
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
