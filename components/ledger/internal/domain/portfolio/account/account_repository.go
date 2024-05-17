package account

import (
	"context"

	"github.com/google/uuid"
)

// Repository provides an interface for operations related to account entities.
//
//go:generate mockgen --destination=../../gen/mock/account/account_mock.go --package=mock . Repository
type Repository interface {
	Create(ctx context.Context, account *Account) (*Account, error)
	FindAll(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID) ([]*Account, error)
	Find(ctx context.Context, organizationID, ledgerID, portfolioID, id uuid.UUID) (*Account, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, ids []uuid.UUID) ([]*Account, error)
	Update(ctx context.Context, organizationID, ledgerID, portfolioID, id uuid.UUID, account *Account) (*Account, error)
	Delete(ctx context.Context, organizationID, ledgerID, portfolioID, id uuid.UUID) error
}
