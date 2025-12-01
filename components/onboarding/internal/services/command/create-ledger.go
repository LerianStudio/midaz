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

// CreateLedger creates a new ledger and persists it to the repository.
//
// Ledgers are the primary accounting containers within an organization.
// They provide isolation for financial data and can represent different
// business units, currencies, or accounting periods.
//
// # Ledger Purpose
//
// A ledger groups related financial entities:
//   - Accounts (chart of accounts)
//   - Portfolios (customer account groupings)
//   - Assets (currencies/instruments)
//   - Transactions (financial operations)
//
// # Name Uniqueness
//
// Ledger names must be unique within an organization. The function checks
// for existing ledgers with the same name before creation. If a duplicate
// name is found, an error is returned.
//
// # Status Management
//
// If no status is provided, ledgers default to "ACTIVE".
// Supported statuses typically include:
//   - ACTIVE: Ledger accepts transactions
//   - INACTIVE: Ledger is disabled (no new transactions)
//   - ARCHIVED: Ledger is read-only for historical reference
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.create_ledger"
//  3. Set default status to "ACTIVE" if not provided
//  4. Check for existing ledger with same name (uniqueness)
//  5. Build ledger model with timestamps
//  6. Persist ledger to PostgreSQL via repository
//  7. Create associated metadata in MongoDB (if provided)
//  8. Return created ledger with metadata
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - organizationID: The organization that will own this ledger
//   - cli: CreateLedgerInput containing ledger details
//
// # Input Fields
//
//   - Name: Unique ledger name within organization
//   - Status: Ledger status (defaults to ACTIVE)
//   - Metadata: Optional key-value pairs for additional data
//
// # Returns
//
//   - *mmodel.Ledger: The created ledger with generated ID
//   - error: If validation fails or database operations fail
//
// # Error Scenarios
//
//   - Duplicate ledger name within organization
//   - Database connection failure during name check
//   - Database connection failure during Create
//   - Metadata creation failure (MongoDB)
//
// # Observability
//
// Creates tracing span "command.create_ledger" with error events on failure.
// Logs operation progress and any errors encountered.
//
// # Example
//
//	input := &mmodel.CreateLedgerInput{
//	    Name: "Main Ledger",
//	    Status: mmodel.Status{Code: "ACTIVE"},
//	    Metadata: map[string]any{"purpose": "primary"},
//	}
//	ledger, err := uc.CreateLedger(ctx, orgID, input)
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
