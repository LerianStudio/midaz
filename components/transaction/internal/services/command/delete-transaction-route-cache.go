// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/google/uuid"
)

// DeleteTransactionRouteCache removes a transaction route cache from Redis.
//
// This method implements cache invalidation for transaction routes:
// 1. Generates internal cache key
// 2. Deletes the key from Redis
// 3. Logs success or failure
//
// Cache Invalidation Triggers:
//   - Transaction route is updated
//   - Transaction route is deleted
//   - Operation routes are modified
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionRouteID: UUID of the transaction route
//
// Returns:
//   - error: nil on success, error if Redis operation fails
//
// OpenTelemetry: Creates span "command.delete_transaction_route_cache"
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
