// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"

	// GetBalances methods responsible to get balances from a database.
	// Returns two slices: before (pre-mutation) and after (post-mutation) balance states.
	// Before is used by BuildOperations for operation snapshots.
	// After is used by UpdateBalances for PostgreSQL persistence.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) GetBalances(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionInput *pkgTransaction.Transaction, validate *pkgTransaction.Responses, transactionStatus string) (before []*mmodel.Balance, after []*mmodel.Balance, err error) {
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
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get account by alias on balance database", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get account by alias on balance database: %v", err))

			return nil, nil, err
		}

		balances = append(balances, balancesByAliases...)
	}

	result, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, transactionID, transactionInput, validate, balances, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balances and update on redis", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get balances and update on redis: %v", err))

		return nil, nil, err
	}

	return result.Before, result.After, nil
}

// ValidateIfBalanceExistsOnRedis func that validate if balance exists on redis before to get on database.
func (uc *UseCase) ValidateIfBalanceExistsOnRedis(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []string) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.validate_if_balance_exists_on_redis")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Checking if balances exists on redis")

	newBalances := make([]*mmodel.Balance, 0)

	newAliases := make([]string, 0)

	for _, alias := range aliases {
		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, alias)

		value, _ := uc.RedisRepo.Get(ctx, internalKey)
		if !libCommons.IsNilOrEmpty(&value) {
			b := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(value), &b); err != nil {
				libOpentelemetry.HandleSpanError(span, "Error to Deserialization json", err)

				logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Error to Deserialization json: %v", err))

				continue
			}

			aliasAndKey := strings.Split(alias, "#")

			balanceKey := constant.DefaultBalanceKey
			if len(aliasAndKey) > 1 {
				balanceKey = aliasAndKey[1]
			}

			newBalances = append(newBalances, &mmodel.Balance{
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
		} else {
			newAliases = append(newAliases, alias)
		}
	}

	return newBalances, newAliases
}

// GetAccountAndLock func responsible to integrate core business logic to redis.
func (uc *UseCase) GetAccountAndLock(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionInput *pkgTransaction.Transaction, validate *pkgTransaction.Responses, balances []*mmodel.Balance, transactionStatus string) (*mmodel.BalanceAtomicResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_account_and_lock")
	defer span.End()

	balanceOperations := make([]mmodel.BalanceOperation, 0)

	for _, balance := range balances {
		aliasKey := balance.Alias + "#" + balance.Key
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

	sort.Slice(balanceOperations, func(i, j int) bool {
		return balanceOperations[i].InternalKey < balanceOperations[j].InternalKey
	})

	err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, balanceOperations, validate)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate accounting rules", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate accounting rules: %v", err))

		return nil, err
	}

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

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate balances: %v", err.Error()))

			return nil, err
		}
	}

	result, err := uc.RedisRepo.ProcessBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID, transactionStatus, validate.Pending, balanceOperations)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to lock balance", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to lock balance: %v", err))

		return nil, err
	}

	return result, nil
}
