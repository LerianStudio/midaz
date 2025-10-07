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

// CountAccounts returns the total count of accounts for pagination.
//
// This method implements the count accounts query use case, which:
// 1. Counts total accounts in PostgreSQL for the given organization and ledger
// 2. Excludes soft-deleted accounts
// 3. Returns the count for X-Total-Count header
//
// This count is used for:
//   - Pagination metadata (total pages calculation)
//   - X-Total-Count HTTP header
//   - Client-side pagination UI
//
// Behavior:
//   - Returns 0 if no accounts exist (not an error)
//   - Excludes soft-deleted accounts (WHERE deleted_at IS NULL)
//   - Count is not affected by pagination parameters
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//
// Returns:
//   - int64: Total count of active accounts
//   - error: Business error if query fails
//
// Possible Errors:
//   - ErrNoAccountsFound: Database error occurred (not for zero count)
//   - Database errors: Connection failures
//
// Example:
//
//	count, err := useCase.CountAccounts(ctx, orgID, ledgerID)
//	if err != nil {
//	    return 0, err
//	}
//	// Use count for X-Total-Count header
//	c.Set(constant.XTotalCount, strconv.FormatInt(count, 10))
//
// OpenTelemetry:
//   - Creates span "query.count_accounts"
//   - Records errors as span events
func (uc *UseCase) CountAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_accounts")
	defer span.End()

	count, err := uc.AccountRepo.Count(ctx, organizationID, ledgerID)
	if err != nil {
		logger.Errorf("Error counting accounts on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())

			logger.Warnf("No accounts found for organization: %s", organizationID.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to count accounts on repo", err)

			return 0, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to count accounts on repo", err)

		return 0, err
	}

	return count, nil
}
