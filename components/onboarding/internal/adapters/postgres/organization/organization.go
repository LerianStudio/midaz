// Package organization provides the repository implementation for organization entity persistence.
//
// This package implements the Repository pattern for the Organization entity, providing
// an abstraction layer between the business logic and PostgreSQL database. It follows
// hexagonal architecture principles by defining repository interfaces and their implementations.
//
// The package contains:
//   - Repository interface: Defines all organization data operations
//   - PostgreSQL implementation: Concrete implementation using pgx driver
//   - Model conversions: Transforms between domain models and database models
//   - Mock implementation: For testing purposes
//
// Key Responsibilities:
//   - CRUD operations for organizations
//   - Query operations with pagination and filtering
//   - Soft delete support
//   - Parent-child relationship management
//   - Database transaction handling
//   - Error conversion from database to domain errors
//
// The repository uses:
//   - pgx/v5: PostgreSQL driver for Go
//   - Squirrel: SQL query builder
//   - Database connection pooling
//   - Prepared statements for performance
//
// Thread Safety:
//   - Repository implementations are thread-safe
//   - Can be shared across goroutines
//   - Database connection pool handles concurrency
package organization

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// OrganizationPostgreSQLModel represents the organization entity in PostgreSQL database schema.
//
// This model maps the domain Organization entity to the database table structure,
// handling SQL-specific types like sql.NullTime for nullable timestamps.
//
// Field Mappings:
//   - ID: UUID stored as string
//   - ParentOrganizationID: Nullable foreign key to parent organization
//   - Status: Stored as separate code and description fields
//   - DeletedAt: sql.NullTime for soft delete support
//   - Address: Embedded struct (stored as JSONB in database)
//   - Metadata: Not stored in this table (stored separately in MongoDB)
//
// The model provides conversion methods:
//   - ToEntity(): Converts database model to domain model
//   - FromEntity(): Converts domain model to database model
type OrganizationPostgreSQLModel struct {
	ID                   string         // UUID primary key
	ParentOrganizationID *string        // Nullable foreign key to parent organization
	LegalName            string         // Official legal name (required)
	DoingBusinessAs      *string        // Trading name (optional)
	LegalDocument        string         // Tax ID or registration number (required, immutable)
	Address              mmodel.Address // Physical address (stored as JSONB)
	Status               string         // Status code (e.g., "ACTIVE", "INACTIVE")
	StatusDescription    *string        // Optional status description
	CreatedAt            time.Time      // Creation timestamp
	UpdatedAt            time.Time      // Last update timestamp
	DeletedAt            sql.NullTime   // Soft delete timestamp (NULL if not deleted)
	Metadata             map[string]any // Not persisted here (stored in MongoDB)
}

// ToEntity converts a PostgreSQL model to a domain Organization entity.
//
// This method transforms the database representation into the domain model used by
// the business logic layer. It handles:
//   - Status reconstruction from separate code and description fields
//   - DeletedAt conversion from sql.NullTime to *time.Time
//   - Metadata field (left nil, populated separately from MongoDB)
//
// Returns:
//   - *mmodel.Organization: Domain model ready for business logic use
//
// Example:
//
//	dbModel := &OrganizationPostgreSQLModel{...}
//	domainModel := dbModel.ToEntity()
//	// domainModel can now be used in service layer
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

// FromEntity converts a domain Organization entity to a PostgreSQL model.
//
// This method transforms the domain model into the database representation for persistence.
// It handles:
//   - UUID generation (UUIDv7 for time-ordered IDs)
//   - Status decomposition into code and description fields
//   - DeletedAt conversion from *time.Time to sql.NullTime
//   - Metadata field (ignored, stored separately in MongoDB)
//
// Parameters:
//   - organization: Domain model to convert
//
// Side Effects:
//   - Modifies the receiver (*t) in place
//   - Generates new UUIDv7 for ID field
//
// Example:
//
//	dbModel := &OrganizationPostgreSQLModel{}
//	dbModel.FromEntity(domainOrganization)
//	// dbModel is now ready for database insertion
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
