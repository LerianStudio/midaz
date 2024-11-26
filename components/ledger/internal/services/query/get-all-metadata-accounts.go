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

// GetAllMetadataAccounts fetch all Accounts from the repository
func (uc *UseCase) GetAllMetadataAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, filter http.QueryHeader) ([]*mmodel.Account, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_accounts")
	defer span.End()

	logger.Infof("Retrieving accounts")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Account{}).Name(), filter)
	if err != nil || metadata == nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get metadata on repo", err)

		return nil, pkg.ValidateBusinessError(constant.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	accounts, err := uc.AccountRepo.ListByIDs(ctx, organizationID, ledgerID, portfolioID, uuids)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get accounts on repo", err)

		logger.Errorf("Error getting accounts on repo by query params: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	for i := range accounts {
		if data, ok := metadataMap[accounts[i].ID]; ok {
			accounts[i].Metadata = data
		}
	}

	return accounts, nil
}
