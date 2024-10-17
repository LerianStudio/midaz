package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	"github.com/google/uuid"
)

// GetLedgerByID Get a ledger from the repository by given id.
func (uc *UseCase) GetLedgerByID(ctx context.Context, organizationID, id uuid.UUID) (*l.Ledger, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving ledger for id: %s", id)

	ledger, err := uc.LedgerRepo.Find(ctx, organizationID, id)
	if err != nil {
		logger.Errorf("Error getting ledger on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrLedgerIDNotFound, reflect.TypeOf(l.Ledger{}).Name())
		}

		return nil, err
	}

	if ledger != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(l.Ledger{}).Name(), id.String())
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
