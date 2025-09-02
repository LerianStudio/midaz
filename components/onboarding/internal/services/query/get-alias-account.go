package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// GetAccountByAlias get an Account from the repository by given alias (including soft-deleted ones).
func (uc *UseCase) GetAccountByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, alias string) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_by_alias")
	defer span.End()

	logger.Infof("Retrieving account for alias: %s", alias)

	account, err := uc.AccountRepo.FindAlias(ctx, organizationID, ledgerID, portfolioID, alias)
	if err != nil {
		logger.Errorf("Error getting account on repo by alias: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountAliasNotFound, reflect.TypeOf(mmodel.Account{}).Name())

			logger.Warnf("No accounts found for alias: %s", alias)

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get account on repo by alias", err)

			return nil, err
		}

		return nil, err
	}

	if account != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Account{}).Name(), alias)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb account", err)

			logger.Errorf("Error get metadata on mongodb account: %v", err)

			return nil, err
		}

		if metadata != nil {
			account.Metadata = metadata.Data
		}
	}

	return account, nil
}
