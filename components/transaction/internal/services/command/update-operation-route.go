package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// UpdateOperationRoute updates an existing operation route with the provided input.
// It validates the operation type and persists changes to the database.
func (uc *UseCase) UpdateOperationRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, input *mmodel.UpdateOperationRouteInput) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_operation_route")
	defer span.End()

	logger.Infof("Trying to update operation route: %v", input)

	operationRoute := &mmodel.OperationRoute{
		Title:       input.Title,
		Description: input.Description,
		Code:        input.Code,
		Account:     input.Account,
	}

	operationRouteUpdated, err := uc.OperationRouteRepo.Update(ctx, organizationID, ledgerID, id, operationRoute)
	if err != nil {
		logger.Errorf("Error updating operation route on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			notFoundErr := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Operation route not found", notFoundErr)

			logger.Warnf("Operation route not found: %s", id.String())

			return nil, notFoundErr
		}

		var entityNotFound *pkg.EntityNotFoundError
		if errors.As(err, &entityNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation route on repo by id", err)

			logger.Warnf("Error updating operation route on repo by id: %v", err)

			return nil, fmt.Errorf("%w: %w", services.ErrOperationRouteNotFound, err)
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation route on repo by id", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.OperationRoute{}).Name())
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), id.String(), input.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		logger.Errorf("Error updating metadata on repo by id: %v", err)

		return nil, err
	}

	operationRouteUpdated.Metadata = metadataUpdated

	return operationRouteUpdated, nil
}
