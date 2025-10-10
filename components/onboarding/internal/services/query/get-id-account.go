// Package query implements read operations (queries) for the onboarding service.
// This file contains the query for retrieving an account by its ID.
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

// GetAccountByID retrieves a single account by its ID, enriched with metadata.
//
// This use case fetches an account from the PostgreSQL database and its corresponding
// metadata from MongoDB, then merges them into a single response.
// Soft-deleted accounts are excluded from the result.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - portfolioID: An optional portfolio ID to filter the account.
//   - id: The UUID of the account to retrieve.
//
// Returns:
//   - *mmodel.Account: The account with its metadata, or nil if not found.
//   - error: An error if the account is not found or if the query fails.
//
// FIXME: The function has duplicate logger.Errorf calls in both error handling
// blocks. The redundant calls should be removed to clean up the code.
func (uc *UseCase) GetAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_by_id")
	defer span.End()

	logger.Infof("Retrieving account for id: %s", id)

	account, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, portfolioID, id)
	if err != nil {
		logger.Errorf("Error getting account on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	if account != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Account{}).Name(), id.String())
		if err != nil {
			logger.Errorf("Error get metadata on mongodb account: %v", err)

			return nil, err
		}

		if metadata != nil {
			account.Metadata = metadata.Data
		}
	}

	return account, nil
}
