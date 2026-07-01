// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
)

// DeleteTransactionRouteCache deletes the cache for a transaction route.
func (uc *UseCase) DeleteTransactionRouteCache(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_transaction_route_cache")
	defer span.End()

	internalKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	err := uc.TransactionRedisRepo.Del(ctx, internalKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to delete transaction route cache", err)

		logger.Log(ctx, libLog.LevelError, "Failed to delete transaction route cache",
			libLog.String("transaction_route_id", transactionRouteID.String()),
			libLog.Err(err))

		return err
	}

	return nil
}
