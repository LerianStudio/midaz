package query

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/google/uuid"
	"reflect"
)

// GetOperationByID gets data in the repository.
func (uc *UseCase) GetOperationByID(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID) (*operation.Operation, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_by_id")
	defer span.End()

	logger.Infof("Trying to get operation")

	o, err := uc.OperationRepo.Find(ctx, organizationID, ledgerID, transactionID, operationID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get operation on repo by id", err)

		logger.Errorf("Error getting operation: %v", err)

		return nil, err
	}

	if o != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb operation", err)

			logger.Errorf("Error get metadata on mongodb operation: %v", err)

			return nil, err
		}

		if metadata != nil {
			o.Metadata = metadata.Data
		}
	}

	return o, nil
}
