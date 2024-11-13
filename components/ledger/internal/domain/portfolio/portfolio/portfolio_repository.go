package portfolio

import (
	"context"
	"github.com/LerianStudio/midaz/common/mmodel"

	"github.com/google/uuid"
)

// Repository provides an interface for operations related to portfolio entities.
//
//go:generate mockgen --destination=../../../gen/mock/portfolio/portfolio_mock.go --package=mock . Repository
type Repository interface {
	Create(ctx context.Context, portfolio *mmodel.Portfolio) (*mmodel.Portfolio, error)
	FindByIDEntity(ctx context.Context, organizationID, ledgerID, entityID uuid.UUID) (*mmodel.Portfolio, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, limit, page int) ([]*mmodel.Portfolio, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Portfolio, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Portfolio, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, portfolio *mmodel.Portfolio) (*mmodel.Portfolio, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
}
