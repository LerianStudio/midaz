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

// GetAccountByIDWithDeleted retrieves an account by ID, including soft-deleted records.
//
// This query is specifically designed for scenarios where access to deleted accounts
// is required, such as:
//   - Audit trail reconstruction
//   - Historical balance verification
//   - Transaction reversal validation
//   - Compliance reporting
//
// Unlike GetAccountByID, this method does NOT filter out soft-deleted records,
// allowing retrieval of accounts that have been logically deleted but still
// exist in the database.
//
// Query Process:
//
//	Step 1: Context Extraction
//	  - Extract logger and tracer from context
//	  - Start tracing span "query.get_account_by_id_with_deleted"
//
//	Step 2: PostgreSQL Query
//	  - Call AccountRepo.FindWithDeleted (bypasses deleted_at filter)
//	  - Handle ErrDatabaseItemNotFound as "account not found"
//
//	Step 3: Metadata Enrichment
//	  - Fetch metadata from MongoDB by entity ID
//	  - Attach metadata to account if found
//
// Parameters:
//   - ctx: Context with observability (logger, tracer, metrics)
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID containing the account
//   - portfolioID: Optional portfolio UUID for filtering (nil = any portfolio)
//   - id: Account UUID to retrieve
//
// Returns:
//   - *mmodel.Account: Account with metadata (may have non-nil DeletedAt)
//   - error: Business error (ErrAccountIDNotFound) or infrastructure error
//
// Error Scenarios:
//   - ErrAccountIDNotFound: Account does not exist (even including deleted)
//   - Database errors: PostgreSQL connection or query failures
//   - Metadata errors: MongoDB query failures
//
// Security Note:
//
// Access to deleted accounts should be restricted to authorized users
// (e.g., auditors, compliance officers) as it may expose historical data.
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
