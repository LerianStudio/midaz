package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreatePortfolio creates a new portfolio and persists it in the repository.
//
// Portfolios are collections of accounts grouped together for specific business purposes,
// such as representing a customer's holdings, a department's accounts, a business unit's
// finances, or any other logical grouping. They help organize accounts and can be associated
// with external entities via the EntityID field.
//
// The function performs the following steps:
// 1. Validates and normalizes the portfolio status (defaults to ACTIVE)
// 2. Generates a UUIDv7 for time-ordered identification
// 3. Persists the portfolio to PostgreSQL
// 4. Stores custom metadata in MongoDB if provided
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning this portfolio
//   - ledgerID: The UUID of the ledger containing this portfolio
//   - cpi: The portfolio creation input containing all required fields
//
// Returns:
//   - *mmodel.Portfolio: The created portfolio with generated ID and metadata
//   - error: Persistence or metadata creation errors
func (uc *UseCase) CreatePortfolio(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *mmodel.CreatePortfolioInput) (*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_portfolio")
	defer span.End()

	logger.Infof("Trying to create portfolio: %v", cpi)

	// Step 1: Determine portfolio status, defaulting to ACTIVE if not specified
	var status mmodel.Status
	if cpi.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cpi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	status.Description = cpi.Status.Description

	// Step 2: Construct portfolio entity with generated UUIDv7 for time-ordered IDs
	portfolio := &mmodel.Portfolio{
		ID:             libCommons.GenerateUUIDv7().String(),
		EntityID:       cpi.EntityID,
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Step 3: Persist the portfolio to PostgreSQL
	port, err := uc.PortfolioRepo.Create(ctx, portfolio)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create portfolio", err)

		logger.Errorf("Error creating portfolio: %v", err)

		return nil, err
	}

	// Step 4: Store custom metadata in MongoDB if provided.
	// Metadata enables flexible extension (e.g., customer info, risk profile, region)
	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), port.ID, cpi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create portfolio metadata", err)

		logger.Errorf("Error creating portfolio metadata: %v", err)

		return nil, err
	}

	port.Metadata = metadata

	return port, nil
}
