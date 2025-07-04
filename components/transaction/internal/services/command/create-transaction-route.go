package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateTransactionRoute creates a new transaction route.
// It returns the created transaction route and an error if the operation fails.
func (uc *UseCase) CreateTransactionRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateTransactionRouteInput) (*mmodel.TransactionRoute, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

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

	// Fetch all operation routes in a single database call
	operationRouteList, err := uc.OperationRouteRepo.FindByIDs(ctx, organizationID, ledgerID, payload.OperationRoutes)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to find operation routes", err)

		logger.Errorf("Failed to find operation routes: %v", err)

		return nil, err
	}

	// Validate operation route types
	if err := validateOperationRouteTypes(operationRouteList); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate operation route types", err)

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
		libOpentelemetry.HandleSpanError(&span, "Failed to create transaction route", err)

		logger.Errorf("Failed to create transaction route: %v", err)

		return nil, err
	}

	// Ensure operation routes are included in the response
	createdTransactionRoute.OperationRoutes = operationRoutes

	if payload.Metadata != nil {
		if err := libCommons.CheckMetadataKeyAndValueLength(100, payload.Metadata); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to check metadata key and value length", err)

			return nil, err
		}

		meta := mongodb.Metadata{
			EntityID:   createdTransactionRoute.ID.String(),
			EntityName: reflect.TypeOf(mmodel.TransactionRoute{}).Name(),
			Data:       payload.Metadata,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), &meta); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to create transaction route metadata", err)

			logger.Errorf("Failed to create transaction route metadata: %v", err)

			return nil, err
		}

		createdTransactionRoute.Metadata = payload.Metadata
	}

	logger.Infof("Successfully created transaction route with %d operation routes", len(createdTransactionRoute.OperationRoutes))

	return createdTransactionRoute, nil
}

// validateOperationRouteTypes validates that operation routes contain both debit and credit types.
// Returns an error if either debit or credit type is missing.
func validateOperationRouteTypes(operationRoutes []*mmodel.OperationRoute) error {
	hasDebit := false
	hasCredit := false

	for _, operationRoute := range operationRoutes {
		if operationRoute.Type == "debit" {
			hasDebit = true
		}

		if operationRoute.Type == "credit" {
			hasCredit = true
		}

		if hasDebit && hasCredit {
			return nil
		}
	}

	return pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
}
