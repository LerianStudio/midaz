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

// GetAllBalancesByAccountID retrieves all balances for a specific account with cache enrichment.
//
// This method returns all balance records (multi-currency) for an account, enriching
// database results with real-time values from Redis cache when available. This ensures
// API responses reflect the most current balance state during active transactions.
//
// Multi-Currency Support:
//
// A single account can have multiple balances, one per asset/currency:
//   - Account "savings" may have balances for USD, EUR, BRL
//   - Each balance has its own Available, OnHold, and Version
//   - Balance key distinguishes currencies (e.g., "savings#USD", "savings#EUR")
//
// Cache Enrichment Strategy:
//
// During active transactions, balances in Redis may be more current than PostgreSQL:
//   - Redis contains in-flight balance updates
//   - PostgreSQL is updated after transaction commit
//   - This method merges both sources for accurate reads
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for distributed tracing
//
//	Step 2: Fetch Balances from PostgreSQL
//	  - Query all balances for the account
//	  - Apply cursor-based pagination
//	  - Return empty slice if no balances found
//
//	Step 3: Build Cache Keys
//	  - Generate Redis keys for each balance
//	  - Key format: balance:{orgID}:{ledgerID}:{alias#key}
//
//	Step 4: Bulk Cache Lookup
//	  - MGET all balance keys in single Redis call
//	  - Returns map of key -> cached value
//	  - Non-fatal error: log and continue with DB values
//
//	Step 5: Enrich with Cached Values
//	  - For each balance with cache hit:
//	  - Deserialize BalanceRedis from JSON
//	  - Override Available, OnHold, Version from cache
//	  - Skip on deserialization error (use DB value)
//
// Parameters:
//   - ctx: Request context with tenant and tracing information
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID for balance scope
//   - accountID: Account UUID to retrieve balances for
//   - filter: Query parameters with cursor pagination
//
// Returns:
//   - []*mmodel.Balance: Balances enriched with cache data
//   - libHTTP.CursorPagination: Pagination cursor for next page
//   - error: Infrastructure error (database unavailable)
//
// Error Scenarios:
//   - Database error: PostgreSQL query failed
//   - Redis error: Logged but non-fatal (falls back to DB values)
//   - Deserialization error: Logged per-balance, continues with DB value
//
// Performance Considerations:
//   - Single PostgreSQL query for all balances
//   - Single MGET for all cache keys (batch operation)
//   - O(n) iteration for cache enrichment
func (uc *UseCase) GetAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances_by_account_id")
	defer span.End()

	balance, cur, err := uc.BalanceRepo.ListAllByAccountID(ctx, organizationID, ledgerID, accountID, filter.ToCursorPagination())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances on repo", err)

		logger.Errorf("Error getting balances on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(balance) == 0 {
		libOpentelemetry.HandleSpanEvent(&span, "No balances found")

		return balance, cur, nil
	}

	balanceCacheKeys := make([]string, len(balance))

	for i, b := range balance {
		balanceCacheKeys[i] = libCommons.BalanceInternalKey(organizationID.String(), ledgerID.String(), b.Alias+"#"+b.Key)
	}

	balanceCacheValues, err := uc.RedisRepo.MGet(ctx, balanceCacheKeys)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance cache values on redis", err)

		logger.Warnf("Failed to get balance cache values on redis: %v", err)
	}

	for i := range balance {
		if data, ok := balanceCacheValues[balanceCacheKeys[i]]; ok {
			cachedBalance := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(data), &cachedBalance); err != nil {
				logger.Warnf("Error unmarshalling balance cache value: %v", err)

				continue
			}

			balance[i].Available = cachedBalance.Available
			balance[i].OnHold = cachedBalance.OnHold
			balance[i].Version = cachedBalance.Version
		}
	}

	return balance, cur, nil
}
