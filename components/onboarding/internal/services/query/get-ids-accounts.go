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

// ListAccountsByIDs get Accounts from the repository by given ids.
func (uc *UseCase) ListAccountsByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.ListAccountsByIDs")
	defer span.End()

	logger.Infof("Retrieving account for id: %s", ids)

	accounts, err := uc.AccountRepo.ListAccountsByIDs(ctx, organizationID, ledgerID, ids)
	if err != nil {
		logger.Errorf("Error getting accounts on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrIDsNotFoundForAccounts, reflect.TypeOf(mmodel.Account{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve Accounts by ids", err)

			logger.Warn("No accounts found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve Accounts by ids", err)

		return nil, err
	}

	return accounts, nil
}
