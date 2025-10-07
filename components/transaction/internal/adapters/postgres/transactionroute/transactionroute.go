// Package transactionroute provides the repository implementation for transaction route entity persistence.
//
// This package implements the Repository pattern for the TransactionRoute entity, providing
// PostgreSQL-based data access. Transaction routes define how transactions flow through
// the system by combining multiple operation routes (source and destination rules).
package transactionroute

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// TransactionRoutePostgreSQLModel represents the PostgreSQL database model for transaction routes.
//
// This model stores transaction routing rules with:
//   - Title and description
//   - Associated operation routes (many-to-many relationship)
//   - Soft delete support
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

// ToEntity converts a PostgreSQL model to a domain TransactionRoute entity.
//
// Transforms database representation to business logic representation,
// handling DeletedAt conversion.
//
// Returns:
//   - *mmodel.TransactionRoute: Domain model with all fields populated
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

// FromEntity converts a domain TransactionRoute entity to a PostgreSQL model.
//
// Transforms business logic representation to database representation,
// handling DeletedAt conversion.
//
// Parameters:
//   - transactionRoute: Domain model to convert
//
// Side Effects:
//   - Modifies the receiver (*m) in place
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
