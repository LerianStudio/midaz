package product

import (
	"context"
	"github.com/LerianStudio/midaz/common/mmodel"

	"github.com/google/uuid"
)

// Repository provides an interface for operations related to product entities.
//
//go:generate mockgen --destination=../../../gen/mock/product/product_mock.go --package=mock . Repository
type Repository interface {
	Create(ctx context.Context, product *mmodel.Product) (*mmodel.Product, error)
	FindByName(ctx context.Context, organizationID, ledgerID uuid.UUID, name string) (bool, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, limit, page int) ([]*mmodel.Product, error)
	FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Product, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Product, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, product *mmodel.Product) (*mmodel.Product, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
}
