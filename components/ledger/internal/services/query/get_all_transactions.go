// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"

	// GetAllTransactions fetch all Transactions from the repository
	libLog "github.com/LerianStudio/lib-observability/log"
)

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
			case constant.BLOCK, constant.UNBLOCK:
				// BLOCK/UNBLOCK operations carry a normal accounting Direction
				// (debit-side -> Source, credit-side -> Destination), so they
				// are classified by Direction exactly as DEBIT/CREDIT are.
				switch op.Direction {
				case constant.DirectionDebit:
					source = append(source, op.AccountAlias)
				case constant.DirectionCredit:
					destination = append(destination, op.AccountAlias)
				}
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

	source := make([]string, 0)
	destination := make([]string, 0)

	for _, op := range operations {
		switch op.Type {
		case constant.DEBIT:
			source = append(source, op.AccountAlias)
		case constant.CREDIT:
			destination = append(destination, op.AccountAlias)
		case constant.BLOCK, constant.UNBLOCK:
			// BLOCK/UNBLOCK operations carry a normal accounting Direction
			// (debit-side -> Source, credit-side -> Destination), so they
			// are classified by Direction exactly as DEBIT/CREDIT are.
			switch op.Direction {
			case constant.DirectionDebit:
				source = append(source, op.AccountAlias)
			case constant.DirectionCredit:
				destination = append(destination, op.AccountAlias)
			}
		}
	}

	tran.Source = source
	tran.Destination = destination
	tran.Operations = operations

	return tran, nil
}
