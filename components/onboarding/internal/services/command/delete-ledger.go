package command

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"reflect"
)

// DeleteLedgerByID deletes a ledger from the repository
func (uc *UseCase) DeleteLedgerByID(ctx context.Context, organizationID, id uuid.UUID) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_ledger_by_id")
	defer span.End()

	logger.Infof("Remove ledger for id: %s", id.String())

	if err := uc.LedgerRepo.Delete(ctx, organizationID, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete ledger on repo by id", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			logger.Errorf("Ledger ID not found: %s", id.String())
			return pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())
		}

		logger.Errorf("Error deleting ledger: %v", err)

		return err
	}

	return nil
}
