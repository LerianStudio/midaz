package operation

import (
	"context"

	"github.com/google/uuid"
)

// Repository provides an interface for operations related to operation template entities.
//
//go:generate mockgen --destination=../../gen/mock/operation/operation_mock.go --package=mock . Repository
type Repository interface {
	Create(ctx context.Context, operation *Operation) (*Operation, error)
	FindAll(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, limit, page int) ([]*Operation, error)
	FindAllByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, limit, page int) ([]*Operation, error)
	FindAllByPortfolio(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, limit, page int) ([]*Operation, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Operation, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Operation, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, operation *Operation) (*Operation, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
}
