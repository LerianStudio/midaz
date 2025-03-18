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

// UpdateAccount update an account from the repository by given id.
func (uc *UseCase) UpdateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, uai *mmodel.UpdateAccountInput) (*mmodel.Account, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.update_account")
	defer span.End()

	// Record operation metrics
	uc.recordOnboardingMetrics(ctx, "account", "update",
		attribute.String("account_id", id.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()))

	logger.Infof("Trying to update account: %v", uai)

	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to find account by alias", err)

		// Record error
		uc.recordOnboardingError(ctx, "account", "find_error",
			attribute.String("account_id", id.String()),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == "external" {
		mopentelemetry.HandleSpanError(&span, "Cannot manipulate external account", constant.ErrForbiddenExternalAccountManipulation)

		// Record error
		uc.recordOnboardingError(ctx, "account", "validation_error",
			attribute.String("account_id", id.String()),
			attribute.String("error_detail", "forbidden_external_account_manipulation"))

		return nil, pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, reflect.TypeOf(mmodel.Account{}).Name())
	}

	account := &mmodel.Account{
		Name:        uai.Name,
		Status:      uai.Status,
		SegmentID:   uai.SegmentID,
		PortfolioID: uai.PortfolioID,
		Metadata:    uai.Metadata,
	}

	accountUpdated, err := uc.AccountRepo.Update(ctx, organizationID, ledgerID, portfolioID, id, account)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update account on repo by id", err)

		logger.Errorf("Error updating account on repo by id: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "account", "update_error",
			attribute.String("account_id", id.String()),
			attribute.String("error_detail", err.Error()))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Account{}).Name(), id.String(), uai.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update metadata", err)

		// Record error
		uc.recordOnboardingError(ctx, "account", "update_metadata_error",
			attribute.String("account_id", id.String()),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	accountUpdated.Metadata = metadataUpdated

	// Record successful completion and duration
	uc.recordOnboardingDuration(ctx, startTime, "account", "update", "success",
		attribute.String("account_id", id.String()),
		attribute.String("account_name", accountUpdated.Name),
		attribute.String("account_type", accFound.Type))

	return accountUpdated, nil
}
