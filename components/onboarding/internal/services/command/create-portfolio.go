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

	// Create a new portfolio operation with telemetry
	portfolioID := pkg.GenerateUUIDv7().String() // Generate ID early for telemetry
	op := uc.Telemetry.NewPortfolioOperation("create", portfolioID)

	// Add important attributes for telemetry
	op.WithAttributes(
		attribute.String("portfolio_name", cpi.Name),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	// Add entity ID if provided
	if cpi.EntityID != "" {
		op.WithAttribute("entity_id", cpi.EntityID)
	}

	// Record system metric
	op.RecordSystemicMetric(ctx)

	// Start trace span for this operation
	ctx = op.StartTrace(ctx)

	defer func() {
		// End span will be done by op.End() at the end of the function
	}()

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
		ID:             portfolioID, // Use the previously generated ID
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
		mopentelemetry.HandleSpanError(&op.span, "Failed to create portfolio", err)
		logger.Errorf("Error creating portfolio: %v", err)

		// Record error
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "creation_error", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), port.ID, cpi.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to create portfolio metadata", err)
		logger.Errorf("Error creating portfolio metadata: %v", err)

		// Record error
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "metadata_error", err)

		return nil, err
	}

	port.Metadata = metadata

	// Mark operation as successful
	op.End(ctx, "success")

	return port, nil
}
