package organization

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
)

// OrganizationPostgreSQLModel represents the entity Organization into SQL context in Database
type OrganizationPostgreSQLModel struct {
	ID                   string
	ParentOrganizationID *string
	LegalName            string
	DoingBusinessAs      *string
	LegalDocument        string
	Address              mmodel.Address
	Status               string
	StatusDescription    *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            sql.NullTime
	Metadata             map[string]any
}

// ToEntity converts an OrganizationPostgreSQLModel to entity.Organization
func (t *OrganizationPostgreSQLModel) ToEntity() *mmodel.Organization {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	organization := &mmodel.Organization{
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
func (t *OrganizationPostgreSQLModel) FromEntity(organization *mmodel.Organization) {
	*t = OrganizationPostgreSQLModel{
		ID:                   pkg.GenerateUUIDv7().String(),
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
