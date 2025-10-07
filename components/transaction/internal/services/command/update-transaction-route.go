// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

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
// This method updates transaction route properties and manages operation route relationships:
// 1. Updates title and description
// 2. Calculates operation routes to add/remove (if provided)
// 3. Validates new operation route set (must have source and destination)
// 4. Updates transaction route in PostgreSQL
// 5. Updates metadata using merge semantics
// 6. Returns updated transaction route
//
// Operation Route Management:
//   - Compares existing vs new operation routes
//   - Determines which relationships to add/remove
//   - Validates that result includes both source and destination
//   - Updates many-to-many relationship table
//
// Business Rules:
//   - Title and description are optional (partial updates)
//   - Operation routes are optional (if provided, must be complete set)
//   - Must maintain at least one source and one destination route
//   - Metadata is merged with existing
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - id: UUID of the transaction route to update
//   - input: Update input with title, description, operation routes, metadata
//
// Returns:
//   - *mmodel.TransactionRoute: Updated transaction route with metadata
//   - error: Business error if not found or validation fails
//
// OpenTelemetry: Creates span "command.update_transaction_route"
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

// handleOperationRouteUpdates calculates operation route relationship changes.
//
// This helper function compares existing operation routes with the new set to determine:
// 1. Which operation routes to add (in new set but not in existing)
// 2. Which operation routes to remove (in existing but not in new set)
// 3. Validates that the new set includes both source and destination types
//
// The function:
//   - Fetches current transaction route with operation routes
//   - Fetches all referenced operation routes to validate they exist
//   - Validates new set has both source and destination
//   - Calculates diff (toAdd, toRemove)
//
// Validation:
//   - Minimum 2 operation routes required (at least 1 source + 1 destination)
//   - All referenced operation routes must exist
//   - Must include both source and destination types
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionRouteID: UUID of the transaction route being updated
//   - newOperationRouteIDs: New set of operation route IDs
//
// Returns:
//   - toAdd: Operation route IDs to add
//   - toRemove: Operation route IDs to remove
//   - err: Validation error if requirements not met
//
// OpenTelemetry: Creates span "command.handle_operation_route_updates"
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
