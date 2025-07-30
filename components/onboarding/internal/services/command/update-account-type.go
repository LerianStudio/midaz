package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// UpdateAccountType updates an account type by its ID.
// It returns the updated account type and an error if the operation fails.
func (uc *UseCase) UpdateAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, input *mmodel.UpdateAccountTypeInput) (*mmodel.AccountType, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_account_type")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_type_id", id.String()),
	)

	if err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.payload", input); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	logger.Infof("Trying to update account type: %v", input)

	accountType := &mmodel.AccountType{
		Name:        input.Name,
		Description: input.Description,
	}

	accountTypeUpdated, err := uc.AccountTypeRepo.Update(ctx, organizationID, ledgerID, id, accountType)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update account type on repo by id", err)

		logger.Errorf("Error updating account type on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.AccountType{}).Name(), id.String(), input.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update metadata", err)

		return nil, err
	}

	accountTypeUpdated.Metadata = metadataUpdated

	logger.Infof("Successfully updated account type with ID: %s", accountTypeUpdated.ID)

	return accountTypeUpdated, nil
}
