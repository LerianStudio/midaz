// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// maxOperationRouteInputs defines the upper bound for the number of operation route
// inputs allowed in a single update request.
const maxOperationRouteInputs = 100

// UpdateTransactionRoute updates a transaction route by ID.
func (uc *UseCase) UpdateTransactionRoute(ctx context.Context, organizationID, ledgerID, id uuid.UUID, input *mmodel.UpdateTransactionRouteInput) (_ *mmodel.TransactionRoute, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction_route")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "update_transaction_route", start, err)
	}()

	transactionRoute := &mmodel.TransactionRoute{
		Title:       input.Title,
		Description: input.Description,
	}

	// Compute the toAdd/toRemove diff when the caller is mutating the
	// link set. handleOperationRouteUpdates also returns the hydrated
	// post-update OperationRoute slice (it already did the FindByIDs)
	// so the streaming emit below can use that without a redundant
	// round-trip.
	var (
		toAdd, toRemove           []uuid.UUID
		postUpdateOperationRoutes []mmodel.OperationRoute
		linksTouchedInThisUpdate  bool
	)

	if input.OperationRoutes != nil {
		var (
			err    error
			routes []*mmodel.OperationRoute
		)

		toAdd, toRemove, routes, err = uc.handleOperationRouteUpdates(ctx, organizationID, ledgerID, id, *input.OperationRoutes)
		if err != nil {
			return nil, err
		}

		postUpdateOperationRoutes = make([]mmodel.OperationRoute, 0, len(routes))
		for _, o := range routes {
			postUpdateOperationRoutes = append(postUpdateOperationRoutes, *o)
		}

		linksTouchedInThisUpdate = true
	}

	transactionRouteUpdated, err := uc.TransactionRouteRepo.Update(ctx, organizationID, ledgerID, id, transactionRoute, toAdd, toRemove)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, constant.EntityTransactionRoute)

			logger.Log(ctx, libLog.LevelWarn, "Transaction route ID not found", libLog.Err(err), libLog.String("transaction_route_id", id.String()))
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction route on repo by id", err)

			return nil, err
		}

		logger.Log(ctx, libLog.LevelError, "Failed to update transaction route on repo by id", libLog.Err(err), libLog.String("transaction_route_id", id.String()))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction route on repo by id", err)

		return nil, err
	}

	// If the caller did not provide a new operation-route set, the
	// repo did not touch the join table — fetch the current link set
	// so the streaming payload (and the returned entity) carry the
	// correct post-state. Two queries: junction-table → operation IDs,
	// then OperationRouteRepo.FindByIDs for the full payload data.
	if !linksTouchedInThisUpdate {
		opIDMap, lookupErr := uc.TransactionRouteRepo.FindOperationRouteIDsByTransactionRouteIDs(ctx, []uuid.UUID{id})
		if lookupErr != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to fetch current operation route IDs", lookupErr)
			logger.Log(ctx, libLog.LevelError, "Failed to fetch current operation route IDs", libLog.Err(lookupErr), libLog.String("transaction_route_id", id.String()))

			return nil, lookupErr
		}

		if existingIDs := opIDMap[id]; len(existingIDs) > 0 {
			ops, hydrateErr := uc.OperationRouteRepo.FindByIDs(ctx, organizationID, ledgerID, existingIDs)
			if hydrateErr != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to hydrate post-update operation routes", hydrateErr)
				logger.Log(ctx, libLog.LevelError, "Failed to hydrate post-update operation routes", libLog.Err(hydrateErr), libLog.String("transaction_route_id", id.String()))

				return nil, hydrateErr
			}

			postUpdateOperationRoutes = make([]mmodel.OperationRoute, 0, len(ops))
			for _, o := range ops {
				postUpdateOperationRoutes = append(postUpdateOperationRoutes, *o)
			}
		}
	}

	transactionRouteUpdated.OperationRoutes = postUpdateOperationRoutes

	uc.emitTransactionRouteUpdatedEvent(ctx, span, logger, transactionRouteUpdated)

	metadataUpdated, err := uc.UpdateTransactionMetadata(ctx, constant.EntityTransactionRoute, id.String(), input.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on repo by id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to update transaction route metadata", libLog.Err(err), libLog.String("transaction_route_id", id.String()))

		return nil, err
	}

	transactionRouteUpdated.Metadata = metadataUpdated

	return transactionRouteUpdated, nil
}

// emitTransactionRouteUpdatedEvent publishes the transaction-route.updated
// event for a successfully persisted update. IMPORTANT posture: build
// and emit failures are span-recorded and logged at Warn, never
// returned. Durability is owned by PG and (follow-up) the outbox + DLQ.
//
// Anchor: invoked between the post-update operation-route hydration
// step and the metadata-write call in UpdateTransactionRoute, so a
// downstream Mongo failure cannot mask the event.
//
// Caller invariant: tr.OperationRoutes must reflect the FINAL
// post-update link set (the use case hydrates this slice from
// FindByIDs above).
//
// Wire-format mapping lives in pkg/streaming/events/transaction_route_updated.go.
func (uc *UseCase) emitTransactionRouteUpdatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, tr *mmodel.TransactionRoute) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.TransactionRouteUpdatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewTransactionRouteUpdated(tr).ToEmitRequest(tenantID, tr.UpdatedAt)
		})
}

// handleOperationRouteUpdates processes operation route relationship updates by comparing
// existing vs new operation routes using operation route IDs. Also
// returns the hydrated post-update OperationRoute slice (in the order
// of deduplicatedInputs) so the caller does not need a second
// FindByIDs round-trip to populate the streaming payload + the
// returned entity.
func (uc *UseCase) handleOperationRouteUpdates(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID, newOperationRouteInputs []uuid.UUID) (toAdd, toRemove []uuid.UUID, postUpdateRoutes []*mmodel.OperationRoute, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.handle_operation_route_updates")
	defer span.End()

	if len(newOperationRouteInputs) < 2 {
		return nil, nil, nil, pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, constant.EntityTransactionRoute)
	}

	if len(newOperationRouteInputs) > maxOperationRouteInputs {
		return nil, nil, nil, pkg.ValidateBusinessError(constant.ErrTooManyOperationRoutes, constant.EntityTransactionRoute)
	}

	currentTransactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, transactionRouteID)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to fetch current transaction route", libLog.Err(err))

		return nil, nil, nil, err
	}

	// Deduplicate input by operation route ID
	seen := make(map[uuid.UUID]bool)

	deduplicatedInputs := make([]uuid.UUID, 0, len(newOperationRouteInputs))

	for _, operationRouteID := range newOperationRouteInputs {
		if !seen[operationRouteID] {
			seen[operationRouteID] = true

			deduplicatedInputs = append(deduplicatedInputs, operationRouteID)
		}
	}

	operationRoutes, err := uc.OperationRouteRepo.FindByIDs(ctx, organizationID, ledgerID, deduplicatedInputs)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to fetch operation routes", libLog.Err(err))

		return nil, nil, nil, err
	}

	if err := validateOperationRouteTypes(operationRoutes); err != nil {
		return nil, nil, nil, err
	}

	existingIDs := make(map[uuid.UUID]bool)
	for _, existingRoute := range currentTransactionRoute.OperationRoutes {
		existingIDs[existingRoute.ID] = true
	}

	newIDs := make(map[uuid.UUID]bool)
	for _, operationRouteID := range deduplicatedInputs {
		newIDs[operationRouteID] = true
	}

	for id := range existingIDs {
		if !newIDs[id] {
			toRemove = append(toRemove, id)
		}
	}

	for id := range newIDs {
		if !existingIDs[id] {
			toAdd = append(toAdd, id)
		}
	}

	return toAdd, toRemove, operationRoutes, nil
}
