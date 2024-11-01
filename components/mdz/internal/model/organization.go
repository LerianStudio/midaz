package model

import "time"

type Organization struct {
	LegalName            string    `json:"legalName"`
	ParentOrganizationID *string   `json:"parentOrganizationId,omitempty"`
	DoingBusinessAs      string    `json:"doingBusinessAs"`
	LegalDocument        string    `json:"legalDocument"`
	Status               Status    `json:"status"`
	Address              Address   `json:"address"`
	Metadata             *Metadata `json:"metadata,omitempty"`
}

type Status struct {
	Code        *string `json:"code,omitempty"`
	Description string  `json:"description"`
}

type Address struct {
	Line1   *string `json:"line1,omitempty"`
	Line2   *string `json:"line2,omitempty"`
	ZipCode *string `json:"zipCode,omitempty"`
	City    *string `json:"city,omitempty"`
	State   *string `json:"state,omitempty"`
	Country string  `json:"country"`
}

type Metadata struct {
	Chave   *string  `json:"chave,omitempty"`
	Bitcoin *string  `json:"bitcoinn,omitempty"`
	Boolean *bool    `json:"boolean,omitempty"`
	Double  *float64 `json:"double,omitempty"`
	Int     *int     `json:"int"`
}

type OrganizationCreate struct {
	ID                   string   `json:"id"`
	ParentOrganizationID string   `json:"parentOrganizationId"`
	LegalName            string   `json:"legalName"`
	DoingBusinessAs      string   `json:"doingBusinessAs"`
	LegalDocument        string   `json:"legalDocument"`
	Address              Address  `json:"address"`
	Status               Status   `json:"status"`
	CreatedAt            string   `json:"createdAt"`
	UpdatedAt            string   `json:"updatedAt"`
	DeletedAt            string   `json:"deletedAt"`
	Metadata             Metadata `json:"metadata"`
}

type OrganizationList struct {
	Items []OrganizationItem `json:"items"`
	Page  int                `json:"page"`
	Limit int                `json:"limit"`
}

type OrganizationItem struct {
	ID                   string     `json:"id"`
	ParentOrganizationID *string    `json:"parentOrganizationId"`
	LegalName            string     `json:"legalName"`
	DoingBusinessAs      string     `json:"doingBusinessAs"`
	LegalDocument        string     `json:"legalDocument"`
	Address              Address    `json:"address"`
	Status               Status     `json:"status"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
	DeletedAt            *time.Time `json:"deletedAt"`
	Metadata             *Metadata  `json:"metadata"`
}

type OrganizationUpdate struct {
	ParentOrganizationID *string       `json:"parentOrganizationId,omitempty"`
	LegalName            string        `json:"legalName"`
	DoingBusinessAs      string        `json:"doingBusinessAs"`
	LegalDocument        *string       `json:"legalDocument,omitempty"`
	Address              Address       `json:"address"`
	Status               *StatusUpdate `json:"status,omitempty"`
	Metadata             *Metadata     `json:"metadata,omitempty"`
}

type StatusUpdate struct {
	Code        *string `json:"code,omitempty"`
	Description *string `json:"description,omitempty"`
}
