// Package command implements write operations (commands) for the transaction service.
// This file contains the command for updating a transaction route.
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

// UpdateTransactionRoute updates an existing transaction route in the repository.
//
// This use case handles partial updates for a transaction route's mutable fields,
// manages its associations with operation routes, and merges any provided metadata.
//
// Business Rules:
//   - The transaction route must exist to be updated.
//   - If operation routes are provided, the new set must include at least one source
//     and one destination.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - id: The UUID of the transaction route to update.
//   - input: The input data containing the fields to update.
//
// Returns:
//   - *mmodel.TransactionRoute: The updated transaction route, including metadata.
//   - error: An error if the route is not found or if validation fails.
func (uc *UseCase) UpdateTransactionRoute(ctx context.Context, organizationID, ledgerID, id uuid.UUID, input *mmodel.UpdateTransactionRouteInput) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
		logger.Errorf("Error updating transaction route on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update transaction route on repo by id", err)

			logger.Warnf("Error updating transaction route on repo by id: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update transaction route on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), id.String(), input.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		logger.Errorf("Error updating metadata on repo by id: %v", err)

		return nil, err
	}

	transactionRouteUpdated.Metadata = metadataUpdated

	return transactionRouteUpdated, nil
}

// handleOperationRouteUpdates calculates the differences in operation route associations.
//
// This helper function compares the existing set of operation routes for a transaction
// route with a new set, determining which associations to add and which to remove.
// It also ensures the new set is valid.
//
// Business Rules:
//   - The new set of operation routes must contain at least one source and one destination.
//   - All referenced operation routes must exist.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - transactionRouteID: The UUID of the transaction route being updated.
//   - newOperationRouteIDs: A slice of UUIDs for the new set of operation routes.
//
// Returns:
//   - toAdd: A slice of operation route UUIDs to associate with the transaction route.
//   - toRemove: A slice of operation route UUIDs to disassociate from the transaction route.
//   - err: An error if validation fails.
func (uc *UseCase) handleOperationRouteUpdates(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID, newOperationRouteIDs []uuid.UUID) (toAdd, toRemove []uuid.UUID, err error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
