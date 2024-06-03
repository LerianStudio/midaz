package ledger

import (
	"context"

	"github.com/google/uuid"
)

// Repository provides an interface for operations related to ledger entities.
//
//go:generate mockgen --destination=../../../gen/mock/ledger/ledger_mock.go --package=mock . Repository
type Repository interface {
	Create(ctx context.Context, ledger *Ledger) (*Ledger, error)
	Find(ctx context.Context, organizationID, id uuid.UUID) (*Ledger, error)
	FindAll(ctx context.Context, organizationID uuid.UUID, limit, page int) ([]*Ledger, error)
	ListByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]*Ledger, error)
	Update(ctx context.Context, organizationID, id uuid.UUID, ledger *Ledger) (*Ledger, error)
	Delete(ctx context.Context, organizationID, id uuid.UUID) error
}
