// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateTransactionRoute creates a new transaction route with associated operation routes.
//
// This method implements the create transaction route use case, which:
// 1. Generates UUIDv7 for the transaction route ID
// 2. Validates that all referenced operation routes exist
// 3. Validates that operation routes include both source and destination types
// 4. Creates the transaction route in PostgreSQL
// 5. Associates operation routes with the transaction route
// 6. Creates metadata in MongoDB
// 7. Returns the complete transaction route
//
// Business Rules:
//   - Transaction route must reference at least one source operation route
//   - Transaction route must reference at least one destination operation route
//   - All referenced operation routes must exist
//   - Title and description are required
//   - Code is optional (for programmatic reference)
//
// Transaction Routes:
//   - Define how transactions flow through the system
//   - Specify which accounts can be sources and destinations
//   - Enable automated transaction routing based on rules
//   - Support complex routing logic (account type, alias patterns)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - payload: Transaction route input with title, description, operation routes
//
// Returns:
//   - *mmodel.TransactionRoute: Created transaction route with metadata
//   - error: Business error if validation or creation fails
//
// OpenTelemetry: Creates span "command.create_transaction_route"
func (uc *UseCase) CreateTransactionRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateTransactionRouteInput) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction_route")
	defer span.End()

	now := time.Now()

	transactionRoute := &mmodel.TransactionRoute{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          payload.Title,
		Description:    payload.Description,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	operationRouteList, err := uc.OperationRouteRepo.FindByIDs(ctx, organizationID, ledgerID, payload.OperationRoutes)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find operation routes", err)

		logger.Errorf("Failed to find operation routes: %v", err)

		return nil, err
	}

	// Validate operation route types
	if err := validateOperationRouteTypes(operationRouteList); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate operation route types", err)

		logger.Errorf("Operation route validation failed: %v", err)

		return nil, err
	}

	// Convert to slice for assignment
	operationRoutes := make([]mmodel.OperationRoute, 0, len(operationRouteList))
	for _, operationRoute := range operationRouteList {
		operationRoutes = append(operationRoutes, *operationRoute)
	}

	transactionRoute.OperationRoutes = operationRoutes

	createdTransactionRoute, err := uc.TransactionRouteRepo.Create(ctx, organizationID, ledgerID, transactionRoute)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction route", err)

		logger.Errorf("Failed to create transaction route: %v", err)

		return nil, err
	}

	createdTransactionRoute.OperationRoutes = operationRoutes

	if payload.Metadata != nil {
		if err := libCommons.CheckMetadataKeyAndValueLength(100, payload.Metadata); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to check metadata key and value length", err)

			return nil, err
		}

		meta := mongodb.Metadata{
			EntityID:   createdTransactionRoute.ID.String(),
			EntityName: reflect.TypeOf(mmodel.TransactionRoute{}).Name(),
			Data:       payload.Metadata,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), &meta); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction route metadata", err)

			logger.Errorf("Failed to create transaction route metadata: %v", err)

			return nil, err
		}

		createdTransactionRoute.Metadata = payload.Metadata
	}

	logger.Infof("Successfully created transaction route with %d operation routes", len(createdTransactionRoute.OperationRoutes))

	return createdTransactionRoute, nil
}

// validateOperationRouteTypes validates that operation routes include both source and destination.
//
// This function ensures transaction routes have complete routing rules by checking:
//   - At least one operation route with type "source" exists
//   - At least one operation route with type "destination" exists
//
// Business Rule:
//   - Transaction routes must define both where money comes from (source) and
//     where it goes to (destination) to enable proper transaction routing
//
// Parameters:
//   - operationRoutes: Array of operation routes to validate
//
// Returns:
//   - error: ErrMissingOperationRoutes if source or destination is missing, nil if valid
func validateOperationRouteTypes(operationRoutes []*mmodel.OperationRoute) error {
	hasSource := false
	hasDestination := false

	for _, operationRoute := range operationRoutes {
		if operationRoute.OperationType == "source" {
			hasSource = true
		}

		if operationRoute.OperationType == "destination" {
			hasDestination = true
		}

		if hasSource && hasDestination {
			return nil
		}
	}

	return pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
}
