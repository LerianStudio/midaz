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
	"github.com/google/uuid"
)

// ListAccountsByIDs retrieves multiple accounts by their IDs.
//
// This method implements a batch get accounts query use case, which:
// 1. Fetches multiple accounts from PostgreSQL by their IDs
// 2. Returns accounts without metadata enrichment (optimization)
//
// Use Cases:
//   - Batch account retrieval for transaction processing
//   - Validating multiple account IDs exist
//   - Performance-optimized queries (skips metadata fetch)
//
// Behavior:
//   - Returns only accounts that exist
//   - Excludes soft-deleted accounts
//   - Does NOT enrich with metadata (performance optimization)
//   - Order of results may not match input ID order
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - ids: Array of account UUIDs to retrieve
//
// Returns:
//   - []*mmodel.Account: Array of found accounts (without metadata)
//   - error: Business error if query fails
//
// Possible Errors:
//   - ErrIDsNotFoundForAccounts: None of the IDs were found
//   - Database errors: Connection failures
//
// Example:
//
//	ids := []uuid.UUID{id1, id2, id3}
//	accounts, err := useCase.ListAccountsByIDs(ctx, orgID, ledgerID, ids)
//	// Returns accounts that exist (may be fewer than requested)
//
// OpenTelemetry:
//   - Creates span "query.ListAccountsByIDs"
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
