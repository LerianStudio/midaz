package portfolio

import (
	"context"

	"github.com/google/uuid"
)

// Repository provides an interface for operations related to portfolio entities.
//
//go:generate mockgen --destination=../../gen/mock/portfolio/portfolio_mock.go --package=mock . Repository
type Repository interface {
	Create(ctx context.Context, portfolio *Portfolio) (*Portfolio, error)
	FindByIDEntity(ctx context.Context, organizationID, ledgerID, entityID uuid.UUID) (*Portfolio, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID) ([]*Portfolio, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Portfolio, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Portfolio, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, portfolio *Portfolio) (*Portfolio, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
}
