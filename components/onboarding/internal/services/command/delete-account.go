package command

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"reflect"
)

// DeleteAccountByID delete an account from the repository by ids.
func (uc *UseCase) DeleteAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_account_by_id")
	defer span.End()

	logger.Infof("Remove account for id: %s", id.String())

	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to find account by alias", err)

		return err
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == "external" {
		return pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, reflect.TypeOf(mmodel.Account{}).Name())
	}

	if err := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, portfolioID, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete account on repo by id", err)

		logger.Errorf("Error deleting account on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return err
	}

	return nil
}
