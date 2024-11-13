package query

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

func (uc *UseCase) GetOperationByPortfolio(ctx context.Context, organizationID, ledgerID, portfolioID, operationID uuid.UUID) (*o.Operation, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_by_portfolio")
	defer span.End()

	logger.Infof("Retrieving operation by account")

	op, err := uc.OperationRepo.FindByPortfolio(ctx, organizationID, ledgerID, portfolioID, operationID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get operation on repo by portfolio", err)

		logger.Errorf("Error getting operation on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrNoOperationsFound, reflect.TypeOf(o.Operation{}).Name())
		}

		return nil, err
	}

	if op != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(o.Operation{}).Name(), operationID.String())
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
