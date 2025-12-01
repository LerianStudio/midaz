package query

import (
	"context"
	"encoding/json"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllBalances retrieves all balances for a ledger with cache overlay.
//
// This method queries account balances from the database and overlays them with
// real-time cached values from Redis. This hybrid approach provides consistency
// (from PostgreSQL) with real-time updates (from Redis) for active transactions.
//
// Query Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "query.get_all_balances"
//
//	Step 2: Database Query
//	  - Query BalanceRepo.ListAll with organization and ledger scope
//	  - Apply cursor pagination from filter
//	  - If retrieval fails: Return error with span event
//	  - If no balances found: Return empty result with span event
//
//	Step 3: Cache Key Construction
//	  - Build Redis keys for each balance: "{org}:{ledger}:{alias}#{key}"
//	  - Keys follow the BalanceInternalKey format from libCommons
//
//	Step 4: Cache Retrieval
//	  - Batch fetch all balance cache values via RedisRepo.MGet
//	  - If Redis fails: Log warning and continue (graceful degradation)
//
//	Step 5: Cache Overlay
//	  - For each balance with cached value:
//	    - Unmarshal JSON from Redis into BalanceRedis struct
//	    - Overlay Available, OnHold, and Version fields
//	  - If unmarshal fails: Log warning and skip (use DB value)
//
//	Step 6: Response
//	  - Return balances with cache overlay and pagination cursor
//
// Cache Overlay Strategy:
//
// The cache overlay is critical for real-time balance accuracy during active
// transactions. PostgreSQL contains the committed state, while Redis contains
// the most recent state including pending operations:
//   - Available: Current available balance (may include pending credits)
//   - OnHold: Amount reserved for pending transactions
//   - Version: Optimistic concurrency version number
//
// Graceful Degradation:
//
// If Redis is unavailable or returns errors, the system falls back to database
// values only. This ensures the API remains functional even during cache outages,
// though balance values may be slightly stale.
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the balances
//   - filter: Query parameters including cursor pagination
//
// Returns:
//   - []*mmodel.Balance: List of balances with cache overlay
//   - libHTTP.CursorPagination: Pagination cursor for next page
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - Database connection failure
//   - Redis errors (logged as warning, not returned as error)
func (uc *UseCase) GetAllBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances")
	defer span.End()

	balances, cur, err := uc.BalanceRepo.ListAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		logger.Errorf("Error getting balances on repo: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(balances) == 0 {
		libOpentelemetry.HandleSpanEvent(&span, "No balances found")

		return nil, libHTTP.CursorPagination{}, nil
	}

	balanceCacheKeys := make([]string, len(balances))

	for i, b := range balances {
		balanceCacheKeys[i] = libCommons.BalanceInternalKey(organizationID.String(), ledgerID.String(), b.Alias+"#"+b.Key)
	}

	balanceCacheValues, err := uc.RedisRepo.MGet(ctx, balanceCacheKeys)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance cache values on redis", err)

		logger.Warnf("Failed to get balance cache values on redis: %v", err)
	}

	for i := range balances {
		if data, ok := balanceCacheValues[balanceCacheKeys[i]]; ok {
			cachedBalance := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(data), &cachedBalance); err != nil {
				logger.Warnf("Error unmarshalling balance cache value: %v", err)

				continue
			}

			balances[i].Available = cachedBalance.Available
			balances[i].OnHold = cachedBalance.OnHold
			balances[i].Version = cachedBalance.Version
		}
	}

	return balances, cur, nil
}

// GetAllBalancesByAlias retrieves all balances for a specific account alias with cache overlay.
//
// This method queries balances by account alias and overlays them with real-time
// cached values from Redis. An alias can have multiple balances with different
// keys (e.g., "default", "pending", "reserved").
//
// Query Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "query.get_all_balances_by_alias"
//
//	Step 2: Database Query
//	  - Query BalanceRepo.ListByAliases with single alias
//	  - If retrieval fails: Return error with span event
//	  - If no balances found: Return nil with span event
//
//	Step 3: Cache Key Construction
//	  - Build Redis keys for each balance: "{org}:{ledger}:{alias}#{key}"
//
//	Step 4: Cache Retrieval
//	  - Batch fetch all balance cache values via RedisRepo.MGet
//	  - If Redis fails: Log warning and continue
//
//	Step 5: Cache Overlay
//	  - For each balance with cached value:
//	    - Unmarshal JSON from Redis into BalanceRedis struct
//	    - Overlay Available, OnHold, and Version fields
//	  - If unmarshal fails: Log warning and skip
//
//	Step 6: Response
//	  - Return balances with cache overlay
//
// Alias-Based Lookup:
//
// Account aliases provide human-readable identifiers for accounts (e.g., "@customer/123").
// An account may have multiple balance keys under the same alias, representing
// different balance types or currencies.
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the balances
//   - alias: Account alias to query balances for
//
// Returns:
//   - []*mmodel.Balance: List of balances for alias with cache overlay
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - Database connection failure
//   - Redis errors (logged as warning, not returned as error)
func (uc *UseCase) GetAllBalancesByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances_by_alias")
	defer span.End()

	logger.Infof("Retrieving all balances by alias")

	balances, err := uc.BalanceRepo.ListByAliases(ctx, organizationID, ledgerID, []string{alias})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to list balances by alias on balance database", err)

		logger.Error("Failed to list balances by alias on balance database", err.Error())

		return nil, err
	}

	if len(balances) == 0 {
		libOpentelemetry.HandleSpanEvent(&span, "No balances found for alias")

		return nil, nil
	}

	balanceCacheKeys := make([]string, len(balances))
	for i, b := range balances {
		balanceCacheKeys[i] = libCommons.BalanceInternalKey(organizationID.String(), ledgerID.String(), b.Alias+"#"+b.Key)
	}

	balanceCacheValues, err := uc.RedisRepo.MGet(ctx, balanceCacheKeys)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance cache values on redis (alias)", err)

		logger.Warnf("Failed to get balance cache values on redis (alias): %v", err)
	}

	for i := range balances {
		if data, ok := balanceCacheValues[balanceCacheKeys[i]]; ok {
			cachedBalance := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(data), &cachedBalance); err != nil {
				logger.Warnf("Error unmarshalling balance cache value (alias): %v", err)

				continue
			}

			balances[i].Available = cachedBalance.Available
			balances[i].OnHold = cachedBalance.OnHold
			balances[i].Version = cachedBalance.Version
		}
	}

	return balances, nil
}
