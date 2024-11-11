package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	"github.com/google/uuid"
)

// GetLedgerByID Get a ledger from the repository by given id.
func (uc *UseCase) GetLedgerByID(ctx context.Context, organizationID, id uuid.UUID) (*l.Ledger, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_ledger_by_id")
	defer span.End()

	logger.Infof("Retrieving ledger for id: %s", id.String())

	ledger, err := uc.LedgerRepo.Find(ctx, organizationID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get ledger on repo by id", err)

		logger.Errorf("Error getting ledger on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrLedgerIDNotFound, reflect.TypeOf(l.Ledger{}).Name())
		}

		return nil, err
	}

	if ledger != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(l.Ledger{}).Name(), id.String())
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb ledger", err)

			logger.Errorf("Error get metadata on mongodb ledger: %v", err)

			return nil, err
		}

		if metadata != nil {
			ledger.Metadata = metadata.Data
		}
	}

	return ledger, nil
}
