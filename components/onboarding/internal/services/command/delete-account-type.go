package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DeleteAccountTypeByID deletes an account type by its ID.
// It returns an error if the operation fails or if the account type is not found.
func (uc *UseCase) DeleteAccountTypeByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_account_type_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_type_id", id.String()),
	)

	logger.Infof("Initiating deletion of Account Type with Account Type ID: %s", id.String())

	if err := uc.AccountTypeRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		logger.Errorf("Failed to delete Account Type with Account Type ID: %s, Error: %s", id.String(), err.Error())

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())

			logger.Warnf("Account Type ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete Account Type on repo", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete Account Type on repo", err)

		return err
	}

	logger.Infof("Successfully deleted Account Type with Account Type ID: %s", id.String())

	return nil
}
