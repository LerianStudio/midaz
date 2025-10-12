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

// DeletePortfolioByID performs a soft delete of a portfolio from the repository.
//
// Soft deletes the portfolio by setting its DeletedAt timestamp. Accounts associated
// with this portfolio are not automatically deleted; they remain in the system but
// lose their portfolio association.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning the portfolio
//   - ledgerID: The UUID of the ledger containing the portfolio
//   - id: The UUID of the portfolio to delete
//
// Returns:
//   - error: ErrPortfolioIDNotFound if not found, or repository errors
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
