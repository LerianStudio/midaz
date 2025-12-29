package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/google/uuid"
)

// GetOperationByID gets data in the repository.
func (uc *UseCase) GetOperationByID(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID) (*operation.Operation, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	assert.That(transactionID != uuid.Nil, "transactionID must not be nil UUID",
		"transactionID", transactionID)
	assert.That(operationID != uuid.Nil, "operationID must not be nil UUID",
		"operationID", operationID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_by_id")
	defer span.End()

	logger.Infof("Trying to get operation")

	o, err := uc.OperationRepo.Find(ctx, organizationID, ledgerID, transactionID, operationID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get operation on repo by id", err)

		logger.Errorf("Error getting operation: %v", err)

		return nil, pkg.ValidateInternalError(err, "Operation")
	}

	if o != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb operation", err)

			logger.Errorf("Error get metadata on mongodb operation: %v", err)

			return nil, pkg.ValidateInternalError(err, "Operation")
		}

		if metadata != nil {
			o.Metadata = metadata.Data
		}
	}

	return o, nil
}
