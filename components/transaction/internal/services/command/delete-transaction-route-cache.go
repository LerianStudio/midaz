package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/google/uuid"
)

// DeleteTransactionRouteCache removes a transaction route from the Redis cache.
//
// This invalidates the cached route configuration, forcing subsequent transactions
// to either fail validation or trigger cache reload. Typically called after deleting
// or updating a transaction route to ensure cache consistency.
func (uc *UseCase) DeleteTransactionRouteCache(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_transaction_route_cache")
	defer span.End()

	logger.Infof("Deleting transaction route cache for transaction route with id: %s", transactionRouteID)

	internalKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	err := uc.RedisRepo.Del(ctx, internalKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete transaction route cache", err)

		logger.Errorf("Failed to delete transaction route cache: %v", err)

		return err
	}

	logger.Infof("Successfully deleted transaction route cache for transaction route with id: %s", transactionRouteID)

	return nil
}
