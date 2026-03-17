// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"

	// UpdateTransactionRoute updates a transaction route by its ID.
	// It returns the updated transaction route and an error if the operation fails.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

// maxOperationRouteInputs defines the upper bound for the number of operation route
// inputs allowed in a single update request.
const maxOperationRouteInputs = 100

func (uc *UseCase) UpdateTransactionRoute(ctx context.Context, organizationID, ledgerID, id uuid.UUID, input *mmodel.UpdateTransactionRouteInput) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction_route")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Trying to update transaction route: %v", input))

	transactionRoute := &mmodel.TransactionRoute{
		Title:       input.Title,
		Description: input.Description,
	}

	// Handle operation route updates if provided
	var toAdd, toRemove []mmodel.OperationRouteActionInput

	if input.OperationRoutes != nil {
		var err error

		toAdd, toRemove, err = uc.handleOperationRouteUpdates(ctx, organizationID, ledgerID, id, *input.OperationRoutes)
		if err != nil {
			return nil, err
		}
	}

	transactionRouteUpdated, err := uc.TransactionRouteRepo.Update(ctx, organizationID, ledgerID, id, transactionRoute, toAdd, toRemove)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error updating transaction route on repo by id: %v", err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction route on repo by id", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Error updating transaction route on repo by id: %v", err))

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction route on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), id.String(), input.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on repo by id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error updating metadata on repo by id: %v", err))

		return nil, err
	}

	transactionRouteUpdated.Metadata = metadataUpdated

	return transactionRouteUpdated, nil
}

// handleOperationRouteUpdates processes operation route relationship updates by comparing
// existing vs new operation routes using operation route IDs.
// It returns arrays of OperationRouteActionInput entries to add and remove, or an error if validation fails.
func (uc *UseCase) handleOperationRouteUpdates(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID, newOperationRouteInputs []mmodel.OperationRouteActionInput) (toAdd, toRemove []mmodel.OperationRouteActionInput, err error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.handle_operation_route_updates")
	defer span.End()

	if len(newOperationRouteInputs) < 2 {
		return nil, nil, pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	}

	if len(newOperationRouteInputs) > maxOperationRouteInputs {
		return nil, nil, pkg.ValidateBusinessError(constant.ErrTooManyOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	}

	currentTransactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, transactionRouteID)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error fetching current transaction route: %v", err))
		return nil, nil, err
	}

	// Deduplicate input by operation route ID
	seen := make(map[uuid.UUID]bool)

	var deduplicatedInputs []mmodel.OperationRouteActionInput

	uniqueIDs := make([]uuid.UUID, 0, len(newOperationRouteInputs))

	for _, input := range newOperationRouteInputs {
		if !seen[input.OperationRouteID] {
			seen[input.OperationRouteID] = true

			deduplicatedInputs = append(deduplicatedInputs, input)
			uniqueIDs = append(uniqueIDs, input.OperationRouteID)
		}
	}

	operationRoutes, err := uc.OperationRouteRepo.FindByIDs(ctx, organizationID, ledgerID, uniqueIDs)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error fetching operation routes: %v", err))
		return nil, nil, err
	}

	// Validate operation route types
	err = validateOperationRouteTypes(operationRoutes)
	if err != nil {
		return nil, nil, err
	}

	// Compare existing vs new operation routes by ID
	existingIDs := make(map[uuid.UUID]bool)
	for _, existingRoute := range currentTransactionRoute.OperationRoutes {
		existingIDs[existingRoute.ID] = true
	}

	newIDs := make(map[uuid.UUID]bool)
	for _, input := range deduplicatedInputs {
		newIDs[input.OperationRouteID] = true
	}

	// Find relationships to remove (exist currently but not in new list)
	for id := range existingIDs {
		if !newIDs[id] {
			toRemove = append(toRemove, mmodel.OperationRouteActionInput{
				OperationRouteID: id,
			})
		}
	}

	// Find relationships to add (in new list but don't exist currently)
	for id := range newIDs {
		if !existingIDs[id] {
			toAdd = append(toAdd, mmodel.OperationRouteActionInput{
				OperationRouteID: id,
			})
		}
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Operation route updates calculated. ToAdd: %d, ToRemove: %d", len(toAdd), len(toRemove)))

	return toAdd, toRemove, nil
}
