package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// GetAllMetadataTransactions fetch all Transactions from the repository
func (uc *UseCase) GetAllMetadataTransactions(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*transaction.Transaction, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_transactions")
	defer span.End()

	logger.Infof("Retrieving transactions")

	metadata, err := uc.fetchTransactionMetadata(ctx, &span, logger, filter)
	if err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	if len(metadata) == 0 {
		logger.Infof("No metadata found")
		return nil, libHTTP.CursorPagination{}, nil
	}

	uuids, metadataMap := uc.prepareMetadataLookup(metadata)

	trans, cur, err := uc.fetchTransactionsWithOperations(ctx, &span, logger, organizationID, ledgerID, uuids, filter)
	if err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	if err := uc.enrichTransactionsWithOperationMetadata(ctx, trans); err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	uc.populateTransactionSourcesAndMetadata(trans, metadataMap)

	return trans, cur, nil
}

// fetchTransactionMetadata fetches metadata for transactions
func (uc *UseCase) fetchTransactionMetadata(ctx context.Context, span *trace.Span, logger libLog.Logger, filter http.QueryHeader) ([]*mongodb.Metadata, error) {
	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), filter)
	if err != nil || metadata == nil {
		businessErr := pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, reflect.TypeOf(transaction.Transaction{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get transactions on repo by metadata", businessErr)
		logger.Warnf("Error getting transactions on repo by metadata: %v", businessErr)

		return nil, businessErr
	}

	return metadata, nil
}

// prepareMetadataLookup prepares UUID list and metadata map from metadata
func (uc *UseCase) prepareMetadataLookup(metadata []*mongodb.Metadata) ([]uuid.UUID, map[string]map[string]any) {
	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		assert.That(assert.ValidUUID(meta.EntityID),
			"metadata entity ID must be valid UUID",
			"value", meta.EntityID,
			"index", i)
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	return uuids, metadataMap
}

// fetchTransactionsWithOperations fetches transactions with their operations
func (uc *UseCase) fetchTransactionsWithOperations(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, uuids []uuid.UUID, filter http.QueryHeader) ([]*transaction.Transaction, libHTTP.CursorPagination, error) {
	trans, cur, err := uc.TransactionRepo.FindOrListAllWithOperations(ctx, organizationID, ledgerID, uuids, filter.ToCursorPagination())
	if err != nil {
		return uc.handleTransactionFetchError(span, logger, err)
	}

	return trans, cur, nil
}

// handleTransactionFetchError handles errors when fetching transactions
func (uc *UseCase) handleTransactionFetchError(span *trace.Span, logger libLog.Logger, err error) ([]*transaction.Transaction, libHTTP.CursorPagination, error) {
	logger.Errorf("Error getting transactions on repo: %v", err)

	if errors.Is(err, services.ErrDatabaseItemNotFound) {
		businessErr := pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, reflect.TypeOf(transaction.Transaction{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get transactions on repo", businessErr)
		logger.Warnf("Error getting transactions on repo: %v", businessErr)

		return nil, libHTTP.CursorPagination{}, businessErr
	}

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get transactions on repo", err)

	return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, "Transaction")
}

// populateTransactionSourcesAndMetadata populates source, destination and metadata for transactions
func (uc *UseCase) populateTransactionSourcesAndMetadata(trans []*transaction.Transaction, metadataMap map[string]map[string]any) {
	for i := range trans {
		source, destination := uc.extractSourcesAndDestinations(trans[i].Operations)

		trans[i].Source = source
		trans[i].Destination = destination

		if data, ok := metadataMap[trans[i].ID]; ok {
			trans[i].Metadata = data
		}
	}
}

// extractSourcesAndDestinations extracts source and destination account aliases from operations
func (uc *UseCase) extractSourcesAndDestinations(operations []*operation.Operation) ([]string, []string) {
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

	return source, destination
}

// enrichTransactionsWithOperationMetadata fetches operation metadata in bulk and assigns it to operations
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

		return pkg.ValidateInternalError(err, "Operation")
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
