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
)

// GetAllMetadataTransactionRoutes retrieves transaction routes filtered by metadata criteria.
//
// This method implements metadata-based filtering for transaction routes. It first
// queries MongoDB for routes matching metadata criteria, then retrieves the full
// route entities from PostgreSQL, effectively using metadata as a secondary index.
//
// Query Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "query.get_all_metadata_transaction_routes"
//
//	Step 2: Metadata Query
//	  - Query MetadataRepo.FindList with TransactionRoute entity type and filter
//	  - If no metadata found or error: Return ErrNoTransactionRoutesFound
//	  - Build metadata map keyed by entity ID
//
//	Step 3: Route Retrieval
//	  - Query TransactionRouteRepo.FindAll with pagination
//	  - If routes not found: Return ErrNoTransactionRoutesFound
//	  - If other error: Return wrapped error with span event
//
//	Step 4: Metadata Join
//	  - Filter routes to only those present in metadata results
//	  - Attach metadata to each matching route
//	  - Build filtered result set
//
//	Step 5: Response
//	  - Return filtered routes with metadata and pagination cursor
//
// Transaction Routes vs Operation Routes:
//
// Transaction routes define rules at the transaction level:
//   - Apply to entire transactions based on type or criteria
//   - Can trigger validation rules, compliance checks
//   - Define transaction-level processing workflows
//
// Operation routes (see GetAllMetadataOperationRoutes) apply at the
// individual operation level within a transaction.
//
// Metadata Filtering Strategy:
//
// This query uses a "filter-then-join" approach:
//  1. MongoDB returns entity IDs matching metadata criteria
//  2. PostgreSQL returns all routes with pagination
//  3. Results are joined in memory, keeping only matching routes
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the routes
//   - filter: Query parameters including metadata criteria and pagination
//
// Returns:
//   - []*mmodel.TransactionRoute: Filtered routes with metadata
//   - libHTTP.CursorPagination: Pagination cursor for next page
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrNoTransactionRoutesFound: No routes match metadata criteria
//   - Database connection failure
//   - MongoDB metadata query failure
func (uc *UseCase) GetAllMetadataTransactionRoutes(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.TransactionRoute, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_transaction_routes")
	defer span.End()

	logger.Infof("Retrieving transaction routes by metadata")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoTransactionRoutesFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction routes on repo by metadata", err)

		logger.Warnf("Error getting transaction routes on repo by metadata: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	allTransactionRoutes, cur, err := uc.TransactionRouteRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		logger.Errorf("Error getting transaction routes on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoTransactionRoutesFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction routes on repo", err)

			logger.Warnf("Error getting transaction routes on repo: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction routes on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	var filteredTransactionRoutes []*mmodel.TransactionRoute

	for _, transactionRoute := range allTransactionRoutes {
		if data, ok := metadataMap[transactionRoute.ID.String()]; ok {
			transactionRoute.Metadata = data
			filteredTransactionRoutes = append(filteredTransactionRoutes, transactionRoute)
		}
	}

	return filteredTransactionRoutes, cur, nil
}
