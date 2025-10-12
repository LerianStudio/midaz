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

// CreateLedger creates a new ledger and persists it in the repository.
//
// Ledgers are organizational units within an organization that group related financial
// accounts and assets. They represent separate books of accounts and enable multi-entity
// accounting within a single organization (e.g., different business units, subsidiaries, etc.).
//
// The function performs the following steps:
// 1. Validates and normalizes the ledger status (defaults to ACTIVE)
// 2. Checks for name uniqueness within the organization
// 3. Persists the ledger to PostgreSQL
// 4. Stores custom metadata in MongoDB if provided
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization that will own this ledger
//   - cli: The ledger creation input containing all required fields
//
// Returns:
//   - *mmodel.Ledger: The created ledger with generated ID and metadata
//   - error: Business validation or persistence errors (e.g., duplicate name)
func (uc *UseCase) CreateLedger(ctx context.Context, organizationID uuid.UUID, cli *mmodel.CreateLedgerInput) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_ledger")
	defer span.End()

	logger.Infof("Trying to create ledger: %v", cli)

	// Step 1: Determine ledger status, defaulting to ACTIVE if not specified.
	// All ledgers must have a valid operational status.
	var status mmodel.Status
	if cli.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cli.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cli.Status
	}

	status.Description = cli.Status.Description

	// Step 2: Verify ledger name uniqueness within the organization.
	// Duplicate ledger names within the same organization are not allowed
	// to prevent confusion and ensure clear identification.
	_, err := uc.LedgerRepo.FindByName(ctx, organizationID, cli.Name)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find ledger by name", err)

		logger.Errorf("Error creating ledger: %v", err)

		return nil, err
	}

	// Step 3: Construct the ledger entity with validated fields and timestamps
	ledger := &mmodel.Ledger{
		OrganizationID: organizationID.String(),
		Name:           cli.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Step 4: Persist the ledger to PostgreSQL
	led, err := uc.LedgerRepo.Create(ctx, ledger)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create ledger", err)

		logger.Errorf("Error creating ledger: %v", err)

		return nil, err
	}

	takeName := reflect.TypeOf(mmodel.Ledger{}).Name()

	// Step 5: Store custom metadata in MongoDB if provided.
	// Metadata provides flexible extension of ledger attributes (e.g., currency, region, etc.)
	metadata, err := uc.CreateMetadata(ctx, takeName, led.ID, cli.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create ledger metadata", err)

		logger.Errorf("Error creating ledger metadata: %v", err)

		return nil, err
	}

	led.Metadata = metadata

	return led, nil
}
