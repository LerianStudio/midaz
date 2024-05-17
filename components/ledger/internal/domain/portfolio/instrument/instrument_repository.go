package instrument

import (
	"context"

	"github.com/google/uuid"
)

// Repository provides an interface for operations related to instrument entities.
//
//go:generate mockgen --destination=../../gen/mock/instrument/instrument_mock.go --package=mock . Repository
type Repository interface {
	Create(ctx context.Context, instrument *Instrument) (*Instrument, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID) ([]*Instrument, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Instrument, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Instrument, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, instrument *Instrument) (*Instrument, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
}
