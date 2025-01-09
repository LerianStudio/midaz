package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/pkg/mgrpc/account"
	"reflect"

	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"

	"github.com/google/uuid"
)

// UpdateAccountByID update an account from the repository by given id.
func (uc *UseCase) UpdateAccountByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, balance *mmodel.Balance) (*mmodel.Account, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

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

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	return accountUpdated, nil
}

func (uc *UseCase) UpdateAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, accounts []*account.Account) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_accounts")
	defer span.End()

	logger.Infof("Trying to update accounts")

	err := uc.AccountRepo.UpdateAccounts(ctx, organizationID, ledgerID, accounts)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update account on repo by id", err)

		logger.Errorf("Error updating account on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return err
	}

	return nil
}
