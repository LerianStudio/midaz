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

// GetAccountByIDWithDeleted retrieves an account by ID including soft-deleted accounts.
//
// This method is similar to GetAccountByID but includes soft-deleted accounts in the results.
// Used for administrative purposes, audit queries, or when checking historical data.
//
// Special Behavior:
//   - Includes accounts with DeletedAt set (soft-deleted)
//   - Still enriches with metadata
//   - Used for validation and audit purposes
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - portfolioID: Optional portfolio ID filter
//   - id: UUID of the account to retrieve
//
// Returns:
//   - *mmodel.Account: Account with metadata (may be soft-deleted)
//   - error: Business error if not found or query fails
//
// Example:
//
//	// Check if account exists even if deleted
//	account, err := useCase.GetAccountByIDWithDeleted(ctx, orgID, ledgerID, nil, accountID)
//	if err != nil {
//	    // Account never existed
//	}
//	if account.DeletedAt != nil {
//	    // Account was deleted
//	}
//
// OpenTelemetry: Creates span "query.get_account_by_id_with_deleted"
func (uc *UseCase) GetAccountByIDWithDeleted(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_by_id_with_deleted")
	defer span.End()

	logger.Infof("Retrieving account for id: %s", id)

	account, err := uc.AccountRepo.FindWithDeleted(ctx, organizationID, ledgerID, portfolioID, id)
	if err != nil {
		logger.Errorf("Error getting account on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get account on repo by id", err)

			logger.Warn("No account found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get account on repo by id", err)

		return nil, err
	}

	if account != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Account{}).Name(), id.String())
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb account", err)

			logger.Warn("No metadata found")

			return nil, err
		}

		if metadata != nil {
			account.Metadata = metadata.Data
		}
	}

	return account, nil
}
