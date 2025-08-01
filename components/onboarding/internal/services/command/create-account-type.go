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

// CreateAccountType creates a new account type.
// It returns the created account type and an error if the operation fails.
func (uc *UseCase) CreateAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateAccountTypeInput) (*mmodel.AccountType, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

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
		libOpentelemetry.HandleSpanError(&span, "Failed to create account type", err)

		logger.Errorf("Failed to create account type: %v", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.AccountType{}).Name(), createdAccountType.ID.String(), payload.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create metadata", err)

		logger.Errorf("Failed to create metadata: %v", err)

		return nil, err
	}

	createdAccountType.Metadata = metadata

	logger.Infof("Successfully created account type with key value: %s", createdAccountType.KeyValue)

	return createdAccountType, nil
}
