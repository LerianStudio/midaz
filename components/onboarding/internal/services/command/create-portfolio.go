package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// CreatePortfolio creates a new portfolio persists data in the repository.
func (uc *UseCase) CreatePortfolio(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *mmodel.CreatePortfolioInput) (*mmodel.Portfolio, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.create_portfolio")
	defer span.End()

	// Record operation metrics
	uc.recordOnboardingMetrics(ctx, "portfolio", "create",
		attribute.String("portfolio_name", cpi.Name),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()))

	logger.Infof("Trying to create portfolio: %v", cpi)

	var status mmodel.Status
	if cpi.Status.IsEmpty() || pkg.IsNilOrEmpty(&cpi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	status.Description = cpi.Status.Description

	portfolio := &mmodel.Portfolio{
		ID:             pkg.GenerateUUIDv7().String(),
		EntityID:       cpi.EntityID,
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	port, err := uc.PortfolioRepo.Create(ctx, portfolio)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create portfolio", err)
		logger.Errorf("Error creating portfolio: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "portfolio", "creation_error",
			attribute.String("portfolio_name", cpi.Name),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), port.ID, cpi.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create portfolio metadata", err)
		logger.Errorf("Error creating portfolio metadata: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "portfolio", "metadata_error",
			attribute.String("portfolio_id", port.ID),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	port.Metadata = metadata

	// Record successful completion and duration
	uc.recordOnboardingDuration(ctx, startTime, "portfolio", "create", "success",
		attribute.String("portfolio_id", port.ID),
		attribute.String("portfolio_name", port.Name),
		attribute.String("entity_id", port.EntityID))

	return port, nil
}
