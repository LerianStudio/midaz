// Package query provides CQRS query handlers for the transaction bounded context.
//
// This package implements read-only operations for retrieving transaction-related
// entities from the ledger system. It follows the CQRS (Command Query Responsibility
// Segregation) pattern, separating read operations from write operations.
//
// Query Handlers:
//
//   - Operations: Retrieve ledger operations by account, transaction, or metadata
//   - Transactions: Fetch transactions with associated operations and metadata
//   - Balances: Query account balances with Redis cache integration
//   - Transaction Routes: Retrieve accounting route configurations
//   - Operation Routes: Fetch operation-level routing rules
//   - Asset Rates: Query currency exchange rates
//
// Architecture:
//
// Query handlers use a multi-store approach:
//   - PostgreSQL: Primary data source for transactional entities
//   - MongoDB: Metadata storage for flexible key-value attributes
//   - Redis/Valkey: Cache layer for balance lookups and route configurations
//
// Data Enrichment Pattern:
//
// Most queries follow a two-phase retrieval:
//  1. Fetch core entity data from PostgreSQL
//  2. Enrich with metadata from MongoDB (joined by entity ID)
//
// This separation allows flexible metadata schemas without PostgreSQL migrations.
//
// Thread Safety:
//
// UseCase is safe for concurrent use. All methods are read-only and use
// context-scoped database connections from the repository layer.
//
// Related Packages:
//   - adapters/postgres: PostgreSQL repository implementations
//   - adapters/mongodb: MongoDB metadata repository
//   - adapters/redis: Redis cache repository
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

// GetAllOperationsByAccount retrieves all ledger operations associated with a specific account.
//
// This method fetches operations where the account participated as either source (debit)
// or destination (credit), enriching each operation with its associated metadata from MongoDB.
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for distributed tracing
//
//	Step 2: Fetch Operations from PostgreSQL
//	  - Query operations by organization, ledger, and account IDs
//	  - Apply optional operation type filter from query header
//	  - Use cursor-based pagination for efficient large result sets
//	  - Return early if no operations found
//
//	Step 3: Collect Operation IDs
//	  - Build slice of operation IDs for bulk metadata lookup
//	  - Avoid N+1 query pattern by batching metadata retrieval
//
//	Step 4: Fetch Metadata from MongoDB
//	  - Bulk query metadata documents by entity IDs
//	  - Build lookup map for O(1) metadata assignment
//
//	Step 5: Enrich Operations with Metadata
//	  - Iterate operations and assign metadata from lookup map
//	  - Preserve operations without metadata (optional field)
//
// Parameters:
//   - ctx: Request context with tenant information and tracing
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID within the organization
//   - accountID: Account UUID to filter operations
//   - filter: Query parameters including pagination and operation type filter
//
// Returns:
//   - []*operation.Operation: Slice of operations with metadata, empty slice if none found
//   - libHTTP.CursorPagination: Pagination cursor for next/previous page
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrNoOperationsFound: No operations exist for the given account
//   - Database connection errors: PostgreSQL or MongoDB unavailable
//   - Metadata lookup failures: MongoDB query errors
func (uc *UseCase) GetAllOperationsByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.QueryHeader) ([]*operation.Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_operations_by_account")
	defer span.End()

	logger.Infof("Retrieving operations by account")

	op, cur, err := uc.OperationRepo.FindAllByAccount(ctx, organizationID, ledgerID, accountID, &filter.OperationType, filter.ToCursorPagination())
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

	if len(op) == 0 {
		return op, cur, nil
	}

	operationIDs := make([]string, len(op))
	for i, o := range op {
		operationIDs[i] = o.ID
	}

	metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationIDs)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb operation", err)

		logger.Warnf("Error getting metadata on mongodb operation: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	for i := range op {
		if data, ok := metadataMap[op[i].ID]; ok {
			op[i].Metadata = data
		}
	}

	return op, cur, nil
}
