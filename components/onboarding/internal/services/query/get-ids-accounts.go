package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"

	"github.com/google/uuid"
)

// ListAccountsByIDs get Accounts from the repository by given ids.
func (uc *UseCase) ListAccountsByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.ListAccountsByIDs")
	defer span.End()

	logger.Infof("Retrieving account for id: %s", ids)

	accounts, err := uc.AccountRepo.ListAccountsByIDs(ctx, organizationID, ledgerID, ids)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Accounts by ids", err)

		logger.Errorf("Error getting accounts on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrIDsNotFoundForAccounts, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	return accounts, nil
}
