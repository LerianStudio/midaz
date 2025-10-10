// Package query implements read operations (queries) for the transaction service.
// This file contains the queries for retrieving and locking balances.
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
// This is the core balance retrieval method for transaction execution. It follows a
// cache-aside pattern, first checking Redis for balances and then falling back to
// PostgreSQL for any that are not found. Once all balances are retrieved, they are
// locked in Redis to prevent concurrent modifications during the transaction.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - transactionID: The UUID of the transaction being processed.
//   - parserDSL: The parsed DSL transaction specification.
//   - validate: The validation response with the calculated from/to amounts.
//   - transactionStatus: The status of the transaction, which affects validation.
//
// Returns:
//   - []*mmodel.Balance: A slice of locked balances, ready for operation creation.
//   - error: An error if a balance is not found or if validation fails.
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

// ValidateIfBalanceExistsOnRedis checks the Redis cache for balances before querying the database.
//
// This optimization reduces database load for frequently accessed accounts by serving
// balance data from the cache. If a balance is not found in Redis, its alias is
// returned to be fetched from PostgreSQL.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - aliases: A slice of account aliases with keys (format: "alias#key").
//
// Returns:
//   - []*mmodel.Balance: A slice of balances found in the Redis cache.
//   - []string: A slice of aliases that were not found in the cache.
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
// This critical use case implements the balance locking mechanism for transaction
// processing. It sorts the balances by a consistent key to prevent deadlocks,
// validates accounting rules, and then locks the balances in Redis.
//
// Deadlock Prevention:
//   - Balances are sorted by an internal key to ensure a consistent lock
//     acquisition order across all transactions, preventing circular wait conditions.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - transactionID: The UUID of the transaction.
//   - parserDSL: The parsed DSL transaction specification.
//   - validate: The validation response with the from/to amounts.
//   - balances: The balances retrieved from the cache or database.
//   - transactionStatus: The status of the transaction.
//
// Returns:
//   - []*mmodel.Balance: A slice of locked balances with updated amounts.
//   - error: An error if validation or locking fails.
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
