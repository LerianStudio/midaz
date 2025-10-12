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

// CreateTransactionRoute creates a new transaction route combining operation routes.
//
// Transaction routes define validated transaction flows by combining multiple operation
// routes (sources and destinations). They enforce accounting policies by ensuring
// transactions follow predefined patterns.
//
// Validation Rules:
// - Must include at least one source (debit) operation route
// - Must include at least one destination (credit) operation route
// - All referenced operation routes must exist
//
// After creation, the route should be cached via CreateAccountingRouteCache for fast validation.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: Organization UUID owning the route
//   - ledgerID: Ledger UUID containing the route
//   - payload: Transaction route input with title, description, and operation route IDs
//
// Returns:
//   - *mmodel.TransactionRoute: The created transaction route with embedded operation routes
//   - error: ErrMissingOperationRoutes if validation fails, or persistence errors
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

// validateOperationRouteTypes validates that operation routes contain both source and destination types.
// Returns an error if either source or destination type is missing.
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
