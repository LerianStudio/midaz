package command

import (
	"context"
	"errors"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"reflect"

	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"

	"github.com/google/uuid"
)

func (uc *UseCase) UpdateAccounts(ctx context.Context, validate goldModel.Responses, organizationID, ledgerID uuid.UUID, accounts []*mmodel.Account) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_accounts")
	defer span.End()

	logger.Infof("Trying to update accounts")

	result := make(chan []*mmodel.Account)

	var accountsToUpdate []*mmodel.Account

	go goldModel.UpdateAccounts(constant.DEBIT, validate.From, accounts, result)
	select {
	case r := <-result:
		accountsToUpdate = append(accountsToUpdate, r...)
	}

	go goldModel.UpdateAccounts(constant.CREDIT, validate.To, accounts, result)
	select {
	case r := <-result:
		accountsToUpdate = append(accountsToUpdate, r...)
	}

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
