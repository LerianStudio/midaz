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
// Transaction routes are templates that define complete transaction patterns by combining
// multiple operation routes. Each transaction route must have at least one source (debit)
// and one destination (credit) operation route to form a valid double-entry accounting pattern.
//
// Creation Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//	  - Generate UUIDv7 for the new transaction route
//
//	Step 2: Fetch Operation Routes
//	  - Retrieve all operation routes by their IDs
//	  - Validate that all referenced operation routes exist
//
//	Step 3: Validate Operation Route Types
//	  - Ensure at least one "source" type operation route exists
//	  - Ensure at least one "destination" type operation route exists
//	  - Return ErrMissingOperationRoutes if validation fails
//
//	Step 4: Create Transaction Route
//	  - Persist transaction route to PostgreSQL
//	  - Associate operation routes via join table
//
//	Step 5: Create Metadata (Optional)
//	  - If metadata provided, store in MongoDB
//	  - Link metadata to transaction route by entity ID
//
// Double-Entry Accounting Requirement:
//
// Every financial transaction must balance: total debits = total credits.
// Transaction routes enforce this by requiring both source and destination
// operation routes, ensuring transactions created from this template
// will maintain accounting integrity.
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - payload: Creation input with Title, Description, OperationRoutes (UUIDs), and optional Metadata
//
// Returns:
//   - *mmodel.TransactionRoute: Created transaction route with operation routes attached
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrMissingOperationRoutes: No source or destination operation route provided
//   - Operation route not found: Referenced operation route ID does not exist
//   - Database errors: PostgreSQL or MongoDB unavailable
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
//
// This validation enforces double-entry accounting principles by ensuring every
// transaction route template can create balanced transactions.
//
// Validation Logic:
//   - Iterate through all operation routes
//   - Track presence of "source" type (debit operations)
//   - Track presence of "destination" type (credit operations)
//   - Return nil if both types found, error otherwise
//
// Parameters:
//   - operationRoutes: Slice of operation routes to validate
//
// Returns:
//   - error: ErrMissingOperationRoutes if validation fails, nil otherwise
//
// Why This Matters:
//
// Without both source and destination routes, a transaction cannot balance.
// This early validation prevents creation of unusable transaction templates.
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
