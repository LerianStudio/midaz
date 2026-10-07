// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// maxOperationRouteInputs defines the upper bound for the number of operation route
// inputs allowed in a single update request.
const maxOperationRouteInputs = 100

// UpdateTransactionRoute updates a transaction route of the organization by ID. Its operation-route
// links may reach operation routes created under any ledger of the organization. policy says what
// the link lists of input mean on the contract that received the request.
func (uc *UseCase) UpdateTransactionRoute(ctx context.Context, organizationID, id uuid.UUID, input *mmodel.UpdateTransactionRouteInput, policy TransactionRouteLinkPolicy) (_ *mmodel.TransactionRoute, err error) {
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

	// Compute the link changes when the caller is mutating the link set.
	// handleOperationRouteUpdates also returns the hydrated post-update
	// OperationRoute slice (it already did the FindByIDs) so the
	// streaming emit below can use that without a redundant round-trip.
	var (
		links                     transactionroute.LinkChanges
		postUpdateOperationRoutes []mmodel.OperationRoute
		postUpdateOptionalIDs     []uuid.UUID
		linksTouchedInThisUpdate  bool
	)

	if touchesLinks(input, policy) {
		update, err := uc.handleOperationRouteUpdates(ctx, organizationID, id, input, policy)
		if err != nil {
			return nil, err
		}

		links = update.changes
		postUpdateOperationRoutes = update.routes
		postUpdateOptionalIDs = update.optional
		linksTouchedInThisUpdate = true
	}

	transactionRouteUpdated, err := uc.TransactionRouteRepo.Update(ctx, organizationID, id, transactionRoute, links)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, constant.EntityTransactionRoute)

			logger.Log(ctx, libLog.LevelWarn, "Transaction route ID not found", libLog.Err(err), libLog.String("transaction_route_id", id.String()))
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction route on repo by id", err)

			return nil, err
		}

		recordCommandError(ctx, span, logger, "Failed to update transaction route on repo by id", err, libLog.String("transaction_route_id", id.String()))

		return nil, err
	}

	// If the caller did not provide a new operation-route set, the
	// repo did not touch the join table — fetch the current link set
	// so the streaming payload (and the returned entity) carry the
	// correct post-state. Two queries: junction-table → operation IDs,
	// then OperationRouteRepo.FindByIDs for the full payload data.
	if !linksTouchedInThisUpdate {
		linkMap, lookupErr := uc.TransactionRouteRepo.FindOperationRouteLinksByTransactionRouteIDs(ctx, []uuid.UUID{id})
		if lookupErr != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to fetch current operation route IDs", lookupErr)
			logger.Log(ctx, libLog.LevelError, "Failed to fetch current operation route IDs", libLog.Err(lookupErr), libLog.String("transaction_route_id", id.String()))

			return nil, lookupErr
		}

		if links := linkMap[id]; len(links) > 0 {
			existingIDs := make([]uuid.UUID, 0, len(links))

			for _, link := range links {
				existingIDs = append(existingIDs, link.OperationRouteID)

				if link.Optional {
					postUpdateOptionalIDs = append(postUpdateOptionalIDs, link.OperationRouteID)
				}
			}

			ops, hydrateErr := uc.OperationRouteRepo.FindByIDs(ctx, organizationID, existingIDs)
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
	transactionRouteUpdated.OptionalOperationRouteIDs = postUpdateOptionalIDs

	uc.emitTransactionRouteUpdatedEvent(ctx, span, logger, transactionRouteUpdated)

	metadataUpdated, err := uc.UpdateTransactionMetadata(ctx, constant.EntityTransactionRoute, id.String(), input.Metadata)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to update metadata on repo by id", err, libLog.String("transaction_route_id", id.String()))

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
	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.TransactionRouteUpdatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewTransactionRouteUpdated(tr).ToEmitRequest(tenantID, tr.UpdatedAt)
		})
}

// linkUpdate is the outcome of an update to a transaction route's links: the changes to
// apply and the post-update link set.
type linkUpdate struct {
	changes transactionroute.LinkChanges
	// routes is every post-update link, optional ones included.
	routes []mmodel.OperationRoute
	// optional names the post-update optional links.
	optional []uuid.UUID
}

// touchesLinks reports whether the update names any link list under policy.
func touchesLinks(input *mmodel.UpdateTransactionRouteInput, policy TransactionRouteLinkPolicy) bool {
	if input.OperationRoutes != nil {
		return true
	}

	return policy == LinksMergePatchV2 && input.OptionalOperationRoutes != nil
}

// handleOperationRouteUpdates computes the post-update link set from the stored one and the
// lists the input carries, validates it as a create would, and diffs it against the stored
// links. It also returns the hydrated post-update OperationRoute slice so the caller does not
// need a second FindByIDs round-trip to populate the streaming payload and the returned
// entity.
func (uc *UseCase) handleOperationRouteUpdates(ctx context.Context, organizationID, transactionRouteID uuid.UUID, input *mmodel.UpdateTransactionRouteInput, policy TransactionRouteLinkPolicy) (linkUpdate, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.handle_operation_route_updates")
	defer span.End()

	if policy == LinksFullSetV1 {
		if err := checkLinkCount(len(*input.OperationRoutes)); err != nil {
			return linkUpdate{}, err
		}
	}

	currentTransactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, transactionRouteID)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to fetch current transaction route", libLog.Err(err))

		return linkUpdate{}, err
	}

	currentRequired, currentOptional := splitLinks(currentTransactionRoute)

	var required, optional, fetch []uuid.UUID

	switch policy {
	case LinksMergePatchV2:
		if err := checkLinkCount(listLen(input.OperationRoutes, currentRequired) + listLen(input.OptionalOperationRoutes, currentOptional)); err != nil {
			return linkUpdate{}, err
		}

		required = replacedOrKept(input.OperationRoutes, currentRequired)
		optional = replacedOrKept(input.OptionalOperationRoutes, currentOptional)

		if err := rejectRouteInBothLinkLists(required, optional); err != nil {
			return linkUpdate{}, err
		}

		fetch = append(append(fetch, required...), optional...)
	default:
		// The full-set contract never changes optionality: a link it keeps
		// stays as it is and a new one is required.
		fetch = uniqueIDs(*input.OperationRoutes)

		for _, id := range fetch {
			if currentTransactionRoute.IsOptional(id) {
				optional = append(optional, id)
			} else {
				required = append(required, id)
			}
		}
	}

	operationRoutes, err := uc.OperationRouteRepo.FindByIDs(ctx, organizationID, fetch)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to fetch operation routes", libLog.Err(err))

		return linkUpdate{}, err
	}

	if err := validateOperationRouteTypes(operationRoutes, idSet(optional)); err != nil {
		return linkUpdate{}, err
	}

	update := linkUpdate{
		changes:  diffLinks(currentTransactionRoute, required, optional),
		routes:   make([]mmodel.OperationRoute, 0, len(operationRoutes)),
		optional: optional,
	}

	for _, route := range operationRoutes {
		update.routes = append(update.routes, *route)
	}

	return update, nil
}

// checkLinkCount bounds the number of links a transaction route may end with.
func checkLinkCount(count int) error {
	if count < 2 {
		return pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, constant.EntityTransactionRoute)
	}

	if count > maxOperationRouteInputs {
		return pkg.ValidateBusinessError(constant.ErrTooManyOperationRoutes, constant.EntityTransactionRoute)
	}

	return nil
}

// splitLinks is the stored route's required and optional link IDs.
func splitLinks(route *mmodel.TransactionRoute) (required, optional []uuid.UUID) {
	for _, operationRoute := range route.OperationRoutes {
		if route.IsOptional(operationRoute.ID) {
			optional = append(optional, operationRoute.ID)
		} else {
			required = append(required, operationRoute.ID)
		}
	}

	return required, optional
}

// listLen is the size of a list a merge patch sends, or of the stored list it keeps.
func listLen(sent *[]uuid.UUID, kept []uuid.UUID) int {
	if sent != nil {
		return len(*sent)
	}

	return len(kept)
}

// replacedOrKept is a merge-patch list: the sent list when present, the stored one otherwise.
func replacedOrKept(sent *[]uuid.UUID, kept []uuid.UUID) []uuid.UUID {
	if sent != nil {
		return uniqueIDs(*sent)
	}

	return kept
}

// diffLinks is what turns the stored links of route into the required and optional targets.
func diffLinks(route *mmodel.TransactionRoute, required, optional []uuid.UUID) transactionroute.LinkChanges {
	target := make(map[uuid.UUID]bool, len(required)+len(optional))
	for _, id := range required {
		target[id] = false
	}

	for _, id := range optional {
		target[id] = true
	}

	var changes transactionroute.LinkChanges

	stored := make(map[uuid.UUID]bool, len(route.OperationRoutes))

	for _, operationRoute := range route.OperationRoutes {
		id := operationRoute.ID
		stored[id] = true

		wantOptional, kept := target[id]

		switch {
		case !kept:
			changes.Remove = append(changes.Remove, id)
		case wantOptional != route.IsOptional(id):
			changes.Retag = append(changes.Retag, transactionroute.OperationRouteLink{OperationRouteID: id, Optional: wantOptional})
		}
	}

	for _, id := range append(append([]uuid.UUID(nil), required...), optional...) {
		if !stored[id] {
			changes.Add = append(changes.Add, transactionroute.OperationRouteLink{OperationRouteID: id, Optional: target[id]})
		}
	}

	return changes
}
