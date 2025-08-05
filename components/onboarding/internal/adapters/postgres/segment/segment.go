package segment

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// SegmentPostgreSQLModel represents the entity Segment into SQL context in Database
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

// ToEntity converts an SegmentPostgreSQLModel to entity.Segment
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

// FromEntity converts an entity.Segment to SegmentPostgreSQLModel
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
