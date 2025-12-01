package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// SyncBalance synchronizes a balance from Redis cache to PostgreSQL database.
//
// During high-throughput transaction processing, balance updates are first applied
// to Redis for speed. This function persists those cached balance values to PostgreSQL
// before the cache entry expires, ensuring data durability.
//
// Synchronization Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//
//	Step 2: Attempt Database Sync
//	  - Call BalanceRepo.Sync with cached balance data
//	  - Repository compares timestamps to prevent stale overwrites
//
//	Step 3: Handle Sync Result
//	  - If synced: Return true (balance was persisted)
//	  - If skipped: Return false (database has newer data)
//
// Optimistic Concurrency:
//
// The sync operation uses optimistic concurrency control. If the database
// balance is newer than the cached balance (based on update timestamps),
// the sync is skipped to prevent overwriting more recent data. This handles
// scenarios where another process updated the balance directly.
//
// When to Call:
//
// This function is typically called by a background worker that monitors
// Redis balance keys for expiration. It ensures cached balances are persisted
// before they're evicted from cache.
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - balance: Cached balance data from Redis to sync
//
// Returns:
//   - bool: true if balance was synced, false if skipped (database newer)
//   - error: Database connection or operation error
//
// Error Scenarios:
//   - Database connection error: PostgreSQL unavailable
//   - Balance not found: Referenced balance doesn't exist in database
func (uc *UseCase) SyncBalance(ctx context.Context, organizationID, ledgerID uuid.UUID, balance mmodel.BalanceRedis) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.sync_balance")
	defer span.End()

	synchedBalance, err := uc.BalanceRepo.Sync(ctx, organizationID, ledgerID, balance)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to sync balance from redis", err)

		logger.Errorf("Failed to sync balance from redis: %v", err)

		return false, err
	}

	if !synchedBalance {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance is newer, skipping sync", nil)

		logger.Infof("Balance is newer, skipping sync")

		return false, nil
	}

	return true, nil
}
