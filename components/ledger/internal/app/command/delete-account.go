package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	"github.com/google/uuid"
)

// DeleteAccountByID delete an account from the repository by ids.
func (uc *UseCase) DeleteAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) error {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_account_by_id")
	defer span.End()

	logger.Infof("Remove account for id: %s", id.String())

	if err := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, portfolioID, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to delete account on repo by id", err)

		logger.Errorf("Error deleting account on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return common.ValidateBusinessError(cn.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return err
	}

	return nil
}
