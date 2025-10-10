// Package mmodel defines domain models for the Midaz platform.
// This file contains Segment-related models and input/output structures.
package mmodel

import "time"

// CreateSegmentInput represents the input data for creating a new segment.
//
// swagger:model CreateSegmentInput
//
// @Description CreateSegmentInput is the input payload to create a segment within a ledger, representing a logical division such as a business area, product line, or customer category.
type CreateSegmentInput struct {
	// The name of the segment.
	Name string `json:"name" validate:"required,max=256" example:"My Segment"`

	// The status of the segment (e.g., ACTIVE, INACTIVE, PENDING).
	Status Status `json:"status"`

	// Custom key-value pairs for extending the segment information.
	// Note: Nested structures are not supported.
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateSegmentInput

// UpdateSegmentInput represents the input data for updating an existing segment.
//
// swagger:model UpdateSegmentInput
//
// @Description UpdateSegmentInput is the input payload to update an existing segment's properties such as name, status, and metadata.
type UpdateSegmentInput struct {
	// The updated name of the segment.
	Name string `json:"name" validate:"max=256" example:"My Segment Updated"`

	// The updated status of the segment.
	Status Status `json:"status"`

	// The updated custom key-value pairs for extending the segment information.
	// Note: Nested structures are not supported.
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateSegmentInput

// Segment represents a logical division within a ledger.
//
// swagger:model Segment
//
// @Description Segment represents a logical division within a ledger such as a business area, product line, or customer category.
type Segment struct {
	// The unique identifier for the segment (UUID format).
	ID string `json:"id" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" format:"uuid"`

	// The name of the segment.
	Name string `json:"name" example:"My Segment" maxLength:"256"`

	// The ID of the ledger this segment belongs to (UUID format).
	LedgerID string `json:"ledgerId" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" format:"uuid"`

	// The ID of the organization that owns this segment (UUID format).
	OrganizationID string `json:"organizationId" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" format:"uuid"`

	// The status of the segment.
	Status Status `json:"status"`

	// The timestamp when the segment was created.
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// The timestamp when the segment was last updated.
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// The timestamp when the segment was soft-deleted.
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the segment information.
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name Segment

// Segments represents a paginated list of segments.
//
// swagger:model Segments
//
// @Description Segments represents a paginated collection of segment records returned by list operations.
type Segments struct {
	// An array of segment records.
	// example: [{"id":"01965ed9-7fa4-75b2-8872-fc9e8509ab0a","name":"My Segment","ledgerId":"01965ed9-7fa4-75b2-8872-fc9e8509ab0a","status":{"code":"ACTIVE"}}]
	Items []Segment `json:"items"`

	// The current page number.
	// example: 1
	// minimum: 1
	Page int `json:"page" example:"1" minimum:"1"`

	// The maximum number of items per page.
	// example: 10
	// minimum: 1
	// maximum: 100
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`
} // @name Segments

// SegmentResponse represents a success response containing a single segment.
//
// swagger:response SegmentResponse
// @Description Successful response containing a single segment entity.
type SegmentResponse struct {
	// in: body
	Body Segment
}

// SegmentsResponse represents a success response containing a paginated list of segments.
//
// swagger:response SegmentsResponse
// @Description Successful response containing a paginated list of segments.
type SegmentsResponse struct {
	// in: body
	Body Segments
}

// SegmentErrorResponse represents an error response for segment operations.
//
// swagger:response SegmentErrorResponse
// @Description Error response for segment operations with error code and message.
type SegmentErrorResponse struct {
	// in: body
	Body struct {
		// The error code identifying the specific error.
		// example: 400001
		Code int `json:"code"`

		// A human-readable error message.
		// example: Invalid input: field 'name' is required
		Message string `json:"message"`

		// Additional error details if available.
		// example: {"field": "name", "violation": "required"}
		Details map[string]any `json:"details,omitempty"`
	}
}
