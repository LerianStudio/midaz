package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// UpdateOperation update an operation from the repository by given id.
func (uc *UseCase) UpdateOperation(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID, uoi *mmodel.UpdateOperationInput) (*mmodel.Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_operation")
	defer span.End()

	logger.Infof("Trying to update operation: %v", uoi)

	op := &mmodel.Operation{
		Description: uoi.Description,
	}

	operationUpdated, err := uc.OperationRepo.Update(ctx, organizationID, ledgerID, transactionID, operationID, op)
	if err != nil {
		logger.Errorf("Error updating op on repo by id: %v", err)

		var entityNotFound *pkg.EntityNotFoundError
		if errors.As(err, &entityNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation on repo by id", err)

			logger.Warnf("Error updating op on repo by id: %v", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Operation{}).Name())
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation on repo by id", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Operation{}).Name())
	}

	assert.NotNil(operationUpdated, "repository Update must return non-nil operation on success",
		"operation_id", operationID,
		"transaction_id", transactionID,
		"organization_id", organizationID)
	assert.That(operationUpdated.ID == operationID.String(), "operation id mismatch after update",
		"expected_id", operationID.String(),
		"actual_id", operationUpdated.ID)
	assert.That(operationUpdated.TransactionID == transactionID.String(), "operation transaction id mismatch after update",
		"expected_transaction_id", transactionID.String(),
		"actual_transaction_id", operationUpdated.TransactionID)
	assert.That(operationUpdated.OrganizationID == organizationID.String(), "operation organization id mismatch after update",
		"expected_organization_id", organizationID.String(),
		"actual_organization_id", operationUpdated.OrganizationID)
	assert.That(operationUpdated.LedgerID == ledgerID.String(), "operation ledger id mismatch after update",
		"expected_ledger_id", ledgerID.String(),
		"actual_ledger_id", operationUpdated.LedgerID)
	if uoi.Description != "" {
		assert.That(operationUpdated.Description == uoi.Description, "operation description mismatch after update",
			"expected_description", uoi.Description,
			"actual_description", operationUpdated.Description,
			"operation_id", operationID)
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Operation{}).Name(), operationID.String(), uoi.Metadata)
	if err != nil {
		wrappedErr := pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", wrappedErr)

		return nil, wrappedErr
	}

	operationUpdated.Metadata = metadataUpdated

	return operationUpdated, nil
}
