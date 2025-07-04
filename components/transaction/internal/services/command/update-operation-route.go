package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

func (uc *UseCase) UpdateOperationRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, input *mmodel.UpdateOperationRouteInput) (*mmodel.OperationRoute, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_operation_route")
	defer span.End()

	logger.Infof("Trying to update operation route: %v", input)

	if len(input.AccountTypes) > 0 && input.AccountAlias != "" {
		err := pkg.ValidateBusinessError(constant.ErrMutuallyExclusiveFields, reflect.TypeOf(mmodel.OperationRoute{}).Name(), "accountTypes", "accountAlias")

		libOpentelemetry.HandleSpanError(&span, "Mutually exclusive fields provided", err)

		return nil, err
	}

	existingOperationRoute, err := uc.OperationRouteRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve existing operation route", err)

		logger.Errorf("Error retrieving existing operation route: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())
		}

		return nil, err
	}

	if (len(existingOperationRoute.AccountTypes) > 0 && input.AccountAlias != "") || (len(input.AccountTypes) > 0 && existingOperationRoute.AccountAlias != "") {
		err := pkg.ValidateBusinessError(constant.ErrMutuallyExclusiveFields, reflect.TypeOf(mmodel.OperationRoute{}).Name(), "accountTypes", "accountAlias")

		libOpentelemetry.HandleSpanError(&span, "Cannot update, a mutually exclusive field is provided", err)

		return nil, err
	}

	operationRoute := &mmodel.OperationRoute{
		Title:        input.Title,
		Description:  input.Description,
		AccountTypes: input.AccountTypes,
		AccountAlias: input.AccountAlias,
	}

	operationRouteUpdated, err := uc.OperationRouteRepo.Update(ctx, organizationID, ledgerID, id, operationRoute)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update operation route on repo by id", err)

		logger.Errorf("Error updating operation route on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())
		}

		return nil, err
	}

	return operationRouteUpdated, nil
}
