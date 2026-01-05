package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// CreateTransactionRoute creates a new transaction route.
// It returns the created transaction route and an error if the operation fails.
func (uc *UseCase) CreateTransactionRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateTransactionRouteInput) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction_route")
	defer span.End()

	now := time.Now()

	transactionRoute := uc.buildTransactionRoute(organizationID, ledgerID, payload, now)

	operationRoutes, err := uc.resolveOperationRoutes(ctx, organizationID, ledgerID, payload, &span, logger)
	if err != nil {
		return nil, err
	}

	transactionRoute.OperationRoutes = operationRoutes

	createdTransactionRoute, err := uc.persistTransactionRoute(ctx, organizationID, ledgerID, transactionRoute, operationRoutes, &span, logger)
	if err != nil {
		return nil, err
	}

	if err := uc.createMetadataIfNeeded(ctx, createdTransactionRoute, payload.Metadata, now, &span, logger); err != nil {
		return nil, err
	}

	logger.Infof("Successfully created transaction route with %d operation routes", len(createdTransactionRoute.OperationRoutes))

	return createdTransactionRoute, nil
}

// buildTransactionRoute creates a new transaction route entity.
func (uc *UseCase) buildTransactionRoute(organizationID, ledgerID uuid.UUID, payload *mmodel.CreateTransactionRouteInput, now time.Time) *mmodel.TransactionRoute {
	return &mmodel.TransactionRoute{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          payload.Title,
		Description:    payload.Description,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// resolveOperationRoutes finds and validates operation routes.
func (uc *UseCase) resolveOperationRoutes(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateTransactionRouteInput, span *trace.Span, logger libLog.Logger) ([]mmodel.OperationRoute, error) {
	operationRouteList, err := uc.findOperationRoutesWithRetry(ctx, organizationID, ledgerID, payload.OperationRoutes, logger)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find operation routes", err)
		logger.Errorf("Failed to find operation routes: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	}

	uc.assertOperationRoutesMatch(operationRouteList, payload.OperationRoutes)

	if err := validateOperationRouteTypes(operationRouteList); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate operation route types", err)
		logger.Errorf("Operation route validation failed: %v", err)

		return nil, err
	}

	operationRoutes := make([]mmodel.OperationRoute, 0, len(operationRouteList))
	for _, operationRoute := range operationRouteList {
		operationRoutes = append(operationRoutes, *operationRoute)
	}

	return operationRoutes, nil
}

// assertOperationRoutesMatch validates that operation routes match the payload.
func (uc *UseCase) assertOperationRoutesMatch(operationRouteList []*mmodel.OperationRoute, payloadRouteIDs []uuid.UUID) {
	assert.That(len(operationRouteList) == len(payloadRouteIDs), "operation routes count mismatch after lookup",
		"expected_count", len(payloadRouteIDs),
		"actual_count", len(operationRouteList))

	payloadIDs := make(map[uuid.UUID]struct{}, len(payloadRouteIDs))
	for _, routeID := range payloadRouteIDs {
		payloadIDs[routeID] = struct{}{}
	}

	for _, operationRoute := range operationRouteList {
		_, ok := payloadIDs[operationRoute.ID]
		assert.That(ok, "operation route id missing from payload after lookup", "operation_route_id", operationRoute.ID)
	}

	operationRouteIDs := make(map[uuid.UUID]struct{}, len(operationRouteList))
	for _, operationRoute := range operationRouteList {
		operationRouteIDs[operationRoute.ID] = struct{}{}
	}

	for _, routeID := range payloadRouteIDs {
		_, ok := operationRouteIDs[routeID]
		assert.That(ok, "payload operation route id missing from lookup results", "operation_route_id", routeID)
	}
}

// persistTransactionRoute saves the transaction route to the repository.
func (uc *UseCase) persistTransactionRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionRoute *mmodel.TransactionRoute, operationRoutes []mmodel.OperationRoute, span *trace.Span, logger libLog.Logger) (*mmodel.TransactionRoute, error) {
	createdTransactionRoute, err := uc.TransactionRouteRepo.Create(ctx, organizationID, ledgerID, transactionRoute)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create transaction route", err)
		logger.Errorf("Failed to create transaction route: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	}

	assert.NotNil(createdTransactionRoute, "repository Create must return non-nil transaction route on success",
		"organization_id", organizationID, "ledger_id", ledgerID)
	assert.That(createdTransactionRoute.OrganizationID == organizationID, "transaction route organization id mismatch after create",
		"expected_organization_id", organizationID, "actual_organization_id", createdTransactionRoute.OrganizationID)
	assert.That(createdTransactionRoute.LedgerID == ledgerID, "transaction route ledger id mismatch after create",
		"expected_ledger_id", ledgerID, "actual_ledger_id", createdTransactionRoute.LedgerID)
	assert.That(len(createdTransactionRoute.OperationRoutes) == len(operationRoutes), "transaction route operation routes count mismatch after create",
		"expected_count", len(operationRoutes), "actual_count", len(createdTransactionRoute.OperationRoutes))

	createdTransactionRoute.OperationRoutes = operationRoutes

	return createdTransactionRoute, nil
}

// createMetadataIfNeeded creates metadata for the transaction route if provided.
func (uc *UseCase) createMetadataIfNeeded(ctx context.Context, transactionRoute *mmodel.TransactionRoute, metadata map[string]any, now time.Time, span *trace.Span, logger libLog.Logger) error {
	if metadata == nil {
		return nil
	}

	meta := mongodb.Metadata{
		EntityID:   transactionRoute.ID.String(),
		EntityName: reflect.TypeOf(mmodel.TransactionRoute{}).Name(),
		Data:       metadata,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), &meta); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create transaction route metadata", err)
		logger.Errorf("Failed to create transaction route metadata: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	}

	transactionRoute.Metadata = metadata

	return nil
}

func (uc *UseCase) findOperationRoutesWithRetry(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID, logger interface{ Warnf(string, ...any) }) ([]*mmodel.OperationRoute, error) {
	var lastErr error

	maxAttempts := uc.RouteLookupMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultRouteLookupMaxAttempts
	}
	// Cap to avoid integer shift overflow producing negative/garbage durations.
	// This still allows very large backoffs (~days) if configured near the cap.
	if maxAttempts > MaxRouteLookupAttemptsCap {
		maxAttempts = MaxRouteLookupAttemptsCap
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
			return nil, fmt.Errorf("%w: %w", services.ErrOperationRouteLookup, err)
		}

		backoff := time.Duration(1<<attempt) * baseBackoff
		logger.Warnf("Operation routes not found (attempt %d/%d), retrying in %s: %v", attempt+1, maxAttempts, backoff, err)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: %w", services.ErrContextCanceled, ctx.Err())
		}
	}

	return nil, lastErr
}

func isOperationRouteNotFoundErr(err error) bool {
	if errors.Is(err, services.ErrOperationRouteNotFound) {
		return true
	}

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
