// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/google/uuid"

	// GetParentByTransactionID gets data in the repository.
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
)

func (uc *UseCase) GetParentByTransactionID(ctx context.Context, organizationID, ledgerID, parentID uuid.UUID) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_parent_by_transaction_id")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Trying to get transaction")

	tran, err := uc.TransactionRepo.FindByParentID(ctx, organizationID, ledgerID, parentID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get parent transaction on repo by id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error getting parent transaction: %v", err))

		return nil, err
	}

	if tran != nil {
		metadata, err := uc.TransactionMetadataRepo.FindByEntity(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), tran.ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to get metadata on mongodb account", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error get metadata on mongodb account: %v", err))

			return nil, err
		}

		if metadata != nil {
			tran.Metadata = metadata.Data
		}
	}

	return tran, nil
}
