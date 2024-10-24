package repository

import "github.com/LerianStudio/midaz/components/mdz/internal/model"

type Organization interface {
	Create(org model.Organization) (*model.OrganizationResponse, error)
}
