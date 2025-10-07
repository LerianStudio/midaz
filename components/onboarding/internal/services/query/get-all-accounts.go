// Package query implements read operations (queries) for the onboarding service.
// This file contains query implementation.

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

// GetAllAccount retrieves a paginated list of accounts with metadata.
//
// This method implements the list accounts query use case, which:
// 1. Fetches accounts from PostgreSQL with pagination
// 2. Fetches metadata for all accounts from MongoDB in batch
// 3. Merges metadata into account objects
// 4. Returns enriched accounts
//
// Query Features:
//   - Pagination: Supports limit and page parameters
//   - Sorting: Supports sort order (asc/desc)
//   - Date filtering: Supports start_date and end_date
//   - Portfolio filtering: Optional portfolio ID filter
//   - Metadata enrichment: Automatically fetches and merges metadata
//
// Behavior:
//   - Returns empty array if no accounts found (not an error)
//   - Metadata is optional (accounts without metadata are still returned)
//   - Soft-deleted accounts are excluded (WHERE deleted_at IS NULL)
//   - Batch fetches metadata for performance
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - portfolioID: Optional portfolio ID filter
//   - filter: Query parameters including pagination, sorting, and date range
//
// Returns:
//   - []*mmodel.Account: Array of accounts with metadata
//   - error: Business error if query fails
//
// Possible Errors:
//   - ErrNoAccountsFound: No accounts match the query (only on database error)
//   - Database errors: Connection failures
//
// Example:
//
//	filter := http.QueryHeader{
//	    Limit:     50,
//	    Page:      1,
//	    SortOrder: "desc",
//	}
//	accounts, err := useCase.GetAllAccount(ctx, orgID, ledgerID, nil, filter)
//	if err != nil {
//	    return nil, err
//	}
//	// Returns accounts with metadata merged
//
// OpenTelemetry:
//   - Creates span "query.get_all_account"
//   - Records errors as span events
func (uc *UseCase) GetAllAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, filter http.QueryHeader) ([]*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_account")
	defer span.End()

	logger.Infof("Retrieving accounts")

	accounts, err := uc.AccountRepo.FindAll(ctx, organizationID, ledgerID, portfolioID, filter.ToOffsetPagination())
	if err != nil {
		logger.Errorf("Error getting accounts on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())

			logger.Warn("No accounts found")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get accounts on repo", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get accounts on repo", err)

		return nil, err
	}

	if len(accounts) == 0 {
		return accounts, nil
	}

	accountIDs := make([]string, len(accounts))
	for i, a := range accounts {
		accountIDs[i] = a.ID
	}

	metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(mmodel.Account{}).Name(), accountIDs)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo", err)

		return nil, err
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

	return accounts, nil
}
