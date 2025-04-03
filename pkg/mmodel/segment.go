package mmodel

import "time"

// CreateSegmentInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateSegmentInput
//
// @Description CreateSegmentInput is the input payload to create a segment within a ledger, representing a logical division such as a business area, product line, or customer category.
type CreateSegmentInput struct {
	// Name of the segment (required, max length 256 characters)
	Name string `json:"name" validate:"required,max=256" example:"My Segment"`

	// Status of the segment (active, inactive, pending)
	Status Status `json:"status"`

	// Additional custom attributes for the segment
	// Keys max length: 100 characters, Values max length: 2000 characters
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateSegmentInput

// UpdateSegmentInput is a struct design to encapsulate request update payload data.
//
// swagger:model UpdateSegmentInput
//
// @Description UpdateSegmentInput is the input payload to update an existing segment's properties such as name, status, and metadata.
type UpdateSegmentInput struct {
	// Updated name of the segment (optional, max length 256 characters)
	Name string `json:"name" validate:"max=256" example:"My Segment Updated"`

	// Updated status of the segment (active, inactive, pending)
	Status Status `json:"status"`

	// Updated or additional custom attributes for the segment
	// Keys max length: 100 characters, Values max length: 2000 characters
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateSegmentInput

// Segment is a struct designed to encapsulate payload data.
//
// swagger:model Segment
//
// @Description Segment represents a logical division within a ledger such as a business area, product line, or customer category.
type Segment struct {
	// Unique identifier for the segment (UUID format)
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Name of the segment (max length 256 characters)
	Name string `json:"name" example:"My Segment" maxLength:"256"`

	// ID of the ledger this segment belongs to (UUID format)
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// ID of the organization that owns this segment (UUID format)
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Status of the segment (active, inactive, pending)
	Status Status `json:"status"`

	// Timestamp when the segment was created
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the segment was last updated
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the segment was deleted (null if not deleted)
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Additional custom attributes for the segment
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name Segment

// Segments struct to return get all.
//
// swagger:model Segments
//
// @Description Segments represents a paginated collection of segment records returned by list operations.
type Segments struct {
	// Array of segment records
	Items []Segment `json:"items"`

	// Current page number
	Page int `json:"page" example:"1" minimum:"1"`

	// Maximum number of items per page
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`
} // @name Segments
