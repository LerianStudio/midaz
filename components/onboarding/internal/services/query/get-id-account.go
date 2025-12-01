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

// GetAccountByID retrieves a single account by its unique identifier.
//
// Accounts are the fundamental entities that hold balances and participate in
// transactions. This method fetches a specific account with its associated
// metadata, providing the complete account configuration.
//
// Domain Context:
//
// An account represents:
//   - A balance-holding entity (e.g., customer wallet, merchant settlement)
//   - A node in the double-entry accounting system
//   - A participant in financial transactions (source or destination)
//   - Custom attributes via metadata (external IDs, tags, etc.)
//
// Account Hierarchy:
//
//	Organization
//	  -> Ledger
//	       -> Portfolio (optional grouping)
//	            -> Account
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for distributed tracing
//	  - Log the account ID being retrieved
//
//	Step 2: Fetch Account from PostgreSQL
//	  - Query by organization, ledger, optional portfolio, and account ID
//	  - Handle not-found with business error
//	  - Handle other errors as infrastructure errors
//
//	Step 3: Fetch Metadata from MongoDB (if account found)
//	  - Query metadata document by account ID
//	  - Assign metadata to account if present
//	  - Handle metadata errors as infrastructure errors
//
// Parameters:
//   - ctx: Request context with tenant and tracing information
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID to scope the account
//   - portfolioID: Optional portfolio UUID (nil if account not in portfolio)
//   - id: Account UUID to retrieve
//
// Returns:
//   - *mmodel.Account: Complete account with metadata
//   - error: Business error (not found) or infrastructure error
//
// Error Scenarios:
//   - ErrAccountIDNotFound: Account does not exist
//   - Database error: PostgreSQL connection or query failure
//   - Metadata error: MongoDB query failure
//
// Portfolio Scoping:
//
// The portfolioID parameter enables portfolio-scoped queries:
//   - If nil: Account is queried directly under the ledger
//   - If provided: Account must belong to the specified portfolio
//
// This supports both portfolio-based and flat account structures.
func (uc *UseCase) GetAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_by_id")
	defer span.End()

	logger.Infof("Retrieving account for id: %s", id)

	account, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, portfolioID, id)
	if err != nil {
		logger.Errorf("Error getting account on repo by id: %v", err)

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

			logger.Errorf("Error get metadata on mongodb account: %v", err)

			return nil, err
		}

		if metadata != nil {
			account.Metadata = metadata.Data
		}
	}

	return account, nil
}
