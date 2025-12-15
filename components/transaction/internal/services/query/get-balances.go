package query

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// GetBalances methods responsible to get balances from a database.
func (uc *UseCase) GetBalances(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL *pkgTransaction.Transaction, validate *pkgTransaction.Responses, transactionStatus string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_balances")
	defer span.End()

	balances := make([]*mmodel.Balance, 0)

	balancesRedis, aliases := uc.ValidateIfBalanceExistsOnRedis(ctx, organizationID, ledgerID, validate.Aliases)
	if len(balancesRedis) > 0 {
		balances = append(balances, balancesRedis...)
	}

	if len(aliases) > 0 {
		// Retry balance lookup to handle transient connection issues during chaos scenarios
		// Max 5 attempts with exponential backoff: 200ms, 400ms, 800ms, 1600ms (total ~3s)
		// This covers typical connection pool warmup time after service restart (2-5s)
		var balancesByAliases []*mmodel.Balance
		var err error

		for attempt := 0; attempt < 5; attempt++ {
			balancesByAliases, err = uc.BalanceRepo.ListByAliasesWithKeys(ctx, organizationID, ledgerID, aliases)
			if err == nil {
				break // Success
			}

			// Check if error is retriable (connection issues, timeouts)
			errStr := strings.ToLower(err.Error())
			isRetriable := strings.Contains(errStr, "connection") ||
				strings.Contains(errStr, "timeout") ||
				strings.Contains(errStr, "deadline") ||
				strings.Contains(errStr, "unavailable")

			if !isRetriable || attempt == 4 {
				// Non-retriable error or last attempt
				libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get account by alias on balance database", err)
				logger.Error("Failed to get account by alias on balance database", err.Error())
				return nil, fmt.Errorf("failed to list balances by aliases with keys after %d attempts: %w", attempt+1, err)
			}

			// Exponential backoff: 200ms, 400ms, 800ms, 1600ms
			backoffMs := 200 * (1 << attempt)
			logger.Warnf("Balance lookup failed (attempt %d/5), retrying in %dms: %v", attempt+1, backoffMs, err)
			time.Sleep(time.Duration(backoffMs) * time.Millisecond)
		}

		balances = append(balances, balancesByAliases...)
	}

	newBalances, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, transactionID, parserDSL, validate, balances, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances and update on redis", err)

		logger.Error("Failed to get balances and update on redis", err.Error())

		return nil, fmt.Errorf("failed to get account and lock: %w", err)
	}

	return newBalances, nil
}

// ValidateIfBalanceExistsOnRedis func that validate if balance exists on redis before to get on database.
func (uc *UseCase) ValidateIfBalanceExistsOnRedis(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []string) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.validate_if_balance_exists_on_redis")
	defer span.End()

	logger.Infof("Checking if balances exists on redis")

	newBalances := make([]*mmodel.Balance, 0)

	newAliases := make([]string, 0)

	for _, alias := range aliases {
		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, alias)

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

// GetAccountAndLock func responsible to integrate core business logic to redis.
func (uc *UseCase) GetAccountAndLock(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL *pkgTransaction.Transaction, validate *pkgTransaction.Responses, balances []*mmodel.Balance, transactionStatus string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_account_and_lock")
	defer span.End()

	balanceOperations := make([]mmodel.BalanceOperation, 0)

	for _, balance := range balances {
		aliasKey := balance.Alias + "#" + balance.Key
		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, aliasKey)

		for k, v := range validate.From {
			if pkgTransaction.SplitAliasWithKey(k) == aliasKey {
				balanceOperations = append(balanceOperations, mmodel.BalanceOperation{
					Balance:     balance,
					Alias:       k,
					Amount:      v,
					InternalKey: internalKey,
				})
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
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate accounting rules", err)

		logger.Error("Failed to validate accounting rules", err)

		return nil, fmt.Errorf("failed to validate accounting rules: %w", err)
	}

	if parserDSL != nil {
		txBalances := make([]*pkgTransaction.Balance, 0, len(balanceOperations))
		for _, bo := range balanceOperations {
			txBalances = append(txBalances, bo.Balance.ToTransactionBalance())
		}

		if err = pkgTransaction.ValidateBalancesRules(
			ctx,
			*parserDSL,
			*validate,
			txBalances,
		); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate balances", err)

			logger.Errorf("Failed to validate balances: %v", err.Error())

			return nil, fmt.Errorf("failed to validate balances rules: %w", err)
		}
	}

	newBalances, err := uc.RedisRepo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, transactionStatus, validate.Pending, balanceOperations)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to lock balance", err)

		logger.Error("Failed to lock balance", err)

		return nil, fmt.Errorf("failed to add sum balances in redis: %w", err)
	}

	return newBalances, nil
}
