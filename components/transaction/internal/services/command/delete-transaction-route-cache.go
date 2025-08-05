package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/google/uuid"
)

// DeleteTransactionRouteCache deletes the cache for a transaction route.
func (uc *UseCase) DeleteTransactionRouteCache(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

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
