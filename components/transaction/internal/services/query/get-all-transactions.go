package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/contextutils"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// GetAllTransactions fetch all Transactions from the repository
func (uc *UseCase) GetAllTransactions(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*transaction.Transaction, libHTTP.CursorPagination, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_transactions")
	defer span.End()

	logger.Infof("Retrieving transactions")

	trans, cur, err := uc.fetchAllTransactions(ctx, &span, logger, organizationID, ledgerID, filter)
	if err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	if len(trans) == 0 {
		return trans, cur, nil
	}

	metadataMap, err := uc.fetchAndMapTransactionMetadata(ctx, &span, logger, trans)
	if err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	if err := uc.enrichTransactionsWithAllMetadata(ctx, trans, metadataMap); err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	setTransactionDefaults(trans)

	return trans, cur, nil
}

// fetchAllTransactions fetches all transactions from repository
func (uc *UseCase) fetchAllTransactions(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*transaction.Transaction, libHTTP.CursorPagination, error) {
	trans, cur, err := uc.TransactionRepo.FindOrListAllWithOperations(ctx, organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination())
	if err != nil {
		return uc.handleFetchTransactionsError(span, logger, err)
	}

	return trans, cur, nil
}

// handleFetchTransactionsError handles errors when fetching transactions
func (uc *UseCase) handleFetchTransactionsError(span *trace.Span, logger libLog.Logger, err error) ([]*transaction.Transaction, libHTTP.CursorPagination, error) {
	logger.Errorf("Error getting transactions on repo: %v", err)

	var entityNotFound *pkg.EntityNotFoundError
	if errors.As(err, &entityNotFound) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get transactions on repo", err)
		logger.Warnf("Error getting transactions on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get transactions on repo", err)

	return nil, libHTTP.CursorPagination{}, err
}

// fetchAndMapTransactionMetadata fetches and maps transaction metadata
func (uc *UseCase) fetchAndMapTransactionMetadata(ctx context.Context, span *trace.Span, logger libLog.Logger, trans []*transaction.Transaction) (map[string]map[string]any, error) {
	transactionIDs := uc.extractTransactionIDs(trans)

	metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), transactionIDs)
	if err != nil {
		businessErr := pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, reflect.TypeOf(transaction.Transaction{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on mongodb transaction", businessErr)
		logger.Warnf("Error getting metadata on mongodb transaction: %v", businessErr)

		return nil, businessErr
	}

	return uc.buildMetadataMap(metadata), nil
}

// extractTransactionIDs extracts transaction IDs from transaction list
func (uc *UseCase) extractTransactionIDs(trans []*transaction.Transaction) []string {
	transactionIDs := make([]string, len(trans))
	for i, t := range trans {
		transactionIDs[i] = t.ID
	}

	return transactionIDs
}

// buildMetadataMap builds a map of entity ID to metadata
func (uc *UseCase) buildMetadataMap(metadata []*mongodb.Metadata) map[string]map[string]any {
	metadataMap := make(map[string]map[string]any, len(metadata))
	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	return metadataMap
}

// enrichTransactionsWithAllMetadata enriches transactions with transaction and operation metadata
func (uc *UseCase) enrichTransactionsWithAllMetadata(ctx context.Context, trans []*transaction.Transaction, metadataMap map[string]map[string]any) error {
	for i := range trans {
		if err := uc.enrichSingleTransaction(ctx, trans[i], metadataMap); err != nil {
			return err
		}
	}

	return nil
}

// enrichSingleTransaction enriches a single transaction with metadata and operation details
func (uc *UseCase) enrichSingleTransaction(ctx context.Context, tran *transaction.Transaction, metadataMap map[string]map[string]any) error {
	source, destination, operationIDs := uc.processTransactionOperations(tran.Operations)

	tran.Source = source
	tran.Destination = destination

	if data, ok := metadataMap[tran.ID]; ok {
		tran.Metadata = data
	}

	if len(operationIDs) == 0 {
		backfillTransactionMetadataFromBody(tran)
		return nil
	}

	if err := uc.enrichTransactionOperationsMetadata(ctx, tran.Operations, operationIDs); err != nil {
		return err
	}

	backfillTransactionMetadataFromBody(tran)

	return nil
}

// processTransactionOperations processes operations and extracts sources, destinations and IDs
func (uc *UseCase) processTransactionOperations(operations []*operation.Operation) ([]string, []string, []string) {
	source := make([]string, 0)
	destination := make([]string, 0)
	operationIDs := make([]string, 0, len(operations))

	for _, op := range operations {
		operationIDs = append(operationIDs, op.ID)

		switch op.Type {
		case constant.DEBIT:
			source = append(source, op.AccountAlias)
		case constant.CREDIT:
			destination = append(destination, op.AccountAlias)
		}
	}

	return source, destination, operationIDs
}

// enrichTransactionOperationsMetadata retrieves and assigns metadata to operations
func (uc *UseCase) enrichTransactionOperationsMetadata(ctx context.Context, operations []*operation.Operation, operationIDs []string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_transactions_enrich_operations_with_metadata")
	defer span.End()

	operationMetadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationIDs)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation metadata", err)
		logger.Warnf("Error getting operation metadata: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(operation.Operation{}).Name())
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

	uc.enrichMissingMetadataFromOutbox(ctx, operations, &span, logger)

	return nil
}

// enrichMissingMetadataFromOutbox fills in missing operation metadata from the outbox.
func (uc *UseCase) enrichMissingMetadataFromOutbox(ctx context.Context, operations []*operation.Operation, span *trace.Span, logger libLog.Logger) {
	if uc.OutboxRepo == nil {
		return
	}

	ids := uc.collectOperationIDsNeedingMetadata(operations)
	if len(ids) == 0 {
		return
	}

	metadataByID, errorsByID := uc.fetchMetadataFromOutboxBatch(ctx, outbox.EntityTypeOperation, ids)
	metricFactory := contextutils.MetricsFromContext(ctx)

	uc.applyOutboxMetadataToOperations(ctx, operations, metadataByID, errorsByID, span, logger, metricFactory)
}

// collectOperationIDsNeedingMetadata returns deduplicated IDs of operations missing metadata.
func (uc *UseCase) collectOperationIDsNeedingMetadata(operations []*operation.Operation) []string {
	ids := make([]string, 0)
	seen := make(map[string]struct{})

	for j := range operations {
		if len(operations[j].Metadata) != 0 {
			continue
		}

		if _, ok := seen[operations[j].ID]; ok {
			continue
		}

		seen[operations[j].ID] = struct{}{}
		ids = append(ids, operations[j].ID)
	}

	return ids
}

// applyOutboxMetadataToOperations applies fetched metadata to operations that need it.
func (uc *UseCase) applyOutboxMetadataToOperations(ctx context.Context, operations []*operation.Operation, metadataByID map[string]map[string]any, errorsByID map[string]error, span *trace.Span, logger libLog.Logger, metricFactory *metrics.MetricsFactory) {
	if len(metadataByID) == 0 && len(errorsByID) == 0 {
		return
	}

	for j := range operations {
		if len(operations[j].Metadata) != 0 {
			continue
		}

		if err, ok := errorsByID[operations[j].ID]; ok && err != nil {
			uc.logOutboxFetchError(ctx, span, logger, metricFactory, operations[j].ID, err)
			continue
		}

		if md, ok := metadataByID[operations[j].ID]; ok && len(md) != 0 {
			operations[j].Metadata = md
		}
	}
}

// logOutboxFetchError logs and records metrics for outbox fetch failures.
func (uc *UseCase) logOutboxFetchError(ctx context.Context, span *trace.Span, logger libLog.Logger, metricFactory *metrics.MetricsFactory, operationID string, err error) {
	libOpentelemetry.HandleSpanError(span, "Failed to fetch operation metadata from outbox", err)

	if metricFactory != nil {
		metricFactory.Counter(utils.TransactionOutboxFetchFailures).
			WithLabels(map[string]string{
				"entity_type": outbox.EntityTypeOperation,
			}).
			AddOne(ctx)
	}

	logger.WithFields(
		"operation_id", operationID,
		"entity_type", outbox.EntityTypeOperation,
		"outbox_fetch_failed", true,
		"outbox_error", err.Error(),
	).Warnf("Error fetching operation metadata from outbox: %v", err)
}

// GetOperationsByTransaction retrieves all operations associated with a transaction and attaches them to the transaction object.
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

	if operations == nil {
		operations = make([]*operation.Operation, 0)
	}

	tran.Source = source
	tran.Destination = destination
	tran.Operations = operations
	backfillTransactionMetadataFromBody(tran)

	return tran, nil
}

// setTransactionDefaults ensures transactions have non-nil metadata and operations slices
func setTransactionDefaults(trans []*transaction.Transaction) {
	for i := range trans {
		if trans[i].Metadata == nil {
			trans[i].Metadata = map[string]any{}
		}

		if trans[i].Operations == nil {
			trans[i].Operations = make([]*operation.Operation, 0)
		}
	}
}
