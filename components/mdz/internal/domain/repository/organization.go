// Package repository defines repository interfaces for the MDZ CLI domain layer.
//
// This package provides abstractions for data access, following the Repository pattern.
// It defines interfaces that are implemented by the REST adapter layer, allowing the
// CLI commands to be independent of the HTTP implementation details.
package repository

import (
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// Organization defines the interface for organization data operations.
//
// This interface abstracts organization CRUD operations, allowing CLI commands
// to work with organizations without knowing the underlying HTTP implementation.
type Organization interface {
	Create(org mmodel.CreateOrganizationInput) (*mmodel.Organization, error)
	Get(limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Organizations, error)
	GetByID(organizationID string) (*mmodel.Organization, error)
	Update(organizationID string, orgInput mmodel.UpdateOrganizationInput) (*mmodel.Organization, error)
	Delete(organizationID string) error
}
