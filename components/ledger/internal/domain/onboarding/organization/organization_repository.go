package organization

import (
	"context"

	"github.com/google/uuid"
)

// Repository provides an interface for operations related to organization entities.
//
//go:generate mockgen --destination=../../../gen/mock/organization/organization_mock.go --package=mock . Repository
type Repository interface {
	Create(ctx context.Context, organization *Organization) (*Organization, error)
	Update(ctx context.Context, id uuid.UUID, organization *Organization) (*Organization, error)
	Find(ctx context.Context, id uuid.UUID) (*Organization, error)
	FindAll(ctx context.Context) ([]*Organization, error)
	ListByIDs(ctx context.Context, ids []uuid.UUID) ([]*Organization, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
