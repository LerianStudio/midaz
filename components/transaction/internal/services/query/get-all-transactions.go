// Package query implements read operations (queries) for the transaction service.
// This file contains queries for retrieving transactions.
package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllTransactions retrieves a paginated list of transactions, enriched with
// their operations and metadata.
//
// This use case fetches transactions and their associated operations from PostgreSQL,
// then enriches both with metadata from MongoDB.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - filter: Query parameters for pagination, sorting, and date range filtering.
//
// Returns:
//   - []*transaction.Transaction: A slice of transactions with their operations and metadata.
//   - libHTTP.CursorPagination: Pagination information for the result set.
//   - error: An error if the retrieval fails.
func (uc *UseCase) GetAllTransactions(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*transaction.Transaction, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_transactions")
	defer span.End()

	logger.Infof("Retrieving transactions")

	trans, cur, err := uc.TransactionRepo.FindOrListAllWithOperations(ctx, organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination())
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

	if len(trans) == 0 {
		return trans, cur, nil
	}

	transactionIDs := make([]string, len(trans))
	for i, t := range trans {
		transactionIDs[i] = t.ID
	}

	metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), transactionIDs)
	if err != nil {
		// FIXME: This error seems incorrect. It should be a more generic error,
		// as the metadata might not be found, which is not a business error.
		err := pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, reflect.TypeOf(transaction.Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb transaction", err)

		logger.Warnf("Error getting metadata on mongodb transaction: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	for i := range trans {
		source := make([]string, 0)
		destination := make([]string, 0)

		operationIDs := make([]string, 0, len(trans[i].Operations))
		for _, op := range trans[i].Operations {
			operationIDs = append(operationIDs, op.ID)

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

		if len(operationIDs) > 0 {
			if err := uc.enrichOperationsWithMetadata(ctx, trans[i].Operations, operationIDs); err != nil {
				return nil, libHTTP.CursorPagination{}, err
			}
		}
	}

	return trans, cur, nil
}

// enrichOperationsWithMetadata fetches and merges metadata for a slice of operations.
//
// This helper function optimizes metadata retrieval by fetching all operation
// metadata for a given set of IDs in a single query to MongoDB.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - operations: A slice of operations to enrich.
//   - operationIDs: A slice of operation IDs to fetch metadata for.
//
// Returns:
//   - error: An error if the metadata retrieval fails.
func (uc *UseCase) enrichOperationsWithMetadata(ctx context.Context, operations []*operation.Operation, operationIDs []string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_transactions_enrich_operations_with_metadata")
	defer span.End()

	operationMetadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationIDs)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation metadata", err)

		logger.Warnf("Error getting operation metadata: %v", err)

		return err
	}

	operationMetadataMap := make(map[string]map[string]any, len(operationMetadata))
	for _, meta := range operationMetadata {
		operationMetadataMap[meta.EntityID] = meta.Data
	}

	for j := range operations {
		if opData, ok := operationMetadataMap[operations[j].ID]; ok {
			operations[j].Metadata = opData
		}
	}

	return nil
}

// GetOperationsByTransaction retrieves all operations for a given transaction
// and enriches the transaction object with them.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - tran: The transaction to enrich with its operations.
//   - filter: Query parameters for the operations.
//
// Returns:
//   - *transaction.Transaction: The transaction enriched with its operations.
//   - error: An error if the retrieval fails.
func (uc *UseCase) GetOperationsByTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, tran *transaction.Transaction, filter http.QueryHeader) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_transactions_get_operations")
	defer span.End()

	logger.Infof("Retrieving Operations")

	operations, _, err := uc.GetAllOperations(ctx, organizationID, ledgerID, tran.IDtoUUID(), filter)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve Operations", err)

		logger.Errorf("Failed to retrieve Operations with ID: %s, Error: %s", tran.IDtoUUID(), err.Error())

		return nil, err
	}

	source := make([]string, 0)
	destination := make([]string, 0)

	for _, op := range operations {
		switch op.Type {
		case constant.DEBIT:
			source = append(source, op.AccountAlias)
		case constant.CREDIT:
			destination = append(destination, op.AccountAlias)
		}
	}

	tran.Source = source
	tran.Destination = destination
	tran.Operations = operations

	return tran, nil
}
