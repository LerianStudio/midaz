// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libCommons "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// DeleteOperationRouteByID deletes an operation route by ID.
func (uc *UseCase) DeleteOperationRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_operation_route_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.operation_route_id", id.String()),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context canceled before deleting operation route", err)

		return err
	}

	hasLinks, err := uc.OperationRouteRepo.HasTransactionRouteLinks(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to check transaction route links", err)

		logger.Log(ctx, libLog.LevelError, "Failed to check transaction route links",
			libLog.Err(err),
			libLog.String("operation_route_id", id.String()),
		)

		return err
	}

	if hasLinks {
		err := pkg.ValidateBusinessError(constant.ErrOperationRouteLinkedToTransactionRoutes, constant.EntityOperationRoute)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Operation route is linked to transaction routes", err)

		logger.Log(ctx, libLog.LevelWarn, "Operation route is linked to transaction routes",
			libLog.String("operation_route_id", id.String()),
		)

		return err
	}

	if err := uc.OperationRouteRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, constant.EntityOperationRoute)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Operation route not found", err)

			logger.Log(ctx, libLog.LevelWarn, "Operation route not found",
				libLog.String("operation_route_id", id.String()),
			)

			return err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to delete operation route", err)

		logger.Log(ctx, libLog.LevelError, "Failed to delete operation route",
			libLog.Err(err),
			libLog.String("operation_route_id", id.String()),
		)

		return err
	}

	uc.emitOperationRouteDeletedEvent(ctx, span, logger, id.String(), organizationID.String(), ledgerID.String(), time.Now())

	return nil
}

// emitOperationRouteDeletedEvent publishes the operation-route.deleted
// event for a successfully soft-deleted operation route. IMPORTANT
// posture: build and emit failures are span-recorded and logged at
// Warn, never returned. Durability of the event is owned by PG and
// (follow-up task) the outbox subsystem + DLQ, not by the synchronous
// Emit call.
//
// Anchor: invoked immediately after OperationRouteRepo.Delete succeeds
// (post-link-check). OperationRouteRepo.Delete does not return the
// post-delete record, so the payload sources identity from the
// use-case parameters (which match the request path) and stamps
// deletedAt with the wall-clock instant captured by the caller. The PG
// deleted_at column is set by the same wall clock at row-update time,
// so the values are effectively identical up to clock skew.
//
// Wire-format mapping lives in pkg/streaming/events/operation_route_deleted.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitOperationRouteDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, id, organizationID, ledgerID string, deletedAt time.Time) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.OperationRouteDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewOperationRouteDeleted(id, organizationID, ledgerID, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
