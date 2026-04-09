// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// GetBalances retrieves balances for the given aliases using a cache-aside
// pattern: checks Redis first, falls back to PostgreSQL for cache misses.
// This is a pure read -- it does not mutate balances or execute any Lua scripts.
func (uc *UseCase) GetBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_balances")
	defer span.End()

	balances, uncachedAliases := uc.getBalancesFromCache(ctx, organizationID, ledgerID, aliases)

	if len(uncachedAliases) > 0 {
		balancesDB, err := uc.BalanceRepo.ListByAliasesWithKeys(ctx, organizationID, ledgerID, uncachedAliases)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balances from database", err)
			logger.Log(ctx, libLog.LevelError, "Failed to get balances from database", libLog.Err(err))

			return nil, err
		}

		balances = append(balances, balancesDB...)
	}

	return balances, nil
}

// getBalancesFromCache checks Redis for cached balances. Returns two slices:
// the balances found in cache, and the aliases that were not found (cache misses)
// which need to be fetched from the database.
func (uc *UseCase) getBalancesFromCache(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []string) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_balances.cache_read")
	defer span.End()

	cached := make([]*mmodel.Balance, 0)
	misses := make([]string, 0)

	for _, alias := range aliases {
		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, alias)

		value, _ := uc.TransactionRedisRepo.Get(ctx, internalKey)
		if libCommons.IsNilOrEmpty(&value) {
			misses = append(misses, alias)

			continue
		}

		var b mmodel.BalanceRedis
		if err := json.Unmarshal([]byte(value), &b); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to deserialize cached balance", err)
			logger.Log(ctx, libLog.LevelWarn, "Failed to deserialize cached balance, falling back to database",
				libLog.String("alias", alias), libLog.Err(err))

			misses = append(misses, alias)

			continue
		}

		aliasAndKey := strings.Split(alias, "#")

		balanceKey := constant.DefaultBalanceKey
		if len(aliasAndKey) > 1 && strings.TrimSpace(aliasAndKey[1]) != "" {
			balanceKey = aliasAndKey[1]
		}

		cached = append(cached, &mmodel.Balance{
			ID:             b.ID,
			AccountID:      b.AccountID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          aliasAndKey[0],
			Key:            balanceKey,
			Available:      b.Available,
			OnHold:         b.OnHold,
			Version:        b.Version,
			AccountType:    b.AccountType,
			AllowSending:   b.AllowSending == 1,
			AllowReceiving: b.AllowReceiving == 1,
			AssetCode:      b.AssetCode,
		})
	}

	logger.Log(ctx, libLog.LevelDebug, "Balance cache lookup complete",
		libLog.Int("cached", len(cached)),
		libLog.Int("misses", len(misses)))

	return cached, misses
}

// ProcessBalanceOperations builds balance operations from the validated
// transaction entries, validates accounting and balance rules, and executes
// the atomic Lua script that mutates balances in Redis.
//
// Returns the before/after balance snapshots (for operation building and
// PostgreSQL persistence) and the transaction route cache (for operation
// route code resolution).
func (uc *UseCase) ProcessBalanceOperations(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionInput *pkgTransaction.Transaction, validate *pkgTransaction.Responses, balances []*mmodel.Balance, transactionStatus string, action string) (*mmodel.BalanceAtomicResult, *mmodel.TransactionRouteCache, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.process_balance_operations")
	defer span.End()

	// Build balance operations from the From/To entries, including double-entry
	// splitting for pending/canceled transactions with route validation enabled.
	balanceOperations := make([]mmodel.BalanceOperation, 0)

	for _, balance := range balances {
		balanceKey := balance.Key
		if balanceKey == "" {
			balanceKey = constant.DefaultBalanceKey
		}

		aliasKey := balance.Alias + "#" + balanceKey
		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, aliasKey)

		for k, v := range validate.From {
			if pkgTransaction.SplitAliasWithKey(k) == aliasKey {
				if pkgTransaction.IsDoubleEntrySource(v) {
					op1, op2 := pkgTransaction.SplitDoubleEntryOps(v)

					balanceOperations = append(balanceOperations, mmodel.BalanceOperation{
						Balance:     balance,
						Alias:       k,
						Amount:      op1,
						InternalKey: internalKey,
					})

					balanceOperations = append(balanceOperations, mmodel.BalanceOperation{
						Balance:     balance,
						Alias:       k,
						Amount:      op2,
						InternalKey: internalKey,
					})
				} else {
					balanceOperations = append(balanceOperations, mmodel.BalanceOperation{
						Balance:     balance,
						Alias:       k,
						Amount:      v,
						InternalKey: internalKey,
					})
				}
			}
		}

		for k, v := range validate.To {
			if pkgTransaction.SplitAliasWithKey(k) == aliasKey {
				balanceOperations = append(balanceOperations, mmodel.BalanceOperation{
					Balance:     balance,
					Alias:       k,
					Amount:      v,
					InternalKey: internalKey,
				})
			}
		}
	}

	// Sort by internal key to prevent deadlocks in the Lua script.
	sort.Slice(balanceOperations, func(i, j int) bool {
		return balanceOperations[i].InternalKey < balanceOperations[j].InternalKey
	})

	// Validate accounting rules (route validation).
	transactionRouteCache, err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, balanceOperations, validate, action)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate accounting rules", err)
		logger.Log(ctx, libLog.LevelError, "Failed to validate accounting rules", libLog.Err(err))

		return nil, nil, err
	}

	// Validate balance rules (eligibility, asset codes, sending/receiving permissions).
	if transactionInput != nil {
		seen := make(map[string]bool)
		txBalances := make([]*pkgTransaction.Balance, 0, len(balanceOperations))

		for _, bo := range balanceOperations {
			if seen[bo.Alias] {
				continue
			}

			seen[bo.Alias] = true

			txBalances = append(txBalances, bo.Balance.ToTransactionBalance())
		}

		if err = pkgTransaction.ValidateBalancesRules(
			ctx,
			*transactionInput,
			*validate,
			txBalances,
		); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate balances", err)
			logger.Log(ctx, libLog.LevelError, "Failed to validate balances", libLog.Err(err))

			return nil, nil, err
		}
	}

	// Execute the atomic Lua script that mutates balances in Redis.
	result, err := uc.TransactionRedisRepo.ProcessBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID, transactionStatus, validate.Pending, balanceOperations)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to execute atomic balance operation", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute atomic balance operation", libLog.Err(err))

		return nil, nil, err
	}

	return result, transactionRouteCache, nil
}
