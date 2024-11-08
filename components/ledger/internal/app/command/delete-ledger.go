package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	"github.com/google/uuid"
)

// DeleteLedgerByID deletes a ledger from the repository
func (uc *UseCase) DeleteLedgerByID(ctx context.Context, organizationID, id uuid.UUID) error {
	logger := mlog.NewLoggerFromContext(ctx)
	tracer := mopentelemetry.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_ledger_by_id")
	defer span.End()

	logger.Infof("Remove ledger for id: %s", id.String())

	if err := uc.LedgerRepo.Delete(ctx, organizationID, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to delete ledger on repo by id", err)

		logger.Errorf("Error deleting ledger on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return common.ValidateBusinessError(cn.ErrLedgerIDNotFound, reflect.TypeOf(l.Ledger{}).Name())
		}

		return err
	}

	return nil
}
