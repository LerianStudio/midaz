// Package repository defines repository interfaces for the MDZ CLI domain layer.
// This file contains the Segment repository interface.
package repository

import "github.com/LerianStudio/midaz/v3/pkg/mmodel"

// Segment defines the interface for segment data operations.
//
// This interface abstracts segment CRUD operations, allowing CLI commands
// to work with segments without knowing the underlying HTTP implementation.
type Segment interface {
	Create(organizationID, ledgerID string, inp mmodel.CreateSegmentInput) (*mmodel.Segment, error)
	Get(organizationID, ledgerID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Segments, error)
	GetByID(organizationID, ledgerID, segmentID string) (*mmodel.Segment, error)
	Update(organizationID, ledgerID, segmentID string, inp mmodel.UpdateSegmentInput) (*mmodel.Segment, error)
	Delete(organizationID, ledgerID, segmentID string) error
}
