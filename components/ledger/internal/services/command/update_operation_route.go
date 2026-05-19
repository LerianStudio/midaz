// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// UpdateOperationRoute updates an operation route by ID.
func (uc *UseCase) UpdateOperationRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, input *mmodel.UpdateOperationRouteInput) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_operation_route")
	defer span.End()

	operationRoute := &mmodel.OperationRoute{
		Title:                input.Title,
		Description:          input.Description,
		Code:                 input.Code,
		Account:              input.Account,
		AccountingEntries:    input.AccountingEntries,
		AccountingEntriesRaw: input.AccountingEntriesRaw,
	}

	operationRouteUpdated, err := uc.OperationRouteRepo.Update(ctx, organizationID, ledgerID, id, operationRoute)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, constant.EntityOperationRoute)

			logger.Log(ctx, libLog.LevelWarn, "Operation route ID not found", libLog.Err(err), libLog.String("operation_route_id", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update operation route on repo by id", err)

			return nil, err
		}

		logger.Log(ctx, libLog.LevelError, "Failed to update operation route on repo by id", libLog.Err(err), libLog.String("operation_route_id", id.String()))

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update operation route on repo by id", err)

		return nil, err
	}

	uc.emitOperationRouteUpdatedEvent(ctx, span, logger, operationRouteUpdated)

	metadataUpdated, err := uc.UpdateTransactionMetadata(ctx, constant.EntityOperationRoute, id.String(), input.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on repo by id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to update operation route metadata", libLog.Err(err), libLog.String("operation_route_id", id.String()))

		return nil, err
	}

	operationRouteUpdated.Metadata = metadataUpdated

	return operationRouteUpdated, nil
}

// emitOperationRouteUpdatedEvent publishes the operation-route.updated
// event for a successfully persisted update. IMPORTANT posture: build
// and emit failures are span-recorded and logged at Warn, never
// returned. Durability of the event is owned by PG and (follow-up
// task) the outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked between the OperationRouteRepo.Update success branch
// and the metadata-write call in UpdateOperationRoute, so a downstream
// Mongo failure cannot mask the event.
//
// Caller invariant: o must be the value returned by
// OperationRouteRepo.Update (post-commit), not the input struct.
// Specifically o.ID, o.UpdatedAt and the persisted account /
// accountingEntries must reflect the row state — the squirrel +
// RETURNING repo refactor guarantees this.
//
// Wire-format mapping lives in pkg/streaming/events/operation_route_updated.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitOperationRouteUpdatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, o *mmodel.OperationRoute) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.OperationRouteUpdatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewOperationRouteUpdated(o).ToEmitRequest(tenantID, o.UpdatedAt)
		})
}
