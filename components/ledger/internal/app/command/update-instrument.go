package command

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

// UpdateInstrumentByID update an instrument from the repository by given id.
func (uc *UseCase) UpdateInstrumentByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, uii *i.UpdateInstrumentInput) (*i.Instrument, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to update instrument: %v", uii)

	if uii.Name == "" && uii.Status.IsEmpty() && uii.Metadata == nil {
		return nil, common.UnprocessableOperationError{
			Message: "at least one of the allowed fields must be sent with a valid value [name, status.code, status.description, metadata]",
			Code:    "0006",
			Err:     nil,
		}
	}

	instrument := &i.Instrument{
		Name: uii.Name,
	}

	if !uii.Status.IsEmpty() {
		instrument.Status = uii.Status
	}

	instrumentUpdated, err := uc.InstrumentRepo.Update(ctx, organizationID, ledgerID, id, instrument)
	if err != nil {
		logger.Errorf("Error updating instrument on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(i.Instrument{}).Name(),
				Message:    fmt.Sprintf("Instrument with ledger id %s and instrument id %s was not found", ledgerID, id),
				Code:       "INSTRUMENT_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	if len(uii.Metadata) > 0 {
		if err := common.CheckMetadataKeyAndValueLength(100, uii.Metadata); err != nil {
			return nil, err
		}

		if err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(i.Instrument{}).Name(), id.String(), uii.Metadata); err != nil {
			return nil, err
		}

		instrumentUpdated.Metadata = uii.Metadata

		return instrumentUpdated, nil
	}

	if err := uc.MetadataRepo.Delete(ctx, reflect.TypeOf(i.Instrument{}).Name(), id.String()); err != nil {
		return nil, err
	}

	return instrumentUpdated, nil
}
