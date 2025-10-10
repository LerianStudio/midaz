// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for creating a new account type.
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

// CreateAccountType creates a new account type in the repository.
//
// This use case is responsible for:
// 1. Generating a UUIDv7 for the new account type.
// 2. Persisting the account type in the PostgreSQL database.
// 3. Storing associated metadata in MongoDB.
// 4. Returning the newly created account type, including its metadata.
//
// Business Rules:
//   - The KeyValue must be unique per organization and ledger.
//   - KeyValue must only contain alphanumeric characters, underscores, and hyphens.
//
// Account types are used for classifying accounts (e.g., "deposit", "loan"),
// which helps in organizing and validating the chart of accounts.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization that owns the account type.
//   - ledgerID: The UUID of the ledger where the account type will be created.
//   - payload: The input data for creating the account type, including name,
//     description, key value, and metadata.
//
// Returns:
//   - *mmodel.AccountType: The created account type, complete with its metadata.
//   - error: An error if the creation fails, such as a business rule violation
//     or a database error.
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
