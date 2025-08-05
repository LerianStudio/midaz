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

// UpdateTransactionRoute updates a transaction route by its ID.
// It returns the updated transaction route and an error if the operation fails.
func (uc *UseCase) UpdateTransactionRoute(ctx context.Context, organizationID, ledgerID, id uuid.UUID, input *mmodel.UpdateTransactionRouteInput) (*mmodel.TransactionRoute, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction_route")
	defer span.End()

	logger.Infof("Trying to update transaction route: %v", input)

	transactionRoute := &mmodel.TransactionRoute{
		Title:       input.Title,
		Description: input.Description,
	}

	// Handle operation route updates if provided
	var toAdd, toRemove []uuid.UUID

	if input.OperationRoutes != nil {
		var err error

		toAdd, toRemove, err = uc.handleOperationRouteUpdates(ctx, organizationID, ledgerID, id, *input.OperationRoutes)
		if err != nil {
			return nil, err
		}
	}

	transactionRouteUpdated, err := uc.TransactionRouteRepo.Update(ctx, organizationID, ledgerID, id, transactionRoute, toAdd, toRemove)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update transaction route on repo by id", err)

		logger.Errorf("Error updating transaction route on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), id.String(), input.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update metadata on repo by id", err)

		logger.Errorf("Error updating metadata on repo by id: %v", err)

		return nil, err
	}

	transactionRouteUpdated.Metadata = metadataUpdated

	return transactionRouteUpdated, nil
}

// handleOperationRouteUpdates processes operation route relationship updates by comparing existing vs new operation routes.
// It returns arrays of operation route IDs to add and remove, or an error if validation fails.
func (uc *UseCase) handleOperationRouteUpdates(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID, newOperationRouteIDs []uuid.UUID) (toAdd, toRemove []uuid.UUID, err error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.handle_operation_route_updates")
	defer span.End()

	if len(newOperationRouteIDs) < 2 {
		return nil, nil, pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	}

	currentTransactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, transactionRouteID)
	if err != nil {
		logger.Errorf("Error fetching current transaction route: %v", err)
		return nil, nil, err
	}

	operationRoutes, err := uc.OperationRouteRepo.FindByIDs(ctx, organizationID, ledgerID, newOperationRouteIDs)
	if err != nil {
		logger.Errorf("Error fetching operation routes: %v", err)
		return nil, nil, err
	}

	// Validate that we have at least 1 debit and 1 credit operation route
	err = validateOperationRouteTypes(operationRoutes)
	if err != nil {
		return nil, nil, err
	}

	// Compare existing vs new operation routes to determine what to add/remove
	existingIDs := make(map[uuid.UUID]bool)
	for _, existingRoute := range currentTransactionRoute.OperationRoutes {
		existingIDs[existingRoute.ID] = true
	}

	newIDs := make(map[uuid.UUID]bool)
	for _, newID := range newOperationRouteIDs {
		newIDs[newID] = true
	}

	// Find relationships to remove (exist currently but not in new list)
	for existingID := range existingIDs {
		if !newIDs[existingID] {
			toRemove = append(toRemove, existingID)
		}
	}

	// Find relationships to add (in new list but don't exist currently)
	for newID := range newIDs {
		if !existingIDs[newID] {
			toAdd = append(toAdd, newID)
		}
	}

	logger.Infof("Operation route updates calculated. ToAdd: %v, ToRemove: %v", toAdd, toRemove)

	return toAdd, toRemove, nil
}
