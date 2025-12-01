// Package segment provides PostgreSQL adapter implementations for segment entity persistence.
//
// This package implements the infrastructure layer for segment storage in PostgreSQL,
// following the hexagonal architecture pattern. Segments provide logical categorization
// of accounts within a ledger (e.g., business lines, departments, cost centers).
//
// Architecture Overview:
//
// The segment adapter provides:
//   - Full CRUD operations for segment entities
//   - Organization and ledger scoped queries
//   - Name uniqueness validation within ledger scope
//   - Soft delete support with audit timestamps
//   - Batch operations for efficient bulk lookups
//
// Domain Concepts:
//
// A Segment in the ledger system:
//   - Represents a logical grouping or category of accounts
//   - Belongs to a ledger within an organization
//   - Enables account classification for reporting
//   - Supports business line or departmental organization
//   - Has unique names within a ledger
//
// Data Flow:
//
//	Domain Entity (mmodel.Segment) → SegmentPostgreSQLModel → PostgreSQL
//	PostgreSQL → SegmentPostgreSQLModel → Domain Entity (mmodel.Segment)
//
// Related Packages:
//   - github.com/LerianStudio/midaz/v3/pkg/mmodel: Domain model definitions
//   - github.com/LerianStudio/lib-commons/v2/commons/postgres: PostgreSQL connection management
package segment

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// SegmentPostgreSQLModel represents the segment entity in PostgreSQL.
//
// This model maps directly to the 'segment' table with proper SQL types.
// It serves as the persistence layer representation, separate from the
// domain model to maintain hexagonal architecture boundaries.
//
// Table Schema:
//
//	CREATE TABLE segment (
//	    id UUID PRIMARY KEY,
//	    name VARCHAR(255) NOT NULL,
//	    ledger_id UUID NOT NULL REFERENCES ledger(id),
//	    organization_id UUID NOT NULL REFERENCES organization(id),
//	    status VARCHAR(50) NOT NULL,
//	    status_description TEXT,
//	    created_at TIMESTAMP WITH TIME ZONE,
//	    updated_at TIMESTAMP WITH TIME ZONE,
//	    deleted_at TIMESTAMP WITH TIME ZONE,
//	    UNIQUE(organization_id, ledger_id, name)
//	);
//
// Thread Safety:
//
// SegmentPostgreSQLModel is not thread-safe. Each goroutine should work with
// its own instance.
type SegmentPostgreSQLModel struct {
	ID                string
	Name              string
	LedgerID          string
	OrganizationID    string
	Status            string
	StatusDescription *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         sql.NullTime
	Metadata          map[string]any
}

// ToEntity converts a SegmentPostgreSQLModel to the domain Segment model.
//
// This method implements the outbound mapping in hexagonal architecture.
//
// Returns:
//   - *mmodel.Segment: Domain model with all fields mapped
func (t *SegmentPostgreSQLModel) ToEntity() *mmodel.Segment {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	segment := &mmodel.Segment{
		ID:             t.ID,
		Name:           t.Name,
		LedgerID:       t.LedgerID,
		OrganizationID: t.OrganizationID,
		Status:         status,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		DeletedAt:      nil,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		segment.DeletedAt = &deletedAtCopy
	}

	return segment
}

// FromEntity converts a domain Segment model to SegmentPostgreSQLModel.
//
// This method implements the inbound mapping in hexagonal architecture.
//
// Parameters:
//   - segment: Domain Segment model to convert
func (t *SegmentPostgreSQLModel) FromEntity(segment *mmodel.Segment) {
	*t = SegmentPostgreSQLModel{
		ID:                libCommons.GenerateUUIDv7().String(),
		Name:              segment.Name,
		LedgerID:          segment.LedgerID,
		OrganizationID:    segment.OrganizationID,
		Status:            segment.Status.Code,
		StatusDescription: segment.Status.Description,
		CreatedAt:         segment.CreatedAt,
		UpdatedAt:         segment.UpdatedAt,
	}

	if segment.DeletedAt != nil {
		deletedAtCopy := *segment.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
