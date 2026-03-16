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
	if err := validateOperationRouteTypes(operationRouteList); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate operation route types", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Operation route validation failed: %v", err))

		return nil, err
	}

	// Convert fetched operation routes to slice for assignment
	operationRoutes := make([]mmodel.OperationRoute, 0, len(operationRouteList))
	for _, fetched := range operationRouteList {
		operationRoutes = append(operationRoutes, *fetched)
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

// validateOperationRouteTypes validates operation route types for a transaction route.
// It ensures that the set of operation routes has at least one source and one destination
// (bidirectional counts as both).
func validateOperationRouteTypes(opRoutes []*mmodel.OperationRoute) error {
	entityType := reflect.TypeOf(mmodel.TransactionRoute{}).Name()

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

	if len(opRoutes) > 0 && !hasSource {
		return pkg.ValidateBusinessError(constant.ErrNoSourceForAction, entityType, "")
	}

	if len(opRoutes) > 0 && !hasDestination {
		return pkg.ValidateBusinessError(constant.ErrNoDestinationForAction, entityType, "")
	}

	return nil
}
