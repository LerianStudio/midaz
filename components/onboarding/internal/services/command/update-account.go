package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

const (
	accountTypeExternal = "external"
)

// UpdateAccount update an account from the repository by given id.
func (uc *UseCase) UpdateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, uai *mmodel.UpdateAccountInput) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_account")
	defer span.End()

	logger.Infof("Trying to update account: %v", uai)

	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		logger.Errorf("Error finding account by alias: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by alias", err)

		return nil, fmt.Errorf("failed to find: %w", err)
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == accountTypeExternal {
		return nil, fmt.Errorf("validation failed: %w", pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, reflect.TypeOf(mmodel.Account{}).Name()))
	}

	account := &mmodel.Account{
		Name:        uai.Name,
		Status:      uai.Status,
		EntityID:    uai.EntityID,
		SegmentID:   uai.SegmentID,
		PortfolioID: uai.PortfolioID,
		Metadata:    uai.Metadata,
		Blocked:     uai.Blocked,
	}

	accountUpdated, err := uc.AccountRepo.Update(ctx, organizationID, ledgerID, portfolioID, id, account)
	if err != nil {
		logger.Errorf("Error updating account on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())

			logger.Warnf("Account ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update account on repo by id", err)

			return nil, fmt.Errorf("validation failed: %w", err)
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update account on repo by id", err)

		return nil, fmt.Errorf("validation failed: %w", err)
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Account{}).Name(), id.String(), uai.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata", err)

		return nil, fmt.Errorf("operation failed: %w", err)
	}

	accountUpdated.Metadata = metadataUpdated

	return accountUpdated, nil
}
