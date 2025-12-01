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

// GetAllTransactionRoutes retrieves all transaction routes configured for a ledger.
//
// Transaction routes define the structure and validation rules for financial transactions.
// Each route specifies source and destination operation routes, enabling predefined
// transaction patterns (e.g., "payment", "transfer", "settlement").
//
// Domain Context:
//
// Transaction routes serve as templates that:
//   - Define allowed source accounts (debit side)
//   - Define allowed destination accounts (credit side)
//   - Specify validation rules for each operation
//   - Enable consistent transaction structures across the system
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//
//	Step 2: Fetch Routes from PostgreSQL
//	  - Query all transaction routes for org/ledger
//	  - Apply cursor-based pagination
//	  - Handle not-found with business error
//
//	Step 3: Prepare Metadata Filter
//	  - Initialize empty BSON filter if nil
//	  - Ensures consistent MongoDB query behavior
//
//	Step 4: Fetch Metadata from MongoDB
//	  - Query metadata matching filter criteria
//	  - Build lookup map by entity ID
//
//	Step 5: Enrich Routes with Metadata
//	  - Assign metadata to each route by ID match
//	  - Convert UUID to string for map lookup
//
// Parameters:
//   - ctx: Request context with tenant and tracing information
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID to scope transaction routes
//   - filter: Query parameters with pagination and optional metadata filter
//
// Returns:
//   - []*mmodel.TransactionRoute: Routes with metadata, nil if none found
//   - libHTTP.CursorPagination: Cursor for paginated results
//   - error: Business error (ErrNoTransactionRoutesFound) or infrastructure error
//
// Error Scenarios:
//   - ErrNoTransactionRoutesFound: No routes configured for ledger
//   - ErrEntityNotFound: Metadata lookup failed
//   - Database errors: PostgreSQL or MongoDB connection issues
func (uc *UseCase) GetAllTransactionRoutes(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.TransactionRoute, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_transaction_routes")
	defer span.End()

	logger.Infof("Retrieving transaction routes")

	transactionRoutes, cur, err := uc.TransactionRouteRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		logger.Errorf("Error getting transaction routes on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoTransactionRoutesFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction routes on repo", err)

			logger.Warnf("Error getting transaction routes on repo: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction routes on repo", err)

		logger.Errorf("Error getting transaction routes on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if transactionRoutes != nil {
		metadataFilter := filter
		if metadataFilter.Metadata == nil {
			metadataFilter.Metadata = &bson.M{}
		}

		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), metadataFilter)
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb transaction route", err)

			logger.Warnf("Error getting metadata on mongodb transaction route: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range transactionRoutes {
			if data, ok := metadataMap[transactionRoutes[i].ID.String()]; ok {
				transactionRoutes[i].Metadata = data
			}
		}
	}

	return transactionRoutes, cur, nil
}
