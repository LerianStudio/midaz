package command

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// DeleteAccountByID delete an account from the repository by ids.
func (uc *UseCase) DeleteAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.delete_account_by_id")
	defer span.End()

	// Record operation metrics
	uc.recordOnboardingMetrics(ctx, "account", "delete",
		attribute.String("account_id", id.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()))

	logger.Infof("Remove account for id: %s", id.String())

	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to find account by alias", err)

		// Record error
		uc.recordOnboardingError(ctx, "account", "find_error",
			attribute.String("account_id", id.String()),
			attribute.String("error_detail", err.Error()))

		return err
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == "external" {
		mopentelemetry.HandleSpanError(&span, "Cannot manipulate external account", constant.ErrForbiddenExternalAccountManipulation)

		// Record error
		uc.recordOnboardingError(ctx, "account", "validation_error",
			attribute.String("account_id", id.String()),
			attribute.String("error_detail", "forbidden_external_account_manipulation"))

		return pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, reflect.TypeOf(mmodel.Account{}).Name())
	}

	if err := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, portfolioID, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to delete account on repo by id", err)

		logger.Errorf("Error deleting account on repo by id: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "account", "delete_error",
			attribute.String("account_id", id.String()),
			attribute.String("error_detail", err.Error()))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return err
	}

	// Record successful completion and duration
	uc.recordOnboardingDuration(ctx, startTime, "account", "delete", "success",
		attribute.String("account_id", id.String()),
		attribute.String("account_type", accFound.Type))

	return nil
}
