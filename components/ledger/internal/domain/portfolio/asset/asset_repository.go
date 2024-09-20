package asset

import (
	"context"

	"github.com/google/uuid"
)

// Repository provides an interface for operations related to asset entities.
//
//go:generate mockgen --destination=../../../gen/mock/asset/asset_mock.go --package=mock . Repository
type Repository interface {
	Create(ctx context.Context, asset *Asset) (*Asset, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, limit, page int) ([]*Asset, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Asset, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Asset, error)
	FindByNameOrCode(ctx context.Context, organizationID, ledgerID uuid.UUID, name, code string) (bool, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, asset *Asset) (*Asset, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
}
