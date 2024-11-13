package command

import (
	"context"
	"errors"
	cn "github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/transaction/internal/app"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	"github.com/google/uuid"
)

// UpdateOperation update an operation from the repository by given id.
func (uc *UseCase) UpdateOperation(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID, uoi *o.UpdateOperationInput) (*o.Operation, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_operation")
	defer span.End()

	logger.Infof("Trying to update operation: %v", uoi)

	operation := &o.Operation{
		Description: uoi.Description,
	}

	operationUpdated, err := uc.OperationRepo.Update(ctx, organizationID, ledgerID, transactionID, operationID, operation)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update operation on repo by id", err)

		logger.Errorf("Error updating operation on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrOperationIDNotFound, reflect.TypeOf(o.Operation{}).Name())
		}

		return nil, err
	}

	if len(uoi.Metadata) > 0 {
		if err := common.CheckMetadataKeyAndValueLength(100, uoi.Metadata); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to check metadata key and value length", err)

			return nil, err
		}

		err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(o.Operation{}).Name(), operationID.String(), uoi.Metadata)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to update metadata on mongodb operation", err)

			return nil, err
		}

		operationUpdated.Metadata = uoi.Metadata
	}

	return operationUpdated, nil
}
