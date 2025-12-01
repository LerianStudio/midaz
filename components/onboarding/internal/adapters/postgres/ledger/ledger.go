// Package ledger provides PostgreSQL adapter implementations for ledger entity persistence.
//
// This package implements the infrastructure layer for ledger storage in PostgreSQL,
// following the hexagonal architecture pattern. Ledgers are the second-level entities
// in the hierarchy, belonging to organizations and containing accounts.
//
// Architecture Overview:
//
// The ledger adapter provides:
//   - Full CRUD operations for ledger entities
//   - Organization-scoped queries (multi-tenant isolation)
//   - Name uniqueness validation within organization
//   - Soft delete support with audit timestamps
//   - Batch operations for efficient bulk lookups
//
// Domain Concepts:
//
// A Ledger in the system:
//   - Represents a logical grouping of accounts
//   - Belongs to exactly one organization
//   - Contains accounts that hold balances
//   - Enforces double-entry accounting rules
//   - Has unique names within an organization
//
// Data Flow:
//
//	Domain Entity (mmodel.Ledger) → LedgerPostgreSQLModel → PostgreSQL
//	PostgreSQL → LedgerPostgreSQLModel → Domain Entity (mmodel.Ledger)
//
// Related Packages:
//   - github.com/LerianStudio/midaz/v3/pkg/mmodel: Domain model definitions
//   - github.com/LerianStudio/lib-commons/v2/commons/postgres: PostgreSQL connection management
package ledger

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// LedgerPostgreSQLModel represents the ledger entity in PostgreSQL.
//
// This model maps directly to the 'ledger' table with proper SQL types.
// It serves as the persistence layer representation, separate from the
// domain model to maintain hexagonal architecture boundaries.
//
// Table Schema:
//
//	CREATE TABLE ledger (
//	    id UUID PRIMARY KEY,
//	    name VARCHAR(255) NOT NULL,
//	    organization_id UUID NOT NULL REFERENCES organization(id),
//	    status VARCHAR(50) NOT NULL,
//	    status_description TEXT,
//	    created_at TIMESTAMP WITH TIME ZONE,
//	    updated_at TIMESTAMP WITH TIME ZONE,
//	    deleted_at TIMESTAMP WITH TIME ZONE,
//	    UNIQUE(organization_id, name)
//	);
//
// Indexing Strategy:
//
// Key indexes for performance:
//   - (organization_id, id): Primary lookup path
//   - (organization_id, name): Name uniqueness and lookups
//   - (deleted_at): Soft delete filtering
//
// Thread Safety:
//
// LedgerPostgreSQLModel is not thread-safe. Each goroutine should work with
// its own instance. The repository handles concurrent access at the database level.
type LedgerPostgreSQLModel struct {
	ID                string
	Name              string
	OrganizationID    string
	Status            string
	StatusDescription *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         sql.NullTime
	Metadata          map[string]any
}

// ToEntity converts a LedgerPostgreSQLModel to the domain Ledger model.
//
// This method implements the outbound mapping in hexagonal architecture,
// transforming the persistence model back to the domain representation.
//
// Mapping Process:
//  1. Convert status fields to Status value object
//  2. Map all direct fields
//  3. Handle nullable DeletedAt for soft delete support
//
// Returns:
//   - *mmodel.Ledger: Domain model with all fields mapped
func (t *LedgerPostgreSQLModel) ToEntity() *mmodel.Ledger {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	ledger := &mmodel.Ledger{
		ID:             t.ID,
		Name:           t.Name,
		OrganizationID: t.OrganizationID,
		Status:         status,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		DeletedAt:      nil,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		ledger.DeletedAt = &deletedAtCopy
	}

	return ledger
}

// FromEntity converts a domain Ledger model to LedgerPostgreSQLModel.
//
// This method implements the inbound mapping in hexagonal architecture,
// transforming the domain representation to the persistence model.
//
// Mapping Process:
//  1. Generate UUID v7 for new ledgers
//  2. Map all direct fields
//  3. Convert Status value object to separate fields
//  4. Handle nullable DeletedAt
//
// Parameters:
//   - ledger: Domain Ledger model to convert
func (t *LedgerPostgreSQLModel) FromEntity(ledger *mmodel.Ledger) {
	*t = LedgerPostgreSQLModel{
		ID:                libCommons.GenerateUUIDv7().String(),
		Name:              ledger.Name,
		OrganizationID:    ledger.OrganizationID,
		Status:            ledger.Status.Code,
		StatusDescription: ledger.Status.Description,
		CreatedAt:         ledger.CreatedAt,
		UpdatedAt:         ledger.UpdatedAt,
	}

	if ledger.DeletedAt != nil {
		deletedAtCopy := *ledger.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
