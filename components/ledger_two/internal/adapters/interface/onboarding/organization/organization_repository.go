package organization

import (
	"context"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/google/uuid"
)

// Repository provides an interface for operations related to organization entities.
//
//go:generate mockgen --destination=../../../mock/onboarding/organization/organization_repository_mock.go --package=organization . Repository
type Repository interface {
	Create(ctx context.Context, organization *mmodel.Organization) (*mmodel.Organization, error)
	Update(ctx context.Context, id uuid.UUID, organization *mmodel.Organization) (*mmodel.Organization, error)
	Find(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error)
	FindAll(ctx context.Context, limit, page int) ([]*mmodel.Organization, error)
	ListByIDs(ctx context.Context, ids []uuid.UUID) ([]*mmodel.Organization, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
