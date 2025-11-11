package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	balanceproto "github.com/LerianStudio/midaz/v3/pkg/mgrpc/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// DeleteAccountByID delete an account from the repository by ids.
func (uc *UseCase) DeleteAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, token string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_account_by_id")
	defer span.End()

	logger.Infof("Remove account for id: %s", id.String())

	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by alias", err)

		logger.Errorf("Error finding account by alias: %v", err)

		return err
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == "external" {
		return pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, reflect.TypeOf(mmodel.Account{}).Name())
	}

	balanceReq := &balanceproto.BalanceRequest{
		OrganizationId: organizationID.String(),
		LedgerId:       ledgerID.String(),
		AccountId:      accFound.ID,
	}

	balancesFound, err := uc.BalanceGRPCRepo.GetBalance(ctx, token, balanceReq)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance via gRPC", err)
		logger.Errorf("Failed to get balance via gRPC: %v", err)
		return err
	}

	if balancesFound == nil || len(balancesFound.Balances) == 0 {
		err = pkg.ValidateBusinessError(constant.ErrBalanceNotFound, reflect.TypeOf(mmodel.Balance{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balances not found", err)
		logger.Errorf("Balances not found: %v", err)
		return err
	}

	for _, balanceFound := range balancesFound.Balances {
		// TODO: Do we want to stop the process if we fail to delete a balance?
		err = uc.deleteBalance(ctx, organizationID, ledgerID, balanceFound, token)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance", err)
			logger.Errorf("Failed to delete balance: %v", err)
			return err
		}
	}

	if err := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, portfolioID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())

			logger.Warnf("Account ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete account on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete account on repo by id", err)

		logger.Errorf("Error deleting account: %v", err)

		return err
	}

	return nil
}

func (uc *UseCase) deleteBalance(ctx context.Context, organizationID, ledgerID uuid.UUID, balanceFound *balanceproto.BalanceResponse, token string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	ctx, span := tracer.Start(ctx, "command.delete_balance")
	defer span.End()

	//TODO: Check if this is the best way to have/access the balance data from the gRPC response.
	grpcBalance := &balanceproto.Balance{
		Available: balanceFound.GetAvailable(),
		OnHold:    balanceFound.GetOnHold(),
	}

	if !grpcBalance.HasZeroFunds() {
		err := pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, reflect.TypeOf(mmodel.Balance{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance funds not zero", err)
		logger.Errorf("Balance funds not zero: %v", err)
		return err
	}

	deleteBalanceReq := &balanceproto.DeleteBalanceRequest{
		Id:             balanceFound.GetId(),
		OrganizationId: organizationID.String(),
		LedgerId:       ledgerID.String(),
	}

	err := uc.BalanceGRPCRepo.DeleteBalance(ctx, token, deleteBalanceReq)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance via gRPC", err)
		logger.Errorf("Failed to delete balance via gRPC: %v", err)
		return err
	}

	return nil
}
