// Package command implements write operations (commands) for the transaction service.
// This file contains the command for creating a new transaction route.
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
// This use case is responsible for:
//  1. Validating that the referenced operation routes exist and include at least one
//     source and one destination.
//  2. Persisting the transaction route in the PostgreSQL database.
//  3. Associating the operation routes with the newly created transaction route.
//  4. Storing any associated metadata in MongoDB.
//  5. Returning the complete transaction route, including its metadata.
//
// Transaction routes define the flow of funds by specifying which accounts can be
// sources or destinations, enabling automated and rule-based transaction processing.
//
// Business Rules:
//   - A transaction route must have at least one source and one destination operation route.
//   - All referenced operation routes must already exist.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - payload: The input data for creating the transaction route.
//
// Returns:
//   - *mmodel.TransactionRoute: The newly created transaction route.
//   - error: An error if validation or creation fails.
func (uc *UseCase) CreateTransactionRoute(ctx context.TBD, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateTransactionRouteInput) (*mmodel.TransactionRoute, error) {
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

// validateOperationRouteTypes validates that a list of operation routes contains at least one source and one destination.
//
// Business Rule:
//   - A transaction route must define both where funds originate (source) and where
//     they are sent (destination) to ensure complete and valid transaction routing.
//
// Parameters:
//   - operationRoutes: A slice of operation routes to validate.
//
// Returns:
//   - error: An error if either a source or a destination is missing.
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
