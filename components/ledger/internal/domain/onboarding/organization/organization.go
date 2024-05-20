package organization

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// OrganizationPostgreSQLModel represents the entity Organization into SQL context in Database
type OrganizationPostgreSQLModel struct {
	ID                   string
	ParentOrganizationID *string
	LegalName            string
	DoingBusinessAs      *string
	LegalDocument        string
	Address              Address
	Status               string
	StatusDescription    *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            sql.NullTime
	Metadata             map[string]any
}

// CreateOrganizationInput is a struct design to encapsulate request create payload data.
type CreateOrganizationInput struct {
	LegalName            string         `json:"legalName"`
	ParentOrganizationID *string        `json:"parentOrganizationId"`
	DoingBusinessAs      *string        `json:"doingBusinessAs,omitempty"`
	LegalDocument        string         `json:"legalDocument"`
	Address              Address        `json:"address"`
	Status               Status         `json:"status,omitempty"`
	Metadata             map[string]any `json:"metadata,omitempty"`
}

// UpdateOrganizationInput is a struct design to encapsulate request update payload data.
type UpdateOrganizationInput struct {
	LegalName       string         `json:"legalName"`
	DoingBusinessAs *string        `json:"doingBusinessAs,omitempty"`
	Address         Address        `json:"address"`
	Status          Status         `json:"status"`
	Metadata        map[string]any `json:"metadata"`
}

// Organization is a struct designed to encapsulate response payload data.
type Organization struct {
	ID                   string         `json:"id,omitempty"`
	ParentOrganizationID *string        `json:"parentOrganizationId,omitempty"`
	LegalName            string         `json:"legalName"`
	DoingBusinessAs      *string        `json:"doingBusinessAs,omitempty"`
	LegalDocument        string         `json:"legalDocument"`
	Address              Address        `json:"address"`
	Status               Status         `json:"status"`
	CreatedAt            time.Time      `json:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"`
	DeletedAt            *time.Time     `json:"deletedAt"`
	Metadata             map[string]any `json:"metadata,omitempty"`
}

// Status structure for marshaling/unmarshalling JSON.
type Status struct {
	Code        string  `json:"code"`
	Description *string `json:"description"`
}

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}

// Address structure for marshaling/unmarshalling JSON.
type Address struct {
	Line1        string  `json:"line1"`
	Line2        *string `json:"line2,omitempty"`
	Neighborhood string  `json:"neighborhood"`
	ZipCode      string  `json:"zipCode"`
	City         string  `json:"city"`
	State        string  `json:"state"`
	Country      string  `json:"country"` // According to ISO 3166-1 alpha-2
}

// IsEmpty method that set empty or nil in fields
func (a Address) IsEmpty() bool {
	return a.Line1 == "" && a.Line2 == nil && a.Neighborhood == "" &&
		a.ZipCode == "" && a.City == "" && a.State == "" && a.Country == ""
}

// ToEntity converts an OrganizationPostgreSQLModel to entity.Organization
func (t *OrganizationPostgreSQLModel) ToEntity() *Organization {
	status := Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	organization := &Organization{
		ID:                   t.ID,
		ParentOrganizationID: t.ParentOrganizationID,
		LegalName:            t.LegalName,
		DoingBusinessAs:      t.DoingBusinessAs,
		LegalDocument:        t.LegalDocument,
		Address:              t.Address,
		Status:               status,
		CreatedAt:            t.CreatedAt,
		UpdatedAt:            t.UpdatedAt,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		organization.DeletedAt = &deletedAtCopy
	}

	return organization
}

// FromEntity converts an entity.Organization to OrganizationPostgresModel
func (t *OrganizationPostgreSQLModel) FromEntity(organization *Organization) {
	*t = OrganizationPostgreSQLModel{
		ID:                   uuid.New().String(),
		ParentOrganizationID: organization.ParentOrganizationID,
		LegalName:            organization.LegalName,
		DoingBusinessAs:      organization.DoingBusinessAs,
		LegalDocument:        organization.LegalDocument,
		Address:              organization.Address,
		Status:               organization.Status.Code,
		StatusDescription:    organization.Status.Description,
		CreatedAt:            organization.CreatedAt,
		UpdatedAt:            organization.UpdatedAt,
	}

	if organization.DeletedAt != nil {
		deletedAtCopy := *organization.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
