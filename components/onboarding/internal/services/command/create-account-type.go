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

// CreateAccountType creates a new account type and persists it to the repository.
//
// Account types define the chart of accounts structure within a ledger.
// They categorize accounts for reporting, compliance, and organizational purposes.
//
// # Account Type Purpose
//
// Account types serve several functions:
//   - Define account categories (Assets, Liabilities, Equity, Revenue, Expenses)
//   - Enable chart of accounts organization
//   - Support regulatory reporting requirements
//   - Facilitate financial statement generation
//
// # KeyValue Field
//
// The KeyValue field provides a unique, human-readable identifier for the account type.
// This is typically used for:
//   - API references (e.g., "ASSET", "LIABILITY")
//   - Integration with external systems
//   - Chart of accounts mapping
//
// KeyValue should be:
//   - Unique within the ledger
//   - Uppercase with underscores (convention)
//   - Immutable after creation
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.create_account_type"
//  3. Generate UUIDv7 for the new account type
//  4. Build account type model with timestamps
//  5. Persist account type to PostgreSQL via repository
//  6. Create associated metadata in MongoDB (if provided)
//  7. Log success with KeyValue
//  8. Return created account type with metadata
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - organizationID: The organization that owns this ledger (tenant isolation)
//   - ledgerID: The ledger where this account type will be created
//   - payload: CreateAccountTypeInput containing account type details
//
// # Input Fields
//
//   - Name: Human-readable account type name
//   - Description: Optional detailed description
//   - KeyValue: Unique identifier within ledger (e.g., "ASSET")
//   - Metadata: Optional key-value pairs for additional data
//
// # Returns
//
//   - *mmodel.AccountType: The created account type with generated ID
//   - error: If database operations fail
//
// # Error Scenarios
//
//   - Duplicate KeyValue within ledger
//   - Database connection failure during Create
//   - Metadata creation failure (MongoDB)
//   - Context cancellation/timeout
//
// # Observability
//
// Creates tracing span "command.create_account_type" with error events on failure.
// Logs operation progress and success with KeyValue.
//
// # Example
//
//	input := &mmodel.CreateAccountTypeInput{
//	    Name:        "Current Assets",
//	    Description: "Short-term assets expected to convert to cash within a year",
//	    KeyValue:    "CURRENT_ASSET",
//	}
//	accountType, err := uc.CreateAccountType(ctx, orgID, ledgerID, input)
func (uc *UseCase) CreateAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateAccountTypeInput) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account_type")
	defer span.End()

	now := time.Now()

	accountType := &mmodel.AccountType{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Name:           payload.Name,
		Description:    payload.Description,
		KeyValue:       payload.KeyValue,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	createdAccountType, err := uc.AccountTypeRepo.Create(ctx, organizationID, ledgerID, accountType)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account type", err)

		logger.Errorf("Failed to create account type: %v", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.AccountType{}).Name(), createdAccountType.ID.String(), payload.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create metadata", err)

		logger.Errorf("Failed to create metadata: %v", err)

		return nil, err
	}

	createdAccountType.Metadata = metadata

	logger.Infof("Successfully created account type with key value: %s", createdAccountType.KeyValue)

	return createdAccountType, nil
}
