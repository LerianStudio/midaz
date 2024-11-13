package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	"github.com/google/uuid"
)

// ListAccountsByAlias get Accounts from the repository by given alias.
func (uc *UseCase) ListAccountsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Account, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.ListAccountsByAlias")
	defer span.End()

	logger.Infof("Retrieving account for alias: %s", aliases)

	accounts, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, aliases)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Accounts by aliases", err)

		logger.Errorf("Error getting accounts on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrFailedToRetrieveAccountsByAliases, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	return accounts, nil
}
