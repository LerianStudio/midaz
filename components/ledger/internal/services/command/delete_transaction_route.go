// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libObs "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// DeleteTransactionRouteByID deletes a transaction route and its operation-route links.
func (uc *UseCase) DeleteTransactionRouteByID(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID) error {
	logger, tracer, _, _ := libObs.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_transaction_route_by_id")
	defer span.End()

	transactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, transactionRouteID)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			logger.Log(ctx, libLog.LevelWarn, "Transaction route ID not found", libLog.String("transaction_route_id", transactionRouteID.String()))

			return pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, constant.EntityTransactionRoute)
		}

		logger.Log(ctx, libLog.LevelError, "Failed to find transaction route", libLog.Err(err), libLog.String("transaction_route_id", transactionRouteID.String()))
		libOpentelemetry.HandleSpanError(span, "Failed to find transaction route", err)

		return err
	}

	operationRoutesToRemove := make([]uuid.UUID, 0, len(transactionRoute.OperationRoutes))
	for _, operationRoute := range transactionRoute.OperationRoutes {
		operationRoutesToRemove = append(operationRoutesToRemove, operationRoute.ID)
	}

	if err := uc.TransactionRouteRepo.Delete(ctx, organizationID, ledgerID, transactionRouteID, operationRoutesToRemove); err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to delete transaction route", libLog.Err(err), libLog.String("transaction_route_id", transactionRouteID.String()))
		libOpentelemetry.HandleSpanError(span, "Failed to delete transaction route", err)

		return err
	}

	uc.emitTransactionRouteDeletedEvent(ctx, span, logger, transactionRouteID.String(), organizationID.String(), ledgerID.String(), time.Now())

	return nil
}

// emitTransactionRouteDeletedEvent publishes the transaction-route.deleted
// event for a successfully soft-deleted transaction route. IMPORTANT
// posture: build and emit failures are span-recorded and logged at
// Warn, never returned. Durability is owned by PG and (follow-up) the
// outbox + DLQ.
//
// Anchor: invoked immediately after TransactionRouteRepo.Delete
// succeeds (which also cascade-soft-deletes the join-table
// operation_transaction_route rows). The use case does not return the
// persisted struct on delete, so the payload sources identity from the
// use-case parameters (request path) and stamps deletedAt with the
// wall-clock instant captured by the caller. The PG deleted_at column
// is set by the same wall clock at row-update time, so the values are
// effectively identical up to clock skew.
//
// Wire-format mapping lives in pkg/streaming/events/transaction_route_deleted.go.
func (uc *UseCase) emitTransactionRouteDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, id, organizationID, ledgerID string, deletedAt time.Time) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.TransactionRouteDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewTransactionRouteDeleted(id, organizationID, ledgerID, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
