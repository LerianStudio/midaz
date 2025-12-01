// Package command provides CQRS command handlers for the onboarding component.
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
// Portfolios group accounts belonging to a single entity (customer, counterparty).
// They provide a logical container for managing all accounts associated with
// one business relationship.
//
// # Portfolio Purpose
//
// Portfolios serve several functions:
//   - Group all accounts for a single customer/entity
//   - Enable customer-level reporting and analytics
//   - Support regulatory reporting (KYC/AML grouping)
//   - Facilitate account hierarchy management
//
// # EntityID
//
// The EntityID field links the portfolio to an external entity (customer ID,
// counterparty ID, etc.). This enables:
//   - Cross-referencing with CRM systems
//   - Customer-level balance aggregation
//   - Compliance reporting
//
// # Status Management
//
// If no status is provided, portfolios default to "ACTIVE".
// Supported statuses typically include:
//   - ACTIVE: Portfolio accepts new accounts and transactions
//   - INACTIVE: Portfolio is disabled
//   - CLOSED: Portfolio is permanently closed
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.create_portfolio"
//  3. Set default status to "ACTIVE" if not provided
//  4. Generate UUIDv7 for the new portfolio
//  5. Build portfolio model with timestamps
//  6. Persist portfolio to PostgreSQL via repository
//  7. Create associated metadata in MongoDB (if provided)
//  8. Return created portfolio with metadata
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - organizationID: The organization that owns this ledger (tenant isolation)
//   - ledgerID: The ledger where this portfolio will be created
//   - cpi: CreatePortfolioInput containing portfolio details
//
// # Input Fields
//
//   - EntityID: External entity identifier (customer/counterparty ID)
//   - Name: Human-readable portfolio name
//   - Status: Portfolio status (defaults to ACTIVE)
//   - Metadata: Optional key-value pairs for additional data
//
// # Returns
//
//   - *mmodel.Portfolio: The created portfolio with generated ID
//   - error: If database operations fail
//
// # Error Scenarios
//
//   - Database connection failure during Create
//   - Metadata creation failure (MongoDB)
//   - Context cancellation/timeout
//
// # Observability
//
// Creates tracing span "command.create_portfolio" with error events on failure.
// Logs operation progress and any errors encountered.
//
// # Example
//
//	input := &mmodel.CreatePortfolioInput{
//	    EntityID: "customer-123",
//	    Name:     "John Doe Portfolio",
//	    Metadata: map[string]any{"segment": "retail"},
//	}
//	portfolio, err := uc.CreatePortfolio(ctx, orgID, ledgerID, input)
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
