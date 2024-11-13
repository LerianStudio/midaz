package asset

import (
	"context"
	"github.com/LerianStudio/midaz/common/mmodel"

	"github.com/google/uuid"
)

// Repository provides an interface for operations related to asset entities.
//
//go:generate mockgen --destination=../../../gen/mock/asset/asset_mock.go --package=mock . Repository
type Repository interface {
	Create(ctx context.Context, asset *mmodel.Asset) (*mmodel.Asset, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, limit, page int) ([]*mmodel.Asset, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Asset, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Asset, error)
	FindByNameOrCode(ctx context.Context, organizationID, ledgerID uuid.UUID, name, code string) (bool, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, asset *mmodel.Asset) (*mmodel.Asset, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
}
