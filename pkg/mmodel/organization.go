package mmodel

import "time"

// CreateOrganizationInput is a struct design to encapsulate request create payload data.
//
// // swagger:model CreateOrganizationInput
// @Description Request payload for creating a new organization. Contains all the necessary fields for organization creation, with required fields marked as such.
type CreateOrganizationInput struct {
	// Official legal name of the organization (required)
	LegalName string `json:"legalName" validate:"required,max=256" example:"Lerian Studio" maxLength:"256"`

	// UUID of the parent organization if this is a child organization (optional)
	ParentOrganizationID *string `json:"parentOrganizationId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Trading or brand name of the organization, if different from legal name (optional)
	DoingBusinessAs *string `json:"doingBusinessAs" validate:"max=256" example:"Lerian Studio" maxLength:"256"`

	// Official tax ID, company registration number, or other legal identification (required)
	LegalDocument string `json:"legalDocument" validate:"required,max=256" example:"00000000000000" maxLength:"256"`

	// Physical address of the organization (optional)
	Address Address `json:"address"`

	// Current operating status of the organization (defaults to ACTIVE if not specified)
	Status Status `json:"status"`

	// Custom key-value pairs for extending the organization information
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateOrganizationInput

// UpdateOrganizationInput is a struct design to encapsulate request update payload data.
//
// // swagger:model UpdateOrganizationInput
// @Description Request payload for updating an existing organization. All fields are optional - only specified fields will be updated. Omitted fields will remain unchanged.
type UpdateOrganizationInput struct {
	// Updated legal name of the organization (optional)
	LegalName string `json:"legalName" validate:"max=256" example:"Lerian Studio Updated" maxLength:"256"`

	// UUID of the parent organization if this is a child organization (optional)
	ParentOrganizationID *string `json:"parentOrganizationId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Updated trading or brand name of the organization (optional)
	DoingBusinessAs string `json:"doingBusinessAs" validate:"max=256" example:"The ledger.io" maxLength:"256"`

	// Updated physical address of the organization (optional)
	Address Address `json:"address"`

	// Updated status of the organization (optional)
	Status Status `json:"status"`

	// Updated custom key-value pairs for extending the organization information (optional)
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateOrganizationInput

// Organization is a struct designed to encapsulate response payload data.
//
// swagger:model Organization
// @Description Complete organization entity containing all fields including system-generated fields like ID, creation timestamps, and metadata. This is the response format for organization operations.
type Organization struct {
	// Unique identifier for the organization (UUID format)
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Reference to the parent organization, if this is a child organization
	ParentOrganizationID *string `json:"parentOrganizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Official legal name of the organization
	LegalName string `json:"legalName" example:"Lerian Studio" maxLength:"256"`

	// Trading or brand name of the organization, if different from legal name
	DoingBusinessAs *string `json:"doingBusinessAs" example:"Lerian Studio" maxLength:"256"`

	// Official tax ID, company registration number, or other legal identification
	LegalDocument string `json:"legalDocument" example:"00000000000000" maxLength:"256"`

	// Physical address of the organization
	Address Address `json:"address"`

	// Current operating status of the organization
	Status Status `json:"status"`

	// Timestamp when the organization was created (RFC3339 format)
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the organization was last updated (RFC3339 format)
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the organization was soft deleted, null if not deleted (RFC3339 format)
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the organization information
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name Organization

// Address structure for marshaling/unmarshalling JSON.
//
// swagger:model Address
// @Description Structured address information following standard postal address format. Country field follows ISO 3166-1 alpha-2 standard (2-letter country codes).
type Address struct {
	// Primary address line (street address or PO Box)
	Line1 string `json:"line1" example:"Street 1" maxLength:"256"`

	// Secondary address information like apartment number, suite, or floor
	Line2 *string `json:"line2" example:"Street 2" maxLength:"256"`

	// Postal code or ZIP code
	ZipCode string `json:"zipCode" example:"00000-000" maxLength:"20"`

	// City or locality name
	City string `json:"city" example:"New York" maxLength:"100"`

	// State, province, or region name or code
	State string `json:"state" example:"NY" maxLength:"100"`

	// Country code in ISO 3166-1 alpha-2 format (two-letter country code)
	Country string `json:"country" example:"US" minLength:"2" maxLength:"2"` // According to ISO 3166-1 alpha-2
} // @name Address

// IsEmpty method that set empty or nil in fields
func (a Address) IsEmpty() bool {
	return a.Line1 == "" && a.Line2 == nil && a.ZipCode == "" && a.City == "" && a.State == "" && a.Country == ""
}

// Organizations struct to return get all.
//
// swagger:model Organizations
// @Description Paginated list of organizations with metadata about the current page, limit, and the organization items themselves.
type Organizations struct {
	Items []Organization `json:"items"`
	Page  int            `json:"page" example:"1"`
	Limit int            `json:"limit" example:"10"`
} // @name Organizations
