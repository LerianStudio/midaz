package repository

import (
	"github.com/LerianStudio/midaz/pkg/mmodel"
)

type Organization interface {
	Create(org mmodel.CreateOrganizationInput) (*mmodel.Organization, error)
	Get(limit, page int) (*mmodel.Organizations, error)
	GetByID(organizationID string) (*mmodel.Organization, error)
	Update(organizationID string, orgInput mmodel.UpdateOrganizationInput) (*mmodel.Organization, error)
	Delete(organizationID string) error
}
