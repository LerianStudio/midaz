package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/google/uuid"
)

func (uc *UseCase) GetAllOperationsByPortfolio(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, filter http.QueryHeader) ([]*operation.Operation, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_operations_by_portfolio")
	defer span.End()

	logger.Infof("Retrieving operations by portfolio")

	op, err := uc.OperationRepo.FindAllByPortfolio(ctx, organizationID, ledgerID, portfolioID, filter.ToPagination())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get all operations on repo by portfolio", err)

		logger.Errorf("Error getting operations on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())
		}

		return nil, err
	}

	return op, nil
}
