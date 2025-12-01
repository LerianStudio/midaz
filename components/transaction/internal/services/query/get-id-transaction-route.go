package query

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

// GetTransactionRouteByID retrieves a single transaction route by its unique identifier.
//
// Transaction routes define the accounting structure for specific transaction types.
// This method fetches the complete route configuration including associated metadata,
// useful for route management interfaces and transaction validation setup.
//
// Domain Context:
//
// A transaction route includes:
//   - Route identification (ID, code, name)
//   - Source operation routes (debit-side rules)
//   - Destination operation routes (credit-side rules)
//   - Metadata for custom attributes
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for distributed tracing
//	  - Log the route ID being retrieved
//
//	Step 2: Fetch Route from PostgreSQL
//	  - Query by organization, ledger, and route ID
//	  - Handle not-found with business error
//	  - Handle other errors with infrastructure error
//
//	Step 3: Fetch Metadata from MongoDB (if route found)
//	  - Query metadata document by route ID
//	  - Assign metadata to route if present
//	  - Handle metadata errors as infrastructure errors
//
// Parameters:
//   - ctx: Request context with tenant and tracing information
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID to scope the route
//   - id: Transaction route UUID to retrieve
//
// Returns:
//   - *mmodel.TransactionRoute: Complete route with metadata
//   - error: Business error (not found) or infrastructure error
//
// Error Scenarios:
//   - ErrTransactionRouteNotFound: Route does not exist
//   - Database error: PostgreSQL connection or query failure
//   - Metadata error: MongoDB query failure
func (uc *UseCase) GetTransactionRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_transaction_route_by_id")
	defer span.End()

	logger.Infof("Retrieving transaction route for id: %s", id)

	transactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Error getting transaction route on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction route", err)

			logger.Warnf("Error getting transaction route on repo by id: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to get transaction route", err)

		return nil, err
	}

	if transactionRoute != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRoute.ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb transaction route", err)

			logger.Errorf("Error get metadata on mongodb transaction route: %v", err)

			return nil, err
		}

		if metadata != nil {
			transactionRoute.Metadata = metadata.Data
		}
	}

	return transactionRoute, nil
}
