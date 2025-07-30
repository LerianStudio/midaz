package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// DeleteOperationRouteByID is a method that deletes Operation Route information.
func (uc *UseCase) DeleteOperationRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_operation_route_by_id")
	defer span.End()

	logger.Infof("Remove operation route for id: %s", id.String())

	hasLinks, err := uc.OperationRouteRepo.HasTransactionRouteLinks(ctx, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to check transaction route links", err)

		logger.Errorf("Error checking transaction route links for operation route %s: %v", id.String(), err)

		return err
	}

	if hasLinks {
		logger.Errorf("Operation Route ID %s cannot be deleted because it is linked to transaction routes", id.String())

		return pkg.ValidateBusinessError(constant.ErrOperationRouteLinkedToTransactionRoutes, reflect.TypeOf(mmodel.OperationRoute{}).Name())
	}

	if err := uc.OperationRouteRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete operation route on repo by id", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			logger.Errorf("Operation Route ID not found: %s", id.String())
			return pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())
		}

		logger.Errorf("Error deleting operation route: %v", err)

		return err
	}

	return nil
}
