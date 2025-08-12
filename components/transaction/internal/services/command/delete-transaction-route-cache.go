package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DeleteTransactionRouteCache deletes the cache for a transaction route.
func (uc *UseCase) DeleteTransactionRouteCache(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_transaction_route_cache")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.transaction_route_id", transactionRouteID.String()),
	)

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
