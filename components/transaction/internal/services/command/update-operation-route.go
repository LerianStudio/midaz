// Package command implements write operations (commands) for the transaction service.
// This file contains the command for updating an operation route.
package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// UpdateOperationRoute updates an existing operation route in the repository.
//
// This use case handles partial updates for an operation route's mutable fields
// and merges any provided metadata with the existing metadata in MongoDB.
//
// Business Rules:
//   - The operation route must exist to be updated.
//   - The operation type is immutable and cannot be changed.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - id: The UUID of the operation route to update.
//   - input: The input data containing the fields to update.
//
// Returns:
//   - *mmodel.OperationRoute: The updated operation route, including merged metadata.
//   - error: An error if the operation route is not found or if the update fails.
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
			err := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation route on repo by id", err)

			logger.Warnf("Error updating operation route on repo by id: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation route on repo by id", err)

		return nil, err
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
