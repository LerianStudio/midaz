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
// Fetches the account from PostgreSQL and enriches it with custom metadata from MongoDB.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning the account
//   - ledgerID: The UUID of the ledger containing the account
//   - portfolioID: Optional portfolio UUID for scoped queries
//   - id: The UUID of the account to retrieve
//
// Returns:
//   - *mmodel.Account: The account with enriched metadata
//   - error: ErrAccountIDNotFound if not found, or repository errors
func (uc *UseCase) GetAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_by_id")
	defer span.End()

	logger.Infof("Retrieving account for id: %s", id)

	// Fetch account from PostgreSQL
	account, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, portfolioID, id)
	if err != nil {
		logger.Errorf("Error getting account on repo by id: %v", err)

		logger.Errorf("Error getting account on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	// Enrich with metadata from MongoDB if account exists
	if account != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Account{}).Name(), id.String())
		if err != nil {
			logger.Errorf("Error get metadata on mongodb account: %v", err)

			logger.Errorf("Error get metadata on mongodb account: %v", err)

			return nil, err
		}

		if metadata != nil {
			account.Metadata = metadata.Data
		}
	}

	return account, nil
}
