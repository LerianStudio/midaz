package query

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// GetBalances retrieves and locks account balances for transaction processing.
//
// This method is the primary entry point for balance retrieval during transaction
// execution. It implements a multi-tier lookup strategy with Redis caching and
// applies balance locking for concurrent transaction safety.
//
// Balance Retrieval Strategy:
//
//	Tier 1: Redis Cache
//	  - Check if balances exist in Redis (hot data)
//	  - Cached balances include current available/onHold amounts
//	  - Cache hit avoids PostgreSQL round-trip
//
//	Tier 2: PostgreSQL Database
//	  - Fetch missing balances from database
//	  - Query by alias#key format for account identification
//	  - Database is source of truth for uncached balances
//
// Transaction Locking:
//
// After retrieval, balances are locked in Redis to prevent concurrent modifications.
// The lock includes:
//   - Transaction ID association
//   - Status tracking (pending, committed, reverted)
//   - Optimistic locking via version numbers
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for distributed tracing
//
//	Step 2: Redis Cache Lookup
//	  - Check each alias against Redis cache
//	  - Separate found balances from cache misses
//	  - ValidateIfBalanceExistsOnRedis handles deserialization
//
//	Step 3: Database Fallback
//	  - Query PostgreSQL for uncached aliases
//	  - ListByAliasesWithKeys returns balance with metadata keys
//	  - Merge database results with cached balances
//
//	Step 4: Account Locking and Validation
//	  - GetAccountAndLock processes all balances
//	  - Validates accounting rules if enabled
//	  - Applies DSL validation rules
//	  - Atomically updates Redis with locked balances
//
// Parameters:
//   - ctx: Request context with tenant and tracing information
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID for balance scope
//   - transactionID: Transaction UUID for lock association
//   - parserDSL: Parsed transaction DSL for rule validation (optional)
//   - validate: Validation context with alias mappings and amounts
//   - transactionStatus: Initial status for locked balances
//
// Returns:
//   - []*mmodel.Balance: Locked balances ready for transaction execution
//   - error: Validation or infrastructure error
//
// Error Scenarios:
//   - Balance not found: Account alias doesn't exist
//   - Validation failure: DSL rules or accounting rules violated
//   - Lock failure: Redis operation failed
//   - Concurrent modification: Version mismatch during lock
func (uc *UseCase) GetBalances(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL *libTransaction.Transaction, validate *libTransaction.Responses, transactionStatus string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_balances")
	defer span.End()

	balances := make([]*mmodel.Balance, 0)

	balancesRedis, aliases := uc.ValidateIfBalanceExistsOnRedis(ctx, organizationID, ledgerID, validate.Aliases)
	if len(balancesRedis) > 0 {
		balances = append(balances, balancesRedis...)
	}

	if len(aliases) > 0 {
		balancesByAliases, err := uc.BalanceRepo.ListByAliasesWithKeys(ctx, organizationID, ledgerID, aliases)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get account by alias on balance database", err)

			logger.Error("Failed to get account by alias on balance database", err.Error())

			return nil, err
		}

		balances = append(balances, balancesByAliases...)
	}

	newBalances, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, transactionID, parserDSL, validate, balances, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances and update on redis", err)

		logger.Error("Failed to get balances and update on redis", err.Error())

		return nil, err
	}

	return newBalances, nil
}

// ValidateIfBalanceExistsOnRedis checks Redis cache for balance data before database lookup.
//
// This method implements the cache-check phase of the multi-tier balance retrieval.
// It deserializes cached balance data and separates cache hits from misses.
//
// Cache Key Format:
//   - Pattern: transaction:{orgID}:{ledgerID}:{alias#key}
//   - Example: transaction:550e8400:6ba7b810:savings#USD
//
// Cache Value Format:
//   - JSON-encoded BalanceRedis struct
//   - Contains: ID, AccountID, Available, OnHold, Version, AccountType, etc.
//
// Process:
//
//	Step 1: Initialize Collections
//	  - Prepare slices for found balances and missing aliases
//
//	Step 2: Iterate Aliases
//	  - Generate internal key for each alias
//	  - Attempt Redis GET operation
//	  - Deserialize JSON if found
//
//	Step 3: Handle Cache Hits
//	  - Parse alias#key format
//	  - Construct Balance from cached data
//	  - Convert flags (AllowSending/AllowReceiving from int to bool)
//
//	Step 4: Track Cache Misses
//	  - Append to newAliases for database lookup
//	  - Log deserialization errors but continue
//
// Parameters:
//   - ctx: Request context for Redis operations
//   - organizationID: Organization UUID for cache key
//   - ledgerID: Ledger UUID for cache key
//   - aliases: Account aliases (format: alias#key) to check
//
// Returns:
//   - []*mmodel.Balance: Balances found in cache
//   - []string: Aliases not found (for database lookup)
func (uc *UseCase) ValidateIfBalanceExistsOnRedis(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []string) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.validate_if_balance_exists_on_redis")
	defer span.End()

	logger.Infof("Checking if balances exists on redis")

	newBalances := make([]*mmodel.Balance, 0)

	newAliases := make([]string, 0)

	for _, alias := range aliases {
		internalKey := utils.TransactionInternalKey(organizationID, ledgerID, alias)

		value, _ := uc.RedisRepo.Get(ctx, internalKey)
		if !libCommons.IsNilOrEmpty(&value) {
			b := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(value), &b); err != nil {
				libOpentelemetry.HandleSpanError(&span, "Error to Deserialization json", err)

				logger.Warnf("Error to Deserialization json: %v", err)

				continue
			}

			aliasAndKey := strings.Split(alias, "#")
			newBalances = append(newBalances, &mmodel.Balance{
				ID:             b.ID,
				AccountID:      b.AccountID,
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          aliasAndKey[0],
				Key:            aliasAndKey[1],
				Available:      b.Available,
				OnHold:         b.OnHold,
				Version:        b.Version,
				AccountType:    b.AccountType,
				AllowSending:   b.AllowSending == 1,
				AllowReceiving: b.AllowReceiving == 1,
				AssetCode:      b.AssetCode,
			})
		} else {
			newAliases = append(newAliases, alias)
		}
	}

	return newBalances, newAliases
}

// GetAccountAndLock processes balances, validates rules, and applies transaction locks.
//
// This method is the core orchestrator for balance preparation before transaction
// execution. It builds operation structures, validates accounting rules, and
// atomically locks balances in Redis.
//
// Operation Building:
//
// For each balance, the method creates BalanceOperation entries that link:
//   - Balance data (amounts, account info)
//   - Operation alias (from DSL)
//   - Transaction amount (from validation context)
//   - Internal key (for Redis locking)
//
// Sorting for Deadlock Prevention:
//
// Operations are sorted by internal key before locking. This ensures consistent
// lock ordering across concurrent transactions, preventing deadlocks when multiple
// transactions involve overlapping accounts.
//
// Process:
//
//	Step 1: Build Balance Operations
//	  - Match balances to From/To mappings in validation context
//	  - Create BalanceOperation for each match
//	  - Include internal key for Redis operations
//
//	Step 2: Sort Operations
//	  - Order by internal key (deterministic ordering)
//	  - Prevents deadlocks in concurrent scenarios
//
//	Step 3: Validate Accounting Rules
//	  - Check operations against transaction route rules
//	  - Skip if validation not enabled for org/ledger
//
//	Step 4: Validate DSL Rules
//	  - Apply transaction DSL validation if provided
//	  - Checks balance sufficiency, limits, etc.
//
//	Step 5: Lock Balances in Redis
//	  - Atomically update balances with transaction association
//	  - Apply pending amounts to available/onHold
//	  - Return updated balances with new versions
//
// Parameters:
//   - ctx: Request context for operations
//   - organizationID: Organization UUID
//   - ledgerID: Ledger UUID
//   - transactionID: Transaction UUID for lock association
//   - parserDSL: Parsed DSL for validation (optional)
//   - validate: Validation context with mappings
//   - balances: Balances to process
//   - transactionStatus: Status for locked balances
//
// Returns:
//   - []*mmodel.Balance: Locked and updated balances
//   - error: Validation or locking failure
//
// Error Scenarios:
//   - Accounting rule violation: Route validation failed
//   - DSL validation failure: Balance rules not met
//   - Lock failure: Redis atomic operation failed
func (uc *UseCase) GetAccountAndLock(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL *libTransaction.Transaction, validate *libTransaction.Responses, balances []*mmodel.Balance, transactionStatus string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_account_and_lock")
	defer span.End()

	balanceOperations := make([]mmodel.BalanceOperation, 0)

	for _, balance := range balances {
		aliasKey := balance.Alias + "#" + balance.Key
		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, aliasKey)

		for k, v := range validate.From {
			if libTransaction.SplitAliasWithKey(k) == aliasKey {
				balanceOperations = append(balanceOperations, mmodel.BalanceOperation{
					Balance:     balance,
					Alias:       k,
					Amount:      v,
					InternalKey: internalKey,
				})
			}
		}

		for k, v := range validate.To {
			if libTransaction.SplitAliasWithKey(k) == aliasKey {
				balanceOperations = append(balanceOperations, mmodel.BalanceOperation{
					Balance:     balance,
					Alias:       k,
					Amount:      v,
					InternalKey: internalKey,
				})
			}
		}
	}

	sort.Slice(balanceOperations, func(i, j int) bool {
		return balanceOperations[i].InternalKey < balanceOperations[j].InternalKey
	})

	err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, balanceOperations, validate)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate accounting rules", err)

		logger.Error("Failed to validate accounting rules", err)

		return nil, err
	}

	if parserDSL != nil {
		if err = libTransaction.ValidateBalancesRules(
			ctx,
			*parserDSL,
			*validate,
			mmodel.ConvertBalanceOperationsToLibBalances(balanceOperations),
		); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate balances", err)

			logger.Errorf("Failed to validate balances: %v", err.Error())

			return nil, err
		}
	}

	newBalances, err := uc.RedisRepo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, transactionStatus, validate.Pending, balanceOperations)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to lock balance", err)

		logger.Error("Failed to lock balance", err)

		return nil, err
	}

	return newBalances, nil
}
