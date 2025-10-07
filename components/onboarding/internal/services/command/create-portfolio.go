// Package command implements write operations (commands) for the onboarding service.
// This file contains the CreatePortfolio command implementation.
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

// CreatePortfolio creates a new portfolio and persists it to the repository.
//
// This method implements the create portfolio use case, which:
// 1. Sets default status to ACTIVE if not provided
// 2. Generates a UUIDv7 for the portfolio ID
// 3. Creates the portfolio in PostgreSQL
// 4. Creates associated metadata in MongoDB
// 5. Returns the complete portfolio with metadata
//
// Business Rules:
//   - Status defaults to ACTIVE if not provided or empty
//   - Name is required (validated at HTTP layer)
//   - Entity ID is optional (for linking to external systems)
//   - Organization and ledger must exist (validated by foreign key constraints)
//
// Portfolios are used to:
//   - Group related accounts (e.g., by business unit, department, client)
//   - Organize accounts for reporting purposes
//   - Link accounts to external entities via entity ID
//
// Data Storage:
//   - Primary data: PostgreSQL (portfolios table)
//   - Metadata: MongoDB (flexible key-value storage)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization that owns this portfolio
//   - ledgerID: UUID of the ledger that contains this portfolio
//   - cpi: Create portfolio input with name, entity ID, status, and metadata
//
// Returns:
//   - *mmodel.Portfolio: Created portfolio with metadata
//   - error: Business error if validation fails, database error if persistence fails
//
// Possible Errors:
//   - ErrLedgerIDNotFound: Ledger doesn't exist
//   - ErrOrganizationIDNotFound: Organization doesn't exist
//   - Database errors: Connection failures, constraint violations
//
// Example:
//
//	input := &mmodel.CreatePortfolioInput{
//	    Name:     "Corporate Accounts",
//	    EntityID: "EXT-CORP-001",
//	    Status:   mmodel.Status{Code: "ACTIVE"},
//	    Metadata: map[string]any{"department": "Treasury"},
//	}
//	portfolio, err := useCase.CreatePortfolio(ctx, orgID, ledgerID, input)
//
// OpenTelemetry:
//   - Creates span "command.create_portfolio"
//   - Records errors as span events
func (uc *UseCase) CreatePortfolio(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *mmodel.CreatePortfolioInput) (*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_portfolio")
	defer span.End()

	logger.Infof("Trying to create portfolio: %v", cpi)

	var status mmodel.Status
	if cpi.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cpi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	status.Description = cpi.Status.Description

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

	port, err := uc.PortfolioRepo.Create(ctx, portfolio)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create portfolio", err)

		logger.Errorf("Error creating portfolio: %v", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), port.ID, cpi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create portfolio metadata", err)

		logger.Errorf("Error creating portfolio metadata: %v", err)

		return nil, err
	}

	port.Metadata = metadata

	return port, nil
}
