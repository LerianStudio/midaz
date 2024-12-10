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
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/google/uuid"
)

// GetAllAccount fetch all Account from the repository
func (uc *UseCase) GetAllAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, filter http.QueryHeader) ([]*mmodel.Account, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_account")
	defer span.End()

	logger.Infof("Retrieving accounts")

	accounts, err := uc.AccountRepo.FindAll(ctx, organizationID, ledgerID, portfolioID, filter.ToPagination())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get accounts on repo", err)

		logger.Errorf("Error getting accounts on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	if accounts != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Account{}).Name(), filter)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on repo", err)

			return nil, pkg.ValidateBusinessError(constant.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range accounts {
			if data, ok := metadataMap[accounts[i].ID]; ok {
				accounts[i].Metadata = data
			}
		}
	}

	return accounts, nil
}
