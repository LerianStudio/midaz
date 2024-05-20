package product

import (
	"context"

	"github.com/google/uuid"
)

// Repository provides an interface for operations related to product entities.
//
//go:generate mockgen --destination=../../../gen/mock/product/product_mock.go --package=mock . Repository
type Repository interface {
	Create(ctx context.Context, product *Product) (*Product, error)
	FindByName(ctx context.Context, organizationID, ledgerID uuid.UUID, name string) (bool, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID) ([]*Product, error)
	FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Product, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Product, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, product *Product) (*Product, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
}
