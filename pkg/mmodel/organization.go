// Package mmodel defines domain models for the Midaz platform.
// This file contains Organization-related models and input/output structures.
package mmodel

import "time"

// CreateOrganizationInput represents the input data for creating a new organization.
//
// swagger:model CreateOrganizationInput
// @Description Request payload for creating a new organization. Contains all the necessary fields for organization creation, with required fields marked as such. Organizations are the top-level entities in the hierarchy and contain ledgers, which in turn contain accounts and assets.
//
//	@example {
//	  "legalName": "Lerian Financial Services Ltd.",
//	  "legalDocument": "123456789012345",
//	  "doingBusinessAs": "Lerian FS",
//	  "address": {
//	    "line1": "123 Financial Avenue",
//	    "line2": "Suite 1500",
//	    "zipCode": "10001",
//	    "city": "New York",
//	    "state": "NY",
//	    "country": "US"
//	  },
//	  "metadata": {
//	    "industry": "Financial Services",
//	    "founded": 2020,
//	    "employees": 150
//	  }
//	}
type CreateOrganizationInput struct {
	// The official legal name of the organization.
	// required: true
	// example: Lerian Financial Services Ltd.
	// maxLength: 256
	LegalName string `json:"legalName" validate:"required,max=256" example:"Lerian Financial Services Ltd." maxLength:"256"`

	// The UUID of the parent organization if this is a child organization.
	// required: false
	// format: uuid
	ParentOrganizationID *string `json:"parentOrganizationId" validate:"omitempty,uuid" format:"uuid"`

	// The trading or brand name of the organization, if different from the legal name.
	// required: false
	// example: Lerian FS
	// maxLength: 256
	DoingBusinessAs *string `json:"doingBusinessAs" validate:"omitempty,max=256" example:"Lerian FS" maxLength:"256"`

	// The official tax ID, company registration number, or other legal identification.
	// required: true
	// example: 123456789012345
	// maxLength: 256
	LegalDocument string `json:"legalDocument" validate:"required,max=256" example:"123456789012345" maxLength:"256"`

	// The physical address of the organization.
	// required: false
	Address Address `json:"address"`

	// The current operating status of the organization (defaults to ACTIVE if not specified).
	// required: false
	Status Status `json:"status"`

	// Custom key-value pairs for extending the organization information.
	// Note: Nested structures are not supported.
	// required: false
	// example: {"industry": "Financial Services", "founded": 2020, "employees": 150}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateOrganizationInput

// UpdateOrganizationInput represents the input data for updating an existing organization.
//
// swagger:model UpdateOrganizationInput
// @Description Request payload for updating an existing organization. All fields are optional - only specified fields will be updated. Omitted fields will remain unchanged.
//
//	@example {
//	  "legalName": "Lerian Financial Group Ltd.",
//	  "doingBusinessAs": "Lerian Group",
//	  "address": {
//	    "line1": "456 Corporate Plaza",
//	    "line2": "Floor 20",
//	    "zipCode": "10002",
//	    "city": "New York",
//	    "state": "NY",
//	    "country": "US"
//	  },
//	  "status": {
//	    "code": "ACTIVE"
//	  },
//	  "metadata": {
//	    "industry": "Financial Technology",
//	    "founded": 2020,
//	    "employees": 200,
//	    "headquarters": "New York"
//	  }
//	}
type UpdateOrganizationInput struct {
	// The updated legal name of the organization.
	// required: false
	// example: Lerian Financial Group Ltd.
	// maxLength: 256
	LegalName string `json:"legalName" validate:"max=256" example:"Lerian Financial Group Ltd." maxLength:"256"`

	// The updated UUID of the parent organization if this is a child organization.
	// required: false
	// format: uuid
	ParentOrganizationID *string `json:"parentOrganizationId" validate:"omitempty,uuid" format:"uuid"`

	// The updated trading or brand name of the organization.
	// required: false
	// example: Lerian Group
	// maxLength: 256
	DoingBusinessAs string `json:"doingBusinessAs" validate:"max=256" example:"Lerian Group" maxLength:"256"`

	// The updated physical address of the organization.
	// required: false
	Address Address `json:"address"`

	// The updated status of the organization.
	// required: false
	Status Status `json:"status"`

	// The updated custom key-value pairs for extending the organization information.
	// Note: Nested structures are not supported.
	// required: false
	// example: {"industry": "Financial Technology", "founded": 2020, "employees": 200, "headquarters": "New York"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateOrganizationInput

// Organization represents an organization in the system.
//
// swagger:model Organization
// @Description Complete organization entity containing all fields including system-generated fields like ID, creation timestamps, and metadata. This is the response format for organization operations. Organizations are the top-level entities in the Midaz platform hierarchy.
//
//	@example {
//	  "id": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
//	  "legalName": "Lerian Financial Services Ltd.",
//	  "doingBusinessAs": "Lerian FS",
//	  "legalDocument": "123456789012345",
//	  "address": {
//	    "line1": "123 Financial Avenue",
//	    "line2": "Suite 1500",
//	    "zipCode": "10001",
//	    "city": "New York",
//	    "state": "NY",
//	    "country": "US"
//	  },
//	  "status": {
//	    "code": "ACTIVE"
//	  },
//	  "createdAt": "2022-04-15T09:30:00Z",
//	  "updatedAt": "2022-04-15T09:30:00Z",
//	  "metadata": {
//	    "industry": "Financial Services",
//	    "founded": 2020,
//	    "employees": 150
//	  }
//	}
type Organization struct {
	// The unique identifier for the organization (UUID format).
	// example: 01965ed9-7fa4-75b2-8872-fc9e8509ab0a
	// format: uuid
	ID string `json:"id" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" format:"uuid"`

	// A reference to the parent organization, if this is a child organization.
	// example: 01965ed9-7fa4-75b2-8872-fc9e8509ab0a
	// format: uuid
	ParentOrganizationID *string `json:"parentOrganizationId" format:"uuid"`

	// The official legal name of the organization.
	// example: Lerian Financial Services Ltd.
	// maxLength: 256
	LegalName string `json:"legalName" example:"Lerian Financial Services Ltd." maxLength:"256"`

	// The trading or brand name of the organization, if different from the legal name.
	// example: Lerian FS
	// maxLength: 256
	DoingBusinessAs *string `json:"doingBusinessAs" example:"Lerian FS" maxLength:"256"`

	// The official tax ID, company registration number, or other legal identification.
	// example: 123456789012345
	// maxLength: 256
	LegalDocument string `json:"legalDocument" example:"123456789012345" maxLength:"256"`

	// The physical address of the organization.
	Address Address `json:"address"`

	// The current operating status of the organization.
	Status Status `json:"status"`

	// The timestamp when the organization was created (RFC3339 format).
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// The timestamp when the organization was last updated (RFC3339 format).
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// The timestamp when the organization was soft-deleted, null if not deleted (RFC3339 format).
	// example: null
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the organization information.
	// example: {"industry": "Financial Services", "founded": 2020, "employees": 150}
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name Organization

// Address represents a physical address.
//
// swagger:model Address
// @Description Structured address information following standard postal address format. Country field follows ISO 3166-1 alpha-2 standard (2-letter country codes). Used for organization physical locations and other address needs.
type Address struct {
	// The primary address line (street address or PO Box).
	// example: 123 Financial Avenue
	// maxLength: 256
	Line1 string `json:"line1" example:"123 Financial Avenue" maxLength:"256"`

	// Secondary address information like apartment number, suite, or floor.
	// example: Suite 1500
	// maxLength: 256
	Line2 *string `json:"line2" example:"Suite 1500" maxLength:"256"`

	// The postal code or ZIP code.
	// example: 10001
	// maxLength: 20
	ZipCode string `json:"zipCode" example:"10001" maxLength:"20"`

	// The city or locality name.
	// example: New York
	// maxLength: 100
	City string `json:"city" example:"New York" maxLength:"100"`

	// The state, province, or region name or code.
	// example: NY
	// maxLength: 100
	State string `json:"state" example:"NY" maxLength:"100"`

	// The country code in ISO 3166-1 alpha-2 format (two-letter country code).
	// example: US
	// minLength: 2
	// maxLength: 2
	Country string `json:"country" example:"US" minLength:"2" maxLength:"2"` // According to ISO 3166-1 alpha-2
} // @name Address

// IsEmpty determines if an Address has no data in any of its fields.
//
// This method checks whether all address fields are empty or nil, which is useful for
// validation logic to determine if an address was provided or should be considered missing.
//
// An address is considered empty if:
//   - Line1 is an empty string.
//   - Line2 is nil.
//   - ZipCode is an empty string.
//   - City is an empty string.
//   - State is an empty string.
//   - Country is an empty string.
//
// Returns:
//   - true if all fields are empty/nil, false if any field has a value.
//
// Example:
//
//	emptyAddr := Address{}
//	if emptyAddr.IsEmpty() {
//	    // Skip address validation
//	}
//
//	partialAddr := Address{City: "New York"}
//	if !partialAddr.IsEmpty() {
//	    // Validate address fields
//	}
func (a Address) IsEmpty() bool {
	return a.Line1 == "" && a.Line2 == nil && a.ZipCode == "" && a.City == "" && a.State == "" && a.Country == ""
}

// Organizations represents a paginated list of organizations.
//
// swagger:model Organizations
// @Description Paginated list of organizations with metadata about the current page, limit, and the organization items themselves. Used for list operations.
//
//	@example {
//	  "items": [
//	    {
//	      "id": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
//	      "legalName": "Lerian Financial Services Ltd.",
//	      "doingBusinessAs": "Lerian FS",
//	      "legalDocument": "123456789012345",
//	      "status": {
//	        "code": "ACTIVE"
//	      },
//	      "createdAt": "2022-04-15T09:30:00Z",
//	      "updatedAt": "2022-04-15T09:30:00Z"
//	    },
//	    {
//	      "id": "b2c3d4e5-f6a1-7890-bcde-2345678901cd",
//	      "legalName": "Global Finance Partners",
//	      "doingBusinessAs": "GFP",
//	      "legalDocument": "987654321012345",
//	      "status": {
//	        "code": "ACTIVE"
//	      },
//	      "createdAt": "2022-03-10T14:15:00Z",
//	      "updatedAt": "2022-03-10T14:15:00Z"
//	    }
//	  ],
//	  "page": 1,
//	  "limit": 10
//	}
type Organizations struct {
	// An array of organization records for the current page.
	// example: [{"id":"01965ed9-7fa4-75b2-8872-fc9e8509ab0a","legalName":"Lerian Financial Services Ltd.","status":{"code":"ACTIVE"}}]
	Items []Organization `json:"items"`

	// The current page number in the pagination.
	// example: 1
	// minimum: 1
	Page int `json:"page" example:"1" minimum:"1"`

	// The maximum number of items per page.
	// example: 10
	// minimum: 1
	// maximum: 100
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`
} // @name Organizations

// OrganizationResponse represents a success response containing a single organization.
//
// swagger:response OrganizationResponse
// @Description Successful response containing a single organization entity.
type OrganizationResponse struct {
	// in: body
	Body Organization
}

// OrganizationsResponse represents a success response containing a paginated list of organizations.
//
// swagger:response OrganizationsResponse
// @Description Successful response containing a paginated list of organizations.
type OrganizationsResponse struct {
	// in: body
	Body Organizations
}

// OrganizationErrorResponse represents an error response for organization operations.
//
// swagger:response OrganizationErrorResponse
// @Description Error response for organization operations with error code and message.
//
//	@example {
//	  "code": 400001,
//	  "message": "Invalid input: field 'legalName' is required",
//	  "details": {
//	    "field": "legalName",
//	    "violation": "required"
//	  }
//	}
type OrganizationErrorResponse struct {
	// in: body
	Body struct {
		// The error code identifying the specific error.
		// example: 400001
		Code int `json:"code"`

		// A human-readable error message.
		// example: Invalid input: field 'legalName' is required
		Message string `json:"message"`

		// Additional error details if available.
		// example: {"field": "legalName", "violation": "required"}
		Details map[string]any `json:"details,omitempty"`
	}
}
