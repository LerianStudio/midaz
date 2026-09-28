// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"

	// GetAllTransactions fetch all Transactions from the repository
	libLog "github.com/LerianStudio/lib-observability/v4/log"
)

// transactionLegAliases answers the source and destination alias lists every
// transaction read returns. A row that kept its submitted body answers the
// body's legs, so a read names the same accounts the create and the pending
// transition answered; the lists describe what was submitted, and the
// operations carry what each balance actually moved. A row without a body
// classifies its operations: DEBIT legs are sources and CREDIT legs
// destinations; BLOCK and UNBLOCK carry a normal accounting Direction and are
// classified by it; every other type is ignored. Both lists are non-nil.
func transactionLegAliases(operations []*operation.Operation, body mtransaction.Transaction) (source, destination []string) {
	if len(body.Send.Source.From) > 0 || len(body.Send.Distribute.To) > 0 {
		return submittedLegAliases(body.Send.Source.From), submittedLegAliases(body.Send.Distribute.To)
	}

	source = make([]string, 0)
	destination = make([]string, 0)

	for _, op := range operations {
		switch op.Type {
		case constant.DEBIT:
			source = append(source, op.AccountAlias)
		case constant.CREDIT:
			destination = append(destination, op.AccountAlias)
		case constant.BLOCK, constant.UNBLOCK:
			switch op.Direction {
			case constant.DirectionDebit:
				source = append(source, op.AccountAlias)
			case constant.DirectionCredit:
				destination = append(destination, op.AccountAlias)
			}
		}
	}

	return source, destination
}

// submittedLegAliases returns the aliases of a persisted body's legs in
// submitted order, as the bare alias whether an entry is stored as "alias",
// "alias#balanceKey", or "index#alias#balanceKey". Entries on the
// system-managed overdraft balance are skipped: they are not client legs.
func submittedLegAliases(entries []mtransaction.FromTo) []string {
	aliases := make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry.BalanceKey == constant.OverdraftBalanceKey {
			continue
		}

		aliases = append(aliases, mtransaction.BareAlias(entry.AccountAlias))
	}

	return aliases
}

func (uc *UseCase) GetAllTransactions(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) (_ []*transaction.Transaction, _ libHTTP.CursorPagination, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_transactions")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "list_transactions", start, err)
	}()

	filter.ApplyDefaultDateRange()

	trans, cur, err := uc.TransactionRepo.FindOrListAllWithOperations(ctx, organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination())
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error getting transactions on repo", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, constant.EntityTransaction)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get transactions on repo", err)

			logger.Log(ctx, libLog.LevelWarn, "Error getting transactions on repo", libLog.Err(err))

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get transactions on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(trans) == 0 {
		return trans, cur, nil
	}

	transactionIDs := make([]string, len(trans))
	for i, t := range trans {
		transactionIDs[i] = t.ID
	}

	metadata, err := uc.TransactionMetadataRepo.FindByEntityIDs(ctx, constant.EntityTransaction, transactionIDs)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, constant.EntityTransaction)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on mongodb transaction", err)

		logger.Log(ctx, libLog.LevelWarn, "Error getting metadata on mongodb transaction", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	for i := range trans {
		operationIDs := make([]string, 0, len(trans[i].Operations))
		for _, op := range trans[i].Operations {
			operationIDs = append(operationIDs, op.ID)
		}

		trans[i].Source, trans[i].Destination = transactionLegAliases(trans[i].Operations, trans[i].Body)

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

// enrichOperationsWithMetadata retrieves and assigns metadata to operations
func (uc *UseCase) enrichOperationsWithMetadata(ctx context.Context, operations []*operation.Operation, operationIDs []string) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_transactions_enrich_operations_with_metadata")
	defer span.End()

	operationMetadata, err := uc.TransactionMetadataRepo.FindByEntityIDs(ctx, constant.EntityOperation, operationIDs)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get operation metadata", err)

		logger.Log(ctx, libLog.LevelWarn, "Error getting operation metadata", libLog.Err(err))

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

func (uc *UseCase) GetOperationsByTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, tran *transaction.Transaction, filter http.QueryHeader) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_transactions_get_operations")
	defer span.End()

	operations, _, err := uc.GetAllOperations(ctx, organizationID, ledgerID, tran.IDtoUUID(), filter)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve Operations", err)

		logger.Log(ctx, libLog.LevelError, "Failed to retrieve operations",
			libLog.String("transaction_id", tran.IDtoUUID().String()), libLog.Err(err))

		return nil, err
	}

	tran.Source, tran.Destination = transactionLegAliases(operations, tran.Body)
	tran.Operations = operations

	return tran, nil
}
