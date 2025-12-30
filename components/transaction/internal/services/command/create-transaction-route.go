package command

import (
	"context"
	"errors"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateTransactionRoute creates a new transaction route.
// It returns the created transaction route and an error if the operation fails.
func (uc *UseCase) CreateTransactionRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateTransactionRouteInput) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction_route")
	defer span.End()

	now := time.Now()

	transactionRoute := &mmodel.TransactionRoute{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          payload.Title,
		Description:    payload.Description,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	operationRouteList, err := uc.findOperationRoutesWithRetry(ctx, organizationID, ledgerID, payload.OperationRoutes, logger)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find operation routes", err)

		logger.Errorf("Failed to find operation routes: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	}

	// Validate operation route types
	if err := validateOperationRouteTypes(operationRouteList); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate operation route types", err)

		logger.Errorf("Operation route validation failed: %v", err)

		return nil, err
	}

	// Convert to slice for assignment
	operationRoutes := make([]mmodel.OperationRoute, 0, len(operationRouteList))
	for _, operationRoute := range operationRouteList {
		operationRoutes = append(operationRoutes, *operationRoute)
	}

	transactionRoute.OperationRoutes = operationRoutes

	createdTransactionRoute, err := uc.TransactionRouteRepo.Create(ctx, organizationID, ledgerID, transactionRoute)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction route", err)

		logger.Errorf("Failed to create transaction route: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
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
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction route metadata", err)

			logger.Errorf("Failed to create transaction route metadata: %v", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
		}

		createdTransactionRoute.Metadata = payload.Metadata
	}

	logger.Infof("Successfully created transaction route with %d operation routes", len(createdTransactionRoute.OperationRoutes))

	return createdTransactionRoute, nil
}

func (uc *UseCase) findOperationRoutesWithRetry(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID, logger interface{ Warnf(string, ...any) }) ([]*mmodel.OperationRoute, error) {
	var lastErr error

	maxAttempts := uc.RouteLookupMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultRouteLookupMaxAttempts
	}
	// Cap to avoid integer shift overflow producing negative/garbage durations.
	// This still allows very large backoffs (~days) if configured near the cap.
	if maxAttempts > 30 {
		maxAttempts = 30
	}

	baseBackoff := uc.RouteLookupBaseBackoff
	if baseBackoff <= 0 {
		baseBackoff = DefaultRouteLookupBaseBackoff
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		operationRouteList, err := uc.OperationRouteRepo.FindByIDs(ctx, organizationID, ledgerID, ids)
		if err == nil {
			return operationRouteList, nil
		}

		lastErr = err
		if !isOperationRouteNotFoundErr(err) || attempt == maxAttempts-1 {
			return nil, err
		}

		backoff := time.Duration(1<<attempt) * baseBackoff
		logger.Warnf("Operation routes not found (attempt %d/%d), retrying in %s: %v", attempt+1, maxAttempts, backoff, err)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, lastErr
}

func isOperationRouteNotFoundErr(err error) bool {
	var notFoundErr pkg.EntityNotFoundError
	if errors.As(err, &notFoundErr) {
		return notFoundErr.Code == constant.ErrOperationRouteNotFound.Error()
	}

	return false
}

// validateOperationRouteTypes validates that operation routes contain both source and destination types.
// Returns an error if either source or destination type is missing.
func validateOperationRouteTypes(operationRoutes []*mmodel.OperationRoute) error {
	hasSource := false
	hasDestination := false

	for _, operationRoute := range operationRoutes {
		if operationRoute.OperationType == "source" {
			hasSource = true
		}

		if operationRoute.OperationType == "destination" {
			hasDestination = true
		}

		if hasSource && hasDestination {
			return nil
		}
	}

	return pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
}
