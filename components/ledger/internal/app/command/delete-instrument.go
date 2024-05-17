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

// DeleteInstrumentByID delete an instrument from the repository by ids.
func (uc *UseCase) DeleteInstrumentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Remove instrument for id: %s", id)

	if err := uc.InstrumentRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		logger.Errorf("Error deleting instrument on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return common.EntityNotFoundError{
				EntityType: reflect.TypeOf(i.Instrument{}).Name(),
				Message:    fmt.Sprintf("Instrument with id %s was not found", id),
				Code:       "INSTRUMENT_NOT_FOUND",
				Err:        err,
			}
		}

		return err
	}

	return nil
}
