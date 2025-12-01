package query

import (
	"context"
	"errors"
	"reflect"

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataTransactions retrieves transactions filtered by metadata criteria.
//
// This method implements a metadata-first query pattern, useful when searching for
// transactions by custom attributes stored in MongoDB rather than core fields in
// PostgreSQL. Common use cases include searching by external reference IDs, custom
// tags, or integration-specific identifiers.
//
// Query Strategy (Metadata-First):
//
// Unlike standard queries that start with PostgreSQL, this method:
//  1. Queries MongoDB first to find matching metadata documents
//  2. Extracts entity IDs from metadata results
//  3. Fetches full transaction records from PostgreSQL by those IDs
//
// This approach is optimal when:
//   - Filtering by metadata fields not indexed in PostgreSQL
//   - Metadata filter is highly selective (few matches expected)
//   - Full transaction data needed after metadata match
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for distributed tracing
//
//	Step 2: Query Metadata from MongoDB
//	  - Apply metadata filter from query header
//	  - Return business error if no metadata matches
//	  - Build lookup map and UUID slice from results
//
//	Step 3: Fetch Transactions from PostgreSQL
//	  - Query transactions by extracted UUIDs
//	  - Include operations in the join (FindOrListAllWithOperations)
//	  - Apply cursor pagination to results
//
//	Step 4: Enrich Operations with Metadata
//	  - Bulk fetch operation metadata for all transactions
//	  - Delegate to enrichTransactionsWithOperationMetadata helper
//
//	Step 5: Build Source/Destination Lists
//	  - Categorize operations by type (DEBIT/CREDIT)
//	  - Populate Source (debit aliases) and Destination (credit aliases)
//	  - Assign transaction metadata from lookup map
//
// Parameters:
//   - ctx: Request context with tenant and tracing information
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID to scope transactions
//   - filter: Query parameters including metadata filter criteria
//
// Returns:
//   - []*transaction.Transaction: Transactions with operations and metadata
//   - libHTTP.CursorPagination: Pagination cursor for next page
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrNoTransactionsFound: No metadata matches or no transactions for matched IDs
//   - Metadata query error: MongoDB unavailable or query syntax error
//   - Database error: PostgreSQL connection or query failure
func (uc *UseCase) GetAllMetadataTransactions(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*transaction.Transaction, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_transactions")
	defer span.End()

	logger.Infof("Retrieving transactions")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, reflect.TypeOf(transaction.Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transactions on repo by metadata", err)

		logger.Warnf("Error getting transactions on repo by metadata: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(metadata) == 0 {
		logger.Infof("No metadata found")

		return nil, libHTTP.CursorPagination{}, nil
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	trans, cur, err := uc.TransactionRepo.FindOrListAllWithOperations(ctx, organizationID, ledgerID, uuids, filter.ToCursorPagination())
	if err != nil {
		logger.Errorf("Error getting transactions on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, reflect.TypeOf(transaction.Transaction{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transactions on repo", err)

			logger.Warnf("Error getting transactions on repo: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transactions on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if err := uc.enrichTransactionsWithOperationMetadata(ctx, trans); err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	for i := range trans {
		source := make([]string, 0)
		destination := make([]string, 0)

		for _, op := range trans[i].Operations {
			switch op.Type {
			case constant.DEBIT:
				source = append(source, op.AccountAlias)
			case constant.CREDIT:
				destination = append(destination, op.AccountAlias)
			}
		}

		trans[i].Source = source
		trans[i].Destination = destination

		if data, ok := metadataMap[trans[i].ID]; ok {
			trans[i].Metadata = data
		}
	}

	return trans, cur, nil
}

// enrichTransactionsWithOperationMetadata fetches and assigns metadata to transaction operations.
//
// This helper method implements bulk metadata retrieval for operations across multiple
// transactions, avoiding the N+1 query problem when each transaction has many operations.
//
// Optimization Strategy:
//
//	Step 1: Count Total Operations
//	  - Sum operations across all transactions
//	  - Early return if no operations exist
//
//	Step 2: Collect Operation IDs
//	  - Pre-allocate slice with known capacity
//	  - Single pass through all transactions
//
//	Step 3: Bulk Metadata Fetch
//	  - Single MongoDB query for all operation IDs
//	  - Build O(1) lookup map for assignment
//
//	Step 4: Assign Metadata
//	  - Iterate operations and assign from map
//	  - Operations without metadata retain nil
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - trans: Slice of transactions with operations to enrich
//
// Returns:
//   - error: Metadata query failure (operations unchanged on error)
//
// Performance:
//   - Single MongoDB query regardless of operation count
//   - O(n) time complexity where n = total operations
//   - Memory: O(n) for ID slice and metadata map
func (uc *UseCase) enrichTransactionsWithOperationMetadata(ctx context.Context, trans []*transaction.Transaction) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_transactions_enrich_operations")
	defer span.End()

	var totalOps int
	for _, t := range trans {
		totalOps += len(t.Operations)
	}

	if totalOps == 0 {
		return nil
	}

	operationIDsAll := make([]string, 0, totalOps)

	for _, t := range trans {
		for _, op := range t.Operations {
			operationIDsAll = append(operationIDsAll, op.ID)
		}
	}

	operationMetadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationIDsAll)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation metadata", err)

		logger.Warnf("Error getting operation metadata: %v", err)

		return err
	}

	opMetadataMap := make(map[string]map[string]any, len(operationMetadata))
	for _, meta := range operationMetadata {
		opMetadataMap[meta.EntityID] = meta.Data
	}

	for i := range trans {
		for _, op := range trans[i].Operations {
			if data, ok := opMetadataMap[op.ID]; ok {
				op.Metadata = data
			}
		}
	}

	return nil
}
