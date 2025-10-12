package organization

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// OrganizationPostgreSQLModel represents the organization entity in PostgreSQL context.
//
// Maps domain organization entity to database schema with SQL-specific types.
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

// ToEntity converts the PostgreSQL model to a domain organization entity.
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

// FromEntity converts a domain organization entity to the PostgreSQL model.
func (t *OrganizationPostgreSQLModel) FromEntity(organization *mmodel.Organization) {
	*t = OrganizationPostgreSQLModel{
		ID:                   libCommons.GenerateUUIDv7().String(),
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
