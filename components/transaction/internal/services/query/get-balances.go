// Package query implements read operations (queries) for the transaction service.
// This file contains query implementation.

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
	"github.com/google/uuid"
)

// GetBalances retrieves and locks balances for transaction processing.
//
// This is the core balance retrieval method for transaction execution, which:
// 1. Checks Redis cache for balances (hot path optimization)
// 2. Fetches missing balances from PostgreSQL by aliases
// 3. Locks balances in Redis for transaction processing
// 4. Validates accounting rules and balance availability
// 5. Returns locked balances ready for operation creation
//
// Balance Locking Strategy:
//   - Check Redis first (cached balances from recent transactions)
//   - Fetch from PostgreSQL if not in cache
//   - Lock in Redis to prevent concurrent modifications
//   - Sorted by internal key to prevent deadlocks
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionID: UUID of the transaction being processed
//   - parserDSL: Parsed DSL transaction specification
//   - validate: Validation responses with from/to amounts
//   - transactionStatus: Transaction status (affects validation)
//
// Returns:
//   - []*mmodel.Balance: Locked balances ready for operations
//   - error: Business error if balance not found or validation fails
//
// OpenTelemetry: Creates span "usecase.get_balances"
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

// ValidateIfBalanceExistsOnRedis checks Redis cache for balances before querying PostgreSQL.
//
// This optimization method checks if balances are already cached in Redis from recent
// transactions. For each alias:
//   - If found in Redis: Deserializes and returns the balance
//   - If not found: Adds to list for PostgreSQL query
//
// This reduces database load for high-frequency accounts by serving from cache.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - aliases: Array of account aliases with keys (format: "alias#key")
//
// Returns:
//   - []*mmodel.Balance: Balances found in Redis cache
//   - []string: Aliases not found in cache (need PostgreSQL query)
//
// OpenTelemetry: Creates span "usecase.validate_if_balance_exists_on_redis"
func (uc *UseCase) ValidateIfBalanceExistsOnRedis(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []string) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.validate_if_balance_exists_on_redis")
	defer span.End()

	logger.Infof("Checking if balances exists on redis")

	newBalances := make([]*mmodel.Balance, 0)

	newAliases := make([]string, 0)

	for _, alias := range aliases {
		internalKey := libCommons.TransactionInternalKey(organizationID, ledgerID, alias)

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

// GetAccountAndLock locks balances in Redis and validates accounting rules.
//
// This critical method implements the balance locking mechanism for transaction processing:
// 1. Matches balances to from/to specifications from validation
// 2. Creates BalanceOperation structs with internal keys
// 3. Sorts operations by internal key (prevents deadlocks)
// 4. Validates accounting rules (allow_sending, allow_receiving)
// 5. Validates balance availability (lib-commons ValidateBalancesRules)
// 6. Locks balances in Redis (AddSumBalancesRedis)
// 7. Returns locked balances with updated amounts
//
// Deadlock Prevention:
//   - Operations sorted by internal key ensures consistent lock ordering
//   - All transactions lock balances in the same order
//   - Prevents circular wait conditions
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionID: UUID of the transaction
//   - parserDSL: Parsed DSL specification
//   - validate: Validation responses with from/to amounts
//   - balances: Retrieved balances (from cache or database)
//   - transactionStatus: Transaction status
//
// Returns:
//   - []*mmodel.Balance: Locked balances with updated amounts
//   - error: Business error if validation or locking fails
//
// OpenTelemetry: Creates span "usecase.get_account_and_lock"
func (uc *UseCase) GetAccountAndLock(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL *libTransaction.Transaction, validate *libTransaction.Responses, balances []*mmodel.Balance, transactionStatus string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_account_and_lock")
	defer span.End()

	balanceOperations := make([]mmodel.BalanceOperation, 0)

	for _, balance := range balances {
		aliasKey := balance.Alias + "#" + balance.Key
		internalKey := libCommons.BalanceInternalKey(organizationID.String(), ledgerID.String(), aliasKey)

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
