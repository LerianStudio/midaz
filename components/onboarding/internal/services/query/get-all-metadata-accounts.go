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

// GetAllMetadataAccounts retrieves accounts filtered by metadata criteria.
//
// This is a metadata-first query that:
// 1. Queries MongoDB for accounts matching metadata filters
// 2. Extracts entity IDs from metadata results
// 3. Fetches corresponding accounts from PostgreSQL
// 4. Merges metadata into account objects
//
// Use Case: Searching accounts by custom metadata fields (e.g., department, cost_center)
//
// Query Flow (Reverse of normal):
//   - Normal: PostgreSQL → MongoDB (fetch then enrich)
//   - Metadata: MongoDB → PostgreSQL (filter then fetch)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - portfolioID: Optional portfolio ID filter
//   - filter: Query parameters including metadata filters (e.g., metadata.department=Sales)
//
// Returns:
//   - []*mmodel.Account: Array of accounts matching metadata criteria
//   - error: Business error if query fails
//
// Example:
//
//	filter := http.QueryHeader{
//	    Metadata: &bson.M{"department": "Finance"},
//	}
//	accounts, err := useCase.GetAllMetadataAccounts(ctx, orgID, ledgerID, nil, filter)
//	// Returns only accounts with metadata.department = "Finance"
//
// OpenTelemetry: Creates span "query.get_all_metadata_accounts"
func (uc *UseCase) GetAllMetadataAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, filter http.QueryHeader) ([]*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_accounts")
	defer span.End()

	logger.Infof("Retrieving accounts")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Account{}).Name(), filter)
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
