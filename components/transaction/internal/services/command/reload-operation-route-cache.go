package command

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// ReloadOperationRouteCache reloads the cache for all transaction routes associated with the given operation route.
// It retrieves all transaction routes linked to the operation route and recreates their cache entries.
func (uc *UseCase) ReloadOperationRouteCache(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.reload_operation_route_cache")
	defer span.End()

	logger.Infof("Reloading operation route cache for operation route with id: %s", id)

	transactionRouteIDs, err := uc.OperationRouteRepo.FindTransactionRouteIDs(ctx, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to find transaction route IDs", err)

		logger.Errorf("Failed to find transaction route IDs for operation route %s: %v", id, err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.OperationRoute{}).Name())
	}

	if len(transactionRouteIDs) == 0 {
		logger.Infof("No transaction routes found for operation route %s, no cache reload needed", id)

		return nil
	}

	logger.Infof("Found %d transaction routes associated with operation route %s", len(transactionRouteIDs), id)

	for _, transactionRouteID := range transactionRouteIDs {
		transactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, transactionRouteID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve transaction route", err)

			logger.Warnf("Failed to retrieve transaction route %s: %v", transactionRouteID, err)

			continue
		}

		if err := uc.CreateAccountingRouteCache(ctx, transactionRoute); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create cache for transaction route", err)

			logger.Warnf("Failed to create cache for transaction route %s: %v", transactionRouteID, err)

			continue
		}

		logger.Infof("Successfully reloaded cache for transaction route %s", transactionRouteID)
	}

	logger.Infof("Successfully completed cache reload for operation route %s", id)

	return nil
}
