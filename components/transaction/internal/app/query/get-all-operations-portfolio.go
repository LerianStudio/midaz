package query

import (
	"context"
	"errors"
	cn "github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/app"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	"github.com/google/uuid"
)

func (uc *UseCase) GetAllOperationsByPortfolio(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, filter commonHTTP.QueryHeader) ([]*o.Operation, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_operations_by_portfolio")
	defer span.End()

	logger.Infof("Retrieving operations by portfolio")

	op, err := uc.OperationRepo.FindAllByPortfolio(ctx, organizationID, ledgerID, portfolioID, filter.Limit, filter.Page)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get all operations on repo by portfolio", err)

		logger.Errorf("Error getting operations on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrNoOperationsFound, reflect.TypeOf(o.Operation{}).Name())
		}

		return nil, err
	}

	return op, nil
}
