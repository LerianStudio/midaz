// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"strings"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v6/commons/net/http"
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

// resolveDestination returns the operation-reconstructed destination when it is
// non-empty; otherwise it falls back to the submitted destination derived from
// the body. A reconstructed destination is never overwritten, and a transaction
// with no submitted destination stays empty. Keeping this decision in one place
// keeps GET-listing and GET-individual reads consistent.
func resolveDestination(reconstructed []string, body mtransaction.Transaction) []string {
	if len(reconstructed) > 0 {
		return reconstructed
	}

	if derived := deriveDestinationFromBody(body); len(derived) > 0 {
		return derived
	}

	return reconstructed
}

// deriveDestinationFromBody returns the submitted destination aliases from a
// persisted transaction body, in the same bare-alias form the write path caches
// via getAliasWithoutKey(filterCompanionAliases(...)): the system-managed
// overdraft companion is skipped and any "#balanceKey" suffix is stripped.
//
// It is the canonical fallback when operation-based reconstruction yields no
// destination — typically a pre-commit overdraft, whose persisted legs are all
// source-side (DEBIT + ON_HOLD + OVERDRAFT) with no CREDIT leg, so the submitted
// destination survives only in the body. Keeping the alias treatment identical
// to the cache path avoids trading a cache-vs-DB emptiness gap for a cache-vs-DB
// format gap.
func deriveDestinationFromBody(body mtransaction.Transaction) []string {
	to := body.Send.Distribute.To
	if len(to) == 0 {
		return nil
	}

	destination := make([]string, 0, len(to))

	for _, entry := range to {
		if entry.BalanceKey == constant.OverdraftBalanceKey {
			continue
		}

		alias := entry.AccountAlias
		if idx := strings.Index(alias, mtransaction.AliasSeparatorString); idx >= 0 {
			alias = alias[:idx]
		}

		destination = append(destination, alias)
	}

	return destination
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
		trans[i].Destination = resolveDestination(destination, trans[i].Body)

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
	tran.Destination = resolveDestination(destination, tran.Body)
	tran.Operations = operations

	return tran, nil
}
