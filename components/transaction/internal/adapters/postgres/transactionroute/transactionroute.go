// Package transactionroute provides PostgreSQL data models for transaction routing configuration.
//
// This package implements the infrastructure layer for transaction route storage in PostgreSQL,
// following the hexagonal architecture pattern. Transaction routes define the complete routing
// rules for a transaction type, including which operation routes apply.
//
// Domain Concept:
//
// A TransactionRoute in the ledger system:
//   - Defines a reusable routing configuration for transactions
//   - Groups related operation routes (debit and credit rules)
//   - Enables validation of transaction structure against business rules
//   - Supports ledger-scoped routing configurations
//
// Routing Hierarchy:
//
//	TransactionRoute
//	    └── OperationRoute (debit rule)
//	    └── OperationRoute (credit rule)
//	    └── ... (additional rules)
//
// Use Cases:
//
// Transaction routes enable:
//   - Standardized transaction types (PAYMENT, TRANSFER, REFUND)
//   - Account type enforcement per transaction type
//   - Business rule validation before execution
//   - Reusable routing across similar transactions
//
// Data Flow:
//
//	Domain Entity (mmodel.TransactionRoute) -> TransactionRoutePostgreSQLModel -> PostgreSQL
//	PostgreSQL -> TransactionRoutePostgreSQLModel -> Domain Entity (mmodel.TransactionRoute)
//
// Related Packages:
//   - operationroute: Child routing rules for individual operations
//   - mmodel: Domain model definitions
package transactionroute

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// TransactionRoutePostgreSQLModel represents the transaction route entity in PostgreSQL.
//
// This model maps directly to the 'transaction_route' table with SQL-specific types.
// It stores routing configuration that defines valid transaction patterns for a ledger.
//
// Table Schema:
//
//	CREATE TABLE transaction_route (
//	    id UUID PRIMARY KEY,
//	    organization_id UUID NOT NULL,
//	    ledger_id UUID NOT NULL,
//	    title VARCHAR(255) NOT NULL,
//	    description TEXT,
//	    created_at TIMESTAMP WITH TIME ZONE,
//	    updated_at TIMESTAMP WITH TIME ZONE,
//	    deleted_at TIMESTAMP WITH TIME ZONE
//	);
//
// Relationship:
//
// TransactionRoute has a one-to-many relationship with OperationRoute via
// a join table (transaction_route_operation_route). This enables:
//   - Multiple operation rules per transaction route
//   - Shared operation rules across transaction routes
//   - Flexible rule composition
//
// Thread Safety:
//
// TransactionRoutePostgreSQLModel is not thread-safe. Each goroutine should work
// with its own instance.
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

// ToEntity converts a TransactionRoutePostgreSQLModel to the domain model.
//
// This method implements the outbound mapping in hexagonal architecture,
// transforming the persistence model back to the domain representation.
//
// Mapping Process:
//  1. Map all direct fields (ID, title, description, timestamps)
//  2. Handle nullable DeletedAt for soft delete support
//
// Note: Associated OperationRoutes are loaded separately via the repository
// and attached to the domain model after this conversion.
//
// Returns:
//   - *mmodel.TransactionRoute: Domain model with all fields mapped
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

// FromEntity converts a domain model to TransactionRoutePostgreSQLModel.
//
// This method implements the inbound mapping in hexagonal architecture,
// transforming the domain representation to the persistence model.
//
// Mapping Process:
//  1. Map all direct fields with type conversions
//  2. Convert nullable DeletedAt to sql.NullTime
//
// Note: Associated OperationRoutes are persisted separately via the repository
// after this conversion.
//
// Parameters:
//   - transactionRoute: Domain TransactionRoute model to convert
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
