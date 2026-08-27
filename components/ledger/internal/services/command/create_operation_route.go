// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v3"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// CreateOperationRoute creates a new operation route.
func (uc *UseCase) CreateOperationRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateOperationRouteInput) (_ *mmodel.OperationRoute, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_operation_route")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "create_operation_route", start, err)
	}()

	now := time.Now()

	operationRoute := &mmodel.OperationRoute{
		ID:                uuid.Must(libCommons.GenerateUUIDv7()),
		OrganizationID:    organizationID,
		LedgerID:          ledgerID,
		Title:             payload.Title,
		Description:       payload.Description,
		Code:              payload.Code, //nolint:staticcheck // legacy Code field persisted for backward compatibility
		OperationType:     payload.OperationType,
		Account:           payload.Account,
		AccountingEntries: payload.AccountingEntries,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	createdOperationRoute, err := uc.OperationRouteRepo.Create(ctx, organizationID, ledgerID, operationRoute)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create operation route", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create operation route", libLog.Err(err))

		return nil, err
	}

	uc.emitOperationRouteCreatedEvent(ctx, span, logger, createdOperationRoute)

	if payload.Metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   createdOperationRoute.ID.String(),
			EntityName: constant.EntityOperationRoute,
			Data:       payload.Metadata,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if err := uc.TransactionMetadataRepo.Create(ctx, constant.EntityOperationRoute, &meta); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create operation route metadata", err)
			logger.Log(ctx, libLog.LevelError, "Failed to create operation route metadata", libLog.Err(err))

			return nil, err
		}

		createdOperationRoute.Metadata = payload.Metadata
	}

	return createdOperationRoute, nil
}

// emitOperationRouteCreatedEvent publishes the operation-route.created
// event for a successfully persisted operation route. IMPORTANT
// posture: build and emit failures are span-recorded and logged at
// Warn, never returned. Durability of the event is owned by PG and
// the persisted database mutation; this helper does not make broker delivery transactional.
//
// Anchor: invoked immediately after OperationRouteRepo.Create succeeds
// and before the metadata-write call in CreateOperationRoute, so a
// downstream Mongo failure cannot mask the event.
//
// Wire-format mapping lives in pkg/streaming/events/operation_route_created.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitOperationRouteCreatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, o *mmodel.OperationRoute) {
	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.OperationRouteCreatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewOperationRouteCreated(o).ToEmitRequest(tenantID, o.CreatedAt)
		})
}
