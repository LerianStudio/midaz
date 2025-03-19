package command

import (
	"context"
	"errors"
	"reflect"

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

	op := uc.Telemetry.NewAccountOperation("update", id.String())

	op.WithAttributes(
		attribute.String("account_id", id.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	if portfolioID != nil {
		op.WithAttribute("portfolio_id", portfolioID.String())
	}

	op.RecordSystemicMetric(ctx)
	ctx = op.StartTrace(ctx)

	logger.Infof("Trying to update account: %v", uai)

	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to find account by alias", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "find_error", err)

		return nil, err
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == "external" {
		mopentelemetry.HandleSpanError(&op.span, "Cannot manipulate external account", constant.ErrForbiddenExternalAccountManipulation)
		op.WithAttribute("error_detail", "forbidden_external_account_manipulation")
		op.RecordError(ctx, "validation_error", constant.ErrForbiddenExternalAccountManipulation)

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
		mopentelemetry.HandleSpanError(&op.span, "Failed to update account on repo by id", err)
		logger.Errorf("Error updating account on repo by id: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "update_error", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	op.WithAttribute("account_name", accountUpdated.Name)
	op.WithAttribute("account_type", accFound.Type)

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Account{}).Name(), id.String(), uai.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to update metadata", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "update_metadata_error", err)

		return nil, err
	}

	accountUpdated.Metadata = metadataUpdated

	op.End(ctx, "success")

	return accountUpdated, nil
}
