// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/google/uuid"

	// ReloadOperationRouteCache reloads the cache for all transaction routes associated with the given operation route.
	// It retrieves all transaction routes linked to the operation route and recreates their cache entries.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) ReloadOperationRouteCache(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.reload_operation_route_cache")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Reloading operation route cache for operation route with id: %s", id))

	transactionRouteIDs, err := uc.OperationRouteRepo.FindTransactionRouteIDs(ctx, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to find transaction route IDs", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to find transaction route IDs for operation route %s: %v", id, err))

		return err
	}

	if len(transactionRouteIDs) == 0 {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("No transaction routes found for operation route %s, no cache reload needed", id))

		return nil
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Found %d transaction routes associated with operation route %s", len(transactionRouteIDs), id))

	for _, transactionRouteID := range transactionRouteIDs {
		transactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, transactionRouteID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve transaction route", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to retrieve transaction route %s: %v", transactionRouteID, err))

			continue
		}

		if err := uc.CreateAccountingRouteCache(ctx, transactionRoute); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create cache for transaction route", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to create cache for transaction route %s: %v", transactionRouteID, err))

			continue
		}

		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully reloaded cache for transaction route %s", transactionRouteID))
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully completed cache reload for operation route %s", id))

	return nil
}
