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

// UpdateTransactionRoute updates an existing transaction route with new properties.
//
// Transaction routes define complete transaction patterns. This function allows
// modifying the route's title, description, and the set of operation routes
// that compose it. When operation routes change, the function calculates
// the differential (adds/removes) to maintain relationship integrity.
//
// Update Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//
//	Step 2: Build Update Model
//	  - Map input fields to TransactionRoute model
//	  - Title and Description can be updated independently
//
//	Step 3: Handle Operation Route Changes (if provided)
//	  - Calculate which operation routes to add vs remove
//	  - Validate new set maintains source+destination requirement
//	  - Return toAdd and toRemove arrays for repository
//
//	Step 4: Repository Update
//	  - Update transaction route properties
//	  - Modify operation route relationships (add/remove)
//	  - Handle not-found scenarios with business error
//
//	Step 5: Metadata Update
//	  - Update associated metadata in MongoDB
//	  - Merge new metadata with existing data
//
// Operation Route Relationship Management:
//
// When updating operation routes, the function:
//  1. Fetches current transaction route with existing relationships
//  2. Compares existing IDs with new IDs
//  3. Calculates differential: toAdd (new - existing), toRemove (existing - new)
//  4. Validates the resulting set has both source and destination
//  5. Passes differential to repository for atomic update
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - id: UUID of the transaction route to update
//   - input: Update payload with optional Title, Description, OperationRoutes, Metadata
//
// Returns:
//   - *mmodel.TransactionRoute: Updated transaction route with refreshed metadata
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrTransactionRouteNotFound: Transaction route with given ID does not exist
//   - ErrMissingOperationRoutes: Updated operation routes missing source or destination
//   - Database errors: PostgreSQL or MongoDB unavailable
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

// handleOperationRouteUpdates processes operation route relationship updates.
//
// This function calculates the differential between current and new operation routes,
// determining which relationships to add and which to remove for an atomic update.
//
// Calculation Process:
//
//	Step 1: Validate Minimum Requirements
//	  - Ensure at least 2 operation routes provided (minimum for source+destination)
//	  - Return ErrMissingOperationRoutes if insufficient
//
//	Step 2: Fetch Current State
//	  - Retrieve current transaction route with existing operation routes
//	  - Build map of existing operation route IDs
//
//	Step 3: Validate New Operation Routes
//	  - Fetch all new operation routes by ID
//	  - Validate both source and destination types present
//
//	Step 4: Calculate Differential
//	  - ToRemove: IDs in existing but not in new
//	  - ToAdd: IDs in new but not in existing
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - transactionRouteID: UUID of the transaction route being updated
//   - newOperationRouteIDs: New set of operation route IDs
//
// Returns:
//   - toAdd: Operation route IDs to add to the relationship
//   - toRemove: Operation route IDs to remove from the relationship
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrMissingOperationRoutes: Less than 2 operation routes or missing source/destination
//   - Database errors: Failed to fetch current state or validate new routes
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
