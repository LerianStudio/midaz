// Package command implements write operations (commands) for the onboarding service.
// This file contains command implementation.

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
// This method implements the create account type use case, which:
// 1. Generates a UUIDv7 for the account type ID
// 2. Creates the account type in PostgreSQL
// 3. Creates associated metadata in MongoDB
// 4. Returns the complete account type with metadata
//
// Business Rules:
//   - Key value must be unique within an organization and ledger
//   - Key value must contain only alphanumeric characters, underscores, and hyphens
//   - Name is required (validated at HTTP layer)
//   - Description is optional
//   - Organization and ledger must exist (validated by foreign key constraints)
//
// Account Types are used to:
//   - Define account classifications (e.g., "deposit", "loan", "revenue")
//   - Enable optional account type validation during account creation
//   - Support chart of accounts structures
//   - Provide consistent account categorization
//
// Data Storage:
//   - Primary data: PostgreSQL (account_types table)
//   - Metadata: MongoDB (flexible key-value storage)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization that owns this account type
//   - ledgerID: UUID of the ledger that contains this account type
//   - payload: Create account type input with name, description, key value, and metadata
//
// Returns:
//   - *mmodel.AccountType: Created account type with metadata
//   - error: Business error if validation fails, database error if persistence fails
//
// Possible Errors:
//   - ErrDuplicateAccountTypeKeyValue: Key value already exists
//   - ErrInvalidAccountTypeKeyValue: Key value contains invalid characters
//   - ErrLedgerIDNotFound: Ledger doesn't exist
//   - ErrOrganizationIDNotFound: Organization doesn't exist
//   - Database errors: Connection failures, constraint violations
//
// Example:
//
//	input := &mmodel.CreateAccountTypeInput{
//	    Name:        "Current Assets",
//	    Description: "Assets expected to be converted to cash within one year",
//	    KeyValue:    "current_assets",
//	    Metadata:    map[string]any{"category": "balance_sheet"},
//	}
//	accountType, err := useCase.CreateAccountType(ctx, orgID, ledgerID, input)
//
// OpenTelemetry:
//   - Creates span "command.create_account_type"
//   - Records errors as span events
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
