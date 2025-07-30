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
	"go.opentelemetry.io/otel/attribute"
	"reflect"
)

// UpdateAccount update an account from the repository by given id.
func (uc *UseCase) UpdateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, uai *mmodel.UpdateAccountInput) (*mmodel.Account, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_account")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", id.String()),
	)

	logger.Infof("Trying to update account: %v", uai)

	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to find account by alias", err)

		return nil, err
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == "external" {
		return nil, pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, reflect.TypeOf(mmodel.Account{}).Name())
	}

	account := &mmodel.Account{
		Name:        uai.Name,
		Status:      uai.Status,
		EntityID:    uai.EntityID,
		SegmentID:   uai.SegmentID,
		PortfolioID: uai.PortfolioID,
		Metadata:    uai.Metadata,
	}

	accountUpdated, err := uc.AccountRepo.Update(ctx, organizationID, ledgerID, portfolioID, id, account)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update account on repo by id", err)

		logger.Errorf("Error updating account on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Account{}).Name(), id.String(), uai.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update metadata", err)

		return nil, err
	}

	accountUpdated.Metadata = metadataUpdated

	return accountUpdated, nil
}
