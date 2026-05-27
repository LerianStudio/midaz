// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libStreaming "github.com/LerianStudio/lib-streaming"
	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// CreateTransactionRoute creates a new transaction route.
func (uc *UseCase) CreateTransactionRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateTransactionRouteInput) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction_route")
	defer span.End()

	now := time.Now()

	transactionRoute := &mmodel.TransactionRoute{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          payload.Title,
		Description:    payload.Description,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	operationRouteList, err := uc.OperationRouteRepo.FindByIDs(ctx, organizationID, ledgerID, payload.OperationRouteIDs())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find operation routes", err)
		logger.Log(ctx, libLog.LevelError, "Failed to find operation routes", libLog.Err(err))

		return nil, err
	}

	if err := validateOperationRouteTypes(operationRouteList); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate operation route types", err)
		logger.Log(ctx, libLog.LevelError, "Operation route validation failed", libLog.Err(err))

		return nil, err
	}

	operationRoutes := make([]mmodel.OperationRoute, 0, len(operationRouteList))
	for _, fetched := range operationRouteList {
		operationRoutes = append(operationRoutes, *fetched)
	}

	transactionRoute.OperationRoutes = operationRoutes

	createdTransactionRoute, err := uc.TransactionRouteRepo.Create(ctx, organizationID, ledgerID, transactionRoute)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create transaction route", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create transaction route", libLog.Err(err))

		return nil, err
	}

	// repo.Create copies the input OperationRoutes onto the returned
	// entity, but be explicit in case that contract loosens — the
	// streaming event below relies on this field being populated.
	createdTransactionRoute.OperationRoutes = operationRoutes

	uc.emitTransactionRouteCreatedEvent(ctx, span, logger, createdTransactionRoute)

	if payload.Metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   createdTransactionRoute.ID.String(),
			EntityName: constant.EntityTransactionRoute,
			Data:       payload.Metadata,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if err := uc.TransactionMetadataRepo.Create(ctx, constant.EntityTransactionRoute, &meta); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create transaction route metadata", err)
			logger.Log(ctx, libLog.LevelError, "Failed to create transaction route metadata", libLog.Err(err))

			return nil, err
		}

		createdTransactionRoute.Metadata = payload.Metadata
	}

	return createdTransactionRoute, nil
}

// emitTransactionRouteCreatedEvent publishes the transaction-route.created
// event for a successfully persisted transaction route. IMPORTANT
// posture: build and emit failures are span-recorded and logged at
// Warn, never returned. Durability of the event is owned by PG and
// (follow-up task) the outbox subsystem + DLQ, not by the synchronous
// Emit call.
//
// Anchor: invoked immediately after TransactionRouteRepo.Create
// succeeds and before the metadata-write call in
// CreateTransactionRoute, so a downstream Mongo failure cannot mask
// the event.
//
// Caller invariant: tr.OperationRoutes must reflect the post-commit
// link set (the use case hydrated this slice from FindByIDs and the
// repo preserves it through the return value).
//
// Wire-format mapping lives in pkg/streaming/events/transaction_route_created.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitTransactionRouteCreatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, tr *mmodel.TransactionRoute) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.TransactionRouteCreatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewTransactionRouteCreated(tr).ToEmitRequest(tenantID, tr.CreatedAt)
		})
}

// validateOperationRouteTypes validates operation route types for a transaction route.
// It ensures that the set of operation routes has at least one source and one destination
// (bidirectional counts as both).
func validateOperationRouteTypes(opRoutes []*mmodel.OperationRoute) error {
	hasSource := false
	hasDestination := false

	for _, route := range opRoutes {
		switch route.OperationType {
		case "source":
			hasSource = true
		case "destination":
			hasDestination = true
		case "bidirectional":
			hasSource = true
			hasDestination = true
		}
	}

	if !hasSource {
		return pkg.ValidateBusinessError(constant.ErrNoSourceForAction, constant.EntityTransactionRoute, "")
	}

	if !hasDestination {
		return pkg.ValidateBusinessError(constant.ErrNoDestinationForAction, constant.EntityTransactionRoute, "")
	}

	return nil
}
