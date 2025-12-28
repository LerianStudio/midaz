package query

import (
	"context"
	"errors"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/google/uuid"
)

// operationTypeName is the type name used for Operation entity in error handling and metadata lookups.
const operationTypeName = "Operation"

// GetOperationByAccount retrieves a specific operation by its ID for a given account.
func (uc *UseCase) GetOperationByAccount(ctx context.Context, organizationID, ledgerID, accountID, operationID uuid.UUID) (*operation.Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_by_account")
	defer span.End()

	logger.Infof("Retrieving operation by account")

	op, err := uc.OperationRepo.FindByAccount(ctx, organizationID, ledgerID, accountID, operationID)
	if err != nil {
		logger.Errorf("Error getting operation on repo: %v", err)

		var entityNotFound *pkg.EntityNotFoundError
		if errors.As(err, &entityNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation on repo by account", err)

			logger.Warnf("Error getting operation on repo: %v", err)

			return nil, pkg.ValidateInternalError(err, operationTypeName)
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation on repo by account", err)

		return nil, pkg.ValidateInternalError(err, operationTypeName)
	}

	if op != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, operationTypeName, operationID.String())
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb operation", err)

			logger.Errorf("Error get metadata on mongodb operation: %v", err)

			return nil, pkg.ValidateInternalError(err, operationTypeName)
		}

		if metadata != nil {
			op.Metadata = metadata.Data
		}
	}

	return op, nil
}
