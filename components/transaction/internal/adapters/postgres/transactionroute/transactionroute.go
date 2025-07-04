package transactionroute

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// TransactionRoutePostgreSQLModel represents the database model for transaction routes
type TransactionRoutePostgreSQLModel struct {
	ID             uuid.UUID    `db:"id"`
	OrganizationID uuid.UUID    `db:"organization_id"`
	LedgerID       uuid.UUID    `db:"ledger_id"`
	Title          string       `db:"title"`
	Description    string       `db:"description"`
	CreatedAt      time.Time    `db:"created_at"`
	UpdatedAt      time.Time    `db:"updated_at"`
	DeletedAt      sql.NullTime `db:"deleted_at"`
}

// ToEntity converts the database model to a domain model
func (m *TransactionRoutePostgreSQLModel) ToEntity() *mmodel.TransactionRoute {
	e := &mmodel.TransactionRoute{
		ID:             m.ID,
		OrganizationID: m.OrganizationID,
		LedgerID:       m.LedgerID,
		Title:          m.Title,
		Description:    m.Description,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}

	if m.DeletedAt.Valid {
		e.DeletedAt = &m.DeletedAt.Time
	}

	return e
}

// FromEntity converts a domain model to the database model
func (m *TransactionRoutePostgreSQLModel) FromEntity(transactionRoute *mmodel.TransactionRoute) {
	m.ID = transactionRoute.ID
	m.OrganizationID = transactionRoute.OrganizationID
	m.LedgerID = transactionRoute.LedgerID
	m.Title = transactionRoute.Title
	m.Description = transactionRoute.Description
	m.CreatedAt = transactionRoute.CreatedAt
	m.UpdatedAt = transactionRoute.UpdatedAt

	if transactionRoute.DeletedAt != nil {
		m.DeletedAt = sql.NullTime{
			Time:  *transactionRoute.DeletedAt,
			Valid: true,
		}
	} else {
		m.DeletedAt = sql.NullTime{}
	}
}
