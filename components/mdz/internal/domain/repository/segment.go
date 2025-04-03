package repository

import "github.com/LerianStudio/midaz/pkg/mmodel"

// \1 represents an entity
type Segment interface {
	Create(organizationID, ledgerID string, inp mmodel.CreateSegmentInput) (*mmodel.Segment, error)
	Get(organizationID, ledgerID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Segments, error)
	GetByID(organizationID, ledgerID, segmentID string) (*mmodel.Segment, error)
	Update(organizationID, ledgerID, segmentID string, inp mmodel.UpdateSegmentInput) (*mmodel.Segment, error)
	Delete(organizationID, ledgerID, segmentID string) error
}
