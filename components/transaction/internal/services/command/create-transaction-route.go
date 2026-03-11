// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"

	// CreateTransactionRoute creates a new transaction route.
	// It returns the created transaction route and an error if the operation fails.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

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

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to find operation routes: %v", err))

		return nil, err
	}

	// Validate operation route types
	if err := validateOperationRouteTypes(payload.OperationRoutes, operationRouteList); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate operation route types", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Operation route validation failed: %v", err))

		return nil, err
	}

	// Build a map from route ID to fetched operation route for lookup
	routeByID := make(map[uuid.UUID]*mmodel.OperationRoute, len(operationRouteList))
	for _, operationRoute := range operationRouteList {
		routeByID[operationRoute.ID] = operationRoute
	}

	// Convert to slice for assignment, preserving the action from the input payload
	operationRoutes := make([]mmodel.OperationRoute, 0, len(payload.OperationRoutes))
	for _, input := range payload.OperationRoutes {
		if fetched, ok := routeByID[input.OperationRouteID]; ok {
			route := *fetched
			route.Action = input.Action

			operationRoutes = append(operationRoutes, route)
		}
	}

	transactionRoute.OperationRoutes = operationRoutes

	createdTransactionRoute, err := uc.TransactionRouteRepo.Create(ctx, organizationID, ledgerID, transactionRoute)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create transaction route", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create transaction route: %v", err))

		return nil, err
	}

	createdTransactionRoute.OperationRoutes = operationRoutes

	if payload.Metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   createdTransactionRoute.ID.String(),
			EntityName: reflect.TypeOf(mmodel.TransactionRoute{}).Name(),
			Data:       payload.Metadata,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), &meta); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create transaction route metadata", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create transaction route metadata: %v", err))

			return nil, err
		}

		createdTransactionRoute.Metadata = payload.Metadata
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully created transaction route with %d operation routes", len(createdTransactionRoute.OperationRoutes)))

	return createdTransactionRoute, nil
}

// validateOperationRouteTypes validates per-action operation route types.
// It checks that each action value is valid, detects duplicate (action, routeID) pairs,
// and ensures every distinct action has at least one source and one destination route
// (bidirectional counts as both).
func validateOperationRouteTypes(actionInputs []mmodel.OperationRouteActionInput, opRoutes []*mmodel.OperationRoute) error {
	entityType := reflect.TypeOf(mmodel.TransactionRoute{}).Name()

	// Build a set of valid actions for O(1) lookup
	validActionSet := make(map[string]bool, len(constant.ValidActions))
	for _, a := range constant.ValidActions {
		validActionSet[a] = true
	}

	// Build a map from route ID to operation route for type lookup
	routeByID := make(map[uuid.UUID]*mmodel.OperationRoute, len(opRoutes))
	for _, r := range opRoutes {
		routeByID[r.ID] = r
	}

	// Validate actions and detect duplicates
	type routeActionKey struct {
		RouteID uuid.UUID
		Action  string
	}

	seen := make(map[routeActionKey]bool, len(actionInputs))

	// Group routes by action: action -> list of operation types
	actionRouteTypes := make(map[string][]string)

	for _, input := range actionInputs {
		if !validActionSet[input.Action] {
			return pkg.ValidateBusinessError(constant.ErrInvalidRouteAction, entityType, input.Action)
		}

		key := routeActionKey{RouteID: input.OperationRouteID, Action: input.Action}
		if seen[key] {
			return pkg.ValidateBusinessError(constant.ErrDuplicateActionRoute, entityType, input.OperationRouteID.String(), input.Action)
		}

		seen[key] = true

		if route, ok := routeByID[input.OperationRouteID]; ok {
			actionRouteTypes[input.Action] = append(actionRouteTypes[input.Action], route.OperationType)
		}
	}

	// For each action group, validate at least 1 source + 1 destination
	for action, opTypes := range actionRouteTypes {
		hasSource := false
		hasDestination := false

		for _, opType := range opTypes {
			switch opType {
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
			return pkg.ValidateBusinessError(constant.ErrNoSourceForAction, entityType, action)
		}

		if !hasDestination {
			return pkg.ValidateBusinessError(constant.ErrNoDestinationForAction, entityType, action)
		}
	}

	return nil
}
