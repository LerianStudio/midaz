// Package command implements write operations (commands) for the onboarding service.
// This file contains the CreateLedger command implementation.
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

// CreateLedger creates a new ledger and persists it to the repository.
//
// This method implements the create ledger use case, which:
// 1. Checks if a ledger with the same name already exists (returns error if found)
// 2. Sets default status to ACTIVE if not provided
// 3. Creates the ledger in PostgreSQL
// 4. Creates associated metadata in MongoDB
// 5. Returns the complete ledger with metadata
//
// Business Rules:
//   - Ledger names must be unique within an organization
//   - Status defaults to ACTIVE if not provided or empty
//   - Organization must exist (validated by foreign key constraint)
//   - Name is required (validated at HTTP layer)
//
// Data Storage:
//   - Primary data: PostgreSQL (ledgers table)
//   - Metadata: MongoDB (flexible key-value storage)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization that will own this ledger
//   - cli: Create ledger input with name, status, and metadata
//
// Returns:
//   - *mmodel.Ledger: Created ledger with metadata
//   - error: Business error if validation fails, database error if persistence fails
//
// Possible Errors:
//   - ErrLedgerNameConflict: Ledger with same name already exists
//   - ErrOrganizationIDNotFound: Organization does not exist
//   - Database errors: Connection failures, constraint violations
//
// Example:
//
//	input := &mmodel.CreateLedgerInput{
//	    Name: "Treasury Operations",
//	    Status: mmodel.Status{Code: "ACTIVE"},
//	    Metadata: map[string]any{"department": "Finance"},
//	}
//	ledger, err := useCase.CreateLedger(ctx, orgID, input)
//	if err != nil {
//	    return nil, err
//	}
//
// OpenTelemetry:
//   - Creates span "command.create_ledger"
//   - Records errors as span events
func (uc *UseCase) CreateLedger(ctx context.Context, organizationID uuid.UUID, cli *mmodel.CreateLedgerInput) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_ledger")
	defer span.End()

	logger.Infof("Trying to create ledger: %v", cli)

	var status mmodel.Status
	if cli.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cli.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cli.Status
	}

	status.Description = cli.Status.Description

	_, err := uc.LedgerRepo.FindByName(ctx, organizationID, cli.Name)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find ledger by name", err)

		logger.Errorf("Error creating ledger: %v", err)

		return nil, err
	}

	ledger := &mmodel.Ledger{
		OrganizationID: organizationID.String(),
		Name:           cli.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	led, err := uc.LedgerRepo.Create(ctx, ledger)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create ledger", err)

		logger.Errorf("Error creating ledger: %v", err)

		return nil, err
	}

	takeName := reflect.TypeOf(mmodel.Ledger{}).Name()

	metadata, err := uc.CreateMetadata(ctx, takeName, led.ID, cli.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create ledger metadata", err)

		logger.Errorf("Error creating ledger metadata: %v", err)

		return nil, err
	}

	led.Metadata = metadata

	return led, nil
}
