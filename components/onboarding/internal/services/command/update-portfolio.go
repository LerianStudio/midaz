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

// UpdatePortfolioByID updates an existing portfolio in the repository.
//
// This function performs a partial update of portfolio properties. Only the fields
// provided in the input will be updated; omitted fields remain unchanged.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning the portfolio
//   - ledgerID: The UUID of the ledger containing the portfolio
//   - id: The UUID of the portfolio to update
//   - upi: The update input containing fields to modify (name, entityID, status, metadata)
//
// Returns:
//   - *mmodel.Portfolio: The updated portfolio with refreshed metadata
//   - error: ErrPortfolioIDNotFound if not found, or repository errors
func (uc *UseCase) UpdatePortfolioByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, upi *mmodel.UpdatePortfolioInput) (*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_portfolio_by_id")
	defer span.End()

	logger.Infof("Trying to update portfolio: %v", upi)

	portfolio := &mmodel.Portfolio{
		EntityID: upi.EntityID,
		Name:     upi.Name,
		Status:   upi.Status,
	}

	portfolioUpdated, err := uc.PortfolioRepo.Update(ctx, organizationID, ledgerID, id, portfolio)
	if err != nil {
		logger.Errorf("Error updating portfolio on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

			logger.Warnf("Portfolio ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update portfolio on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update portfolio on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), id.String(), upi.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	portfolioUpdated.Metadata = metadataUpdated

	return portfolioUpdated, nil
}
