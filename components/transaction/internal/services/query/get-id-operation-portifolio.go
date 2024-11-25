package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/google/uuid"
)

func (uc *UseCase) GetOperationByPortfolio(ctx context.Context, organizationID, ledgerID, portfolioID, operationID uuid.UUID) (*operation.Operation, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_by_portfolio")
	defer span.End()

	logger.Infof("Retrieving operation by account")

	op, err := uc.OperationRepo.FindByPortfolio(ctx, organizationID, ledgerID, portfolioID, operationID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get operation on repo by portfolio", err)

		logger.Errorf("Error getting operation on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())
		}

		return nil, err
	}

	if op != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationID.String())
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb operation", err)

			logger.Errorf("Error get metadata on mongodb operation: %v", err)

			return nil, err
		}

		if metadata != nil {
			op.Metadata = metadata.Data
		}
	}

	return op, nil
}
