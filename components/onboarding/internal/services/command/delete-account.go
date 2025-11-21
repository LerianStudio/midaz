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
	logger, tracer, requestID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_account_by_id")
	defer span.End()

	logger.Infof("Remove account for id: %s", id.String())

	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by id", err)

		logger.Errorf("Error finding account by id: %v", err)

		return err
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == "external" {
		return pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, reflect.TypeOf(mmodel.Account{}).Name())
	}

	balanceDeleteRequest := &balanceproto.DeleteAllBalancesByAccountIDRequest{
		OrganizationId: organizationID.String(),
		LedgerId:       ledgerID.String(),
		AccountId:      accFound.ID,
		RequestId:      requestID,
	}

	err = uc.BalanceGRPCRepo.DeleteAllBalancesByAccountID(ctx, token, balanceDeleteRequest)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete all balances by account id via gRPC", err)

		logger.Errorf("Failed to delete all balances by account id via gRPC: %v", err)

		var (
			unauthorized pkg.UnauthorizedError
			forbidden    pkg.ForbiddenError
		)

		if errors.As(err, &unauthorized) || errors.As(err, &forbidden) {
			return err
		}

		return pkg.ValidateBusinessError(constant.ErrAccountBalanceDeletion, reflect.TypeOf(mmodel.Account{}).Name())
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
