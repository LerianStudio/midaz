package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// GetLedgerByID Get a ledger from the repository by given id.
func (uc *UseCase) GetLedgerByID(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_ledger_by_id")
	defer span.End()

	logger.Infof("Retrieving ledger for id: %s", id.String())

	ledger, err := uc.LedgerRepo.Find(ctx, organizationID, id)
	if err != nil {
		logger.Errorf("Error getting ledger on repo by id: %v", err)

		var entityNotFound *pkg.EntityNotFoundError
		if errors.As(err, &entityNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get ledger on repo by id", err)

			logger.Warn("No ledger found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get ledger on repo by id", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	if ledger != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Ledger{}).Name(), id.String())
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb ledger", err)

			logger.Warn("No metadata found")

			return nil, err
		}

		if metadata != nil {
			ledger.Metadata = metadata.Data
		}
	}

	return ledger, nil
}
