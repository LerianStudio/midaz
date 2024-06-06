package account

import (
	"context"

	"github.com/google/uuid"
)

// Repository provides an interface for operations related to account entities.
//
//go:generate mockgen --destination=../../../gen/mock/account/account_mock.go --package=mock . Repository
type Repository interface {
	Create(ctx context.Context, account *Account) (*Account, error)
	FindAll(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, limit, page int) ([]*Account, error)
	Find(ctx context.Context, organizationID, ledgerID, portfolioID, id uuid.UUID) (*Account, error)
	FindByAlias(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, alias string) (bool, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, ids []uuid.UUID) ([]*Account, error)
	ListByAlias(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, alias []string) ([]*Account, error)
	Update(ctx context.Context, organizationID, ledgerID, portfolioID, id uuid.UUID, account *Account) (*Account, error)
	Delete(ctx context.Context, organizationID, ledgerID, portfolioID, id uuid.UUID) error
}
