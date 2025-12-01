package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataOperations retrieves operations filtered by metadata criteria for a specific account.
//
// This method implements a metadata-first query pattern for operations, useful when
// searching by custom attributes stored in MongoDB. Unlike GetAllOperationsByAccount
// which fetches all operations then enriches with metadata, this method filters
// by metadata first, returning only operations that match the metadata criteria.
//
// Use Cases:
//   - Find operations tagged with specific external reference IDs
//   - Search operations by custom integration attributes
//   - Filter operations by user-defined metadata fields
//
// Query Strategy (Metadata-First):
//
//	1. Query MongoDB for metadata matching filter criteria
//	2. Build lookup map of entity ID -> metadata
//	3. Fetch operations from PostgreSQL for the account
//	4. Return only operations that have matching metadata
//
// This approach differs from standard queries:
//   - Standard: Fetch all operations, then enrich with metadata
//   - Metadata-first: Filter by metadata, then fetch matching operations
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//
//	Step 2: Query Metadata from MongoDB
//	  - Apply metadata filter from query header
//	  - Return business error if no matches
//	  - Build lookup map indexed by entity ID
//
//	Step 3: Fetch Operations from PostgreSQL
//	  - Query operations for the account
//	  - Apply operation type filter if specified
//	  - Use cursor-based pagination
//
//	Step 4: Filter and Enrich Operations
//	  - Only include operations with matching metadata
//	  - Assign metadata to each matching operation
//	  - Non-matching operations are excluded from results
//
// Parameters:
//   - ctx: Request context with tenant and tracing information
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID for operation scope
//   - accountID: Account UUID to filter operations
//   - filter: Query parameters including metadata filter criteria
//
// Returns:
//   - []*operation.Operation: Operations with matching metadata
//   - libHTTP.CursorPagination: Pagination cursor for next page
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrNoOperationsFound: No metadata matches or no operations for account
//   - Metadata query error: MongoDB unavailable or query failure
//   - Database error: PostgreSQL connection or query failure
//
// Note: Pagination applies to the PostgreSQL query, not the final filtered result.
// This may result in fewer results than requested if many operations lack matching metadata.
func (uc *UseCase) GetAllMetadataOperations(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.QueryHeader) ([]*operation.Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_operations")
	defer span.End()

	logger.Infof("Retrieving operations")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(operation.Operation{}).Name(), filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operations on repo by metadata", err)

		logger.Warnf("Error getting operations on repo by metadata: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	oper, cur, err := uc.OperationRepo.FindAllByAccount(ctx, organizationID, ledgerID, accountID, &filter.OperationType, filter.ToCursorPagination())
	if err != nil {
		logger.Errorf("Error getting operations on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operations on repo", err)

			logger.Warnf("Error getting operations on repo: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operations on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	var operations []*operation.Operation

	for _, o := range oper {
		if data, ok := metadataMap[o.ID]; ok {
			o.Metadata = data
			operations = append(operations, o)
		}
	}

	return operations, cur, nil
}
