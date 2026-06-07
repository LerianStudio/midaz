// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
)

// ReloadOperationRouteCache reloads the cache for all transaction routes associated with the given operation route.
// It retrieves all transaction routes linked to the operation route and recreates their cache entries.
func (uc *UseCase) ReloadOperationRouteCache(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.reload_operation_route_cache")
	defer span.End()

	operationRouteIDStr := id.String()

	transactionRouteIDs, err := uc.OperationRouteRepo.FindTransactionRouteIDs(ctx, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to find transaction route IDs", err)

		logger.Log(ctx, libLog.LevelError, "Failed to find transaction route IDs",
			libLog.String("operation_route_id", operationRouteIDStr),
			libLog.Err(err))

		return err
	}

	if len(transactionRouteIDs) == 0 {
		logger.Log(ctx, libLog.LevelDebug, "No transaction routes found for operation route, no cache reload needed",
			libLog.String("operation_route_id", operationRouteIDStr))

		return nil
	}

	for _, transactionRouteID := range transactionRouteIDs {
		transactionRouteIDStr := transactionRouteID.String()

		transactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, transactionRouteID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve transaction route", err)

			logger.Log(ctx, libLog.LevelWarn, "Failed to retrieve transaction route",
				libLog.String("transaction_route_id", transactionRouteIDStr),
				libLog.Err(err))

			continue
		}

		if err := uc.CreateAccountingRouteCache(ctx, transactionRoute); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create cache for transaction route", err)

			logger.Log(ctx, libLog.LevelWarn, "Failed to create cache for transaction route",
				libLog.String("transaction_route_id", transactionRouteIDStr),
				libLog.Err(err))

			continue
		}
	}

	return nil
}
