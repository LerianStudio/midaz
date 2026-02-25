// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// DeleteWriteBehindTransaction removes the transaction from the write-behind cache.
// Called by the consumer after successfully persisting the transaction to Postgres.
// Errors are logged but do not block the consumer flow.
func (uc *UseCase) DeleteWriteBehindTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionID string) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_write_behind_transaction")
	defer span.End()

	logger.Infof("Removing transaction from write-behind cache")

	key := utils.WriteBehindTransactionKey(organizationID, ledgerID, transactionID)

	if err := uc.RedisRepo.Del(ctx, key); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to remove transaction from write-behind cache", err)
		logger.Warnf("Failed to remove transaction from write-behind cache: %v", err)

		return
	}

	logger.Infof("Transaction removed from write-behind cache: %s", key)
}
