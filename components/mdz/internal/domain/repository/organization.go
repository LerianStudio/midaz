package repository

import "github.com/LerianStudio/midaz/components/mdz/internal/model"

type Organization interface {
	Create(org model.Organization) (*model.OrganizationCreate, error)
	Get(limit, page int) (*model.OrganizationList, error)
	GetByID(organizationID string) (*model.OrganizationItem, error)
	Update(organizationID string, orgInput model.OrganizationUpdate) (*model.OrganizationItem, error)
	Delete(organizationID string) error
}
