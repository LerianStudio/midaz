// Package accounttype provides PostgreSQL data models for account type classification.
//
// This package implements the infrastructure layer for account type storage in PostgreSQL,
// following the hexagonal architecture pattern. Account types define the classification
// of accounts within a ledger (e.g., deposit, credit, liability, equity).
//
// Domain Concept:
//
// An AccountType in the ledger system:
//   - Defines a category for accounts (deposit, credit, liability, etc.)
//   - Belongs to a ledger within an organization
//   - Has a unique key_value for programmatic reference
//   - Enables account classification for reporting and routing
//   - Used by operation routes to validate account eligibility
//
// Classification Purpose:
//
// Account types enable:
//   - Balance sheet categorization (assets, liabilities, equity)
//   - Transaction routing rules (only credit accounts can be credited)
//   - Regulatory reporting (segregated by account type)
//   - Business analytics (balance by account type)
//
// Data Flow:
//
//	Domain Entity (mmodel.AccountType) -> AccountTypePostgreSQLModel -> PostgreSQL
//	PostgreSQL -> AccountTypePostgreSQLModel -> Domain Entity (mmodel.AccountType)
//
// Related Packages:
//   - mmodel: Domain model definitions
//   - operationroute: Uses account types for routing validation
//   - account: References account type for classification
package accounttype

import (
	"database/sql"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// AccountTypePostgreSQLModel represents the account type entity in PostgreSQL.
//
// This model maps directly to the 'account_type' table with SQL-specific types.
// It stores account classification metadata that enables routing and reporting.
//
// Table Schema:
//
//	CREATE TABLE account_type (
//	    id UUID PRIMARY KEY,
//	    organization_id UUID NOT NULL,
//	    ledger_id UUID NOT NULL,
//	    name VARCHAR(255) NOT NULL,
//	    description TEXT,
//	    key_value VARCHAR(100) NOT NULL,  -- Unique per ledger, lowercase
//	    created_at TIMESTAMP WITH TIME ZONE,
//	    updated_at TIMESTAMP WITH TIME ZONE,
//	    deleted_at TIMESTAMP WITH TIME ZONE,
//	    UNIQUE(organization_id, ledger_id, key_value)
//	);
//
// Key Value:
//
// The KeyValue field provides a programmatic identifier for the account type:
//   - Always stored in lowercase for consistent matching
//   - Used by operation routes for account validation
//   - Examples: "deposit", "credit", "liability", "equity"
//
// Thread Safety:
//
// AccountTypePostgreSQLModel is not thread-safe. Each goroutine should work
// with its own instance.
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

// ToEntity converts an AccountTypePostgreSQLModel to the domain model.
//
// This method implements the outbound mapping in hexagonal architecture,
// transforming the persistence model back to the domain representation.
//
// Mapping Process:
//  1. Map all direct fields (ID, name, description, key_value)
//  2. Handle nullable DeletedAt for soft delete support
//
// Returns:
//   - *mmodel.AccountType: Domain model with all fields mapped
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

// FromEntity converts a domain model to AccountTypePostgreSQLModel.
//
// This method implements the inbound mapping in hexagonal architecture,
// transforming the domain representation to the persistence model.
//
// Mapping Process:
//  1. Map all direct fields with type conversions
//  2. Normalize KeyValue to lowercase for consistent matching
//  3. Convert nullable DeletedAt to sql.NullTime
//
// KeyValue Normalization:
//
// The KeyValue is always converted to lowercase before storage to ensure
// case-insensitive matching in operation route validation.
//
// Parameters:
//   - accountType: Domain AccountType model to convert
func (m *AccountTypePostgreSQLModel) FromEntity(accountType *mmodel.AccountType) {
	m.ID = accountType.ID
	m.OrganizationID = accountType.OrganizationID
	m.LedgerID = accountType.LedgerID
	m.Name = accountType.Name
	m.Description = accountType.Description
	m.KeyValue = strings.ToLower(accountType.KeyValue)
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
