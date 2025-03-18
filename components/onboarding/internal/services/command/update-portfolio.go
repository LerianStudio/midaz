package command

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// UpdatePortfolioByID update a portfolio from the repository by given id.
func (uc *UseCase) UpdatePortfolioByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, upi *mmodel.UpdatePortfolioInput) (*mmodel.Portfolio, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.update_portfolio_by_id")
	defer span.End()

	// Record operation metrics
	uc.recordOnboardingMetrics(ctx, "portfolio", "update",
		attribute.String("portfolio_id", id.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()))

	logger.Infof("Trying to update portfolio: %v", upi)

	portfolio := &mmodel.Portfolio{
		Name:   upi.Name,
		Status: upi.Status,
	}

	portfolioUpdated, err := uc.PortfolioRepo.Update(ctx, organizationID, ledgerID, id, portfolio)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update portfolio on repo by id", err)

		logger.Errorf("Error updating portfolio on repo by id: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "portfolio", "update_error",
			attribute.String("portfolio_id", id.String()),
			attribute.String("error_detail", err.Error()))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), id.String(), upi.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update metadata on repo by id", err)

		// Record error
		uc.recordOnboardingError(ctx, "portfolio", "update_metadata_error",
			attribute.String("portfolio_id", id.String()),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	portfolioUpdated.Metadata = metadataUpdated

	// Record successful completion and duration
	uc.recordOnboardingDuration(ctx, startTime, "portfolio", "update", "success",
		attribute.String("portfolio_id", id.String()),
		attribute.String("portfolio_name", portfolioUpdated.Name))

	return portfolioUpdated, nil
}
