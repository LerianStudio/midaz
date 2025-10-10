// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for creating a new portfolio.
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

// CreatePortfolio creates a new portfolio in the repository.
//
// This use case is responsible for:
// 1. Setting a default status of "ACTIVE" if none is provided.
// 2. Generating a UUIDv7 for the new portfolio.
// 3. Persisting the portfolio in the PostgreSQL database.
// 4. Storing any associated metadata in MongoDB.
// 5. Returning the newly created portfolio, including its metadata.
//
// Portfolios are used to group related accounts, often for reporting or organizational purposes.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization that owns the portfolio.
//   - ledgerID: The UUID of the ledger where the portfolio will be created.
//   - cpi: The input data for creating the portfolio.
//
// Returns:
//   - *mmodel.Portfolio: The created portfolio, complete with its metadata.
//   - error: An error if the persistence fails.
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
