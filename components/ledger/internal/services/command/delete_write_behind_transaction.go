// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"

	// DeleteWriteBehindTransaction removes the transaction from the write-behind cache.
	// Called by the consumer after successfully persisting the transaction to Postgres.
	// Errors are logged but do not block the consumer flow.
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
)

func (uc *UseCase) DeleteWriteBehindTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionID string) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_write_behind_transaction")
	defer span.End()

	tenantID := tmcore.GetTenantIDContext(ctx)

	key := utils.WriteBehindTransactionKey(organizationID, ledgerID, transactionID)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("[DEBUG] DeleteWriteBehindTransaction: tenantID=%q raw_key=%s", tenantID, key))

	if err := uc.TransactionRedisRepo.Del(ctx, key); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to remove transaction from write-behind cache", err)
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to remove transaction from write-behind cache: %v", err))

		return
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Transaction removed from write-behind cache: %s", key))
}
