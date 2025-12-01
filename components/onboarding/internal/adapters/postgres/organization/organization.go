// Package organization provides PostgreSQL adapter implementations for organization entity persistence.
//
// This package implements the infrastructure layer for organization storage in PostgreSQL,
// following the hexagonal architecture pattern. Organizations are the top-level tenant
// entities in the ledger system, representing legal entities that operate ledgers.
//
// Architecture Overview:
//
// The organization adapter provides:
//   - Full CRUD operations for organization entities
//   - Hierarchical organization support (parent-child relationships)
//   - Soft delete support with audit timestamps
//   - Address storage as JSONB for flexible structure
//   - Legal document storage for compliance
//
// Domain Concepts:
//
// An Organization in the ledger system:
//   - Represents a legal entity (company, institution)
//   - Acts as the root tenant for multi-tenancy
//   - Contains ledgers which contain accounts
//   - Has legal identity (name, document, address)
//   - Can have parent organizations for group structures
//
// Data Flow:
//
//	Domain Entity (mmodel.Organization) → OrganizationPostgreSQLModel → PostgreSQL
//	PostgreSQL → OrganizationPostgreSQLModel → Domain Entity (mmodel.Organization)
//
// Related Packages:
//   - github.com/LerianStudio/midaz/v3/pkg/mmodel: Domain model definitions
//   - github.com/LerianStudio/lib-commons/v2/commons/postgres: PostgreSQL connection management
package organization

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// OrganizationPostgreSQLModel represents the organization entity in PostgreSQL.
//
// This model maps directly to the 'organization' table with proper SQL types.
// It serves as the persistence layer representation, separate from the
// domain model to maintain hexagonal architecture boundaries.
//
// Table Schema:
//
//	CREATE TABLE organization (
//	    id UUID PRIMARY KEY,
//	    parent_organization_id UUID REFERENCES organization(id),
//	    legal_name VARCHAR(255) NOT NULL,
//	    doing_business_as VARCHAR(255),
//	    legal_document VARCHAR(100) NOT NULL,
//	    address JSONB,
//	    status VARCHAR(50) NOT NULL,
//	    status_description TEXT,
//	    created_at TIMESTAMP WITH TIME ZONE,
//	    updated_at TIMESTAMP WITH TIME ZONE,
//	    deleted_at TIMESTAMP WITH TIME ZONE
//	);
//
// Address Storage:
//
// The Address field is stored as JSONB in PostgreSQL, allowing:
//   - Flexible address formats for different countries
//   - No schema migration for address field changes
//   - Rich querying capabilities on address components
//
// Thread Safety:
//
// OrganizationPostgreSQLModel is not thread-safe. Each goroutine should work with
// its own instance. The repository handles concurrent access at the database level.
type OrganizationPostgreSQLModel struct {
	ID                   string
	ParentOrganizationID *string
	LegalName            string
	DoingBusinessAs      *string
	LegalDocument        string
	Address              mmodel.Address
	Status               string
	StatusDescription    *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            sql.NullTime
	Metadata             map[string]any
}

// ToEntity converts an OrganizationPostgreSQLModel to the domain Organization model.
//
// This method implements the outbound mapping in hexagonal architecture,
// transforming the persistence model back to the domain representation.
//
// Mapping Process:
//  1. Convert status fields to Status value object
//  2. Map all direct fields including Address
//  3. Handle nullable DeletedAt for soft delete support
//
// Returns:
//   - *mmodel.Organization: Domain model with all fields mapped
func (t *OrganizationPostgreSQLModel) ToEntity() *mmodel.Organization {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	organization := &mmodel.Organization{
		ID:                   t.ID,
		ParentOrganizationID: t.ParentOrganizationID,
		LegalName:            t.LegalName,
		DoingBusinessAs:      t.DoingBusinessAs,
		LegalDocument:        t.LegalDocument,
		Address:              t.Address,
		Status:               status,
		CreatedAt:            t.CreatedAt,
		UpdatedAt:            t.UpdatedAt,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		organization.DeletedAt = &deletedAtCopy
	}

	return organization
}

// FromEntity converts a domain Organization model to OrganizationPostgreSQLModel.
//
// This method implements the inbound mapping in hexagonal architecture,
// transforming the domain representation to the persistence model.
//
// Mapping Process:
//  1. Generate UUID v7 for new organizations
//  2. Map all direct fields with type conversions
//  3. Handle optional fields (DoingBusinessAs, ParentOrganizationID)
//  4. Convert Status value object to separate fields
//  5. Handle nullable DeletedAt
//
// Parameters:
//   - organization: Domain Organization model to convert
//
// ID Generation:
//
// Uses UUID v7 which provides:
//   - Time-ordered IDs for index efficiency
//   - Globally unique identifiers
//   - Sortable by creation time
func (t *OrganizationPostgreSQLModel) FromEntity(organization *mmodel.Organization) {
	*t = OrganizationPostgreSQLModel{
		ID:                   libCommons.GenerateUUIDv7().String(),
		ParentOrganizationID: organization.ParentOrganizationID,
		LegalName:            organization.LegalName,
		DoingBusinessAs:      organization.DoingBusinessAs,
		LegalDocument:        organization.LegalDocument,
		Address:              organization.Address,
		Status:               organization.Status.Code,
		StatusDescription:    organization.Status.Description,
		CreatedAt:            organization.CreatedAt,
		UpdatedAt:            organization.UpdatedAt,
	}

	if organization.DeletedAt != nil {
		deletedAtCopy := *organization.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
