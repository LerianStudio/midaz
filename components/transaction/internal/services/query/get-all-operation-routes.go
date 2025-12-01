package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// GetAllOperationRoutes retrieves all operation routes configured for a ledger.
//
// Operation routes define the accounting rules that govern individual operations within
// a transaction route. They specify which accounts can participate in operations and
// what validation rules apply (e.g., account type restrictions, alias matching).
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//
//	Step 2: Fetch Operation Routes from PostgreSQL
//	  - Query all operation routes for the organization/ledger
//	  - Apply cursor-based pagination from filter
//	  - Handle not-found case with business error
//
//	Step 3: Prepare Metadata Filter
//	  - Initialize empty BSON filter if not provided
//	  - Ensures consistent MongoDB query behavior
//
//	Step 4: Fetch Metadata from MongoDB
//	  - Query metadata documents matching the filter
//	  - Build lookup map indexed by entity ID
//
//	Step 5: Enrich Operation Routes with Metadata
//	  - Assign metadata to each operation route by ID match
//	  - Convert UUID to string for map lookup
//
// Parameters:
//   - ctx: Request context with tenant and tracing information
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID to scope operation routes
//   - filter: Query parameters with pagination and optional metadata filter
//
// Returns:
//   - []*mmodel.OperationRoute: Operation routes with metadata, nil if none found
//   - libHTTP.CursorPagination: Cursor for paginated results
//   - error: Business error (ErrNoOperationRoutesFound) or infrastructure error
//
// Error Scenarios:
//   - ErrNoOperationRoutesFound: No operation routes configured for ledger
//   - ErrEntityNotFound: Metadata lookup failed
//   - Database errors: PostgreSQL or MongoDB connection issues
func (uc *UseCase) GetAllOperationRoutes(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.OperationRoute, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_operation_routes")
	defer span.End()

	logger.Infof("Retrieving operation routes")

	operationRoutes, cur, err := uc.OperationRouteRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoOperationRoutesFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation routes on repo", err)

			logger.Warnf("Error getting operation routes on repo: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation routes on repo", err)

		logger.Errorf("Error getting operation routes on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if operationRoutes != nil {
		metadataFilter := filter
		if metadataFilter.Metadata == nil {
			metadataFilter.Metadata = &bson.M{}
		}

		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), metadataFilter)
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb operation route", err)

			logger.Warnf("Error getting metadata on mongodb operation route: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range operationRoutes {
			if data, ok := metadataMap[operationRoutes[i].ID.String()]; ok {
				operationRoutes[i].Metadata = data
			}
		}
	}

	return operationRoutes, cur, nil
}
