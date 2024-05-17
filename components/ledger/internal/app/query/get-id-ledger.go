package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	"github.com/google/uuid"
)

// GetLedgerByID Get a ledger from the repository by given id.
func (uc *UseCase) GetLedgerByID(ctx context.Context, organizationID, id string) (*l.Ledger, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving ledger for id: %s", id)

	ledger, err := uc.LedgerRepo.Find(ctx, uuid.MustParse(organizationID), uuid.MustParse(id))
	if err != nil {
		logger.Errorf("Error getting ledger on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(l.Ledger{}).Name(),
				Message:    fmt.Sprintf("Ledger with id %s was not found", id),
				Code:       "LEDGER_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	if ledger != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(l.Ledger{}).Name(), id)
		if err != nil {
			logger.Errorf("Error get metadata on mongodb ledger: %v", err)
			return nil, err
		}

		if metadata != nil {
			ledger.Metadata = metadata.Data
		}
	}

	return ledger, nil
}
