package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
)

// UpdateOperation update an operation from the repository by given id.
func (uc *UseCase) UpdateOperation(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID, uoi *operation.UpdateOperationInput) (*operation.Operation, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_operation")
	defer span.End()

	logger.Infof("Trying to update operation: %v", uoi)

	op := &operation.Operation{
		Description: uoi.Description,
	}

	operationUpdated, err := uc.OperationRepo.Update(ctx, organizationID, ledgerID, transactionID, operationID, op)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update operation on repo by id", err)

		logger.Errorf("Error updating op on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrOperationIDNotFound, reflect.TypeOf(operation.Operation{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationID.String(), uoi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	operationUpdated.Metadata = metadataUpdated

	return operationUpdated, nil
}
