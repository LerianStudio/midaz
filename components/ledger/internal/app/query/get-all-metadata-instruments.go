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
	"go.mongodb.org/mongo-driver/bson"
)

// GetAllMetadataInstruments fetch all Instruments from the repository
func (uc *UseCase) GetAllMetadataInstruments(ctx context.Context, key, value, organizationID, ledgerID string) ([]*i.Instrument, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving instruments")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(i.Instrument{}).Name(), bson.M{key: value})
	if err != nil || metadata == nil {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(i.Instrument{}).Name(),
			Message:    "Instruments by metadata was not found",
			Code:       "INSTRUMENT_NOT_FOUND",
			Err:        err,
		}
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for idx, meta := range metadata {
		uuids[idx] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	instruments, err := uc.InstrumentRepo.ListByIDs(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuids)
	if err != nil {
		logger.Errorf("Error getting instruments on repo by query params: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(i.Instrument{}).Name(),
				Message:    "Instruments by metadata was not found",
				Code:       "INSTRUMENT_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	for idx := range instruments {
		if data, ok := metadataMap[instruments[idx].ID]; ok {
			instruments[idx].Metadata = data
		}
	}

	return instruments, nil
}
