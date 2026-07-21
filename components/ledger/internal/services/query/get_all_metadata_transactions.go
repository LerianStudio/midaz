// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/google/uuid"

	// GetAllMetadataTransactions fetch all Transactions from the repository
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) GetAllMetadataTransactions(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*transaction.Transaction, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_transactions")
	defer span.End()

	filter.ApplyDefaultDateRange()

	metadata, err := uc.TransactionMetadataRepo.FindList(ctx, constant.EntityTransaction, filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoTransactionsFound, constant.EntityTransaction)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get transactions on repo by metadata", err)

		logger.Log(ctx, libLog.LevelWarn, "Error getting transactions on repo by metadata", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(metadata) == 0 {
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
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get operation metadata", err)

		logger.Log(ctx, libLog.LevelWarn, "Error getting operation metadata", libLog.Err(err))

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
