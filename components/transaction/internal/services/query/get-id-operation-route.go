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

// GetOperationRouteByID retrieves an operation route by its unique identifier.
//
// Operation routes define how operations should be processed based on matching
// criteria such as asset codes, account types, or custom rules. This method
// retrieves a specific route configuration with its associated metadata.
//
// Query Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "query.get_operation_route_by_id"
//	  - Log route ID being retrieved
//
//	Step 2: Route Retrieval
//	  - Query OperationRouteRepo.FindByID with organization and ledger scope
//	  - If route not found: Return ErrOperationRouteNotFound business error
//	  - If other error: Return wrapped error with span event
//
//	Step 3: Metadata Enrichment
//	  - If route found: Query MongoDB for associated metadata
//	  - If metadata retrieval fails: Return error
//	  - If metadata exists: Attach to route entity
//
//	Step 4: Response
//	  - Log successful retrieval
//	  - Return enriched operation route with metadata
//
// Operation Route Purpose:
//
// Operation routes enable dynamic transaction processing by defining rules
// for how operations should be handled. They can specify:
//   - Fee calculations and destination accounts
//   - Compliance checks and validations
//   - Custom processing logic triggers
//   - Multi-step operation workflows
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the route
//   - portfolioID: Optional portfolio UUID (may be nil)
//   - id: UUID of the operation route to retrieve
//
// Returns:
//   - *mmodel.OperationRoute: Operation route with metadata if found
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrOperationRouteNotFound: Route does not exist
//   - Database connection failure
//   - MongoDB metadata retrieval failure
func (uc *UseCase) GetOperationRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_route_by_id")
	defer span.End()

	logger.Infof("Retrieving operation route for id: %s", id)

	operationRoute, err := uc.OperationRouteRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Error getting operation route on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation route on repo by id", err)

			logger.Warnf("Error getting operation route on repo by id: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation route on repo by id", err)

		return nil, err
	}

	if operationRoute != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), operationRoute.ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb operation route", err)

			logger.Errorf("Error get metadata on mongodb operation route: %v", err)

			return nil, err
		}

		if metadata != nil {
			operationRoute.Metadata = metadata.Data
		}
	}

	logger.Infof("Successfully retrieved operation route for id: %s", id)

	return operationRoute, nil
}
