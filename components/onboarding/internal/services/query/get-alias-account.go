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

// GetAccountByAlias retrieves a single account by alias with metadata.
//
// This method implements the get account by alias query use case, which:
// 1. Fetches the account from PostgreSQL by alias
// 2. Fetches associated metadata from MongoDB
// 3. Merges metadata into the account object
// 4. Returns the enriched account
//
// Special Behavior:
//   - Includes soft-deleted accounts (unlike GetAccountByID)
//   - This is intentional for alias uniqueness validation
//   - Used during account creation to check if alias is available
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - portfolioID: Optional portfolio ID filter
//   - alias: Account alias to search for (e.g., "@corporate_checking")
//
// Returns:
//   - *mmodel.Account: Account with metadata (may be soft-deleted)
//   - error: Business error if not found or query fails
//
// Possible Errors:
//   - ErrAccountAliasNotFound: No account with this alias exists
//
// Example:
//
//	account, err := useCase.GetAccountByAlias(ctx, orgID, ledgerID, nil, "@corporate_checking")
//	if err != nil {
//	    // Alias is available
//	}
//
// OpenTelemetry:
//   - Creates span "query.get_account_by_alias"
func (uc *UseCase) GetAccountByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, alias string) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_by_alias")
	defer span.End()

	logger.Infof("Retrieving account for alias: %s", alias)

	account, err := uc.AccountRepo.FindAlias(ctx, organizationID, ledgerID, portfolioID, alias)
	if err != nil {
		logger.Errorf("Error getting account on repo by alias: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountAliasNotFound, reflect.TypeOf(mmodel.Account{}).Name())

			logger.Warnf("No accounts found for alias: %s", alias)

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get account on repo by alias", err)

			return nil, err
		}

		return nil, err
	}

	if account != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Account{}).Name(), alias)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb account", err)

			logger.Errorf("Error get metadata on mongodb account: %v", err)

			return nil, err
		}

		if metadata != nil {
			account.Metadata = metadata.Data
		}
	}

	return account, nil
}
