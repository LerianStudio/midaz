package mmodel

import "time"

// CreateOrganizationInput is a struct design to encapsulate request create payload data.
type CreateOrganizationInput struct {
	LegalName            string         `json:"legalName" validate:"required,max=256"`
	ParentOrganizationID *string        `json:"parentOrganizationId" validate:"omitempty,uuid"`
	DoingBusinessAs      *string        `json:"doingBusinessAs" validate:"max=256"`
	LegalDocument        string         `json:"legalDocument" validate:"required,max=256"`
	Address              Address        `json:"address"`
	Status               Status         `json:"status"`
	Metadata             map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// UpdateOrganizationInput is a struct design to encapsulate request update payload data.
type UpdateOrganizationInput struct {
	LegalName            string         `json:"legalName" validate:"required,max=256"`
	ParentOrganizationID *string        `json:"parentOrganizationId" validate:"omitempty,uuid"`
	DoingBusinessAs      *string        `json:"doingBusinessAs" validate:"max=256"`
	Address              Address        `json:"address"`
	Status               Status         `json:"status"`
	Metadata             map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// Organization is a struct designed to encapsulate response payload data.
type Organization struct {
	ID                   string         `json:"id"`
	ParentOrganizationID *string        `json:"parentOrganizationId"`
	LegalName            string         `json:"legalName"`
	DoingBusinessAs      *string        `json:"doingBusinessAs"`
	LegalDocument        string         `json:"legalDocument"`
	Address              Address        `json:"address"`
	Status               Status         `json:"status"`
	CreatedAt            time.Time      `json:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"`
	DeletedAt            *time.Time     `json:"deletedAt"`
	Metadata             map[string]any `json:"metadata,omitempty"`
}

type Organizations struct {
	Items []Organization `json:"items"`
	Page  int            `json:"page"`
	Limit int            `json:"limit"`
}

// Status structure for marshaling/unmarshalling JSON.
type Status struct {
	Code        string  `json:"code" validate:"max=100"`
	Description *string `json:"description" validate:"omitempty,max=256"`
}

// Address structure for marshaling/unmarshalling JSON.
type Address struct {
	Line1   string  `json:"line1"`
	Line2   *string `json:"line2"`
	ZipCode string  `json:"zipCode"`
	City    string  `json:"city"`
	State   string  `json:"state"`
	Country string  `json:"country"` // According to ISO 3166-1 alpha-2
}
