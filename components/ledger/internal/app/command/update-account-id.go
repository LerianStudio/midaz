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

// UpdateAccountByID update an account from the repository by given id.
func (uc *UseCase) UpdateAccountByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, balance *mmodel.Balance) (*mmodel.Account, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.UpdateAccountByID")
	defer span.End()

	logger.Infof("Trying to update account by id: %v", id)

	account := &mmodel.Account{
		Balance: *balance,
	}

	accountUpdated, err := uc.AccountRepo.UpdateAccountByID(ctx, organizationID, ledgerID, id, account)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update account on repo by id", err)

		logger.Errorf("Error updating account on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	return accountUpdated, nil
}
