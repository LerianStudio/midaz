// Package command implements write operations (commands) for the onboarding service.
// This file contains command implementation.

package command

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

// DeletePortfolioByID soft-deletes a portfolio from the repository.
//
// This method implements the delete portfolio use case, which performs a soft delete
// by setting the DeletedAt timestamp. The portfolio record remains in the database
// but is excluded from normal queries.
//
// Business Rules:
//   - Portfolio must exist and not be already deleted
//   - Portfolio should not have active accounts referencing it
//   - Soft delete is idempotent (deleting already deleted portfolio returns error)
//
// Soft Deletion:
//   - Sets DeletedAt timestamp to current time
//   - Portfolio remains in database for audit purposes
//   - Excluded from list and get operations (WHERE deleted_at IS NULL)
//   - Can be used for historical reporting
//   - Cannot be undeleted (no restore operation)
//
// Cascade Behavior:
//   - Accounts referencing this portfolio are NOT automatically updated
//   - Consider updating accounts to remove portfolio reference before deletion
//   - Foreign key constraints may prevent deletion if accounts reference it
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - id: UUID of the portfolio to delete
//
// Returns:
//   - error: Business error if portfolio not found, database error if deletion fails
//
// Possible Errors:
//   - ErrPortfolioIDNotFound: Portfolio doesn't exist or already deleted
//   - Database errors: Foreign key violations, connection failures
//
// Example:
//
//	err := useCase.DeletePortfolioByID(ctx, orgID, ledgerID, portfolioID)
//	if err != nil {
//	    return err
//	}
//	// Portfolio is soft-deleted
//
// OpenTelemetry:
//   - Creates span "command.delete_portfolio_by_id"
//   - Records errors as span events
func (uc *UseCase) DeletePortfolioByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_portfolio_by_id")
	defer span.End()

	logger.Infof("Remove portfolio for id: %s", id.String())

	if err := uc.PortfolioRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

			logger.Warnf("Portfolio ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete portfolio on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete portfolio on repo by id", err)

		logger.Errorf("Error deleting portfolio: %v", err)

		return err
	}

	return nil
}
