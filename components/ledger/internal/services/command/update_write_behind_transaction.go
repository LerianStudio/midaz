// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"

	// UpdateWriteBehindTransaction re-serializes and updates the transaction in the write-behind cache.
	// Called after cancel/commit to reflect the updated status and operations. Errors are logged but
	// do not block the transaction flow.
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
)

func (uc *UseCase) UpdateWriteBehindTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, tran *transaction.Transaction) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_write_behind_transaction")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Updating transaction in write-behind cache")

	data, err := msgpack.Marshal(tran)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal transaction for write-behind cache update", err)
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to marshal transaction for write-behind cache update: %v", err))

		return
	}

	key := utils.WriteBehindTransactionKey(organizationID, ledgerID, tran.ID)

	// 86400 seconds = 24 hours (SetBytes multiplies by time.Second internally)
	if err := uc.TransactionRedisRepo.SetBytes(ctx, key, data, time.Duration(86400)); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to update transaction in write-behind cache", err)
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to update transaction in write-behind cache: %v", err))

		return
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Transaction updated in write-behind cache: %s", key))
}
