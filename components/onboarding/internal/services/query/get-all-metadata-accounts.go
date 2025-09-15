package query

import (
    "context"
    "errors"
    "reflect"
    "time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
    libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
    "github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
    "github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
    "github.com/LerianStudio/midaz/v3/pkg"
    "github.com/LerianStudio/midaz/v3/pkg/constant"
    "github.com/LerianStudio/midaz/v3/pkg/mmodel"
    "github.com/LerianStudio/midaz/v3/pkg/net/http"
    "github.com/google/uuid"
)

// GetAllMetadataAccounts fetch all Accounts from the repository
func (uc *UseCase) GetAllMetadataAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, filter http.QueryHeader) ([]*mmodel.Account, error) {
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_accounts")
	defer span.End()

	logger.Infof("Retrieving accounts")

    var metadata []*mongodb.Metadata
    var err error
    for attempt := 0; attempt < 50; attempt++ {
        metadata, err = uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Account{}).Name(), filter)
        if err == nil && metadata != nil && len(metadata) > 0 { break }
        time.Sleep(100 * time.Millisecond)
    }
    logger.Infof("Accounts metadata query: use=%v filter=%v results=%d", filter.UseMetadata, filter.Metadata, len(metadata))
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo", err)

		logger.Warn("No metadata found")

		return nil, err
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

    accounts, err := uc.AccountRepo.ListByIDs(ctx, organizationID, ledgerID, portfolioID, uuids)
    if err != nil {
		logger.Errorf("Error getting accounts on repo by query params: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())

			logger.Warn("No accounts found")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get accounts on repo", err)

			return nil, err
    }

    logger.Infof("Accounts fetched by IDs for org=%s ledger=%s -> %d", organizationID.String(), ledgerID.String(), len(accounts))

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get accounts on repo", err)

		return nil, err
	}

	for i := range accounts {
		if data, ok := metadataMap[accounts[i].ID]; ok {
			accounts[i].Metadata = data
		}
	}

	return accounts, nil
}
