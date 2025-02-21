package mmodel

import "time"

// CreateOrganizationInput is a struct design to encapsulate request create payload data.
//
// // swagger:model CreateOrganizationInput
// @Description CreateOrganizationInput is the input payload to create an organization.
type CreateOrganizationInput struct {
	LegalName            string         `json:"legalName" validate:"required,max=256" example:"Lerian Studio"`
	ParentOrganizationID *string        `json:"parentOrganizationId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000"`
	DoingBusinessAs      *string        `json:"doingBusinessAs" validate:"max=256" example:"Lerian Studio"`
	LegalDocument        string         `json:"legalDocument" validate:"required,max=256" example:"00000000000000"`
	Address              Address        `json:"address"`
	Status               Status         `json:"status"`
	Metadata             map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateOrganizationInput

// UpdateOrganizationInput is a struct design to encapsulate request update payload data.
//
// // swagger:model UpdateOrganizationInput
// @Description UpdateOrganizationInput is the input payload to update an organization.
type UpdateOrganizationInput struct {
	LegalName            string         `json:"legalName" validate:"max=256" example:"Lerian Studio Updated"`
	ParentOrganizationID *string        `json:"parentOrganizationId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000"`
	DoingBusinessAs      string         `json:"doingBusinessAs" validate:"max=256" example:"The ledger.io"`
	Address              Address        `json:"address"`
	Status               Status         `json:"status"`
	Metadata             map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateOrganizationInput

// Organization is a struct designed to encapsulate response payload data.
//
// swagger:model Organization
// @Description Organization is a struct designed to store organization data.
type Organization struct {
	ID                   string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	ParentOrganizationID *string        `json:"parentOrganizationId" example:"00000000-0000-0000-0000-000000000000"`
	LegalName            string         `json:"legalName" example:"Lerian Studio"`
	DoingBusinessAs      *string        `json:"doingBusinessAs" example:"Lerian Studio"`
	LegalDocument        string         `json:"legalDocument" example:"00000000000000"`
	Address              Address        `json:"address"`
	Status               Status         `json:"status"`
	CreatedAt            time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt            time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt            *time.Time     `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata             map[string]any `json:"metadata,omitempty"`
} // @name Organization

// Address structure for marshaling/unmarshalling JSON.
//
// swagger:model Address
// @Description Address is a struct designed to store the address data of an organization.
type Address struct {
	Line1   string  `json:"line1" example:"Street 1"`
	Line2   *string `json:"line2" example:"Street 2"`
	ZipCode string  `json:"zipCode" example:"00000-000"`
	City    string  `json:"city" example:"New York"`
	State   string  `json:"state" example:"NY"`
	Country string  `json:"country" example:"US"` // According to ISO 3166-1 alpha-2
} // @name Address

// IsEmpty method that set empty or nil in fields
func (a Address) IsEmpty() bool {
	return a.Line1 == "" && a.Line2 == nil && a.ZipCode == "" && a.City == "" && a.State == "" && a.Country == ""
}

// Organizations struct to return get all.
//
// swagger:model Organizations
// @Description Organizations is the struct designed to return a list of organizations with pagination.
type Organizations struct {
	Items []Organization `json:"items"`
	Page  int            `json:"page" example:"1"`
	Limit int            `json:"limit" example:"10"`
} // @name Organizations
