package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	i "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/instrument"
	"github.com/google/uuid"
)

// GetInstrumentByID get an Instrument from the repository by given id.
func (uc *UseCase) GetInstrumentByID(ctx context.Context, organizationID, ledgerID, id string) (*i.Instrument, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving instrument for id: %s", id)

	instrument, err := uc.InstrumentRepo.Find(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(id))
	if err != nil {
		logger.Errorf("Error getting instrument on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(i.Instrument{}).Name(),
				Message:    fmt.Sprintf("Instrument with id %s was not found", id),
				Code:       "INSTRUMENT_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	if instrument != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(i.Instrument{}).Name(), id)
		if err != nil {
			logger.Errorf("Error get metadata on mongodb instrument: %v", err)
			return nil, err
		}

		if metadata != nil {
			instrument.Metadata = metadata.Data
		}
	}

	return instrument, nil
}
