package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	i "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/instrument"
	"github.com/google/uuid"
)

// GetAllInstruments fetch all Instrument from the repository
func (uc *UseCase) GetAllInstruments(ctx context.Context, organizationID, ledgerID string, filter common.QueryHeader) ([]*i.Instrument, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving instruments")

	instruments, err := uc.InstrumentRepo.FindAll(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), filter.Limit, filter.Page)
	if err != nil {
		logger.Errorf("Error getting instruments on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(i.Instrument{}).Name(),
				Message:    "Instrument was not found",
				Code:       "INSTRUMENT_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	if instruments != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(i.Instrument{}).Name(), filter)
		if err != nil {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(i.Instrument{}).Name(),
				Message:    "Metadata was not found",
				Code:       "INSTRUMENT_NOT_FOUND",
				Err:        err,
			}
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for idx := range instruments {
			if data, ok := metadataMap[instruments[idx].ID]; ok {
				instruments[idx].Metadata = data
			}
		}
	}

	return instruments, nil
}
