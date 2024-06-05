package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	"github.com/google/uuid"
)

// GetAllLedgers fetch all Ledgers from the repository
func (uc *UseCase) GetAllLedgers(ctx context.Context, organizationID string, filter commonHTTP.QueryHeader) ([]*l.Ledger, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving ledgers")

	ledgers, err := uc.LedgerRepo.FindAll(ctx, uuid.MustParse(organizationID), filter.Limit, filter.Page)
	if err != nil {
		logger.Errorf("Error getting ledgers on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(l.Ledger{}).Name(),
				Message:    "Ledgers was not found",
				Code:       "LEDGER_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	if ledgers != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(l.Ledger{}).Name(), filter)
		if err != nil {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(l.Ledger{}).Name(),
				Message:    "Metadata was not found",
				Code:       "LEDGER_NOT_FOUND",
				Err:        err,
			}
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range ledgers {
			if data, ok := metadataMap[ledgers[i].ID]; ok {
				ledgers[i].Metadata = data
			}
		}
	}

	return ledgers, nil
}
