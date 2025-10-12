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

// CreateAccountType creates a new account type and persists it in the repository.
//
// Account types define the categories of accounts within a chart of accounts,
// such as "asset", "liability", "equity", "revenue", "expense", or custom types
// like "current_assets", "fixed_assets", "accounts_payable", etc. They enable
// enforcement of accounting rules and proper financial categorization.
//
// When account type validation is enabled (via ACCOUNT_TYPE_VALIDATION env var),
// accounts can only be created if their type matches a defined AccountType in
// the ledger's chart of accounts. This ensures consistency with accounting standards
// and prevents the creation of accounts with undefined types.
//
// The keyValue field serves as the unique identifier used in account creation and
// must follow naming conventions (alphanumeric, underscores, hyphens, no spaces).
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning this account type
//   - ledgerID: The UUID of the ledger containing this account type
//   - payload: The account type creation input with name, description, and keyValue
//
// Returns:
//   - *mmodel.AccountType: The created account type with generated ID and metadata
//   - error: Persistence or metadata creation errors (including unique constraint violations on keyValue)
func (uc *UseCase) CreateAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateAccountTypeInput) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account_type")
	defer span.End()

	now := time.Now()

	// Construct account type entity with validated fields
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

	// Persist the account type to PostgreSQL
	createdAccountType, err := uc.AccountTypeRepo.Create(ctx, organizationID, ledgerID, accountType)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account type", err)

		logger.Errorf("Failed to create account type: %v", err)

		return nil, err
	}

	// Store custom metadata in MongoDB if provided
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
