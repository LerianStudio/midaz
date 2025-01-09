package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
)

// GetAccountByAlias get an Account from the repository by given alias (including soft-deleted ones).
func (uc *UseCase) GetAccountByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, alias string) (*mmodel.Account, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_by_alias")

	logger.Infof("Retrieving account for alias: %s", alias)

	account, err := uc.AccountRepo.FindAlias(ctx, organizationID, ledgerID, portfolioID, alias)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get account on repo by alias", err)

		logger.Errorf("Error getting account on repo by alias: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrAccountAliasNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	if account != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Account{}).Name(), alias)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb account", err)

			logger.Errorf("Error get metadata on mongodb account: %v", err)

			return nil, err
		}

		if metadata != nil {
			account.Metadata = metadata.Data
		}
	}

	return account, nil
}
