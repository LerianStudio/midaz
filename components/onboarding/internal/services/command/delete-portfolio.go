// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for deleting a portfolio.
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
// This use case performs a soft delete by setting the DeletedAt timestamp.
// The portfolio record is preserved for auditing but is excluded from normal queries.
// Accounts referencing the portfolio are not automatically updated.
//
// Business Rules:
//   - The portfolio must exist and not have been previously deleted.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - id: The UUID of the portfolio to be deleted.
//
// Returns:
//   - error: An error if the portfolio is not found or if the deletion fails.
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
