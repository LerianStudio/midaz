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
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataAccounts retrieves accounts filtered by metadata criteria.
//
// This function enables metadata-driven queries, allowing users to search for accounts
// based on custom metadata attributes (e.g., find all accounts where metadata.department = "Treasury").
//
// The query starts from MongoDB metadata collection and then hydrates full account
// entities from PostgreSQL. This inverts the typical query flow to support flexible
// metadata-based filtering.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning the accounts
//   - ledgerID: The UUID of the ledger containing the accounts
//   - portfolioID: Optional portfolio UUID for additional scoping
//   - filter: Query parameters including metadata filters (e.g., "metadata.department=Treasury")
//
// Returns:
//   - []*mmodel.Account: List of accounts matching the metadata criteria
//   - error: ErrNoAccountsFound if none match, or repository errors
func (uc *UseCase) GetAllMetadataAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, filter http.QueryHeader) ([]*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_accounts")
	defer span.End()

	logger.Infof("Retrieving accounts")

	// Step 1: Query MongoDB for metadata matching the filter criteria
	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Account{}).Name(), filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo", err)

		logger.Warn("No metadata found")

		return nil, err
	}

	// Step 2: Extract entity IDs from metadata results and build metadata map
	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	// Step 3: Batch fetch full account entities from PostgreSQL using the extracted IDs
	accounts, err := uc.AccountRepo.ListByIDs(ctx, organizationID, ledgerID, portfolioID, uuids)
	if err != nil {
		logger.Errorf("Error getting accounts on repo by query params: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())

			logger.Warn("No accounts found")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get accounts on repo", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get accounts on repo", err)

		return nil, err
	}

	// Step 4: Enrich accounts with their metadata
	for i := range accounts {
		if data, ok := metadataMap[accounts[i].ID]; ok {
			accounts[i].Metadata = data
		}
	}

	return accounts, nil
}
