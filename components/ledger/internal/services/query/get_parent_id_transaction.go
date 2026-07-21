// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"

	// GetParentByTransactionID gets data in the repository.
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) GetParentByTransactionID(ctx context.Context, organizationID, ledgerID, parentID uuid.UUID) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_parent_by_transaction_id")
	defer span.End()

	tran, err := uc.TransactionRepo.FindByParentID(ctx, organizationID, ledgerID, parentID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get parent transaction on repo by id", err)

		logger.Log(ctx, libLog.LevelError, "Error getting parent transaction", libLog.Err(err))

		return nil, err
	}

	if tran != nil {
		metadata, err := uc.TransactionMetadataRepo.FindByEntity(ctx, constant.EntityTransaction, tran.ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to get metadata on mongodb account", err)

			logger.Log(ctx, libLog.LevelError, "Error get metadata on mongodb account", libLog.Err(err))

			return nil, err
		}

		if metadata != nil {
			tran.Metadata = metadata.Data
		}
	}

	return tran, nil
}
