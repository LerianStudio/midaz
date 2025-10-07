// Package segment provides the repository implementation for segment entity persistence.
//
// This package implements the Repository pattern for the Segment entity, providing
// PostgreSQL-based data access. Segments provide logical divisions within a ledger
// for organizational and reporting purposes.
package segment

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// SegmentPostgreSQLModel represents the PostgreSQL database model for segments.
//
// Segments provide logical divisions within a ledger (e.g., by region, business unit,
// or cost center) for organizational and reporting purposes.
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

// ToEntity converts a PostgreSQL model to a domain Segment entity.
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

// FromEntity converts a domain Segment entity to a PostgreSQL model.
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
