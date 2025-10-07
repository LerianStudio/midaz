// Package query implements read operations (queries) for the onboarding service.
// This file contains query implementation.

package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// GetAccountByID retrieves a single account by ID with metadata.
//
// This method implements the get account query use case, which:
// 1. Fetches the account from PostgreSQL by ID
// 2. Fetches associated metadata from MongoDB
// 3. Merges metadata into the account object
// 4. Returns the enriched account
//
// Query Features:
//   - Retrieves single entity by UUID
//   - Automatically enriches with metadata
//   - Excludes soft-deleted accounts
//   - Supports portfolio filtering
//
// Behavior:
//   - Returns error if account not found
//   - Metadata is optional (account returned even if metadata fetch fails)
//   - Soft-deleted accounts are not returned
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - portfolioID: Optional portfolio ID filter
//   - id: UUID of the account to retrieve
//
// Returns:
//   - *mmodel.Account: Account with metadata
//   - error: Business error if not found or query fails
//
// Possible Errors:
//   - ErrAccountIDNotFound: Account doesn't exist or is deleted
//   - Database errors: Connection failures
//
// Example:
//
//	account, err := useCase.GetAccountByID(ctx, orgID, ledgerID, nil, accountID)
//	if err != nil {
//	    return nil, err
//	}
//	// Returns account with metadata
//
// OpenTelemetry:
//   - Creates span "query.get_account_by_id"
//
// BUG: Contains duplicate logger.Errorf calls (lines 29-30, 42-44). See BUGS.md.
func (uc *UseCase) GetAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_by_id")
	defer span.End()

	logger.Infof("Retrieving account for id: %s", id)

	account, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, portfolioID, id)
	if err != nil {
		logger.Errorf("Error getting account on repo by id: %v", err)

		logger.Errorf("Error getting account on repo by id: %v", err) // BUG: Duplicate log call

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	if account != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Account{}).Name(), id.String())
		if err != nil {
			logger.Errorf("Error get metadata on mongodb account: %v", err)

			logger.Errorf("Error get metadata on mongodb account: %v", err) // BUG: Duplicate log call

			return nil, err
		}

		if metadata != nil {
			account.Metadata = metadata.Data
		}
	}

	return account, nil
}
