// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libObs "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// DeleteWriteBehindTransaction removes the transaction from the write-behind cache.
// Called by the consumer after successfully persisting the transaction to Postgres.
// Errors are logged but do not block the consumer flow.
func (uc *UseCase) DeleteWriteBehindTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionID string) {
	logger, tracer, _, _ := libObs.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_write_behind_transaction")
	defer span.End()

	tenantID := tmcore.GetTenantIDContext(ctx)

	key := utils.WriteBehindTransactionKey(organizationID, ledgerID, transactionID)

	logger.Log(ctx, libLog.LevelDebug, "DeleteWriteBehindTransaction",
		libLog.String("tenant_id", tenantID),
		libLog.String("raw_key", key))

	if err := uc.TransactionRedisRepo.Del(ctx, key); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to remove transaction from write-behind cache", err)
		logger.Log(ctx, libLog.LevelError, "Failed to remove transaction from write-behind cache",
			libLog.String("raw_key", key),
			libLog.Err(err))

		return
	}

	logger.Log(ctx, libLog.LevelDebug, "Transaction removed from write-behind cache",
		libLog.String("raw_key", key))
}
