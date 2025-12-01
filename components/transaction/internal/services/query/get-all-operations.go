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

// GetAllOperations retrieves all operations for a transaction with metadata.
//
// This method fetches all operations belonging to a specific transaction,
// enriching each operation with its associated metadata from MongoDB.
// Operations are returned with cursor-based pagination support.
//
// Query Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "query.get_all_operations"
//
//	Step 2: Operations Retrieval
//	  - Query OperationRepo.FindAll with organization, ledger, and transaction IDs
//	  - Apply cursor pagination from filter
//	  - If operations not found: Return ErrNoOperationsFound business error
//	  - If other error: Return wrapped error with span event
//
//	Step 3: Empty Check
//	  - If no operations returned: Return empty slice with pagination
//	  - Skip metadata enrichment for empty results
//
//	Step 4: Operation IDs Collection
//	  - Build list of operation IDs for batch metadata query
//	  - Enables efficient single MongoDB query for all metadata
//
//	Step 5: Metadata Batch Retrieval
//	  - Query MetadataRepo.FindByEntityIDs for all operation IDs
//	  - If metadata retrieval fails: Return ErrNoOperationsFound
//	  - Build metadata map keyed by entity ID
//
//	Step 6: Metadata Attachment
//	  - Iterate through operations
//	  - Attach matching metadata from map to each operation
//
//	Step 7: Response
//	  - Return enriched operations with pagination cursor
//
// Operation Contents:
//
// Each operation in the result contains:
//   - ID: Unique operation identifier
//   - TransactionID: Parent transaction reference
//   - AccountID: Account affected by operation
//   - Type: "debit" or "credit"
//   - Amount: Monetary value
//   - AssetCode: Currency/asset code
//   - BalanceBefore/BalanceAfter: Balance snapshots
//   - Metadata: Custom key-value data
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the operations
//   - transactionID: UUID of the parent transaction
//   - filter: Query parameters including cursor pagination
//
// Returns:
//   - []*operation.Operation: Operations with metadata
//   - libHTTP.CursorPagination: Pagination cursor for next page
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrNoOperationsFound: Transaction has no operations or not found
//   - Database connection failure
//   - MongoDB metadata retrieval failure
func (uc *UseCase) GetAllOperations(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, filter http.QueryHeader) ([]*operation.Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_operations")
	defer span.End()

	logger.Infof("Retrieving operations by account")

	op, cur, err := uc.OperationRepo.FindAll(ctx, organizationID, ledgerID, transactionID, filter.ToCursorPagination())
	if err != nil {
		logger.Errorf("Error getting all operations on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get all operations on repo", err)

			logger.Warnf("Error getting all operations on repo: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get all operations on repo", err)

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
