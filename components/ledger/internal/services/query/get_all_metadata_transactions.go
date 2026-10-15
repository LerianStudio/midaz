// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// GetAllMetadataTransactions fetches the transactions whose metadata matches the filter.
func (uc *UseCase) GetAllMetadataTransactions(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*transaction.Transaction, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_transactions")
	defer span.End()

	filter.ApplyDefaultDateRange()

	metadata, err := uc.TransactionMetadataRepo.FindList(ctx, constant.EntityTransaction, filter)
	if err != nil {
		recordQueryError(ctx, span, logger, "Failed to get transaction metadata on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(metadata) == 0 {
		span.AddEvent("No metadata matched the filter")

		return []*transaction.Transaction{}, libHTTP.CursorPagination{}, nil
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	trans, cur, err := uc.TransactionRepo.FindOrListAllWithOperations(ctx, organizationID, ledgerID, uuids, filter.ToCursorPagination())
	if err != nil {
		recordQueryError(ctx, span, logger, "Failed to get transactions on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if trans == nil {
		trans = []*transaction.Transaction{}
	}

	if err := uc.enrichTransactionsWithOperationMetadata(ctx, trans); err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	for i := range trans {
		trans[i].Source, trans[i].Destination = transactionLegAliases(trans[i].Operations, trans[i].Body)

		if data, ok := metadataMap[trans[i].ID]; ok {
			trans[i].Metadata = data
		}
	}

	return trans, cur, nil
}

// enrichTransactionsWithOperationMetadata fetches operation metadata in bulk and assigns it to operations
func (uc *UseCase) enrichTransactionsWithOperationMetadata(ctx context.Context, trans []*transaction.Transaction) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

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

	operationMetadata, err := uc.TransactionMetadataRepo.FindByEntityIDs(ctx, constant.EntityOperation, operationIDsAll)
	if err != nil {
		recordQueryError(ctx, span, logger, "Failed to get operation metadata on repo", err)

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
