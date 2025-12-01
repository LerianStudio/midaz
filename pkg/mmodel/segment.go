package mmodel

import "time"

// CreateSegmentInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateSegmentInput
// @Description Request payload for creating a new segment within a ledger. Segments represent logical divisions such as business areas, product lines, or customer categories for organizing accounts.
//
//	@example {
//	  "name": "Retail Banking",
//	  "status": {
//	    "code": "ACTIVE"
//	  },
//	  "metadata": {
//	    "businessUnit": "Consumer Banking",
//	    "region": "North America",
//	    "productLine": "Checking Accounts"
//	  }
//	}
type CreateSegmentInput struct {
	// Human-readable name of the segment
	// required: true
	// example: Retail Banking
	// maxLength: 256
	Name string `json:"name" validate:"required,max=256" example:"Retail Banking" maxLength:"256"`

	// Current operating status of the segment (defaults to ACTIVE if not specified)
	// required: false
	Status Status `json:"status"`

	// Custom key-value pairs for extending the segment information
	// required: false
	// example: {"businessUnit": "Consumer Banking", "region": "North America", "productLine": "Checking Accounts"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateSegmentInput

// UpdateSegmentInput is a struct design to encapsulate request update payload data.
//
// swagger:model UpdateSegmentInput
// @Description Request payload for updating an existing segment. All fields are optional - only specified fields will be updated. Omitted fields will remain unchanged.
//
//	@example {
//	  "name": "Global Retail Banking",
//	  "status": {
//	    "code": "ACTIVE"
//	  },
//	  "metadata": {
//	    "businessUnit": "Global Consumer Banking",
//	    "region": "Global",
//	    "productLine": "All Products"
//	  }
//	}
type UpdateSegmentInput struct {
	// Updated name of the segment
	// required: false
	// example: Global Retail Banking
	// maxLength: 256
	Name string `json:"name" validate:"max=256" example:"Global Retail Banking" maxLength:"256"`

	// Updated status of the segment
	// required: false
	Status Status `json:"status"`

	// Updated custom key-value pairs for extending the segment information
	// required: false
	// example: {"businessUnit": "Global Consumer Banking", "region": "Global", "productLine": "All Products"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateSegmentInput

// Segment is a struct designed to encapsulate payload data.
//
// swagger:model Segment
// @Description Complete segment entity containing all fields including system-generated fields like ID, creation timestamps, and metadata. This is the response format for segment operations. Segments represent logical divisions within a ledger for organizing accounts.
//
//	@example {
//	  "id": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
//	  "name": "Retail Banking",
//	  "ledgerId": "c3d4e5f6-a1b2-7890-cdef-3456789012de",
//	  "organizationId": "b2c3d4e5-f6a1-7890-bcde-2345678901cd",
//	  "status": {
//	    "code": "ACTIVE"
//	  },
//	  "createdAt": "2022-04-15T09:30:00Z",
//	  "updatedAt": "2022-04-15T09:30:00Z",
//	  "metadata": {
//	    "businessUnit": "Consumer Banking",
//	    "region": "North America"
//	  }
//	}
type Segment struct {
	// Unique identifier for the segment (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Human-readable name of the segment
	// example: Retail Banking
	// maxLength: 256
	Name string `json:"name" example:"Retail Banking" maxLength:"256"`

	// ID of the ledger this segment belongs to (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// ID of the organization that owns this segment (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Current operating status of the segment
	Status Status `json:"status"`

	// Timestamp when the segment was created (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the segment was last updated (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the segment was soft deleted, null if not deleted (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the segment information
	// example: {"businessUnit": "Consumer Banking", "region": "North America"}
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name Segment

// Segments struct to return get all.
//
// swagger:model Segments
//
// @Description Segments represents a paginated collection of segment records returned by list operations.
type Segments struct {
	// Array of segment records
	// example: [{"id":"00000000-0000-0000-0000-000000000000","name":"My Segment","ledgerId":"00000000-0000-0000-0000-000000000000","status":{"code":"ACTIVE"}}]
	Items []Segment `json:"items"`

	// Current page number
	// example: 1
	// minimum: 1
	Page int `json:"page" example:"1" minimum:"1"`

	// Maximum number of items per page
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
		// Error code identifying the specific error
		// example: 400001
		Code int `json:"code"`

		// Human-readable error message
		// example: Invalid input: field 'name' is required
		Message string `json:"message"`

		// Additional error details if available
		// example: {"field": "name", "violation": "required"}
		Details map[string]any `json:"details,omitempty"`
	}
}
