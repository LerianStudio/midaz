package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// GetAccountTypeByID get an Account Type from the repository by given id.
func (uc *UseCase) GetAccountTypeByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountType, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_type_by_id")
	defer span.End()

	logger.Infof("Retrieving account type for id: %s", id)

	accountType, err := uc.AccountTypeRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get account type on repo by id", err)

		logger.Errorf("Error getting account type on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())
		}

		return nil, err
	}

	if accountType != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.AccountType{}).Name(), id.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb account type", err)

			logger.Errorf("Error get metadata on mongodb account type: %v", err)

			return nil, err
		}

		if metadata != nil {
			accountType.Metadata = metadata.Data
		}
	}

	return accountType, nil
}
