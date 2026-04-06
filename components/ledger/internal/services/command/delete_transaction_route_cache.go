// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"

	// DeleteTransactionRouteCache deletes the cache for a transaction route.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) DeleteTransactionRouteCache(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_transaction_route_cache")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Deleting transaction route cache for transaction route with id: %s", transactionRouteID))

	internalKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	err := uc.TransactionRedisRepo.Del(ctx, internalKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to delete transaction route cache", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to delete transaction route cache: %v", err))

		return err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully deleted transaction route cache for transaction route with id: %s", transactionRouteID))

	return nil
}
